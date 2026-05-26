import type {
	Context,
	ContextInput,
	Label,
	LabelInput,
	Project,
	ProjectInput,
	ProjectSection,
	SectionInput
} from '$lib/api/types';
import { getDB, type EntityKind, type StoredEntity, type TurboistDB } from './db';
import { newClientId } from './ids';
import { dropPendingFor, enqueue } from './outbox';
import { emitDbChanged } from './stores';

const SYNTHETIC_ID_BASE = -2_000_000_000;
let syntheticCounter = 0;

const mintSyntheticId = (): number => {
	syntheticCounter += 1;
	return SYNTHETIC_ID_BASE - syntheticCounter;
};

const findClientIdBySyntheticId = async (
	db: TurboistDB,
	kind: EntityKind,
	syntheticId: number
): Promise<string | null> => {
	const row = await db
		.table(kind)
		.filter((r: StoredEntity) => (r.data as { id?: number }).id === syntheticId)
		.first();
	return row?.clientId ?? null;
};

const nowIso = (): string => new Date().toISOString();

interface EntityRecord {
	id: number;
	clientId: string;
	updatedAt: string;
	[k: string]: unknown;
}

interface StoredEntityRow<T extends EntityRecord> {
	clientId: string;
	serverId: number | null;
	updatedAt: string;
	deletedAt: string | null;
	data: T;
}

const toStored = <T extends EntityRecord>(row: StoredEntityRow<T>): StoredEntity => ({
	clientId: row.clientId,
	serverId: row.serverId,
	updatedAt: row.updatedAt,
	deletedAt: row.deletedAt,
	data: row.data as unknown as Record<string, unknown>
});

const fromStored = <T extends EntityRecord>(row: StoredEntity): StoredEntityRow<T> => ({
	clientId: row.clientId,
	serverId: row.serverId,
	updatedAt: row.updatedAt,
	deletedAt: row.deletedAt,
	data: row.data as unknown as T
});

const loadRow = async <T extends EntityRecord>(
	db: TurboistDB,
	kind: EntityKind,
	ref: { id: number; clientId?: string }
): Promise<StoredEntityRow<T> | undefined> => {
	if (ref.clientId) {
		const byCid = (await db.table(kind).get(ref.clientId)) as StoredEntity | undefined;
		if (byCid) return fromStored<T>(byCid);
	}
	if (ref.id > 0) {
		const byServer = (await db.table(kind).where('serverId').equals(ref.id).first()) as
			| StoredEntity
			| undefined;
		if (byServer) return fromStored<T>(byServer);
	}
	return undefined;
};

export interface OfflineMutationOptions {
	db?: TurboistDB;
}

interface EntityConfig<TEntity extends EntityRecord, TInput> {
	kind: EntityKind;
	listPath: (input: TInput, opts: Record<string, unknown>) => string;
	itemPath: (serverId: number | string) => string;
	buildOptimistic: (
		clientId: string,
		syntheticId: number,
		input: TInput,
		opts: Record<string, unknown>
	) => TEntity;
}

const createOffline = async <T extends EntityRecord, I extends Record<string, unknown>>(
	cfg: EntityConfig<T, I>,
	input: I,
	opts: Record<string, unknown> & OfflineMutationOptions
): Promise<T> => {
	const db = (opts.db as TurboistDB | undefined) ?? getDB();
	const clientId = newClientId();
	const syntheticId = mintSyntheticId();
	const entity = cfg.buildOptimistic(clientId, syntheticId, input, opts);
	const row: StoredEntityRow<T> = {
		clientId,
		serverId: null,
		updatedAt: entity.updatedAt,
		deletedAt: null,
		data: entity
	};
	const parentClientId = (opts.parentClientId as string | null | undefined) ?? null;
	await db.transaction('rw', db.table(cfg.kind), db.outbox, async () => {
		await db.table(cfg.kind).put(toStored(row));
		await enqueue(
			{
				entity: cfg.kind,
				op: 'create',
				clientId,
				parentClientId,
				payload: {
					method: 'POST',
					path: cfg.listPath(input, opts),
					body: { ...input, clientId }
				}
			},
			db
		);
	});
	emitDbChanged(cfg.kind);
	return entity;
};

const updateOffline = async <T extends EntityRecord, I extends Record<string, unknown>>(
	cfg: EntityConfig<T, I>,
	ref: { id: number; clientId?: string },
	patch: I,
	opts: OfflineMutationOptions
): Promise<T> => {
	const db = opts.db ?? getDB();
	const row = await loadRow<T>(db, cfg.kind, ref);
	if (!row) throw new Error(`updateOffline(${cfg.kind}): row not found (id=${ref.id})`);
	const ts = nowIso();
	const baseUpdatedAt = row.updatedAt;
	const nextData = {
		...row.data,
		...(patch as unknown as Partial<T>),
		updatedAt: ts,
		clientId: row.clientId
	} as T;
	const next: StoredEntityRow<T> = { ...row, updatedAt: ts, data: nextData };
	await db.transaction('rw', db.table(cfg.kind), db.outbox, async () => {
		await db.table(cfg.kind).put(toStored(next));
		await enqueue(
			{
				entity: cfg.kind,
				op: 'update',
				clientId: row.clientId,
				payload: {
					method: 'PATCH',
					path: row.serverId ? cfg.itemPath(row.serverId) : cfg.itemPath('{serverId}'),
					body: { ...patch, baseUpdatedAt }
				}
			},
			db
		);
	});
	emitDbChanged(cfg.kind);
	return nextData;
};

const deleteOffline = async <T extends EntityRecord, I extends Record<string, unknown>>(
	cfg: EntityConfig<T, I>,
	ref: { id: number; clientId?: string },
	opts: OfflineMutationOptions
): Promise<void> => {
	const db = opts.db ?? getDB();
	const row = await loadRow<T>(db, cfg.kind, ref);
	if (!row) return;
	const ts = nowIso();
	await db.transaction('rw', db.table(cfg.kind), db.outbox, async () => {
		await db.table(cfg.kind).put(toStored({ ...row, deletedAt: ts, updatedAt: ts }));
		const entry = await enqueue(
			{
				entity: cfg.kind,
				op: 'delete',
				clientId: row.clientId,
				payload: {
					method: 'DELETE',
					path: row.serverId ? cfg.itemPath(row.serverId) : cfg.itemPath('{serverId}')
				}
			},
			db
		);
		await dropPendingFor(cfg.kind, row.clientId, entry.id, db);
	});
	emitDbChanged(cfg.kind);
};

const projectConfig: EntityConfig<Project & EntityRecord, ProjectInput & Record<string, unknown>> = {
	kind: 'projects',
	listPath: (_input, opts) => {
		const contextId = opts.contextId as number | undefined;
		if (!contextId) throw new Error('createProjectOffline: contextId required');
		return `/api/v1/contexts/${contextId}/projects`;
	},
	itemPath: (id) => `/api/v1/projects/${id}`,
	buildOptimistic: (clientId, syntheticId, input, opts) => {
		const ts = nowIso();
		return {
			id: syntheticId,
			contextId: (opts.contextId as number | undefined) ?? 0,
			title: input.title ?? '',
			description: input.description ?? '',
			color: input.color ?? 'gray',
			status: 'open',
			projectType: input.projectType ?? 'generic',
			isPinned: false,
			pinnedAt: null,
			isPrivate: input.isPrivate ?? false,
			labels: [],
			troikiCategory: null,
			createdAt: ts,
			updatedAt: ts,
			clientId
		} as Project & EntityRecord;
	}
};

const sectionConfig: EntityConfig<
	ProjectSection & EntityRecord,
	SectionInput & Record<string, unknown>
> = {
	kind: 'sections',
	listPath: (_input, opts) => {
		const projectId = opts.projectId as number | undefined;
		if (!projectId) throw new Error('createSectionOffline: projectId required');
		const projectClientId = opts.projectClientId as string | undefined;
		if (isSyntheticEntityId(projectId) && projectClientId) {
			return `/api/v1/projects/{ref:${projectClientId}}/sections`;
		}
		return `/api/v1/projects/${projectId}/sections`;
	},
	itemPath: (id) => `/api/v1/sections/${id}`,
	buildOptimistic: (clientId, syntheticId, input, opts) => {
		const ts = nowIso();
		return {
			id: syntheticId,
			projectId: (opts.projectId as number | undefined) ?? 0,
			title: input.title ?? '',
			position: 0,
			createdAt: ts,
			updatedAt: ts,
			clientId
		} as ProjectSection & EntityRecord;
	}
};

const labelConfig: EntityConfig<Label & EntityRecord, LabelInput & Record<string, unknown>> = {
	kind: 'labels',
	listPath: () => '/api/v1/labels',
	itemPath: (id) => `/api/v1/labels/${id}`,
	buildOptimistic: (clientId, syntheticId, input) => {
		const ts = nowIso();
		return {
			id: syntheticId,
			name: input.name ?? '',
			color: input.color ?? 'gray',
			isFavourite: input.isFavourite ?? false,
			isPrivate: input.isPrivate ?? false,
			createdAt: ts,
			updatedAt: ts,
			clientId
		} as Label & EntityRecord;
	}
};

const contextConfig: EntityConfig<Context & EntityRecord, ContextInput & Record<string, unknown>> =
	{
		kind: 'contexts',
		listPath: () => '/api/v1/contexts',
		itemPath: (id) => `/api/v1/contexts/${id}`,
		buildOptimistic: (clientId, syntheticId, input) => {
			const ts = nowIso();
			return {
				id: syntheticId,
				name: input.name ?? '',
				color: input.color ?? 'gray',
				isFavourite: input.isFavourite ?? false,
				createdAt: ts,
				updatedAt: ts,
				clientId
			} as Context & EntityRecord;
		}
	};

export interface CreateProjectOptions extends OfflineMutationOptions {
	contextId: number;
}

export const createProjectOffline = (
	input: ProjectInput,
	opts: CreateProjectOptions
): Promise<Project & { clientId: string }> =>
	createOffline(projectConfig, input as ProjectInput & Record<string, unknown>, {
		...opts,
		contextId: opts.contextId
	}) as Promise<Project & { clientId: string }>;

export const updateProjectOffline = (
	ref: { id: number; clientId?: string },
	patch: ProjectInput,
	opts: OfflineMutationOptions = {}
): Promise<Project & { clientId: string }> =>
	updateOffline(
		projectConfig,
		ref,
		patch as ProjectInput & Record<string, unknown>,
		opts
	) as Promise<Project & { clientId: string }>;

export const deleteProjectOffline = (
	ref: { id: number; clientId?: string },
	opts: OfflineMutationOptions = {}
): Promise<void> => deleteOffline(projectConfig, ref, opts);

export interface CreateSectionOptions extends OfflineMutationOptions {
	projectId: number;
}

export const createSectionOffline = async (
	input: SectionInput,
	opts: CreateSectionOptions
): Promise<ProjectSection & { clientId: string }> => {
	const db = opts.db ?? getDB();
	let projectClientId: string | null = null;
	if (isSyntheticEntityId(opts.projectId)) {
		projectClientId = await findClientIdBySyntheticId(db, 'projects', opts.projectId);
	}
	return createOffline(sectionConfig, input as SectionInput & Record<string, unknown>, {
		...opts,
		db,
		projectId: opts.projectId,
		projectClientId,
		parentClientId: projectClientId
	}) as Promise<ProjectSection & { clientId: string }>;
};

export const updateSectionOffline = (
	ref: { id: number; clientId?: string },
	patch: SectionInput,
	opts: OfflineMutationOptions = {}
): Promise<ProjectSection & { clientId: string }> =>
	updateOffline(
		sectionConfig,
		ref,
		patch as SectionInput & Record<string, unknown>,
		opts
	) as Promise<ProjectSection & { clientId: string }>;

export const deleteSectionOffline = (
	ref: { id: number; clientId?: string },
	opts: OfflineMutationOptions = {}
): Promise<void> => deleteOffline(sectionConfig, ref, opts);

export const createLabelOffline = (
	input: LabelInput,
	opts: OfflineMutationOptions = {}
): Promise<Label & { clientId: string }> =>
	createOffline(labelConfig, input as LabelInput & Record<string, unknown>, {
		...opts
	}) as Promise<Label & { clientId: string }>;

export const updateLabelOffline = (
	ref: { id: number; clientId?: string },
	patch: LabelInput,
	opts: OfflineMutationOptions = {}
): Promise<Label & { clientId: string }> =>
	updateOffline(
		labelConfig,
		ref,
		patch as LabelInput & Record<string, unknown>,
		opts
	) as Promise<Label & { clientId: string }>;

export const deleteLabelOffline = (
	ref: { id: number; clientId?: string },
	opts: OfflineMutationOptions = {}
): Promise<void> => deleteOffline(labelConfig, ref, opts);

export const createContextOffline = (
	input: ContextInput,
	opts: OfflineMutationOptions = {}
): Promise<Context & { clientId: string }> =>
	createOffline(contextConfig, input as ContextInput & Record<string, unknown>, {
		...opts
	}) as Promise<Context & { clientId: string }>;

export const updateContextOffline = (
	ref: { id: number; clientId?: string },
	patch: ContextInput,
	opts: OfflineMutationOptions = {}
): Promise<Context & { clientId: string }> =>
	updateOffline(
		contextConfig,
		ref,
		patch as ContextInput & Record<string, unknown>,
		opts
	) as Promise<Context & { clientId: string }>;

export const deleteContextOffline = (
	ref: { id: number; clientId?: string },
	opts: OfflineMutationOptions = {}
): Promise<void> => deleteOffline(contextConfig, ref, opts);

export const isSyntheticEntityId = (id: number): boolean => id <= SYNTHETIC_ID_BASE;

import type { Task, TaskInput, TaskMoveInput, TaskPlanInput } from '$lib/api/types';
import { getDB, type StoredEntity, type TurboistDB } from './db';
import { newClientId } from './ids';
import { dropPendingByPathSuffix, dropPendingFor, enqueue } from './outbox';
import { emitDbChanged } from './stores';

const COMPLETE_TOGGLE_SUFFIXES = ['/complete', '/uncomplete'] as const;

const SYNTHETIC_ID_BASE = -1_000_000_000;
let syntheticCounter = 0;

const mintSyntheticId = (): number => {
	syntheticCounter += 1;
	return SYNTHETIC_ID_BASE - syntheticCounter;
};

const nowIso = (): string => new Date().toISOString();

type TaskData = Task & { clientId: string };

interface StoredTaskRow {
	clientId: string;
	serverId: number | null;
	updatedAt: string;
	deletedAt: string | null;
	data: TaskData;
}

const toStored = (row: StoredTaskRow): StoredEntity => ({
	clientId: row.clientId,
	serverId: row.serverId,
	updatedAt: row.updatedAt,
	deletedAt: row.deletedAt,
	data: row.data as unknown as Record<string, unknown>
});

const fromStored = (row: StoredEntity): StoredTaskRow => ({
	clientId: row.clientId,
	serverId: row.serverId,
	updatedAt: row.updatedAt,
	deletedAt: row.deletedAt,
	data: row.data as unknown as TaskData
});

const loadByServerId = async (
	db: TurboistDB,
	serverId: number
): Promise<StoredTaskRow | undefined> => {
	const row = await db.tasks.where('serverId').equals(serverId).first();
	return row ? fromStored(row) : undefined;
};

const loadByClientId = async (
	db: TurboistDB,
	clientId: string
): Promise<StoredTaskRow | undefined> => {
	const row = await db.tasks.get(clientId);
	return row ? fromStored(row) : undefined;
};

const resolveTaskRow = async (
	db: TurboistDB,
	task: Pick<Task, 'id'> & { clientId?: string }
): Promise<StoredTaskRow | undefined> => {
	if (task.clientId) {
		const byCid = await loadByClientId(db, task.clientId);
		if (byCid) return byCid;
	}
	if (task.id > 0) return loadByServerId(db, task.id);
	return undefined;
};

export interface CreateTaskOptions {
	contextId?: number | null;
	projectId?: number | null;
	sectionId?: number | null;
	parentId?: number | null;
	inbox?: boolean;
	db?: TurboistDB;
}

const buildOptimisticTask = (
	clientId: string,
	syntheticId: number,
	input: TaskInput,
	opts: CreateTaskOptions
): Task & { clientId: string } => {
	const ts = nowIso();
	return {
		id: syntheticId,
		title: input.title ?? '',
		description: input.description ?? '',
		inboxId: opts.inbox ? 1 : null,
		contextId: opts.contextId ?? null,
		projectId: opts.projectId ?? null,
		sectionId: opts.sectionId ?? null,
		parentId: opts.parentId ?? null,
		priority: input.priority ?? 'no-priority',
		status: 'open',
		dueAt: input.dueAt ?? null,
		dueHasTime: input.dueHasTime ?? false,
		deadlineAt: input.deadlineAt ?? null,
		deadlineHasTime: input.deadlineHasTime ?? false,
		dayPart: input.dayPart ?? 'none',
		planState: input.planState ?? 'none',
		isPinned: false,
		pinnedAt: null,
		isPrivate: input.isPrivate ?? false,
		completedAt: null,
		recurrenceRule: input.recurrenceRule ?? null,
		postponeCount: 0,
		labels: [],
		url: '',
		createdAt: ts,
		updatedAt: ts,
		clientId
	};
};

const buildCreatePath = (opts: CreateTaskOptions, parentClientId: string | null): string => {
	if (opts.inbox) return '/api/v1/inbox/tasks';
	if (opts.parentId != null && opts.parentId > 0) {
		return `/api/v1/tasks/${opts.parentId}/subtasks`;
	}
	if (opts.parentId != null && parentClientId) {
		return `/api/v1/tasks/{ref:${parentClientId}}/subtasks`;
	}
	if (opts.contextId != null) return `/api/v1/contexts/${opts.contextId}/tasks`;
	throw new Error('createTaskOffline: contextId, parentId or inbox flag required');
};

const findTaskClientIdBySyntheticId = async (
	db: TurboistDB,
	syntheticId: number
): Promise<string | null> => {
	const row = await db.tasks
		.filter((r) => (r.data as { id?: number }).id === syntheticId)
		.first();
	return row?.clientId ?? null;
};

export const createTaskOffline = async (
	input: TaskInput,
	opts: CreateTaskOptions = {}
): Promise<Task & { clientId: string }> => {
	const db = opts.db ?? getDB();
	const clientId = newClientId();
	const syntheticId = mintSyntheticId();
	const task = buildOptimisticTask(clientId, syntheticId, input, opts);
	const row: StoredTaskRow = {
		clientId,
		serverId: null,
		updatedAt: task.updatedAt,
		deletedAt: null,
		data: task
	};
	let parentClientId: string | null = null;
	if (opts.parentId != null && opts.parentId <= SYNTHETIC_ID_BASE) {
		parentClientId = await findTaskClientIdBySyntheticId(db, opts.parentId);
	}
	const path = buildCreatePath(opts, parentClientId);
	await db.transaction('rw', db.tasks, db.outbox, async () => {
		await db.tasks.put(toStored(row));
		await enqueue(
			{
				entity: 'tasks',
				op: 'create',
				clientId,
				parentClientId,
				payload: {
					method: 'POST',
					path,
					body: { ...input, clientId }
				}
			},
			db
		);
	});
	emitDbChanged('tasks');
	return task;
};

export interface UpdateTaskOptions {
	db?: TurboistDB;
}

const mergeAndStore = async (
	db: TurboistDB,
	row: StoredTaskRow,
	patch: Partial<Task>
): Promise<TaskData> => {
	const ts = nowIso();
	const nextData: TaskData = { ...row.data, ...patch, updatedAt: ts, clientId: row.clientId };
	const next: StoredTaskRow = {
		...row,
		updatedAt: ts,
		data: nextData
	};
	await db.tasks.put(toStored(next));
	return nextData;
};

export const updateTaskOffline = async (
	task: Pick<Task, 'id'> & { clientId?: string },
	patch: TaskInput,
	opts: UpdateTaskOptions = {}
): Promise<Task & { clientId: string }> => {
	const db = opts.db ?? getDB();
	const row = await resolveTaskRow(db, task);
	if (!row) throw new Error(`updateTaskOffline: task not found in Dexie (id=${task.id})`);
	const baseUpdatedAt = row.updatedAt;
	let next: TaskData;
	await db.transaction('rw', db.tasks, db.outbox, async () => {
		next = await mergeAndStore(db, row, patch as Partial<Task>);
		await enqueue(
			{
				entity: 'tasks',
				op: 'update',
				clientId: row.clientId,
				payload: {
					method: 'PATCH',
					path: row.serverId
						? `/api/v1/tasks/${row.serverId}`
						: '/api/v1/tasks/{serverId}',
					body: { ...patch, baseUpdatedAt }
				}
			},
			db
		);
	});
	emitDbChanged('tasks');
	return next!;
};

export const deleteTaskOffline = async (
	task: Pick<Task, 'id'> & { clientId?: string },
	opts: UpdateTaskOptions = {}
): Promise<void> => {
	const db = opts.db ?? getDB();
	const row = await resolveTaskRow(db, task);
	if (!row) return;
	const ts = nowIso();
	await db.transaction('rw', db.tasks, db.outbox, async () => {
		await db.tasks.put(toStored({ ...row, deletedAt: ts, updatedAt: ts }));
		const entry = await enqueue(
			{
				entity: 'tasks',
				op: 'delete',
				clientId: row.clientId,
				payload: {
					method: 'DELETE',
					path: row.serverId
						? `/api/v1/tasks/${row.serverId}`
						: '/api/v1/tasks/{serverId}'
				}
			},
			db
		);
		await dropPendingFor('tasks', row.clientId, entry.id, db);
	});
	emitDbChanged('tasks');
};

export const completeTaskOffline = async (
	task: Pick<Task, 'id'> & { clientId?: string },
	completedAt?: string,
	opts: UpdateTaskOptions = {}
): Promise<Task & { clientId: string }> => {
	const db = opts.db ?? getDB();
	const row = await resolveTaskRow(db, task);
	if (!row) throw new Error(`completeTaskOffline: task not found (id=${task.id})`);
	const ts = completedAt ?? nowIso();
	let next: TaskData;
	await db.transaction('rw', db.tasks, db.outbox, async () => {
		next = await mergeAndStore(db, row, { status: 'completed', completedAt: ts });
		const entry = await enqueue(
			{
				entity: 'tasks',
				op: 'update',
				clientId: row.clientId,
				payload: {
					method: 'POST',
					path: row.serverId
						? `/api/v1/tasks/${row.serverId}/complete`
						: '/api/v1/tasks/{serverId}/complete',
					body: completedAt ? { completedAt } : {}
				}
			},
			db
		);
		await dropPendingByPathSuffix(
			'tasks',
			row.clientId,
			COMPLETE_TOGGLE_SUFFIXES,
			entry.id,
			db
		);
	});
	emitDbChanged('tasks');
	return next!;
};

export const uncompleteTaskOffline = async (
	task: Pick<Task, 'id'> & { clientId?: string },
	opts: UpdateTaskOptions = {}
): Promise<Task & { clientId: string }> => {
	const db = opts.db ?? getDB();
	const row = await resolveTaskRow(db, task);
	if (!row) throw new Error(`uncompleteTaskOffline: task not found (id=${task.id})`);
	let next: TaskData;
	await db.transaction('rw', db.tasks, db.outbox, async () => {
		next = await mergeAndStore(db, row, { status: 'open', completedAt: null });
		const entry = await enqueue(
			{
				entity: 'tasks',
				op: 'update',
				clientId: row.clientId,
				payload: {
					method: 'POST',
					path: row.serverId
						? `/api/v1/tasks/${row.serverId}/uncomplete`
						: '/api/v1/tasks/{serverId}/uncomplete',
					body: {}
				}
			},
			db
		);
		await dropPendingByPathSuffix(
			'tasks',
			row.clientId,
			COMPLETE_TOGGLE_SUFFIXES,
			entry.id,
			db
		);
	});
	emitDbChanged('tasks');
	return next!;
};

export const moveTaskOffline = async (
	task: Pick<Task, 'id'> & { clientId?: string },
	move: TaskMoveInput,
	opts: UpdateTaskOptions = {}
): Promise<Task & { clientId: string }> => {
	const db = opts.db ?? getDB();
	const row = await resolveTaskRow(db, task);
	if (!row) throw new Error(`moveTaskOffline: task not found (id=${task.id})`);
	const patch: Partial<Task> = {};
	if ('inboxId' in move) {
		patch.inboxId = move.inboxId;
		patch.contextId = null;
		patch.projectId = null;
		patch.sectionId = null;
	} else if ('parentId' in move) {
		patch.parentId = move.parentId;
	} else {
		patch.inboxId = null;
		patch.contextId = move.contextId;
		patch.projectId = move.projectId ?? null;
		patch.sectionId = move.sectionId ?? null;
	}
	let next: TaskData;
	await db.transaction('rw', db.tasks, db.outbox, async () => {
		next = await mergeAndStore(db, row, patch);
		await enqueue(
			{
				entity: 'tasks',
				op: 'update',
				clientId: row.clientId,
				payload: {
					method: 'POST',
					path: row.serverId
						? `/api/v1/tasks/${row.serverId}/move`
						: '/api/v1/tasks/{serverId}/move',
					body: move as unknown as Record<string, unknown>
				}
			},
			db
		);
	});
	emitDbChanged('tasks');
	return next!;
};

export const planTaskOffline = async (
	task: Pick<Task, 'id'> & { clientId?: string },
	plan: TaskPlanInput,
	opts: UpdateTaskOptions = {}
): Promise<Task & { clientId: string }> => {
	const db = opts.db ?? getDB();
	const row = await resolveTaskRow(db, task);
	if (!row) throw new Error(`planTaskOffline: task not found (id=${task.id})`);
	let next: TaskData;
	await db.transaction('rw', db.tasks, db.outbox, async () => {
		next = await mergeAndStore(db, row, { planState: plan.state });
		await enqueue(
			{
				entity: 'tasks',
				op: 'update',
				clientId: row.clientId,
				payload: {
					method: 'POST',
					path: row.serverId
						? `/api/v1/tasks/${row.serverId}/plan`
						: '/api/v1/tasks/{serverId}/plan',
					body: { state: plan.state }
				}
			},
			db
		);
	});
	emitDbChanged('tasks');
	return next!;
};

export const isSyntheticTaskId = (id: number): boolean => id <= SYNTHETIC_ID_BASE;

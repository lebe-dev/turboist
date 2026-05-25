import {
	getDB,
	ENTITY_TABLES,
	type EntityKind,
	type OutboxEntry,
	type StoredEntity,
	type TurboistDB
} from './db';
import { isRef } from './ids';
import {
	listReady,
	markFailed,
	markInflight,
	remove as outboxRemove
} from './outbox';
import { emitDbChanged } from './stores';

export interface FetchResponse {
	status: number;
	data: unknown;
}

export interface SyncFetch {
	(
		path: string,
		init: {
			method: string;
			headers?: Record<string, string>;
			body?: unknown;
		}
	): Promise<FetchResponse>;
}

export interface ServerEntity {
	id: number;
	clientId?: string | null;
	updatedAt: string;
	deletedAt?: string | null;
	[k: string]: unknown;
}

export interface PullBundle {
	now: string;
	tasks: ServerEntity[];
	projects: ServerEntity[];
	sections: ServerEntity[];
	labels: ServerEntity[];
	contexts: ServerEntity[];
}

export interface FlushResult {
	sent: number;
	failed: number;
	dropped: number;
}

export interface OutboxDispatch {
	method?: 'POST' | 'PATCH' | 'DELETE';
	path: string;
	body?: Record<string, unknown>;
}

const PULL_PATH = '/api/v1/sync/pull';

export const pull = async (
	since: string | undefined,
	fetcher: SyncFetch,
	db: TurboistDB = getDB()
): Promise<PullBundle> => {
	const path = since ? `${PULL_PATH}?since=${encodeURIComponent(since)}` : PULL_PATH;
	const res = await fetcher(path, { method: 'POST' });
	if (res.status < 200 || res.status >= 300) {
		throw new Error(`sync pull failed: ${res.status}`);
	}
	const bundle = res.data as PullBundle;
	for (const kind of ENTITY_TABLES) {
		const rows = (bundle as unknown as Record<EntityKind, ServerEntity[] | undefined>)[kind];
		if (!rows || rows.length === 0) continue;
		for (const row of rows) {
			await upsertEntity(db, kind, row);
		}
		emitDbChanged(kind);
	}
	if (bundle.now) {
		await db.meta.put({ key: 'lastPulledAt', value: bundle.now });
	}
	return bundle;
};

const upsertEntity = async (
	db: TurboistDB,
	kind: EntityKind,
	row: ServerEntity
): Promise<void> => {
	const table = db.table(kind);
	const byServer = (await table.where('serverId').equals(row.id).first()) as
		| StoredEntity
		| undefined;
	const clientId = byServer?.clientId ?? row.clientId ?? `srv:${row.id}`;
	const stored: StoredEntity = {
		clientId,
		serverId: row.id,
		updatedAt: row.updatedAt,
		deletedAt: row.deletedAt ?? null,
		data: row as Record<string, unknown>
	};
	await table.put(stored);
};

export const flush = async (
	fetcher: SyncFetch,
	db: TurboistDB = getDB()
): Promise<FlushResult> => {
	const result: FlushResult = { sent: 0, failed: 0, dropped: 0 };
	const MAX_PASSES = 8;
	for (let pass = 0; pass < MAX_PASSES; pass++) {
		const entries = await listReady(Date.now(), db);
		if (entries.length === 0) break;
		let progressed = false;
		for (const entry of entries) {
			const outcome = await sendOne(db, fetcher, entry);
			if (outcome === 'sent') {
				result.sent += 1;
				progressed = true;
			} else if (outcome === 'dropped') {
				result.dropped += 1;
				progressed = true;
			} else if (outcome === 'failed') {
				result.failed += 1;
			}
		}
		if (!progressed) break;
	}
	return result;
};

type SendOutcome = 'sent' | 'failed' | 'dropped' | 'deferred';

const sendOne = async (
	db: TurboistDB,
	fetcher: SyncFetch,
	entry: OutboxEntry
): Promise<SendOutcome> => {
	const prepared = await prepareDispatch(db, entry);
	if (prepared === 'unresolved') return 'deferred';
	if (prepared === 'noop-delete') {
		await outboxRemove(entry.id, db);
		await markRowTombstone(db, entry);
		emitDbChanged(entry.entity);
		return 'dropped';
	}
	await markInflight(entry.id, db);
	let res: FetchResponse;
	try {
		res = await fetcher(prepared.path, {
			method: prepared.method,
			headers: { 'Idempotency-Key': entry.idempotencyKey },
			body: prepared.body
		});
	} catch (err) {
		await markFailed(entry.id, errMessage(err), {}, db);
		return 'failed';
	}
	if (res.status === 410) {
		await markRowTombstone(db, entry);
		await outboxRemove(entry.id, db);
		emitDbChanged(entry.entity);
		return 'dropped';
	}
	if (res.status >= 200 && res.status < 300) {
		if (entry.op === 'delete') {
			await markRowTombstone(db, entry);
		} else {
			const server = res.data as ServerEntity | null;
			if (server && typeof server.id === 'number') {
				await applyServerResponse(db, entry, server);
			}
		}
		await outboxRemove(entry.id, db);
		emitDbChanged(entry.entity);
		return 'sent';
	}
	if (res.status >= 400 && res.status < 500) {
		await markFailed(entry.id, `http_${res.status}`, { permanent: true }, db);
		return 'failed';
	}
	await markFailed(entry.id, `http_${res.status}`, {}, db);
	return 'failed';
};

interface Dispatch {
	method: 'POST' | 'PATCH' | 'DELETE';
	path: string;
	body: Record<string, unknown> | undefined;
}

const prepareDispatch = async (
	db: TurboistDB,
	entry: OutboxEntry
): Promise<Dispatch | 'unresolved' | 'noop-delete'> => {
	const payload = entry.payload as Partial<OutboxDispatch>;
	const method: 'POST' | 'PATCH' | 'DELETE' =
		payload.method ?? (entry.op === 'create' ? 'POST' : entry.op === 'delete' ? 'DELETE' : 'PATCH');

	const local = (await db.table(entry.entity).get(entry.clientId)) as StoredEntity | undefined;

	let path = payload.path ?? '';
	if (path.includes('{serverId}')) {
		const serverId = local?.serverId;
		if (!serverId) {
			if (entry.op === 'delete') return 'noop-delete';
			return 'unresolved';
		}
		path = path.replaceAll('{serverId}', String(serverId));
	}

	const refPattern = /\{ref:([^}]+)\}/g;
	if (refPattern.test(path)) {
		const replacements: Array<{ token: string; cid: string }> = [];
		path.replace(/\{ref:([^}]+)\}/g, (token, cid: string) => {
			replacements.push({ token, cid });
			return token;
		});
		for (const { token, cid } of replacements) {
			const resolved = await resolveClientIdToServerId(db, cid);
			if (resolved === null) return 'unresolved';
			path = path.replace(token, String(resolved));
		}
	}

	let body: Record<string, unknown> | undefined;
	if (payload.body) {
		const resolved = await resolveBody(db, payload.body);
		if (resolved === 'unresolved') return 'unresolved';
		body = resolved;
	}
	return { method, path, body };
};

const resolveBody = async (
	db: TurboistDB,
	body: Record<string, unknown>
): Promise<Record<string, unknown> | 'unresolved'> => {
	const out: Record<string, unknown> = {};
	for (const [k, v] of Object.entries(body)) {
		if (isRef(v)) {
			const sid = await resolveClientIdToServerId(db, v.clientId);
			if (sid === null) return 'unresolved';
			out[k] = sid;
		} else {
			out[k] = v;
		}
	}
	return out;
};

const resolveClientIdToServerId = async (
	db: TurboistDB,
	clientId: string
): Promise<number | null> => {
	for (const kind of ENTITY_TABLES) {
		const row = (await db.table(kind).get(clientId)) as StoredEntity | undefined;
		if (row?.serverId) return row.serverId;
	}
	return null;
};

const applyServerResponse = async (
	db: TurboistDB,
	entry: OutboxEntry,
	server: ServerEntity
): Promise<void> => {
	const tbl = db.table(entry.entity);
	const existing = (await tbl.get(entry.clientId)) as StoredEntity | undefined;
	const stored: StoredEntity = {
		clientId: existing?.clientId ?? entry.clientId,
		serverId: server.id,
		updatedAt: server.updatedAt,
		deletedAt: server.deletedAt ?? null,
		data: server as Record<string, unknown>
	};
	await tbl.put(stored);
};

const markRowTombstone = async (db: TurboistDB, entry: OutboxEntry): Promise<void> => {
	const tbl = db.table(entry.entity);
	const existing = (await tbl.get(entry.clientId)) as StoredEntity | undefined;
	if (!existing) return;
	await tbl.put({ ...existing, deletedAt: new Date().toISOString() });
};

export interface ApiClientLike {
	rawFetch(
		path: string,
		init?: {
			method?: string;
			headers?: Record<string, string>;
			body?: unknown;
		}
	): Promise<{ status: number; data: unknown }>;
}

// fetcherFromClient adapts an ApiClient to the SyncFetch shape: requests go
// through the client's auth/refresh pipeline, but the offline sync sees raw
// status codes instead of thrown ApiError instances.
export const fetcherFromClient = (client: ApiClientLike): SyncFetch =>
	(path, init) =>
		client.rawFetch(path, {
			method: init.method,
			headers: init.headers,
			body: init.body
		});

const errMessage = (err: unknown): string => {
	if (err instanceof Error) return err.message || 'network';
	return 'network';
};

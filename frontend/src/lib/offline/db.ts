import { openDB, type DBSchema, type IDBPDatabase } from 'idb';
import { canonicalizeQuery } from '../api/client';

// IndexedDB layer for the offline module (FEATURE-OFFLINE-ARCH.md §4.2, §4.10).
//
// One database, `turboist-offline`, holds four stores: read-through cache
// (`responses`), the mutation queue (`outbox`), quarantined mutations
// (`failedOps`) and a small key/value `meta` store. The database is bound to a
// single server: when the native `serverUrl` changes the whole database is
// dropped, because a cache/outbox from another server is meaningless.
//
// If IndexedDB is unavailable (old Safari private mode, SSR) `openOfflineDB`
// returns a no-op implementation so the app behaves exactly like the
// pure-online build — no store, no queue, every call a silent no-op.

export const DB_NAME = 'turboist-offline';

/** Structural IndexedDB version — bump only when the object stores change. */
export const DB_VERSION = 1;

/**
 * Logical data schema version stored in `meta.schemaVersion`. Bump when the
 * *shape* of cached/queued values changes without an IndexedDB store change:
 * on mismatch the cache is wiped and incompatible outbox rows are quarantined.
 */
export const SCHEMA_VERSION = 1;

/** Current `QueuedOp.v`; outbox rows with a different `v` are quarantined. */
export const OUTBOX_OP_VERSION = 1;

/** Hard cap on cached responses; the oldest by `storedAt` are evicted first. */
export const MAX_RESPONSES = 500;

export type OfflineOpType = 'task.complete' | 'task.uncomplete' | 'task.createInbox';

export interface CachedResponse {
	cacheKey: string;
	payload: unknown;
	/** ISO-8601 UTC — also the eviction key (lexicographic === chronological). */
	storedAt: string;
	/** Request path, kept for cross-entry lookups (e.g. findTask in readCache). */
	path: string;
}

export interface QueuedOp {
	/** Op format version; lets a queue survive a bundle deploy (§4.5). */
	v: number;
	/** autoIncrement primary key — FIFO order. */
	seq: number;
	opId: string;
	/** Generated once at enqueue; replay resends the same key for idempotency. */
	idempotencyKey: string;
	type: OfflineOpType;
	payload: Record<string, unknown>;
	createdAt: string;
	attempts: number;
}

export interface FailedOp extends QueuedOp {
	failedAt: string;
	errorCode: string;
	errorMessage: string;
}

/** Failure metadata stamped onto an op when it is quarantined (§4.6). */
export type OpFailure = Pick<FailedOp, 'failedAt' | 'errorCode' | 'errorMessage'>;

interface MetaRow {
	key: string;
	value: unknown;
}

export interface OfflineDB {
	/** false when running against the no-op fallback (no IndexedDB). */
	readonly available: boolean;

	// responses (read-through cache)
	getResponse(cacheKey: string): Promise<CachedResponse | null>;
	putResponse(entry: CachedResponse): Promise<void>;
	getAllResponses(): Promise<CachedResponse[]>;
	deleteResponse(cacheKey: string): Promise<void>;
	clearResponses(): Promise<void>;

	// outbox (queued mutations)
	enqueue(op: Omit<QueuedOp, 'seq'>): Promise<number>;
	listOutbox(): Promise<QueuedOp[]>;
	updateOutbox(op: QueuedOp): Promise<void>;
	deleteOutbox(seq: number): Promise<void>;
	/** Atomically relocate a queued op into `failedOps`, stamping failure metadata (§4.6). */
	moveToFailed(op: QueuedOp, failure: OpFailure): Promise<void>;

	// failedOps (quarantined mutations, surfaced to the user)
	listFailed(): Promise<FailedOp[]>;
	pushFailed(op: FailedOp): Promise<void>;
	deleteFailed(seq: number): Promise<void>;
	clearFailed(): Promise<void>;

	// meta (key/value)
	getMeta<T = unknown>(key: string): Promise<T | undefined>;
	setMeta(key: string, value: unknown): Promise<void>;
	/**
	 * Mint the next tmp id for an offline-created entity: a strictly-decreasing
	 * NEGATIVE integer `-(++meta.tmpIdCounter)`, persisted so ids stay unique
	 * across restarts and never collide with server autoincrement ids (§4.5).
	 */
	nextTmpId(): Promise<number>;

	// whole database
	clearAll(): Promise<void>;
	close(): void;
}

interface OfflineSchema extends DBSchema {
	responses: {
		key: string;
		value: CachedResponse;
		indexes: { 'by-storedAt': string };
	};
	outbox: {
		key: number;
		value: QueuedOp;
	};
	failedOps: {
		key: number;
		value: FailedOp;
	};
	meta: {
		key: string;
		value: MetaRow;
	};
}

/**
 * Canonical cache key: `path` plus a canonicalized query. Shares
 * `canonicalizeQuery` with `ApiClient.buildUrl` (the single source of truth for
 * query serialization), so query-order variations collapse to one cache entry
 * and a request URL and its cache key always agree (§4.2).
 */
export function buildCacheKey(path: string, query?: unknown): string {
	const qs = canonicalizeQuery(query);
	return qs ? `${path}?${qs}` : path;
}

/**
 * Open the offline database bound to `serverUrl`, reconciling server-url and
 * schema-version drift. Never throws: on any failure it warns once and returns
 * a no-op implementation so callers can treat offline support as best-effort.
 */
export async function openOfflineDB(serverUrl: string): Promise<OfflineDB> {
	if (typeof indexedDB === 'undefined') {
		console.warn('[offline] IndexedDB is unavailable; running in pure-online mode');
		return createNoopDB();
	}

	let db: IDBPDatabase<OfflineSchema>;
	try {
		db = await openDB<OfflineSchema>(DB_NAME, DB_VERSION, {
			upgrade(database) {
				if (!database.objectStoreNames.contains('responses')) {
					const store = database.createObjectStore('responses', { keyPath: 'cacheKey' });
					store.createIndex('by-storedAt', 'storedAt');
				}
				if (!database.objectStoreNames.contains('outbox')) {
					database.createObjectStore('outbox', { keyPath: 'seq', autoIncrement: true });
				}
				if (!database.objectStoreNames.contains('failedOps')) {
					database.createObjectStore('failedOps', { keyPath: 'seq' });
				}
				if (!database.objectStoreNames.contains('meta')) {
					database.createObjectStore('meta', { keyPath: 'key' });
				}
			}
		});
	} catch (err) {
		console.warn('[offline] failed to open IndexedDB; running in pure-online mode', err);
		return createNoopDB();
	}

	const wrapper = createWrapper(db);
	try {
		await reconcile(wrapper, serverUrl);
	} catch (err) {
		// Reconciliation is best-effort — a functioning DB is still useful.
		console.warn('[offline] failed to reconcile offline database', err);
	}
	return wrapper;
}

function createWrapper(db: IDBPDatabase<OfflineSchema>): OfflineDB {
	return {
		available: true,

		async getResponse(cacheKey) {
			return (await db.get('responses', cacheKey)) ?? null;
		},
		async putResponse(entry) {
			const tx = db.transaction('responses', 'readwrite');
			await tx.store.put(entry);
			const count = await tx.store.count();
			if (count > MAX_RESPONSES) {
				let remaining = count - MAX_RESPONSES;
				let cursor = await tx.store.index('by-storedAt').openCursor();
				while (cursor && remaining > 0) {
					await cursor.delete();
					remaining -= 1;
					cursor = await cursor.continue();
				}
			}
			await tx.done;
		},
		async getAllResponses() {
			return db.getAll('responses');
		},
		async deleteResponse(cacheKey) {
			await db.delete('responses', cacheKey);
		},
		async clearResponses() {
			await db.clear('responses');
		},

		async enqueue(op) {
			// autoIncrement fills `seq`; the input intentionally omits it.
			return db.add('outbox', op as QueuedOp);
		},
		async listOutbox() {
			return db.getAll('outbox');
		},
		async updateOutbox(op) {
			await db.put('outbox', op);
		},
		async deleteOutbox(seq) {
			await db.delete('outbox', seq);
		},
		async moveToFailed(op, failure) {
			// One transaction so an op is never lost between the two stores.
			const tx = db.transaction(['outbox', 'failedOps'], 'readwrite');
			await tx.objectStore('failedOps').put({ ...op, ...failure });
			await tx.objectStore('outbox').delete(op.seq);
			await tx.done;
		},

		async listFailed() {
			return db.getAll('failedOps');
		},
		async pushFailed(op) {
			await db.put('failedOps', op);
		},
		async deleteFailed(seq) {
			await db.delete('failedOps', seq);
		},
		async clearFailed() {
			await db.clear('failedOps');
		},

		async getMeta<T = unknown>(key: string) {
			const row = await db.get('meta', key);
			return row === undefined ? undefined : (row.value as T);
		},
		async setMeta(key, value) {
			await db.put('meta', { key, value });
		},
		async nextTmpId() {
			// Read-modify-write the counter in one transaction so concurrent enqueues
			// never mint the same tmp id.
			const tx = db.transaction('meta', 'readwrite');
			const store = tx.objectStore('meta');
			const row = await store.get('tmpIdCounter');
			const current = typeof row?.value === 'number' ? row.value : 0;
			const next = current + 1;
			await store.put({ key: 'tmpIdCounter', value: next });
			await tx.done;
			return -next;
		},

		async clearAll() {
			const tx = db.transaction(['responses', 'outbox', 'failedOps', 'meta'], 'readwrite');
			await Promise.all([
				tx.objectStore('responses').clear(),
				tx.objectStore('outbox').clear(),
				tx.objectStore('failedOps').clear(),
				tx.objectStore('meta').clear()
			]);
			await tx.done;
		},
		close() {
			db.close();
		}
	};
}

/**
 * Bring a freshly-opened database into a consistent state:
 *  1. server-url change → drop the whole database (§4.2); warn if the outbox
 *     was non-empty, since queued work for the old server is discarded;
 *  2. schema-version drift → wipe the cache and quarantine incompatible
 *     outbox rows rather than silently losing user work.
 */
async function reconcile(db: OfflineDB, serverUrl: string): Promise<void> {
	const storedServer = await db.getMeta<string>('serverUrl');
	if (storedServer !== undefined && storedServer !== serverUrl) {
		const pending = await db.listOutbox();
		if (pending.length > 0) {
			console.warn(
				`[offline] server changed (${storedServer} → ${serverUrl}); discarding ${pending.length} queued operation(s)`
			);
		}
		await db.clearAll();
		await db.setMeta('serverUrl', serverUrl);
		await db.setMeta('schemaVersion', SCHEMA_VERSION);
		return;
	}
	if (storedServer === undefined) {
		await db.setMeta('serverUrl', serverUrl);
	}

	const storedSchema = await db.getMeta<number>('schemaVersion');
	if (storedSchema === undefined) {
		await db.setMeta('schemaVersion', SCHEMA_VERSION);
		return;
	}
	if (storedSchema !== SCHEMA_VERSION) {
		await migrateSchema(db);
		await db.setMeta('schemaVersion', SCHEMA_VERSION);
	}
}

/** Wipe the cache; keep trivially-compatible outbox ops, quarantine the rest. */
async function migrateSchema(db: OfflineDB): Promise<void> {
	await db.clearResponses();
	const now = new Date().toISOString();
	for (const op of await db.listOutbox()) {
		if (op.v === OUTBOX_OP_VERSION) continue;
		await db.pushFailed({
			...op,
			failedAt: now,
			errorCode: 'schema_migration',
			errorMessage: `queued operation format v${op.v} is no longer supported`
		});
		await db.deleteOutbox(op.seq);
	}
}

function createNoopDB(): OfflineDB {
	// In-memory only; nothing persists. Offline mutations are unsupported without
	// IndexedDB (the bridge answers `offline_unsupported`), so this counter never
	// needs to survive a restart — it exists only so the surface stays total.
	let tmpCounter = 0;
	return {
		available: false,
		getResponse: async () => null,
		putResponse: async () => {},
		getAllResponses: async () => [],
		deleteResponse: async () => {},
		clearResponses: async () => {},
		enqueue: async () => -1,
		listOutbox: async () => [],
		updateOutbox: async () => {},
		deleteOutbox: async () => {},
		moveToFailed: async () => {},
		listFailed: async () => [],
		pushFailed: async () => {},
		deleteFailed: async () => {},
		clearFailed: async () => {},
		getMeta: async () => undefined,
		setMeta: async () => {},
		nextTmpId: async () => -(++tmpCounter),
		clearAll: async () => {},
		close: () => {}
	};
}

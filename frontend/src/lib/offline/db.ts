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
//
// The connection itself is never treated as permanent: see `createConnection`.

export const DB_NAME = 'turboist-offline';

/** Structural IndexedDB version — bump only when the object stores change. */
export const DB_VERSION = 1;

/**
 * Logical data schema version stored in `meta.schemaVersion`. Bump when the
 * *shape* of cached/queued values changes without an IndexedDB store change:
 * on mismatch the cache is wiped and incompatible outbox rows are quarantined.
 *
 * 2 (v1.15): `/api/v1/config` grew `harpoon` and `taskTemplates`, and the shell
 * now fans both out on boot. A pre-v1.15 config payload left in the cache would
 * be read by that new code during an offline-first launch after an upgrade, so
 * the stale responses must go. Costs one cold-cache launch after upgrading.
 *
 * 3: `Task` grew `blockedByCount`/`relationCount`, and the offline complete guard
 * reads them to refuse completing a blocked task. A cached task written before the
 * fields existed would read as unblocked and queue a completion the server then
 * rejects on replay, so those responses must go too.
 */
export const SCHEMA_VERSION = 3;

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
	/**
	 * Request path (query included). Read back by readCache's path→extractor
	 * registry to know where a given endpoint keeps its task arrays.
	 */
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

type Conn = IDBPDatabase<OfflineSchema>;

function createStores(database: Conn): void {
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

/**
 * True for errors that mean "this handle is dead", not "this operation is wrong":
 *
 *  - `InvalidStateError` — `IDBDatabase.transaction()` on a connection that is
 *    closing, i.e. every call made through a handle the browser already killed;
 *  - `AbortError` — a transaction that was in flight when that happened;
 *  - `UnknownError` — WebKit's catch-all for a failed IndexedDB backing store
 *    ("An internal error was encountered in the Indexed Database server"), which
 *    iOS raises on a connection that outlived a web-view suspend.
 *
 * All three are worth one reopen-and-retry; any other failure is a real error and
 * is rethrown untouched.
 */
function isConnectionLost(err: unknown): boolean {
	const name = (err as { name?: unknown } | null | undefined)?.name;
	return name === 'InvalidStateError' || name === 'AbortError' || name === 'UnknownError';
}

/**
 * Run the body of a multi-step transaction, making sure `tx.done` is always
 * observed.
 *
 * `idb` rejects `tx.done` independently of the individual request promises, so a
 * body that throws (a rejected `put`, an aborted cursor walk) leaves `tx.done`
 * rejected with nobody awaiting it — an unhandled promise rejection that reaches
 * Sentry as a bare `UnknownError` even though the caller handled the failure it
 * already got. On the happy path the commit is awaited as usual.
 */
async function commit<T>(tx: { done: Promise<void> }, body: () => Promise<T>): Promise<T> {
	const done = tx.done;
	// Attach the guard up front: the body can reject before we get to await it.
	done.catch(() => {});
	const result = await body();
	await done;
	return result;
}

/**
 * A connection to the offline database that reopens itself.
 *
 * iOS suspends the WKWebView when the Capacitor app is backgrounded, and WebKit
 * force-closes its IndexedDB connections. The handle the app kept is then dead:
 * every transaction throws `InvalidStateError: the database connection is
 * closing`, and — because the handle used to be opened once per page load —
 * nothing short of restarting the app brought the cache and the outbox back.
 *
 * So the handle is treated as disposable. `terminated` (fired when the browser
 * closes a connection behind our back) forgets it, and `withConnection` also
 * heals on demand: no `terminated` is guaranteed to arrive, and one may arrive
 * only after a call has already failed.
 */
interface Connection {
	/** The open connection, opening one if there is none. */
	get(): Promise<Conn>;
	/** Forget `dead` if it is still the current handle, so the next `get` reopens. */
	discard(dead: Conn): void;
	/** Close and forget the current handle; a later `get` transparently reopens. */
	close(): void;
}

function createConnection(): Connection {
	let opening: Promise<Conn> | null = null;
	let live: Conn | null = null;
	// Identifies the current open attempt, so a late `terminated` from a connection
	// that has already been replaced cannot drop its successor.
	let generation = 0;

	function forget(): void {
		opening = null;
		live = null;
	}

	function get(): Promise<Conn> {
		if (opening) return opening;
		const mine = ++generation;
		const pending: Promise<Conn> = (async () => {
			try {
				const db = await openDB<OfflineSchema>(DB_NAME, DB_VERSION, {
					upgrade: createStores,
					terminated() {
						if (generation === mine) forget();
					}
				});
				live = db;
				return db;
			} catch (err) {
				if (generation === mine) forget();
				throw err;
			}
		})();
		opening = pending;
		return pending;
	}

	return {
		get,
		discard(dead) {
			// A concurrent caller may already have healed the connection — in that
			// case `live` is a fresh handle and must be left alone.
			if (live !== dead) return;
			forget();
			try {
				dead.close();
			} catch {
				// Already closing; nothing to release.
			}
		},
		close() {
			const db = live;
			forget();
			db?.close();
		}
	};
}

/**
 * Run one database operation, reopening the connection and retrying once if the
 * handle turns out to be dead (see `createConnection`). A second failure is the
 * caller's to handle.
 */
async function withConnection<T>(conn: Connection, fn: (db: Conn) => Promise<T>): Promise<T> {
	const db = await conn.get();
	try {
		return await fn(db);
	} catch (err) {
		if (!isConnectionLost(err)) throw err;
		console.warn('[offline] IndexedDB connection was closed; reopening', err);
		conn.discard(db);
		return fn(await conn.get());
	}
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

	const conn = createConnection();
	try {
		await conn.get();
	} catch (err) {
		// Only a failure to open at all is fatal — a connection lost later is healed
		// by `withConnection` instead of downgrading the app to pure-online.
		console.warn('[offline] failed to open IndexedDB; running in pure-online mode', err);
		return createNoopDB();
	}

	const wrapper = createWrapper(conn);
	try {
		await reconcile(wrapper, serverUrl);
	} catch (err) {
		// Reconciliation is best-effort — a functioning DB is still useful.
		console.warn('[offline] failed to reconcile offline database', err);
	}
	return wrapper;
}

function createWrapper(conn: Connection): OfflineDB {
	// Every operation goes through the connection guard: any of them can be the
	// first call after the web view was suspended.
	const run = <T>(fn: (db: Conn) => Promise<T>): Promise<T> => withConnection(conn, fn);

	return {
		available: true,

		async getResponse(cacheKey) {
			return run(async (db) => (await db.get('responses', cacheKey)) ?? null);
		},
		async putResponse(entry) {
			await run(async (db) => {
				const tx = db.transaction('responses', 'readwrite');
				await commit(tx, async () => {
					await tx.store.put(entry);
					const count = await tx.store.count();
					if (count <= MAX_RESPONSES) return;
					let remaining = count - MAX_RESPONSES;
					let cursor = await tx.store.index('by-storedAt').openCursor();
					while (cursor && remaining > 0) {
						await cursor.delete();
						remaining -= 1;
						cursor = await cursor.continue();
					}
				});
			});
		},
		async getAllResponses() {
			return run((db) => db.getAll('responses'));
		},
		async deleteResponse(cacheKey) {
			await run((db) => db.delete('responses', cacheKey));
		},
		async clearResponses() {
			await run((db) => db.clear('responses'));
		},

		async enqueue(op) {
			// autoIncrement fills `seq`; the input intentionally omits it.
			return run((db) => db.add('outbox', op as QueuedOp));
		},
		async listOutbox() {
			return run((db) => db.getAll('outbox'));
		},
		async updateOutbox(op) {
			await run((db) => db.put('outbox', op));
		},
		async deleteOutbox(seq) {
			await run((db) => db.delete('outbox', seq));
		},
		async moveToFailed(op, failure) {
			// One transaction so an op is never lost between the two stores.
			await run(async (db) => {
				const tx = db.transaction(['outbox', 'failedOps'], 'readwrite');
				await commit(tx, async () => {
					await tx.objectStore('failedOps').put({ ...op, ...failure });
					await tx.objectStore('outbox').delete(op.seq);
				});
			});
		},

		async listFailed() {
			return run((db) => db.getAll('failedOps'));
		},
		async pushFailed(op) {
			await run((db) => db.put('failedOps', op));
		},
		async deleteFailed(seq) {
			await run((db) => db.delete('failedOps', seq));
		},
		async clearFailed() {
			await run((db) => db.clear('failedOps'));
		},

		async getMeta<T = unknown>(key: string) {
			const row = await run((db) => db.get('meta', key));
			return row === undefined ? undefined : (row.value as T);
		},
		async setMeta(key, value) {
			await run((db) => db.put('meta', { key, value }));
		},
		async nextTmpId() {
			// Read-modify-write the counter in one transaction so concurrent enqueues
			// never mint the same tmp id.
			return run(async (db) => {
				const tx = db.transaction('meta', 'readwrite');
				return commit(tx, async () => {
					const store = tx.objectStore('meta');
					const row = await store.get('tmpIdCounter');
					const current = typeof row?.value === 'number' ? row.value : 0;
					const next = current + 1;
					await store.put({ key: 'tmpIdCounter', value: next });
					return -next;
				});
			});
		},

		async clearAll() {
			await run(async (db) => {
				const tx = db.transaction(['responses', 'outbox', 'failedOps', 'meta'], 'readwrite');
				await commit(tx, () =>
					Promise.all([
						tx.objectStore('responses').clear(),
						tx.objectStore('outbox').clear(),
						tx.objectStore('failedOps').clear(),
						tx.objectStore('meta').clear()
					])
				);
			});
		},
		close() {
			conn.close();
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

import type { Task } from '../api/types';
import { getApiClient } from '../api/client';
import { ApiError } from '../api/errors';
import { OUTBOX_OP_VERSION, type OfflineDB, type OfflineOpType, type QueuedOp } from './db';
import { opByType } from './ops/registry';

// Outbox helpers (FEATURE-OFFLINE-ARCH.md §4.5). The `QueuedOp` storage type and
// the IndexedDB stores live in `db.ts`; this module is the thin policy layer
// around them: it stamps the invariant op envelope on enqueue, exposes FIFO
// list / remove / fail helpers, and hosts the replay engine (§4.6).

export { OUTBOX_OP_VERSION } from './db';
// QueuedOp v1 (§4.5): { v, seq, opId, idempotencyKey, type, payload, createdAt, attempts }.
export type { QueuedOp, FailedOp } from './db';

/**
 * Input to `enqueueOp`. The invariant fields (`v`, `attempts`, the autoIncrement
 * `seq`, and — unless overridden — `opId`/`idempotencyKey`/`createdAt`) are
 * stamped for you.
 */
export interface EnqueueInput {
	type: OfflineOpType;
	payload: Record<string, unknown>;
	/** Idempotency key to persist; replay resends it. A fresh UUID by default. */
	idempotencyKey?: string;
	/** Op id. A fresh UUID by default. */
	opId?: string;
	/** Creation timestamp (ISO-8601 UTC). `now()` by default. */
	createdAt?: string;
}

/**
 * Enqueue a mutation, stamping the QueuedOp v1 envelope (§4.5): `v` =
 * `OUTBOX_OP_VERSION`, a generated `opId`/`idempotencyKey`, `createdAt` and
 * `attempts: 0`. Returns the stored op including its autoIncrement `seq`.
 */
export async function enqueueOp(
	db: OfflineDB,
	input: EnqueueInput,
	now: () => string = () => new Date().toISOString()
): Promise<QueuedOp> {
	const op: Omit<QueuedOp, 'seq'> = {
		v: OUTBOX_OP_VERSION,
		opId: input.opId ?? crypto.randomUUID(),
		idempotencyKey: input.idempotencyKey ?? crypto.randomUUID(),
		type: input.type,
		payload: input.payload,
		createdAt: input.createdAt ?? now(),
		attempts: 0
	};
	const seq = await db.enqueue(op);
	return { ...op, seq };
}

/** The queue in FIFO order (the autoIncrement `seq` is the store key). */
export function listQueue(db: OfflineDB): Promise<QueuedOp[]> {
	return db.listOutbox();
}

/** Drop a completed op from the queue. */
export function removeOp(db: OfflineDB, seq: number): Promise<void> {
	return db.deleteOutbox(seq);
}

/**
 * Move an op out of the queue into `failedOps` with failure metadata (§4.6): a
 * 4xx conflict, or a 5xx once the retry budget is spent. Surfaced to the user.
 */
export function failOp(
	db: OfflineDB,
	op: QueuedOp,
	errorCode: string,
	errorMessage: string,
	now: () => string = () => new Date().toISOString()
): Promise<void> {
	return db.moveToFailed(op, { failedAt: now(), errorCode, errorMessage });
}

/**
 * Recover a quarantined op (the settings "Retry" action, §4.7.3): re-enqueue it
 * with `attempts` reset to 0 — but keeping the SAME `idempotencyKey` and `opId`,
 * so the backend still treats a resend as a lost-response retry rather than a
 * fresh mutation — then drop it from `failedOps`. Returns the requeued op, or
 * `null` when no failed op has that `seq` (nothing is touched in that case).
 */
export async function retryFailedOp(db: OfflineDB, seq: number): Promise<QueuedOp | null> {
	const failed = (await db.listFailed()).find((op) => op.seq === seq);
	if (!failed) return null;
	const requeued = await enqueueOp(db, {
		type: failed.type,
		payload: failed.payload,
		idempotencyKey: failed.idempotencyKey,
		opId: failed.opId
	});
	await db.deleteFailed(seq);
	return requeued;
}

// ---------------------------------------------------------------------------
// Replay engine (FEATURE-OFFLINE-ARCH.md §4.6)
// ---------------------------------------------------------------------------

/** Default debounce before a `kick()` runs, coalescing a burst of triggers. */
export const REPLAY_DEBOUNCE_MS = 300;

/** How many times a 5xx op is retried across kicks before it is quarantined. */
const MAX_ATTEMPTS = 5;

/**
 * Minimal `ApiClient.fetch` surface the replay engine needs. Structurally
 * satisfied by the real client; a stub is injected in tests.
 */
export interface ReplayClient {
	// Non-generic on purpose: the engine handles opaque server responses (it only
	// reads `.id` off a task.createInbox reply, via a cast), and this keeps stub
	// clients in tests assignable without per-mock casts. The real generic
	// `ApiClient.fetch<T>` is structurally assignable to this.
	fetch(
		path: string,
		init?: { method?: string; body?: unknown; skipOffline?: boolean; idempotencyKey?: string }
	): Promise<unknown>;
}

/** Side-effect callbacks the engine drives (kept out of the engine so it stays testable). */
export interface ReplayHooks {
	/** Push the current outbox length — the source of truth for the pending badge (§4.7). */
	setPendingOps(count: number): void;
	/** One-shot "replay finished" signal that triggers a catch-up refetch (§4.8). */
	noteSynced(): void;
	/** Fired once per drain when one or more ops were quarantined this run (toast, §4.7). */
	onFailed?(count: number): void;
}

export interface ReplayEngineOptions {
	/** Resolves the shared offline DB handle; called on every drain. */
	getDb: () => Promise<OfflineDB>;
	/** Resolves the client used to reissue requests. Defaults to the app singleton. */
	getClient?: () => ReplayClient;
	hooks: ReplayHooks;
	/** Injectable clock (ISO-8601 UTC) for deterministic tests. */
	now?: () => string;
	debounceMs?: number;
}

export interface ReplayEngine {
	/** Debounced, single-flight replay trigger for automatic sources (§4.6). */
	kick(): void;
	/** Replay now (manual "Sync" button / tests); resolves when the pass settles. */
	sync(): Promise<void>;
	/** True while a drain is in progress. */
	readonly replaying: boolean;
	/** tmpId → server id for offline-created tasks, until the post-replay refetch (§4.6). */
	readonly tmpIdMap: ReadonlyMap<number, number>;
	resolveTmpId(tmpId: number): number | undefined;
}

/**
 * Assemble the outbox replay engine (§4.6). A `kick()` debounces then runs a
 * single-flight drain; `sync()` runs immediately (manual trigger). The drain is
 * strictly FIFO by `seq`, one op at a time:
 *
 *   - success        → delete from the outbox (and, for `task.createInbox`, record
 *                       tmpId → server id so the UI can patch until the refetch);
 *   - status 0       → network dropped again: bump `attempts`, stop the loop, wait
 *                       for the next kick;
 *   - 5xx            → bump `attempts`; quarantine once the retry budget is spent,
 *                       otherwise stop the loop (don't hammer a struggling server);
 *   - 4xx (or other) → the server is right (task deleted elsewhere, validation, …):
 *                       quarantine immediately and keep draining the rest.
 *
 * When the queue empties it stamps `meta.lastSyncAt`, zeroes the pending count and
 * fires `noteSynced()` (the refetch signal, §4.8). Degrades to a no-op without
 * IndexedDB (`db.available === false`).
 */
export function createReplayEngine(options: ReplayEngineOptions): ReplayEngine {
	const now = options.now ?? (() => new Date().toISOString());
	const debounceMs = options.debounceMs ?? REPLAY_DEBOUNCE_MS;
	const getClient = options.getClient ?? (() => getApiClient() as ReplayClient);
	const tmpIdMap = new Map<number, number>();

	let inflight: Promise<void> | null = null;
	let rerun = false;
	let debounceTimer: ReturnType<typeof setTimeout> | null = null;

	async function drainOnce(): Promise<void> {
		const db = await options.getDb();
		// No IndexedDB (private-mode Safari): nothing is ever queued, so replay is a
		// pure no-op and must NOT emit a refetch signal.
		if (!db.available) return;
		const queue = await listQueue(db);
		// An empty queue means there was nothing to replay — do not signal a sync
		// (that would fire a pointless refetch on every trigger).
		if (queue.length === 0) return;

		const client = getClient();
		let failedThisRun = 0;

		for (const op of queue) {
			const offlineOp = opByType(op.type);
			if (!offlineOp) {
				// Unknown op type (e.g. a queue row from a rolled-back bundle) — cannot
				// be replayed; quarantine it and move on.
				await failOp(db, op, 'unknown_op', `no handler for ${op.type}`, now);
				failedThisRun += 1;
				continue;
			}

			const req = offlineOp.buildRequest(op.payload);
			try {
				// skipOffline: a replayed request must not read the cache or re-enqueue
				// itself if the network drops again. idempotencyKey: the backend returns
				// its recorded response for a resent key, so a lost response never dupes.
				const response = await client.fetch(req.path, {
					method: req.method,
					body: req.body,
					skipOffline: true,
					idempotencyKey: op.idempotencyKey
				});
				await removeOp(db, op.seq);
				if (op.type === 'task.createInbox') {
					const tmpId = op.payload.tmpId;
					const realId = (response as Task | null)?.id;
					if (typeof tmpId === 'number' && typeof realId === 'number' && realId > 0) {
						tmpIdMap.set(tmpId, realId);
					}
				}
			} catch (err) {
				const status = err instanceof ApiError ? err.status : null;
				const code = err instanceof ApiError ? err.code : 'unknown_error';
				const message = err instanceof Error ? err.message : String(err);

				if (status === 0) {
					// Network dropped mid-replay: keep the op, remember the attempt, and
					// wait for the next kick — do not spin.
					op.attempts += 1;
					await db.updateOutbox(op);
					break;
				}
				if (status !== null && status >= 500) {
					// Server error: back off. Quarantine only once the budget is spent.
					op.attempts += 1;
					if (op.attempts >= MAX_ATTEMPTS) {
						await failOp(db, op, code, message, now);
						failedThisRun += 1;
					} else {
						await db.updateOutbox(op);
					}
					break; // don't hammer a struggling server; retry on the next kick
				}
				// 4xx (or any other non-network, non-5xx): a conflict the server is
				// right about. Quarantine and keep draining the rest of the queue.
				await failOp(db, op, code, message, now);
				failedThisRun += 1;
			}
		}

		const remaining = await listQueue(db);
		options.hooks.setPendingOps(remaining.length);
		if (remaining.length === 0) {
			await db.setMeta('lastSyncAt', now());
			options.hooks.noteSynced();
		}
		if (failedThisRun > 0) {
			options.hooks.onFailed?.(failedThisRun);
		}
	}

	function run(): Promise<void> {
		// Single-flight: a trigger arriving mid-drain re-runs once the current pass
		// settles (so an op enqueued during replay is not stranded).
		if (inflight) {
			rerun = true;
			return inflight;
		}
		inflight = (async () => {
			try {
				do {
					rerun = false;
					await drainOnce();
				} while (rerun);
			} finally {
				inflight = null;
			}
		})();
		return inflight;
	}

	return {
		kick(): void {
			if (debounceTimer !== null) clearTimeout(debounceTimer);
			debounceTimer = setTimeout(() => {
				debounceTimer = null;
				void run();
			}, debounceMs);
		},
		sync(): Promise<void> {
			if (debounceTimer !== null) {
				clearTimeout(debounceTimer);
				debounceTimer = null;
			}
			return run();
		},
		get replaying(): boolean {
			return inflight !== null;
		},
		get tmpIdMap(): ReadonlyMap<number, number> {
			return tmpIdMap;
		},
		resolveTmpId(tmpId: number): number | undefined {
			return tmpIdMap.get(tmpId);
		}
	};
}

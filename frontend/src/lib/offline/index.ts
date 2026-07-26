import { get } from 'svelte/store';
import { t } from '$lib/i18n';
import { toast } from 'svelte-sonner';
import type { OfflineBridge } from '../api/client';
import { openOfflineDB, type FailedOp } from './db';
import {
	createReplayEngine,
	enqueueOp,
	listQueue,
	retryFailedOp,
	type ReplayEngine
} from './outbox';
import { matchOp } from './ops/registry';
import { createReadCache, matchTaskDetailPath, type ReadCache } from './readCache';
import { statusStore } from './status.svelte';

export { statusStore, createStatusStore } from './status.svelte';
export type { QueuedOp, FailedOp } from './db';
export {
	cacheWarmer,
	createCacheWarmer,
	warmTargets,
	WARM_DEBOUNCE_MS,
	type CacheWarmer,
	type CacheWarmerOptions
} from './warmCache';

export interface OfflineBridgeOptions {
	/** Server URL used to namespace the IndexedDB (§4.2); '' on web. */
	serverUrl: string;
}

/**
 * Assemble the concrete `OfflineBridge` (FEATURE-OFFLINE-ARCH.md §4.10) that
 * `ApiClient` consults on the GET path (§4.4). Reads are fully wired — the
 * read-through cache plus the online/offline status heuristic — while the
 * mutation-enqueue path (`tryEnqueue`) lands in Epic C and reports "not
 * supported offline" (null) until then.
 *
 * The IndexedDB opens asynchronously and never throws: on a private-mode Safari
 * / unavailable IndexedDB it resolves to a silent no-op DB, so the whole bridge
 * degrades to a pure-online pass-through (every cache read misses, every mutation
 * reports unsupported).
 */
/**
 * Last resort for `GET /api/v1/tasks/:id` (the task detail page): the detail GET
 * itself is only cached after the page has been opened online, so a task the user
 * can see in a cached list would otherwise fail to open offline. Rebuild the
 * response from the tasks the cache already holds.
 */
async function taskDetailFallback(
	cache: ReadCache,
	path: string,
	query: unknown
): Promise<{ payload: unknown; storedAt: string } | null> {
	const id = matchTaskDetailPath(path);
	if (id === null) return null;
	const wantsSubtasks = (query as { subtasks?: unknown } | undefined)?.subtasks === 'true';
	return cache.findTaskDetail(id, wantsSubtasks);
}

export function createOfflineBridge(options: OfflineBridgeOptions): OfflineBridge {
	// Open the DB once and share the handle between the read-through cache and the
	// outbox: the mutation path needs both the raw DB (enqueue / tmpId / queue
	// length) and the ReadCache (synthesize + patch), and they must be the same
	// underlying database instance.
	const dbPromise = openOfflineDB(options.serverUrl);
	// Retain the handle at module scope so `clearOfflineData` (the confirmed-logout
	// wipe, §4.9) can reach the same database without another open.
	dbHandle = dbPromise;
	const cachePromise: Promise<ReadCache> = dbPromise.then((db) => createReadCache(db));

	// Seed the reactive counters from what a previous (offline) session left behind
	// so a queue/quarantine survives a restart in the UI, not just on disk (§4.7).
	void dbPromise.then(async (db) => {
		statusStore.setPendingOps((await db.listOutbox()).length);
		statusStore.setFailedOps((await db.listFailed()).length);
	});

	// The replay engine (§4.6) shares the bridge's DB handle: the mutation path
	// enqueues, replay drains. Triggers are wired separately (wireReplayTriggers +
	// the app shell) so simply assembling a bridge attaches no global listeners.
	replayEngine = createReplayEngine({
		getDb: () => dbPromise,
		hooks: {
			setPendingOps: (n) => statusStore.setPendingOps(n),
			noteSynced: () => statusStore.noteSynced(),
			onFailed: (count) => {
				toast.error(get(t)('offline.replayFailed', { values: { count } }));
				// A drain quarantined ops — refresh the failed counter from the store of
				// truth so the settings recovery UI and logout gate stay accurate (§4.7).
				void dbPromise.then(async (db) => statusStore.setFailedOps((await db.listFailed()).length));
			}
		}
	});

	return {
		isOffline(): boolean {
			return !statusStore.online;
		},
		async cacheGet(path, query) {
			const cache = await cachePromise;
			const hit = (await cache.get(path, query)) ?? (await taskDetailFallback(cache, path, query));
			if (!hit) return null;
			// A cache hit is only ever consulted when live data could not be served
			// (cache-first while offline, or a network-error fallback), so the view
			// is being rendered from stale cache (§4.7).
			statusStore.markStale();
			return { payload: hit.payload, storedAt: hit.storedAt };
		},
		async cachePut(path, query, payload) {
			const cache = await cachePromise;
			await cache.put(path, query, payload);
			// Fresh data landed → the view is live again.
			statusStore.clearStale();
		},
		async tryEnqueue(path, method, body, idempotencyKey) {
			const db = await dbPromise;
			// No IndexedDB (private-mode Safari): the queue cannot persist, so an
			// offline mutation is unsupported and the caller answers offline_unsupported.
			if (!db.available) return null;
			const match = matchOp(path, method, body);
			// Not a whitelisted op, or a `blocked` op on a tmp task (id < 0) whose
			// creation is itself still queued — either way, unsupported offline (§4.5).
			if (!match || match.kind !== 'op') return null;

			const { op, payload } = match;
			const cache = await cachePromise;
			// task.createInbox: mint the stable negative id BEFORE synthesizing, so the
			// tmp task keeps the same id across a restart and is shared with applyToCache.
			if (op.type === 'task.createInbox') {
				payload.tmpId = await db.nextTmpId();
			}
			const response = await op.synthesizeResponse(payload, cache);
			// Patch cached GET responses so the change survives a restart (C4 fills these).
			await op.applyToCache(payload, cache);
			// Persist the op under the SAME idempotency key the failed foreground
			// request already sent, so the replay engine (C3) resends it and the
			// backend recognises a lost-response retry instead of re-executing (§6.3).
			// enqueueOp falls back to a fresh key only if the caller had none.
			await enqueueOp(db, { type: op.type, payload, idempotencyKey });
			// The outbox is the source of truth for the pending count (§4.7).
			statusStore.setPendingOps((await listQueue(db)).length);
			return { response };
		},
		noteRequestOutcome(ok: boolean): void {
			statusStore.noteOutcome(ok);
		}
	};
}

// Singleton replay engine, assembled alongside the bridge (`createOfflineBridge`)
// so it shares the same DB handle. Null until the bridge is built; the trigger
// helpers below are safe no-ops before then.
let replayEngine: ReplayEngine | null = null;

// The shared offline-DB handle, captured when the bridge is assembled so
// `clearOfflineData` can wipe the same database on a confirmed logout.
let dbHandle: ReturnType<typeof openOfflineDB> | null = null;

/**
 * Discard the entire offline database — read-through cache, outbox and
 * quarantined ops — and reset the pending counter (§4.9). Called by the logout
 * flow ONLY after the user has confirmed discarding any unsent changes. A safe
 * no-op before the bridge is assembled or without IndexedDB.
 */
export async function clearOfflineData(): Promise<void> {
	if (!dbHandle) return;
	const db = await dbHandle;
	await db.clearAll();
	statusStore.setPendingOps(0);
	statusStore.setFailedOps(0);
}

/**
 * The quarantined ops surfaced in the settings "Unsent changes" recovery UI
 * (§4.7.3). Empty before the bridge is assembled or without IndexedDB.
 */
export async function listFailedOps(): Promise<FailedOp[]> {
	if (!dbHandle) return [];
	const db = await dbHandle;
	return db.listFailed();
}

/**
 * Recover a quarantined op (settings "Retry", §4.7.3): re-enqueue it (attempts
 * reset, idempotency key preserved — see `retryFailedOp`), refresh both reactive
 * counters, then drain the outbox so it is resent immediately.
 */
export async function retryFailed(seq: number): Promise<void> {
	if (!dbHandle) return;
	const db = await dbHandle;
	await retryFailedOp(db, seq);
	statusStore.setPendingOps((await db.listOutbox()).length);
	statusStore.setFailedOps((await db.listFailed()).length);
	void syncReplay();
}

/**
 * Discard a single quarantined op (settings "Remove", §4.7.3) and refresh the
 * failed counter. Only the chosen op is dropped; the queue is untouched.
 */
export async function removeFailed(seq: number): Promise<void> {
	if (!dbHandle) return;
	const db = await dbHandle;
	await db.deleteFailed(seq);
	statusStore.setFailedOps((await db.listFailed()).length);
}

/**
 * Trigger a debounced, single-flight outbox drain (§4.6) — the entry point for
 * automatic sources (`online`, `visibilitychange`, SSE reconnect, app start).
 */
export function kickReplay(): void {
	replayEngine?.kick();
}

/**
 * Drain the outbox immediately (manual "Sync" / offline-banner retry). Resolves
 * when the pass settles; a no-op resolving promise before the bridge is assembled.
 */
export function syncReplay(): Promise<void> {
	return replayEngine?.sync() ?? Promise.resolve();
}

/**
 * Attach the DOM/lifecycle replay triggers (§4.6): `window 'online'`,
 * `visibilitychange → visible`, plus an immediate app-start kick to drain a queue
 * left over from a previous offline session. The reactive SSE-connected trigger
 * lives in the app shell (it needs a runes scope). Returns a disposer; a no-op on
 * the server.
 */
export function wireReplayTriggers(): () => void {
	if (typeof window === 'undefined') return () => {};
	const onOnline = (): void => kickReplay();
	const onVisible = (): void => {
		if (document.visibilityState === 'visible') kickReplay();
	};
	window.addEventListener('online', onOnline);
	document.addEventListener('visibilitychange', onVisible);
	kickReplay();
	return () => {
		window.removeEventListener('online', onOnline);
		document.removeEventListener('visibilitychange', onVisible);
	};
}

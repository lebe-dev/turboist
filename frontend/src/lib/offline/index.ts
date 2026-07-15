import type { OfflineBridge } from '../api/client';
import { openOfflineDB } from './db';
import { createReadCache, type ReadCache } from './readCache';
import { statusStore } from './status.svelte';

export { statusStore, createStatusStore } from './status.svelte';
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
 * degrades to a pure-online pass-through (every cache read misses).
 */
export function createOfflineBridge(options: OfflineBridgeOptions): OfflineBridge {
	const cachePromise: Promise<ReadCache> = openOfflineDB(options.serverUrl).then((db) =>
		createReadCache(db)
	);

	return {
		isOffline(): boolean {
			return !statusStore.online;
		},
		async cacheGet(path, query) {
			const cache = await cachePromise;
			const hit = await cache.get(path, query);
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
		async tryEnqueue() {
			// Epic C wires the outbox; until then mutations are unsupported offline.
			return null;
		},
		noteRequestOutcome(ok: boolean): void {
			statusStore.noteOutcome(ok);
		}
	};
}

import { buildCacheKey, type CachedResponse, type OfflineDB } from './db';

// Read-through cache for GET responses (FEATURE-OFFLINE-ARCH.md §4.3).
//
// Successful GETs are written through to the `responses` store; reads look them
// up by (path, query). The cache is deliberately dumb: online mode always hits
// the network and simply overwrites entries with fresh responses (existing SSE
// invalidations and `onMutation` refetch drive those writes), so there is no
// separate invalidation mechanism — on logout the whole store is cleared.
//
// This module owns no IndexedDB itself; it is a thin policy layer over an
// `OfflineDB` (which is a silent no-op when IndexedDB is unavailable, so the
// cache degrades to a pure-online pass-through with zero extra guards here).

/** Only GETs under this prefix are cached; `/auth/*` and `/api/config` are not. */
const CACHEABLE_PREFIX = '/api/v1/';

/**
 * Never cached (§4.3): the one-shot SSE ticket (single use) and the backup blob
 * (large binary export). Matched against the path with any query string removed.
 */
const EXCLUDED_PATHS = new Set(['/api/v1/events/ticket', '/api/v1/backup']);

/** True when a GET to `path` may be written through to / served from the cache. */
export function isCacheable(path: string): boolean {
	const clean = path.split('?', 1)[0];
	if (!clean.startsWith(CACHEABLE_PREFIX)) return false;
	return !EXCLUDED_PATHS.has(clean);
}

/**
 * Read side of the cache, used to serve stale GETs and to synthesize offline
 * responses for queued operations (§4.5).
 */
export interface ReadCacheReader {
	/** Look up a cached response by (path, query); null on a miss. */
	get(path: string, query?: unknown): Promise<CachedResponse | null>;
	/**
	 * Locate a cached Task by id across the known response shapes. Stub for now:
	 * Epic C (outbox) implements the cross-shape scan; today it always reports
	 * "not found" (null) and op synthesizers fall back to a minimal Task (§4.5).
	 */
	findTask(id: number): Promise<unknown | null>;
}

/**
 * Write side of the cache, used by GET write-through and by op cache-patchers
 * (`applyToCache`, §4.5) that rewrite entries so an offline change survives a
 * restart.
 */
export interface ReadCacheWriter {
	/** Every cached response, for cross-entry patching. */
	getAll(): Promise<CachedResponse[]>;
	/** Write-through a successful GET (no-op for uncacheable paths / empty bodies). */
	put(path: string, query: unknown, payload: unknown): Promise<void>;
	/** Overwrite a full cache entry (e.g. after patching its payload). */
	putEntry(entry: CachedResponse): Promise<void>;
}

export interface ReadCache extends ReadCacheReader, ReadCacheWriter {}

/**
 * Build a `ReadCache` over an `OfflineDB`. `now` is injectable for deterministic
 * `storedAt` timestamps in tests; it defaults to the wall clock in ISO-8601 UTC.
 */
export function createReadCache(
	db: OfflineDB,
	now: () => string = () => new Date().toISOString()
): ReadCache {
	return {
		async get(path, query) {
			return db.getResponse(buildCacheKey(path, query));
		},
		async findTask() {
			// Epic C implements the cross-shape scan; null === not cached.
			return null;
		},
		async getAll() {
			return db.getAllResponses();
		},
		async put(path, query, payload) {
			// Write-through only cacheable GETs; skip empty (204) bodies so a hit
			// always carries a real payload.
			if (payload === undefined || !isCacheable(path)) return;
			await db.putResponse({
				cacheKey: buildCacheKey(path, query),
				payload,
				storedAt: now(),
				path
			});
		},
		async putEntry(entry) {
			await db.putResponse(entry);
		}
	};
}

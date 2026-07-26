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
	 * Locate a cached Task by id across the known response shapes, for op response
	 * synthesis (§4.5). Each entry is traversed with the extractor registered for
	 * its path (see `EXTRACTORS`), which covers the list views, the bundles and the
	 * aggregates alike. Returns null when the task is nowhere in cache, and op
	 * synthesizers fall back to a minimal Task.
	 */
	findTask(id: number): Promise<unknown | null>;
	/**
	 * Synthesize a `GET /api/v1/tasks/:id` response for the task detail page from
	 * whatever the cache already holds. The page is reachable from every list, but
	 * its own detail GET is only cached once it has been opened online (and under
	 * its exact `?subtasks=true` key), so without this a cached task opened offline
	 * fails with a bare network error. The task is looked up across all cached
	 * shapes (like `findTask`) and, when `includeSubtasks` is set, its children are
	 * gathered by `parentId` from every cached entry. Null when the task is nowhere
	 * in cache.
	 */
	findTaskDetail(
		id: number,
		includeSubtasks: boolean
	): Promise<{ payload: unknown; storedAt: string } | null>;
}

/** `GET /api/v1/tasks/:id` (task detail, tmp ids included) → the id; null otherwise. */
export function matchTaskDetailPath(path: string): number | null {
	const clean = path.split('?', 1)[0];
	const match = /^\/api\/v1\/tasks\/(-?\d+)$/.exec(clean);
	return match ? Number(match[1]) : null;
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
		async findTask(id) {
			for (const entry of await db.getAllResponses()) {
				const found = findTaskInPayload(entry.payload, id, entry.path);
				if (found) return found;
			}
			return null;
		},
		async findTaskDetail(id, includeSubtasks) {
			// One pass over the cache: remember every task it holds (for the subtask
			// scan) and keep the freshest copy of the requested one.
			const known = new Map<number, Record<string, unknown>>();
			let found: Record<string, unknown> | null = null;
			let storedAt: string | null = null;
			for (const entry of await db.getAllResponses()) {
				const inEntry = new Map<number, Record<string, unknown>>();
				collectTasks(entry.payload, inEntry, entry.path);
				const hit = inEntry.get(id);
				if (hit && (storedAt === null || entry.storedAt > storedAt)) {
					found = hit;
					storedAt = entry.storedAt;
				}
				for (const [taskId, task] of inEntry) {
					if (!known.has(taskId)) known.set(taskId, task);
				}
			}
			if (!found || storedAt === null) return null;

			const { subtasks: _embedded, ...task } = found;
			if (!includeSubtasks) return { payload: task, storedAt };

			const items = collectDescendants(known, id);
			return {
				payload: { ...task, subtasks: { items, total: items.length, limit: items.length, offset: 0 } },
				storedAt
			};
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

/**
 * A cached object is a Task only if it carries a Task-distinctive field on top of
 * `id`/`status` — `id`/`status` alone also match a Project, which shares the
 * `responses` store. `dayPart`/`planState`/`postponeCount` are Task-only.
 *
 * Exported so the op cache-patchers (`ops/patch.ts`, §4.5) apply the exact same
 * task/project discrimination as `findTask` when rewriting cached records.
 */
export function looksLikeTask(value: unknown): value is { id: number } {
	if (!value || typeof value !== 'object') return false;
	const o = value as Record<string, unknown>;
	return (
		typeof o.id === 'number' &&
		'status' in o &&
		('dayPart' in o || 'planState' in o || 'postponeCount' in o)
	);
}

/** Strip the query string from a cache-entry path. */
function stripQuery(path: string): string {
	return path.split('?', 1)[0];
}

/** Collects the task arrays a given response shape exposes. */
type Extractor = (p: Record<string, unknown>, push: (value: unknown) => void) => void;

/**
 * TroikiViewResponse slots: tasks live two levels down, under each of the three
 * slots (important/medium/rest) → projects[].tasks[]. `holder` is the payload
 * root for `GET /api/v1/troiki` and `payload.troiki` for the config aggregate —
 * the same shape at two different depths, which is exactly the mistake a
 * top-level-only sniffer makes.
 */
function pushTroikiSlots(holder: unknown, push: (value: unknown) => void): void {
	if (!holder || typeof holder !== 'object') return;
	const h = holder as Record<string, unknown>;
	for (const slot of [h.important, h.medium, h.rest]) {
		const projects = (slot as { projects?: unknown } | undefined)?.projects;
		if (!Array.isArray(projects)) continue;
		for (const project of projects) {
			push((project as { tasks?: unknown } | undefined)?.tasks);
		}
	}
}

/**
 * Where each endpoint keeps its tasks. Keyed by path because response shapes are
 * a property of the endpoint, not something reliably inferable from the payload:
 * aggregates nest their task lists at depths and under key names that a
 * top-level probe cannot see (`ConfigResponse.pinnedTasks`,
 * `ConfigResponse.troiki.*.projects[].tasks`, `SidebarStatsResponse.pinned.items`,
 * `WeekSummaryResponse.completed`). Missing one is silent: the task simply
 * cannot be found offline, and an offline complete of it does not survive a
 * restart because `patchCachedTask` never rewrites that entry.
 *
 * `CachedResponse.path` is persisted alongside every entry, so this works for
 * rows written in an earlier session too.
 */
const EXTRACTORS: Record<string, Extractor> = {
	'/api/v1/config': (p, push) => {
		push(p.pinnedTasks);
		pushTroikiSlots(p.troiki, push);
	},
	'/api/v1/troiki': (p, push) => pushTroikiSlots(p, push),
	'/api/v1/stats/sidebar': (p, push) => push(p.pinned),
	'/api/v1/stats/week-summary': (p, push) => push(p.completed),
	'/api/v1/tasks/today': (p, push) => {
		push(p.today);
		push(p.overdue);
		push(p.completedToday);
	},
	'/api/v1/inbox': (p, push) => push(p.tasks),
	'/api/v1/search': (p, push) => push(p.tasks)
};

/**
 * Relation peers on a task detail payload: `relations[].task`, one level down.
 *
 * The pushed array is freshly built, so its *elements* are the same objects the
 * payload holds — mutating an item (what patchCachedTask does) writes through, while
 * pushing into it would not persist. Nothing pushes into relations, so that is fine.
 *
 * Without this, completing a peer offline would leave the other task's relations
 * section still showing it as open after a restart.
 */
function pushRelationPeers(relations: unknown, push: (value: unknown) => void): void {
	if (!Array.isArray(relations)) return;
	push(relations.map((r) => (r as { task?: unknown } | undefined)?.task).filter(Boolean));
}

/** Parameterised paths, tried after the exact map. */
const EXTRACTOR_PATTERNS: [RegExp, Extractor][] = [
	[/^\/api\/v1\/projects\/-?\d+\/bundle$/, (p, push) => push(p.tasks)],
	// Task detail: the payload is itself a Task (handled by looksLikeTask at the
	// callsites); this exposes the embedded subtask page and the relation peers.
	[
		/^\/api\/v1\/tasks\/-?\d+$/,
		(p, push) => {
			push(p.subtasks);
			pushRelationPeers(p.relations, push);
		}
	]
];

/** Every list view: Page<Task> / ViewList<Task>. */
const DEFAULT_EXTRACTOR: Extractor = (p, push) => push(p.items);

/**
 * Fallback for an unregistered or unknown path: probe every key any known shape
 * has ever used. Registry-first keeps this from producing false positives on
 * ordinary responses, while the fallback keeps entries written by an older build
 * — or by an endpoint nobody remembered to register — traversable rather than
 * silently invisible.
 */
const SNIFF_EXTRACTOR: Extractor = (p, push) => {
	push(p.items);
	push(p.tasks);
	push(p.today);
	push(p.overdue);
	push(p.completedToday);
	push(p.completed);
	push(p.pinned);
	push(p.pinnedTasks);
	push(p.subtasks);
	pushRelationPeers(p.relations, push);
	pushTroikiSlots(p, push);
	pushTroikiSlots(p.troiki, push);
};

function extractorFor(path: string | undefined): Extractor {
	if (path === undefined) return SNIFF_EXTRACTOR;
	const clean = stripQuery(path);
	const exact = EXTRACTORS[clean];
	if (exact) return exact;
	for (const [pattern, extractor] of EXTRACTOR_PATTERNS) {
		if (pattern.test(clean)) return extractor;
	}
	return clean.startsWith(CACHEABLE_PREFIX) ? DEFAULT_EXTRACTOR : SNIFF_EXTRACTOR;
}

/**
 * Every task array reachable from a cached response (§4.5). Pass the entry's
 * `path` so the right extractor is chosen; omit it and a shape-sniffing fallback
 * is used.
 *
 * Exported so the op cache-patchers (`ops/patch.ts`) traverse the same shapes as
 * `findTask` — the returned arrays are the live payload arrays, so mutating their
 * items (or pushing) rewrites the payload the caller then persists via `putEntry`.
 */
export function candidateLists(payload: unknown, path?: string): unknown[][] {
	if (Array.isArray(payload)) return [payload];
	if (!payload || typeof payload !== 'object') return [];
	const lists: unknown[][] = [];
	const push = (value: unknown): void => {
		if (Array.isArray(value)) {
			lists.push(value);
		} else if (value && typeof value === 'object' && Array.isArray((value as { items?: unknown }).items)) {
			lists.push((value as { items: unknown[] }).items);
		}
	};
	extractorFor(path)(payload as Record<string, unknown>, push);
	// A registered endpoint whose payload turns out not to match (an older cached
	// shape, a partial response) still gets the broad probe rather than nothing.
	if (lists.length === 0 && path !== undefined) {
		SNIFF_EXTRACTOR(payload as Record<string, unknown>, push);
	}
	return lists;
}

/**
 * Index every Task reachable from a cached response into `out` (first copy wins),
 * descending into a detail response's embedded `subtasks.items` so children that
 * appear nowhere else are still known to `findTaskDetail`.
 */
function collectTasks(
	payload: unknown,
	out: Map<number, Record<string, unknown>>,
	path?: string
): void {
	if (looksLikeTask(payload)) addTask(payload, out);
	for (const list of candidateLists(payload, path)) {
		for (const item of list) {
			if (looksLikeTask(item)) addTask(item, out);
		}
	}
}

function addTask(task: { id: number }, out: Map<number, Record<string, unknown>>): void {
	const record = task as Record<string, unknown>;
	if (!out.has(task.id)) out.set(task.id, record);
	const embedded = (record.subtasks as { items?: unknown } | undefined)?.items;
	if (!Array.isArray(embedded)) return;
	for (const child of embedded) {
		if (looksLikeTask(child)) addTask(child, out);
	}
}

/**
 * Every descendant of `rootId` in `known` (children, grandchildren, ...),
 * mirroring the backend's recursive subtree query so the offline-degraded
 * task detail view nests the same way the online one does.
 */
function collectDescendants(
	known: Map<number, Record<string, unknown>>,
	rootId: number
): Record<string, unknown>[] {
	const out: Record<string, unknown>[] = [];
	const queue = [rootId];
	while (queue.length) {
		const parentId = queue.shift()!;
		for (const task of known.values()) {
			if (task.parentId === parentId) {
				out.push(task);
				queue.push(task.id as number);
			}
		}
	}
	return out;
}

function findTaskInPayload(payload: unknown, id: number, path?: string): unknown | null {
	// A bare Task response (GET /api/v1/tasks/:id).
	if (looksLikeTask(payload) && payload.id === id) return payload;
	for (const list of candidateLists(payload, path)) {
		for (const item of list) {
			if (looksLikeTask(item) && item.id === id) return item;
		}
	}
	return null;
}

import { getApiClient } from '../api/client';
import { contexts as contextsApi } from '../api/endpoints/contexts';
import { labels as labelsApi } from '../api/endpoints/labels';
import { projects as projectsApi } from '../api/endpoints/projects';
import { tasks as tasksApi } from '../api/endpoints/tasks';
import { troiki as troikiApi } from '../api/endpoints/troiki';
import { views as viewsApi } from '../api/endpoints/views';
import { statusStore } from './status.svelte';

/** Debounce window before a scheduled warm run fires (ms). */
export const WARM_DEBOUNCE_MS = 800;

/**
 * Issue the GETs that back the key screens — today / tomorrow / week / inbox and
 * the projects / labels / contexts lists (FEATURE-OFFLINE-ARCH.md §7 B5) —
 * through the shared `ApiClient`. Each request write-throughs into the
 * read-through cache on success (§4.4), so those screens open offline later even
 * if they were never visited online.
 *
 * The paths and queries mirror exactly what the pages and domain stores request,
 * so a warmed response lands under the same canonical cache key
 * (`canonicalizeQuery`, §4.2/§4.3):
 *   - today / tomorrow / week / backlog / pinned are warmed WITHOUT a context
 *     filter, i.e. the common no-active-context case (the pages send
 *     `{ contextId: undefined }`, which canonicalizes to no query — the same key).
 *     A context-scoped visit caches its own key via that page's own online
 *     write-through.
 *   - `backlog` backs the Next-week screen (it fetches backlog + week); `troiki`
 *     backs the Troiki board — both are primary sidebar destinations that would
 *     otherwise fail offline until visited online once.
 *   - the list limits match the stores (projects/labels 500, contexts 200).
 *   - project bundles are per-id and cannot all be pre-warmed, so only the
 *     *pinned* projects' bundles are warmed (`warmPinnedProjectBundles`): those
 *     are the ones surfaced at the top of the sidebar and thus the likely offline
 *     targets. Any other project still caches its bundle on its own online visit;
 *     an unvisited one shows a localized "needs connection" message offline.
 *
 * Returns the in-flight promises; the caller swallows rejections.
 */
export function warmTargets(): Promise<unknown>[] {
	const client = getApiClient();
	return [
		viewsApi.today(client),
		viewsApi.tomorrow(client),
		viewsApi.week(client),
		viewsApi.backlog(client),
		viewsApi.pinned(client),
		tasksApi.inbox(client),
		troikiApi.view(client),
		// Fetches (and thus caches) the projects list, then the pinned bundles.
		warmPinnedProjectBundles(client),
		labelsApi.list(client, { limit: 500 }),
		contextsApi.list(client, { limit: 200 })
	];
}

/**
 * Warm the projects list and then the bundle of each pinned project, so the
 * pinned projects open offline (their board — sections + tasks) without a prior
 * online visit. The list GET write-throughs into the cache on success just like
 * any other, so this also warms the `/projects?limit=500` entry the projects
 * store reads. Bundle failures are isolated (`allSettled`) — one uncached
 * project never blocks the others.
 */
async function warmPinnedProjectBundles(client: ReturnType<typeof getApiClient>): Promise<void> {
	const list = await projectsApi.list(client, { limit: 500 });
	const pinned = list.items.filter((p) => p.isPinned);
	await Promise.allSettled(pinned.map((p) => projectsApi.bundle(client, p.id)));
}

export interface CacheWarmer {
	/** Debounced trigger. Rapid calls collapse into a single run. */
	schedule(): void;
	/** Drop any pending scheduled run (teardown). */
	cancel(): void;
}

export interface CacheWarmerOptions {
	/** Produces the in-flight warm GETs. Defaults to `warmTargets`. */
	run?: () => Promise<unknown>[];
	/** Online gate — a warm run is skipped when offline. Defaults to the status heuristic. */
	isOnline?: () => boolean;
	/** Debounce window in ms. Defaults to `WARM_DEBOUNCE_MS`. */
	debounceMs?: number;
}

/**
 * Background cache warmer (§7 B5). `schedule()` is debounced and fire-and-forget:
 * it never blocks the UI, only runs while online, never overlaps an in-flight
 * run, and swallows every error (a warm GET failing just leaves that one screen
 * uncached).
 */
export function createCacheWarmer(options: CacheWarmerOptions = {}): CacheWarmer {
	const run = options.run ?? warmTargets;
	const isOnline = options.isOnline ?? (() => statusStore.online);
	const debounceMs = options.debounceMs ?? WARM_DEBOUNCE_MS;

	let timer: ReturnType<typeof setTimeout> | null = null;
	let running = false;

	function fire(): void {
		timer = null;
		// Never overlap runs, and never warm while offline — the GETs would just
		// fail and, worse, be served from the very cache we are trying to fill.
		if (running || !isOnline()) return;
		let inflight: Promise<unknown>[];
		try {
			inflight = run();
		} catch {
			// e.g. the ApiClient is not initialised yet — abort quietly.
			return;
		}
		running = true;
		void Promise.allSettled(inflight).finally(() => {
			running = false;
		});
	}

	return {
		schedule(): void {
			if (timer !== null) clearTimeout(timer);
			timer = setTimeout(fire, debounceMs);
		},
		cancel(): void {
			if (timer !== null) {
				clearTimeout(timer);
				timer = null;
			}
		}
	};
}

/** App-wide singleton wired to the live status heuristic and the real warm GETs. */
export const cacheWarmer = createCacheWarmer();

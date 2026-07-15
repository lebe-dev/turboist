import { getApiClient } from '../api/client';
import { contexts as contextsApi } from '../api/endpoints/contexts';
import { labels as labelsApi } from '../api/endpoints/labels';
import { projects as projectsApi } from '../api/endpoints/projects';
import { tasks as tasksApi } from '../api/endpoints/tasks';
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
 *   - today / tomorrow / week are warmed WITHOUT a context filter, i.e. the
 *     common no-active-context case (the pages send `{ contextId: undefined }`,
 *     which canonicalizes to no query — the same key). A context-scoped visit
 *     caches its own key via that page's own online write-through.
 *   - the list limits match the stores (projects/labels 500, contexts 200).
 *
 * Returns the in-flight promises; the caller swallows rejections.
 */
export function warmTargets(): Promise<unknown>[] {
	const client = getApiClient();
	return [
		viewsApi.today(client),
		viewsApi.tomorrow(client),
		viewsApi.week(client),
		tasksApi.inbox(client),
		projectsApi.list(client, { limit: 500 }),
		labelsApi.list(client, { limit: 500 }),
		contextsApi.list(client, { limit: 200 })
	];
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

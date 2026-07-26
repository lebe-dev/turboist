import { getApiClient } from '../api/client';
import { views as viewsApi } from '../api/endpoints/views';
import { configStore } from '../stores/config.svelte';
import { planStatsStore } from '../stores/planStats.svelte';
import { inboxStatsStore } from '../stores/inboxStats.svelte';
import { pinnedTasksStore } from '../stores/pinnedTasks.svelte';
import type { EventScope } from './events.svelte';

// The two shapes of "pull server truth again", shared by the SSE handlers in
// the (app) layout and by selfRefresh (this tab's own mutations). Both are
// aggregates on purpose: the shell used to issue one GET per store, which on
// mobile means one radio wake-up per store on every reconnect.
//
// This module exists separately from selfRefresh.ts so the layout can import it
// without the two forming an import cycle.

/**
 * Refresh the sidebar aggregates — plan counters, the inbox badge and the
 * pinned list — with a single `GET /api/v1/stats/sidebar`.
 *
 * Use this when only counters can have moved. It replaced the standalone
 * `GET /api/v1/stats/plan` (a strict subset, since removed) and the separate
 * `/inbox` and `/tasks/pinned` refetches.
 */
export async function refreshSidebarBundle(): Promise<void> {
	const stats = await viewsApi.sidebarStats(getApiClient());
	planStatsStore.setValue(stats.planStats);
	inboxStatsStore.set(stats.inboxStats.count, stats.inboxStats.warnThresholdExceeded);
	pinnedTasksStore.setItems(stats.pinned.items);
}

/**
 * Refresh the whole workspace with a single `GET /api/v1/config`: contexts,
 * projects, labels, troiki, the sidebar aggregates, harpoon and templates.
 *
 * Use this when entity lists may have changed (or after a reconnect, when we
 * simply do not know what we missed). It fans out only the server-owned slices
 * — see `configStore.refresh()` for why settings/appSettings are excluded.
 */
export async function refreshWorkspace(): Promise<void> {
	await configStore.refresh();
}

/**
 * Pick the ONE aggregate request that covers a coalesced burst of scopes.
 *
 * Ordering matters: `/api/v1/config` is a superset of the sidebar bundle, so a
 * burst that touches entity lists must not also fire the bundle. A burst of
 * scopes the shell holds no data for (`calendar`, `sections`) refetches nothing
 * here — the page-level views revalidate off the scopes themselves.
 */
export async function refreshForScopes(scopes: Set<EventScope>): Promise<void> {
	if (scopes.has('contexts') || scopes.has('labels') || scopes.has('projects')) {
		await refreshWorkspace();
		return;
	}
	if (scopes.has('tasks') || scopes.has('plan') || scopes.has('inbox')) {
		await refreshSidebarBundle();
	}
}

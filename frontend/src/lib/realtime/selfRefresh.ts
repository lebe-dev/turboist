import { browser } from '$app/environment';
import { getApiClient } from '../api/client';
import { views as viewsApi } from '../api/endpoints/views';
import { projectsStore } from '../stores/projects.svelte';
import { labelsStore } from '../stores/labels.svelte';
import { contextsStore } from '../stores/contexts.svelte';
import { planStatsStore } from '../stores/planStats.svelte';
import { inboxStatsStore } from '../stores/inboxStats.svelte';
import { pinnedTasksStore } from '../stores/pinnedTasks.svelte';
import type { EventScope } from './events.svelte';

// selfRefresh keeps the originating client's derived data fresh after its own
// mutations. The backend suppresses the SSE echo of a client's own changes (see
// lib/realtime/origin.ts), so the layout's SSE handlers — which other tabs use
// to invalidate — never fire here. This module is the local replacement.
//
// Design:
//   - The entity the user just edited is already updated at the mutation
//     callsite (mutator.replace / hydrate from the response), so we deliberately
//     do NOT re-broadcast the `tasks` scope: that redundant refetch is exactly
//     the flicker we are removing.
//   - Sidebar aggregates (plan/inbox/pinned) cannot be derived locally, so we
//     refetch them — but as a single coalesced /stats/sidebar bundle instead of
//     the three separate GETs the SSE handlers issued.
//   - Entity lists in the sidebar (projects/labels/contexts) are not always
//     updated optimistically, so we reload their stores and re-dispatch the
//     matching invalidate event for any list view that listens.

// scopesForPath mirrors the backend's scopesForPath (publish_middleware.go):
// the same request path maps to the same change scopes, so the client refreshes
// exactly what the server would have published.
function scopesForPath(path: string): EventScope[] {
	const prefix = '/api/v1/';
	if (!path.startsWith(prefix)) return [];
	const rest = path.slice(prefix.length);
	if (rest === '') return [];
	const domain = rest.split('/', 1)[0];
	switch (domain) {
		case 'events':
			return [];
		case 'tasks':
			return ['tasks', 'plan', 'inbox'];
		case 'inbox':
			return ['tasks', 'inbox', 'plan'];
		case 'projects':
			return ['projects', 'tasks'];
		case 'labels':
			return ['labels', 'tasks'];
		case 'contexts':
			return ['contexts'];
		case 'sections':
			return ['sections', 'tasks'];
		case 'calendars':
			return ['calendar'];
		case 'troiki':
			return ['tasks', 'plan'];
		case 'app-settings':
			return ['labels', 'tasks'];
		default:
			return [];
	}
}

const COALESCE_MS = 200;

const pending = new Set<EventScope>();
let timer: ReturnType<typeof setTimeout> | null = null;

function dispatchInvalidate(scope: EventScope): void {
	if (!browser) return;
	window.dispatchEvent(new CustomEvent('turboist:invalidate', { detail: { scope } }));
}

async function refreshSidebarBundle(): Promise<void> {
	const stats = await viewsApi.sidebarStats(getApiClient());
	planStatsStore.setValue(stats.planStats);
	inboxStatsStore.set(stats.inboxStats.count, stats.inboxStats.warnThresholdExceeded);
	pinnedTasksStore.setItems(stats.pinned.items);
}

function flush(): void {
	timer = null;
	const scopes = new Set(pending);
	pending.clear();

	// One bundle call covers every count-style scope.
	if (scopes.has('tasks') || scopes.has('plan') || scopes.has('inbox')) {
		void refreshSidebarBundle().catch((err) => {
			console.warn('selfRefresh: sidebar bundle failed', err);
		});
	}
	// Entity-list stores: reload + nudge any list view. The `tasks` scope is
	// intentionally not re-dispatched (the active view already applied the
	// change from the mutation response).
	if (scopes.has('projects')) {
		void projectsStore.load().catch(() => {});
		dispatchInvalidate('projects');
	}
	if (scopes.has('labels')) {
		void labelsStore.load().catch(() => {});
		dispatchInvalidate('labels');
	}
	if (scopes.has('contexts')) {
		void contextsStore.load().catch(() => {});
		dispatchInvalidate('contexts');
	}
	if (scopes.has('sections')) {
		dispatchInvalidate('sections');
	}
	if (scopes.has('calendar')) {
		dispatchInvalidate('calendar');
	}
}

// onSelfMutation is wired to ApiClient.onMutation. It records the scopes the
// mutation touched and flushes them after a short coalescing window, so a burst
// of mutations (e.g. add-subtask then reload) results in a single refresh.
export function onSelfMutation(path: string): void {
	const scopes = scopesForPath(path);
	if (scopes.length === 0) return;
	for (const s of scopes) pending.add(s);
	if (timer !== null) return;
	timer = setTimeout(flush, COALESCE_MS);
}

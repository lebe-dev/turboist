import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { EventScope } from './events.svelte';

vi.mock('../api/client', () => ({ getApiClient: () => ({}) }));

const sidebarStats = vi.fn();
vi.mock('../api/endpoints/views', () => ({ views: { sidebarStats: () => sidebarStats() } }));

const configRefresh = vi.fn();
vi.mock('../stores/config.svelte', () => ({ configStore: { refresh: () => configRefresh() } }));

const planSet = vi.fn();
const inboxSet = vi.fn();
const pinnedSet = vi.fn();
vi.mock('../stores/planStats.svelte', () => ({
	planStatsStore: { setValue: (v: unknown) => planSet(v) }
}));
vi.mock('../stores/inboxStats.svelte', () => ({
	inboxStatsStore: { set: (c: number, w: boolean) => inboxSet(c, w) }
}));
vi.mock('../stores/pinnedTasks.svelte', () => ({
	pinnedTasksStore: { setItems: (i: unknown) => pinnedSet(i) }
}));

import { refreshForScopes, refreshSidebarBundle } from './refresh';

const bundle = {
	planStats: { week: 3, backlog: 1 },
	inboxStats: { count: 2, warnThresholdExceeded: false },
	pinned: { items: [{ id: 9 }], total: 1 }
};

function scopes(...s: EventScope[]): Set<EventScope> {
	return new Set(s);
}

describe('refreshSidebarBundle', () => {
	beforeEach(() => {
		sidebarStats.mockReset().mockResolvedValue(bundle);
		planSet.mockClear();
		inboxSet.mockClear();
		pinnedSet.mockClear();
	});

	it('fans the one bundle response out to all three sidebar stores', async () => {
		await refreshSidebarBundle();

		expect(sidebarStats).toHaveBeenCalledTimes(1);
		expect(planSet).toHaveBeenCalledWith(bundle.planStats);
		expect(inboxSet).toHaveBeenCalledWith(2, false);
		expect(pinnedSet).toHaveBeenCalledWith(bundle.pinned.items);
	});
});

// The routing decision behind the shell's SSE handling: a coalesced burst must
// resolve to AT MOST ONE aggregate request, which is the whole point of
// coalescing in the first place.
describe('refreshForScopes', () => {
	beforeEach(() => {
		sidebarStats.mockReset().mockResolvedValue(bundle);
		configRefresh.mockReset().mockResolvedValue(null);
	});

	it.each<EventScope>(['contexts', 'labels', 'projects'])(
		'pulls the config aggregate for the %s scope',
		async (scope) => {
			await refreshForScopes(scopes(scope));

			expect(configRefresh).toHaveBeenCalledTimes(1);
			expect(sidebarStats).not.toHaveBeenCalled();
		}
	);

	it.each<EventScope>(['tasks', 'plan', 'inbox'])(
		'pulls only the sidebar bundle for the %s scope',
		async (scope) => {
			await refreshForScopes(scopes(scope));

			expect(sidebarStats).toHaveBeenCalledTimes(1);
			expect(configRefresh).not.toHaveBeenCalled();
		}
	);

	// /api/v1/config already carries the sidebar aggregates, so firing both would
	// reintroduce the second round-trip this design exists to remove.
	it('does not also fetch the sidebar bundle when the burst spans both groups', async () => {
		await refreshForScopes(scopes('tasks', 'plan', 'inbox', 'projects', 'labels'));

		expect(configRefresh).toHaveBeenCalledTimes(1);
		expect(sidebarStats).not.toHaveBeenCalled();
	});

	// The shell holds no state for these; only the page-level views care, and
	// they revalidate off the dispatched scopes.
	it('fetches nothing for a calendar/sections-only burst', async () => {
		await refreshForScopes(scopes('calendar', 'sections'));

		expect(configRefresh).not.toHaveBeenCalled();
		expect(sidebarStats).not.toHaveBeenCalled();
	});

	it('fetches nothing for an empty burst', async () => {
		await refreshForScopes(scopes());

		expect(configRefresh).not.toHaveBeenCalled();
		expect(sidebarStats).not.toHaveBeenCalled();
	});

	// The layout does `void refreshForScopes(...).catch(...)`; a rejection must
	// stay a rejection rather than being swallowed here.
	it('propagates a failed refresh to the caller', async () => {
		configRefresh.mockRejectedValue(new Error('offline'));

		await expect(refreshForScopes(scopes('projects'))).rejects.toThrow('offline');
	});
});

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('$app/environment', () => ({ browser: true }));
vi.mock('../api/client', () => ({ getApiClient: () => ({}) }));

const sidebarStats = vi.fn();
vi.mock('../api/endpoints/views', () => ({ views: { sidebarStats: () => sidebarStats() } }));

const projectsLoad = vi.fn().mockResolvedValue(undefined);
const labelsLoad = vi.fn().mockResolvedValue(undefined);
const contextsLoad = vi.fn().mockResolvedValue(undefined);
vi.mock('../stores/projects.svelte', () => ({ projectsStore: { load: () => projectsLoad() } }));
vi.mock('../stores/labels.svelte', () => ({ labelsStore: { load: () => labelsLoad() } }));
vi.mock('../stores/contexts.svelte', () => ({ contextsStore: { load: () => contextsLoad() } }));

const planSet = vi.fn();
const inboxSet = vi.fn();
const pinnedSet = vi.fn();
vi.mock('../stores/planStats.svelte', () => ({ planStatsStore: { setValue: (v: unknown) => planSet(v) } }));
vi.mock('../stores/inboxStats.svelte', () => ({
	inboxStatsStore: { set: (c: number, w: boolean) => inboxSet(c, w) }
}));
vi.mock('../stores/pinnedTasks.svelte', () => ({
	pinnedTasksStore: { setItems: (i: unknown) => pinnedSet(i) }
}));

import { onSelfMutation } from './selfRefresh';

function invalidations(): string[] {
	const seen: string[] = [];
	window.addEventListener('turboist:invalidate', (e) => {
		seen.push((e as CustomEvent<{ scope: string }>).detail.scope);
	});
	return seen;
}

const bundle = {
	planStats: { week: 3, backlog: 1 },
	inboxStats: { count: 2, warnThresholdExceeded: false },
	pinned: { items: [{ id: 9 }], total: 1 }
};

describe('selfRefresh.onSelfMutation', () => {
	beforeEach(() => {
		vi.useFakeTimers();
		sidebarStats.mockReset().mockResolvedValue(bundle);
		projectsLoad.mockClear();
		labelsLoad.mockClear();
		contextsLoad.mockClear();
		planSet.mockClear();
		inboxSet.mockClear();
		pinnedSet.mockClear();
	});
	afterEach(() => {
		vi.useRealTimers();
	});

	it('refreshes the sidebar bundle once for a task mutation and does not re-broadcast tasks', async () => {
		const seen = invalidations();
		onSelfMutation('/api/v1/tasks/1');
		await vi.advanceTimersByTimeAsync(250);

		expect(sidebarStats).toHaveBeenCalledTimes(1);
		expect(planSet).toHaveBeenCalledWith(bundle.planStats);
		expect(inboxSet).toHaveBeenCalledWith(2, false);
		expect(pinnedSet).toHaveBeenCalledWith(bundle.pinned.items);
		// The active view already applied the change from the response — no echo.
		expect(seen).not.toContain('tasks');
	});

	it('coalesces a burst of mutations into a single bundle refresh', async () => {
		onSelfMutation('/api/v1/tasks/1');
		onSelfMutation('/api/v1/tasks/2');
		onSelfMutation('/api/v1/inbox/tasks');
		await vi.advanceTimersByTimeAsync(250);
		expect(sidebarStats).toHaveBeenCalledTimes(1);
	});

	it('reloads the projects store and nudges project views for a project mutation', async () => {
		const seen = invalidations();
		onSelfMutation('/api/v1/projects/5');
		await vi.advanceTimersByTimeAsync(250);
		expect(projectsLoad).toHaveBeenCalledTimes(1);
		expect(seen).toContain('projects');
	});

	it('refreshes counters after a template is instantiated', async () => {
		onSelfMutation('/api/v1/task-templates/4/instantiate');
		await vi.advanceTimersByTimeAsync(250);
		expect(sidebarStats).toHaveBeenCalledTimes(1);
	});

	it('ignores paths that map to no scope (events, auth)', async () => {
		onSelfMutation('/api/v1/events/ticket');
		onSelfMutation('/auth/login');
		await vi.advanceTimersByTimeAsync(250);
		expect(sidebarStats).not.toHaveBeenCalled();
		expect(projectsLoad).not.toHaveBeenCalled();
	});
});

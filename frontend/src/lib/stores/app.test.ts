import { describe, it, expect, vi, afterEach } from 'vitest';

vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

vi.mock('$lib/stores/logger', () => ({
	logger: { log: vi.fn(), warn: vi.fn(), error: vi.fn() }
}));

const mockConfig = {
	settings: { poll_interval: 30, max_pinned: 5 },
	contexts: [],
	projects: [],
	labels: [],
	label_configs: [],
	quick_capture: null,
	state: {
		pinned_tasks: [],
		active_context_id: '',
		active_view: 'all',
		collapsed_ids: [],
		sidebar_collapsed: false,
		planning_open: false
	}
};

vi.mock('$lib/api/client', () => ({
	getAppConfig: vi.fn(() => Promise.resolve(mockConfig)),
	patchState: vi.fn(() => Promise.resolve())
}));

vi.mock('$lib/sync/db', () => ({
	saveAppConfig: vi.fn(() => Promise.resolve()),
	loadAppConfig: vi.fn(() => Promise.resolve(null)),
	loadTaskSnapshot: vi.fn(() => Promise.resolve(null)),
	loadCompletedTasks: vi.fn(() => Promise.resolve(null)),
	saveCompletedTasks: vi.fn(() => Promise.resolve())
}));

vi.mock('$lib/sync/snapshot-writer', () => ({
	writeSnapshotImmediate: vi.fn(),
	scheduleSnapshotWrite: vi.fn()
}));

const mockWsClient = {
	connected: false,
	connect: vi.fn(),
	disconnect: vi.fn(),
	subscribe: vi.fn(),
	unsubscribe: vi.fn(),
	onMessage: vi.fn(() => vi.fn()),
	onStateChange: vi.fn(() => vi.fn())
};

vi.mock('$lib/ws/client.svelte', () => ({
	wsClient: mockWsClient
}));

vi.mock('./contexts.svelte', () => ({
	contextsStore: {
		activeView: 'all',
		activeContextId: null,
		init: vi.fn()
	}
}));

vi.mock('./pinned.svelte', () => ({
	pinnedStore: { init: vi.fn() }
}));

vi.mock('./collapsed.svelte', () => ({
	collapsedStore: { init: vi.fn() }
}));

vi.mock('./sidebar.svelte', () => ({
	sidebarStore: { init: vi.fn() }
}));

vi.mock('./planning.svelte', () => ({
	planningStore: { initActive: vi.fn() }
}));

vi.mock('./tasks.svelte', () => ({
	tasksStore: {
		start: vi.fn(() => Promise.resolve()),
		stop: vi.fn()
	}
}));

describe('appStore', () => {
	afterEach(() => {
		vi.clearAllMocks();
		vi.resetModules();
	});

	it('init sets initialized=true', async () => {
		const { appStore } = await import('./app.svelte');

		expect(appStore.initialized).toBe(false);
		await appStore.init();
		expect(appStore.initialized).toBe(true);
	});

	it('init connects WebSocket and starts tasks', async () => {
		const { appStore } = await import('./app.svelte');
		const { tasksStore } = await import('./tasks.svelte');

		await appStore.init();

		expect(mockWsClient.connect).toHaveBeenCalled();
		expect(tasksStore.start).toHaveBeenCalled();
	});

	it('init hydrates sub-stores', async () => {
		const { appStore } = await import('./app.svelte');
		const { contextsStore } = await import('./contexts.svelte');
		const { pinnedStore } = await import('./pinned.svelte');
		const { collapsedStore } = await import('./collapsed.svelte');
		const { sidebarStore } = await import('./sidebar.svelte');
		const { planningStore } = await import('./planning.svelte');

		await appStore.init();

		expect(contextsStore.init).toHaveBeenCalled();
		expect(pinnedStore.init).toHaveBeenCalled();
		expect(collapsedStore.init).toHaveBeenCalled();
		expect(sidebarStore.init).toHaveBeenCalled();
		expect(planningStore.initActive).toHaveBeenCalled();
	});

	it('destroy stops tasks, disconnects WS, and resets initialized', async () => {
		const { appStore } = await import('./app.svelte');
		const { tasksStore } = await import('./tasks.svelte');

		await appStore.init();
		expect(appStore.initialized).toBe(true);

		appStore.destroy();

		expect(tasksStore.stop).toHaveBeenCalled();
		expect(mockWsClient.disconnect).toHaveBeenCalled();
		expect(appStore.initialized).toBe(false);
	});

	it('can re-init after destroy', async () => {
		const { appStore } = await import('./app.svelte');

		await appStore.init();
		appStore.destroy();
		expect(appStore.initialized).toBe(false);

		await appStore.init();
		expect(appStore.initialized).toBe(true);
	});
});

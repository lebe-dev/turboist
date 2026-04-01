import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import type { Task, Meta } from '$lib/api/types';

// Mock dependencies before importing the store
vi.mock('$lib/stores/logger', () => ({
	logger: { log: vi.fn(), warn: vi.fn(), error: vi.fn() }
}));

vi.mock('$lib/api/client', () => ({
	getAppConfig: vi.fn(() =>
		Promise.resolve({
			settings: { poll_interval: 30 }
		})
	),
	getCompletedTasks: vi.fn(() => Promise.resolve({ tasks: [], meta: {} }))
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

vi.mock('$lib/sync/db', () => ({
	loadCompletedTasks: vi.fn(() => Promise.resolve(null)),
	saveCompletedTasks: vi.fn(() => Promise.resolve())
}));

const mockActionQueue = {
	items: [] as { type: string; payload: unknown; status: string }[],
	pendingCount: 0,
	failedCount: 0,
	flushing: false,
	init: vi.fn(),
	enqueue: vi.fn(),
	flush: vi.fn(),
	flushNow: vi.fn(() => Promise.resolve()),
	startAutoFlush: vi.fn(),
	stopAutoFlush: vi.fn(),
	clear: vi.fn(),
	retryFailed: vi.fn(),
	discard: vi.fn()
};

vi.mock('$lib/sync/action-queue.svelte', () => ({
	actionQueue: mockActionQueue
}));

vi.mock('./contexts.svelte', () => ({
	contextsStore: {
		activeView: 'today',
		activeContextId: null
	}
}));

vi.mock('$lib/state/index.svelte', () => ({
	isStateReady: () => true,
	persistTasks: vi.fn(),
	persistMeta: vi.fn(),
	loadPersistedTasks: vi.fn(() => []),
	loadPersistedMeta: vi.fn(() => null),
	initState: vi.fn(() => Promise.resolve()),
	destroyState: vi.fn()
}));

function makeTask(id: string, children: Task[] = []): Task {
	return {
		id,
		content: `Task ${id}`,
		description: '',
		project_id: 'p1',
		section_id: null,
		parent_id: null,
		labels: [],
		priority: 1,
		due: null,
		sub_task_count: children.length,
		completed_sub_task_count: 0,
		completed_at: null,
		added_at: '2024-01-01T00:00:00Z',
		is_project_task: false,
		postpone_count: 0,
		children
	};
}

describe('tasksStore lifecycle', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		mockWsClient.onStateChange.mockImplementation(() => vi.fn());
	});

	afterEach(() => {
		vi.resetModules();
	});

	it('start() is idempotent — second call is a no-op', async () => {
		const { tasksStore } = await import('./tasks.svelte');

		await tasksStore.start();
		await tasksStore.start();

		// onStateChange should be registered only once
		expect(mockWsClient.onStateChange).toHaveBeenCalledTimes(1);
		// subscribe should be called only once
		expect(mockWsClient.subscribe).toHaveBeenCalledTimes(1);

		tasksStore.stop();
	});

	it('stop() is idempotent — second call is a no-op', async () => {
		const { tasksStore } = await import('./tasks.svelte');

		await tasksStore.start();
		tasksStore.stop();
		tasksStore.stop();

		// unsubscribe should be called only once
		expect(mockWsClient.unsubscribe).toHaveBeenCalledTimes(1);
	});

	it('stop() then start() works', async () => {
		const { tasksStore } = await import('./tasks.svelte');

		await tasksStore.start();
		tasksStore.stop();
		await tasksStore.start();

		expect(mockWsClient.onStateChange).toHaveBeenCalledTimes(2);
		expect(mockWsClient.subscribe).toHaveBeenCalledTimes(2);

		tasksStore.stop();
	});

	it('stop() calls cleanup functions', async () => {
		const cleanup = vi.fn();
		mockWsClient.onStateChange.mockReturnValue(cleanup);

		const { tasksStore } = await import('./tasks.svelte');

		await tasksStore.start();
		tasksStore.stop();

		expect(cleanup).toHaveBeenCalledTimes(1);
		expect(mockWsClient.unsubscribe).toHaveBeenCalledWith('tasks');
	});
});

describe('tasksStore local mutations', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		mockWsClient.onStateChange.mockImplementation(() => vi.fn());
	});

	afterEach(() => {
		vi.resetModules();
	});

	async function setupWithTasks(tasks: Task[]) {
		const { tasksStore } = await import('./tasks.svelte');

		// Get the snapshot handler registered at module load time (most recent call)
		const calls = mockWsClient.onMessage.mock.calls as unknown[][];
		const snapshotCall = calls.findLast(
			(c) => c[0] === 'snapshot' && c[1] === 'tasks'
		);
		if (!snapshotCall) throw new Error('snapshot handler not registered');
		const handleSnapshot = snapshotCall[2] as (data: unknown) => void;

		// Deliver a snapshot to populate tasks
		const meta: Meta = {
			context: '',
			weekly_limit: 0,
			weekly_count: 0,
			backlog_limit: 0,
			backlog_count: 0,
			last_synced_at: new Date().toISOString()
		};
		handleSnapshot({ tasks, meta });

		return { tasksStore, handleSnapshot, meta };
	}

	it('addTaskLocal prepends task', async () => {
		const { tasksStore } = await setupWithTasks([makeTask('1')]);

		tasksStore.addTaskLocal(makeTask('new'));

		expect(tasksStore.tasks).toHaveLength(2);
		expect(tasksStore.tasks[0].id).toBe('new');
	});

	it('removeTaskLocal removes task', async () => {
		const { tasksStore } = await setupWithTasks([makeTask('1'), makeTask('2')]);

		tasksStore.removeTaskLocal('1');

		expect(tasksStore.tasks).toHaveLength(1);
		expect(tasksStore.tasks[0].id).toBe('2');
	});

	it('updateTaskLocal modifies task in place', async () => {
		const { tasksStore } = await setupWithTasks([makeTask('1')]);

		tasksStore.updateTaskLocal('1', (t) => ({ ...t, content: 'Updated' }));

		expect(tasksStore.tasks[0].content).toBe('Updated');
	});

	it('insertAfterLocal inserts after sibling', async () => {
		const { tasksStore } = await setupWithTasks([makeTask('1'), makeTask('3')]);

		tasksStore.insertAfterLocal('1', makeTask('2'));

		expect(tasksStore.tasks.map((t) => t.id)).toEqual(['1', '2', '3']);
	});

	it('removed tasks stay removed across snapshots until server catches up', async () => {
		const { tasksStore, handleSnapshot, meta } = await setupWithTasks([
			makeTask('1'),
			makeTask('2')
		]);

		tasksStore.removeTaskLocal('1');
		expect(tasksStore.tasks).toHaveLength(1);

		// New snapshot still contains '1' — removal should persist
		handleSnapshot({ tasks: [makeTask('1'), makeTask('2')], meta });
		expect(tasksStore.tasks).toHaveLength(1);
		expect(tasksStore.tasks[0].id).toBe('2');

		// Server catches up — '1' is no longer in snapshot
		handleSnapshot({ tasks: [makeTask('2')], meta });
		expect(tasksStore.tasks).toHaveLength(1);
	});

	it('clearPendingRemoval allows task to reappear', async () => {
		const { tasksStore, handleSnapshot, meta } = await setupWithTasks([
			makeTask('1'),
			makeTask('2')
		]);

		tasksStore.removeTaskLocal('1');
		tasksStore.clearPendingRemoval('1');

		handleSnapshot({ tasks: [makeTask('1'), makeTask('2')], meta });
		expect(tasksStore.tasks).toHaveLength(2);
	});
});

describe('tasksStore pending queue updates overlay', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		mockActionQueue.items = [];
		mockWsClient.onStateChange.mockImplementation(() => vi.fn());
	});

	afterEach(() => {
		mockActionQueue.items = [];
		vi.resetModules();
	});

	async function setupWithTasks(tasks: Task[]) {
		const { tasksStore } = await import('./tasks.svelte');
		const calls = mockWsClient.onMessage.mock.calls as unknown[][];
		const snapshotCall = calls.findLast(
			(c) => c[0] === 'snapshot' && c[1] === 'tasks'
		);
		if (!snapshotCall) throw new Error('snapshot handler not registered');
		const handleSnapshot = snapshotCall[2] as (data: unknown) => void;
		const deltaCall = calls.findLast(
			(c) => c[0] === 'delta' && c[1] === 'tasks'
		);
		if (!deltaCall) throw new Error('delta handler not registered');
		const handleDelta = deltaCall[2] as (data: unknown) => void;

		const meta: Meta = {
			context: '',
			weekly_limit: 0,
			weekly_count: 0,
			backlog_limit: 0,
			backlog_count: 0,
			last_synced_at: new Date().toISOString()
		};
		handleSnapshot({ tasks, meta });
		return { tasksStore, handleSnapshot, handleDelta, meta };
	}

	it('pending label update survives WS snapshot', async () => {
		const task = { ...makeTask('1'), labels: ['weekly', 'work'] };
		const { tasksStore, handleSnapshot, meta } = await setupWithTasks([task]);

		// Simulate: user removed labels, action queued but not flushed
		mockActionQueue.items = [
			{ type: 'updateTask', payload: { id: '1', data: { labels: ['work'] } }, status: 'pending' }
		];

		// Server snapshot still has old labels
		handleSnapshot({ tasks: [{ ...makeTask('1'), labels: ['weekly', 'work'] }], meta });

		// Pending update should overlay: 'weekly' removed
		expect(tasksStore.tasks[0].labels).toEqual(['work']);
	});

	it('pending priority update survives WS snapshot', async () => {
		const { tasksStore, handleSnapshot, meta } = await setupWithTasks([makeTask('1')]);

		mockActionQueue.items = [
			{ type: 'updateTask', payload: { id: '1', data: { priority: 4 } }, status: 'pending' }
		];

		handleSnapshot({ tasks: [makeTask('1')], meta });
		expect(tasksStore.tasks[0].priority).toBe(4);
	});

	it('pending update applies to nested children', async () => {
		const child = { ...makeTask('child-1'), labels: ['old'], parent_id: 'parent-1' };
		const parent = makeTask('parent-1', [child]);
		const { tasksStore, handleSnapshot, meta } = await setupWithTasks([parent]);

		mockActionQueue.items = [
			{ type: 'updateTask', payload: { id: 'child-1', data: { labels: ['new'] } }, status: 'pending' }
		];

		handleSnapshot({
			tasks: [makeTask('parent-1', [{ ...makeTask('child-1'), labels: ['old'], parent_id: 'parent-1' }])],
			meta
		});
		expect(tasksStore.tasks[0].children[0].labels).toEqual(['new']);
	});

	it('pending update survives WS delta', async () => {
		const task = { ...makeTask('1'), labels: ['weekly'] };
		const { tasksStore, handleDelta } = await setupWithTasks([task]);

		mockActionQueue.items = [
			{ type: 'updateTask', payload: { id: '1', data: { labels: [] } }, status: 'pending' }
		];

		// Delta upserts the same task with old server labels
		handleDelta({ upserted: [{ ...makeTask('1'), labels: ['weekly'] }], removed: [] });
		expect(tasksStore.tasks[0].labels).toEqual([]);
	});

	it('no overlay when queue is empty', async () => {
		const task = { ...makeTask('1'), labels: ['weekly'] };
		const { tasksStore, handleSnapshot, meta } = await setupWithTasks([task]);

		mockActionQueue.items = [];

		handleSnapshot({ tasks: [{ ...makeTask('1'), labels: ['weekly'] }], meta });
		expect(tasksStore.tasks[0].labels).toEqual(['weekly']);
	});

	it('processing actions are also overlaid', async () => {
		const { tasksStore, handleSnapshot, meta } = await setupWithTasks([makeTask('1')]);

		mockActionQueue.items = [
			{ type: 'updateTask', payload: { id: '1', data: { content: 'New content' } }, status: 'processing' }
		];

		handleSnapshot({ tasks: [makeTask('1')], meta });
		expect(tasksStore.tasks[0].content).toBe('New content');
	});

	it('applyPendingTaskUpdate works for single task', async () => {
		const { tasksStore } = await setupWithTasks([makeTask('1')]);

		mockActionQueue.items = [
			{ type: 'updateTask', payload: { id: '1', data: { labels: ['updated'] } }, status: 'pending' }
		];

		const result = tasksStore.applyPendingTaskUpdate(makeTask('1'));
		expect(result.labels).toEqual(['updated']);
	});

	it('applyPendingTaskUpdate is identity when queue is empty', async () => {
		const { tasksStore } = await setupWithTasks([]);

		mockActionQueue.items = [];

		const task = makeTask('1');
		const result = tasksStore.applyPendingTaskUpdate(task);
		expect(result).toBe(task);
	});
});

describe('tasksStore pending overlay — due_string recurrence', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		mockActionQueue.items = [];
		mockWsClient.onStateChange.mockImplementation(() => vi.fn());
	});

	afterEach(() => {
		mockActionQueue.items = [];
		vi.resetModules();
	});

	async function setupWithTasks(tasks: Task[]) {
		const { tasksStore } = await import('./tasks.svelte');
		const calls = mockWsClient.onMessage.mock.calls as unknown[][];
		const snapshotCall = calls.findLast(
			(c) => c[0] === 'snapshot' && c[1] === 'tasks'
		);
		if (!snapshotCall) throw new Error('snapshot handler not registered');
		const handleSnapshot = snapshotCall[2] as (data: unknown) => void;

		const meta: Meta = {
			context: '',
			weekly_limit: 0,
			weekly_count: 0,
			backlog_limit: 0,
			backlog_count: 0,
			last_synced_at: new Date().toISOString()
		};
		handleSnapshot({ tasks, meta });
		return { tasksStore, handleSnapshot, meta };
	}

	it('pending due_string update sets recurring to true in overlay', async () => {
		const task = { ...makeTask('1'), due: { date: '2026-04-01', recurring: false } };
		const { tasksStore, handleSnapshot, meta } = await setupWithTasks([task]);

		// User sets recurrence — queued as due_string update
		mockActionQueue.items = [
			{ type: 'updateTask', payload: { id: '1', data: { due_string: 'every weekday' } }, status: 'pending' }
		];

		// Server snapshot still shows non-recurring
		handleSnapshot({
			tasks: [{ ...makeTask('1'), due: { date: '2026-04-01', recurring: false } }],
			meta
		});

		// Overlay should reflect that the task is now recurring
		expect(tasksStore.tasks[0].due?.recurring).toBe(true);
		// Date should be preserved
		expect(tasksStore.tasks[0].due?.date).toBe('2026-04-01');
	});

	it('pending due_string preserves existing date when task has no prior due', async () => {
		const task = makeTask('1'); // due: null
		const { tasksStore, handleSnapshot, meta } = await setupWithTasks([task]);

		mockActionQueue.items = [
			{ type: 'updateTask', payload: { id: '1', data: { due_string: 'every day' } }, status: 'pending' }
		];

		handleSnapshot({ tasks: [makeTask('1')], meta });

		// Task should show as recurring even if it had no prior due date
		expect(tasksStore.tasks[0].due?.recurring).toBe(true);
	});

	it('applyPendingTaskUpdate reflects due_string on single task', async () => {
		const { tasksStore } = await setupWithTasks([
			{ ...makeTask('1'), due: { date: '2026-04-02', recurring: false } }
		]);

		mockActionQueue.items = [
			{ type: 'updateTask', payload: { id: '1', data: { due_string: 'every month on the 2nd' } }, status: 'pending' }
		];

		const result = tasksStore.applyPendingTaskUpdate(
			{ ...makeTask('1'), due: { date: '2026-04-02', recurring: false } }
		);
		expect(result.due?.recurring).toBe(true);
		expect(result.due?.date).toBe('2026-04-02');
	});
});

describe('tasksStore delta temp→real reconciliation', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		mockActionQueue.items = [];
		mockWsClient.onStateChange.mockImplementation(() => vi.fn());
	});

	afterEach(() => {
		mockActionQueue.items = [];
		vi.resetModules();
	});

	async function setupWithTasks(tasks: Task[]) {
		const { tasksStore } = await import('./tasks.svelte');
		const calls = mockWsClient.onMessage.mock.calls as unknown[][];
		const snapshotCall = calls.findLast(
			(c) => c[0] === 'snapshot' && c[1] === 'tasks'
		);
		if (!snapshotCall) throw new Error('snapshot handler not registered');
		const handleSnapshot = snapshotCall[2] as (data: unknown) => void;
		const deltaCall = calls.findLast(
			(c) => c[0] === 'delta' && c[1] === 'tasks'
		);
		if (!deltaCall) throw new Error('delta handler not registered');
		const handleDelta = deltaCall[2] as (data: unknown) => void;

		const meta: Meta = {
			context: '',
			weekly_limit: 0,
			weekly_count: 0,
			backlog_limit: 0,
			backlog_count: 0,
			last_synced_at: new Date().toISOString()
		};
		handleSnapshot({ tasks, meta });
		return { tasksStore, handleSnapshot, handleDelta, meta };
	}

	it('delta: reconciled temp task preserves position', async () => {
		const { tasksStore, handleDelta } = await setupWithTasks([makeTask('A'), makeTask('B')]);

		// User duplicated task A — temp task inserted after A
		const tempTask: Task = { ...makeTask('A'), id: 'temp-dup-1', content: 'Task A copy' };
		tasksStore.insertAfterLocal('A', tempTask);
		expect(tasksStore.tasks.map((t) => t.id)).toEqual(['A', 'temp-dup-1', 'B']);

		// Server sends delta with real task (same content, real ID)
		const realTask = { ...makeTask('real-1'), content: 'Task A copy' };
		handleDelta({ upserted: [realTask], removed: [] });

		// Real task should replace temp task AT THE SAME POSITION
		expect(tasksStore.tasks.map((t) => t.id)).toEqual(['A', 'real-1', 'B']);
	});

	it('delta: multiple temp tasks preserve their positions', async () => {
		const { tasksStore, handleDelta } = await setupWithTasks([
			makeTask('A'),
			makeTask('B'),
			makeTask('C')
		]);

		// Two duplicates at different positions
		const temp1: Task = { ...makeTask('A'), id: 'temp-dup-1', content: 'Copy of A' };
		const temp2: Task = { ...makeTask('B'), id: 'temp-dup-2', content: 'Copy of B' };
		tasksStore.insertAfterLocal('A', temp1);
		tasksStore.insertAfterLocal('B', temp2);
		expect(tasksStore.tasks.map((t) => t.id)).toEqual(['A', 'temp-dup-1', 'B', 'temp-dup-2', 'C']);

		// Server returns both real tasks in a single delta
		handleDelta({
			upserted: [
				{ ...makeTask('real-1'), content: 'Copy of A' },
				{ ...makeTask('real-2'), content: 'Copy of B' }
			],
			removed: []
		});

		expect(tasksStore.tasks.map((t) => t.id)).toEqual(['A', 'real-1', 'B', 'real-2', 'C']);
	});

	it('delta: genuinely new task (no temp) appends to end', async () => {
		const { tasksStore, handleDelta } = await setupWithTasks([makeTask('A'), makeTask('B')]);

		// Server sends a completely new task (no temp predecessor)
		const newTask = { ...makeTask('new-1'), content: 'Brand new task' };
		handleDelta({ upserted: [newTask], removed: [] });

		expect(tasksStore.tasks.map((t) => t.id)).toEqual(['A', 'B', 'new-1']);
	});
});

describe('tasksStore snapshot temp task protection', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		mockActionQueue.items = [];
		mockWsClient.onStateChange.mockImplementation(() => vi.fn());
	});

	afterEach(() => {
		mockActionQueue.items = [];
		vi.resetModules();
	});

	async function setupWithTasks(tasks: Task[]) {
		const { tasksStore } = await import('./tasks.svelte');
		const calls = mockWsClient.onMessage.mock.calls as unknown[][];
		const snapshotCall = calls.findLast(
			(c) => c[0] === 'snapshot' && c[1] === 'tasks'
		);
		if (!snapshotCall) throw new Error('snapshot handler not registered');
		const handleSnapshot = snapshotCall[2] as (data: unknown) => void;
		const deltaCall = calls.findLast(
			(c) => c[0] === 'delta' && c[1] === 'tasks'
		);
		if (!deltaCall) throw new Error('delta handler not registered');
		const handleDelta = deltaCall[2] as (data: unknown) => void;

		const meta: Meta = {
			context: '',
			weekly_limit: 0,
			weekly_count: 0,
			backlog_limit: 0,
			backlog_count: 0,
			last_synced_at: new Date().toISOString()
		};
		handleSnapshot({ tasks, meta });
		return { tasksStore, handleSnapshot, handleDelta, meta };
	}

	it('snapshot: pending temp tasks survive snapshot replacement', async () => {
		const { tasksStore, handleSnapshot, meta } = await setupWithTasks([makeTask('A'), makeTask('B')]);

		// User created a temp task (duplicate after A)
		const tempTask: Task = { ...makeTask('A'), id: 'temp-dup-1', content: 'Task A copy' };
		tasksStore.insertAfterLocal('A', tempTask);

		// Simulate pending createTask action in queue
		mockActionQueue.items = [
			{ type: 'createTask', payload: { data: { content: 'Task A copy' } }, status: 'pending' }
		];

		// Server sends snapshot WITHOUT the new task (hasn't processed it yet)
		handleSnapshot({ tasks: [makeTask('A'), makeTask('B')], meta });

		// Temp task should survive the snapshot
		const ids = tasksStore.tasks.map((t) => t.id);
		expect(ids).toContain('temp-dup-1');
		expect(ids).toEqual(['A', 'temp-dup-1', 'B']);
	});

	it('snapshot: temp task not reinjected if server has real version', async () => {
		const { tasksStore, handleSnapshot, meta } = await setupWithTasks([makeTask('A'), makeTask('B')]);

		// User created a temp task
		const tempTask: Task = { ...makeTask('A'), id: 'temp-dup-1', content: 'Task A copy' };
		tasksStore.insertAfterLocal('A', tempTask);

		mockActionQueue.items = [
			{ type: 'createTask', payload: { data: { content: 'Task A copy' } }, status: 'pending' }
		];

		// Server snapshot already includes the real version
		const realTask = { ...makeTask('real-1'), content: 'Task A copy' };
		handleSnapshot({ tasks: [makeTask('A'), realTask, makeTask('B')], meta });

		// Should NOT have both temp and real — only real
		const ids = tasksStore.tasks.map((t) => t.id);
		expect(ids).not.toContain('temp-dup-1');
		expect(ids).toContain('real-1');
	});

	it('snapshot: temp task not reinjected if in pendingRemovals', async () => {
		const { tasksStore, handleSnapshot, meta } = await setupWithTasks([makeTask('A'), makeTask('B')]);

		// User created a temp task, then removed it (e.g. duplicate failed)
		const tempTask: Task = { ...makeTask('A'), id: 'temp-dup-1', content: 'Task A copy' };
		tasksStore.insertAfterLocal('A', tempTask);
		tasksStore.removeTaskLocal('temp-dup-1');

		mockActionQueue.items = [
			{ type: 'createTask', payload: { data: { content: 'Task A copy' } }, status: 'pending' }
		];

		// Snapshot arrives
		handleSnapshot({ tasks: [makeTask('A'), makeTask('B')], meta });

		// Temp task should stay removed
		const ids = tasksStore.tasks.map((t) => t.id);
		expect(ids).not.toContain('temp-dup-1');
	});
});

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
	onStateChange: vi.fn(() => vi.fn()),
	currentSeq: 0
};

vi.mock('$lib/ws/client.svelte', () => ({
	wsClient: mockWsClient
}));

vi.mock('./contexts.svelte', () => ({
	contextsStore: {
		activeView: 'today',
		activeContextId: null
	}
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

describe('tasksStore delta temp→real reconciliation', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		mockWsClient.onStateChange.mockImplementation(() => vi.fn());
	});

	afterEach(() => {
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

	it('delta upsert clears pendingRemoval — recurring task reappears with next due date', async () => {
		const recurringTask: Task = { ...makeTask('1'), due: { date: '2026-04-07', recurring: true } };
		const { tasksStore, handleDelta } = await setupWithTasks([recurringTask, makeTask('2')]);

		tasksStore.removeTaskLocal('1');
		expect(tasksStore.tasks).toHaveLength(1);

		// Server processes item_close: same ID, next due date
		handleDelta({ upserted: [{ ...makeTask('1'), due: { date: '2026-04-14', recurring: true } }], removed: [] });

		expect(tasksStore.tasks).toHaveLength(2);
		expect(tasksStore.tasks.find((t) => t.id === '1')!.due!.date).toBe('2026-04-14');
	});
});

describe('tasksStore WS reconnect resubscribe', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		mockWsClient.onStateChange.mockImplementation(() => vi.fn());
	});

	afterEach(() => {
		vi.resetModules();
	});

	it('WS reconnect resubscribes when stale', async () => {
		const { tasksStore } = await import('./tasks.svelte');

		await tasksStore.start();

		// Find the onStateChange registration
		const stateChangeCalls = mockWsClient.onStateChange.mock.calls as unknown as [
			[(connected: boolean) => void]
		];
		expect(stateChangeCalls.length).toBeGreaterThan(0);
		const stateChangeHandler = stateChangeCalls[stateChangeCalls.length - 1][0];

		// Reset mocks to track new calls
		mockWsClient.subscribe.mockClear();

		// Set stale state by simulating a stale scenario
		// First deliver a snapshot, then the handler won't trigger resubscribe unless stale
		const snapshotCalls = mockWsClient.onMessage.mock.calls as unknown[][];
		const snapshotCall = snapshotCalls.findLast(
			(c) => c[0] === 'snapshot' && c[1] === 'tasks'
		);
		if (!snapshotCall) throw new Error('snapshot handler not registered');
		const handleSnapshot = snapshotCall[2] as (data: unknown) => void;

		// Deliver snapshot with old timestamp to make isStale=true
		handleSnapshot({
			tasks: [],
			meta: {
				context: '',
				weekly_limit: 0,
				weekly_count: 0,
				backlog_limit: 0,
				backlog_count: 0,
				last_synced_at: new Date(Date.now() - 5 * 60 * 1000).toISOString()
			}
		});

		expect(tasksStore.isStale).toBe(true);

		// Simulate reconnect
		stateChangeHandler(true);

		// Should resubscribe
		expect(mockWsClient.subscribe).toHaveBeenCalledWith('tasks', expect.objectContaining({ view: 'today' }));

		tasksStore.stop();
	});
});

describe('tasksStore applyPendingTaskUpdate', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		mockWsClient.onStateChange.mockImplementation(() => vi.fn());
	});

	afterEach(() => {
		vi.resetModules();
	});

	it('applyPendingTaskUpdate is identity (no queue)', async () => {
		const { tasksStore } = await import('./tasks.svelte');

		const task = makeTask('1');
		const result = tasksStore.applyPendingTaskUpdate(task);
		expect(result).toBe(task);
	});
});

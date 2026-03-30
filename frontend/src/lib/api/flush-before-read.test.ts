import { describe, it, expect, vi, beforeEach } from 'vitest';
import { QueuedBackend } from './queued-backend';
import type { BackendConnector } from './backend';
import type { Task, CreateTaskRequest } from './types';
import type { actionQueue as ActionQueueType } from '$lib/sync/action-queue.svelte';

vi.mock('$lib/stores/logger', () => ({
	logger: { log: vi.fn(), warn: vi.fn(), error: vi.fn() }
}));

type ActionQueue = typeof ActionQueueType;

function makeTask(overrides: Partial<Task> = {}): Task {
	return {
		id: 'parent-1',
		content: 'Parent task',
		description: '',
		project_id: 'proj-1',
		section_id: null,
		parent_id: null,
		labels: [],
		priority: 1,
		due: null,
		sub_task_count: 0,
		completed_sub_task_count: 0,
		completed_at: null,
		added_at: '2026-01-01T00:00:00Z',
		is_project_task: false,
		children: [],
		...overrides
	};
}

/**
 * These tests verify the flush-before-read pattern used in TaskDetailPanel:
 *
 *   createTask()          // enqueues in QueuedBackend
 *     .then(flushNow)     // sends queued actions to backend
 *     .then(getTask)      // re-fetches with real data
 *
 * Without flushing first, getTask returns stale data because the
 * mutation is still sitting in the IndexedDB queue.
 */
describe('flush-before-read pattern', () => {
	let inner: BackendConnector;
	let queue: ActionQueue;
	let queuedBackend: QueuedBackend;
	let pendingActions: Array<{ type: string; payload: unknown }>;

	beforeEach(() => {
		vi.clearAllMocks();
		pendingActions = [];

		const parentWithoutChild = makeTask();
		const parentWithChild = makeTask({
			children: [
				makeTask({ id: 'child-1', content: 'New subtask', parent_id: 'parent-1' })
			],
			sub_task_count: 1
		});

		inner = {
			login: vi.fn(),
			logout: vi.fn(),
			me: vi.fn(),
			getTasks: vi.fn(),
			// getTask returns stale data until flush delivers the createTask
			getTask: vi.fn()
				.mockImplementation(() => {
					// If queue has been flushed (pendingActions empty), return fresh data
					if (pendingActions.length === 0) {
						return Promise.resolve(parentWithChild);
					}
					// Otherwise return stale data (no children)
					return Promise.resolve(parentWithoutChild);
				}),
			getInboxTasks: vi.fn(),
			getWeeklyTasks: vi.fn(),
			getNextWeekTasks: vi.fn(),
			getTodayTasks: vi.fn(),
			getTomorrowTasks: vi.fn(),
			getCompletedTasks: vi.fn(),
			getBacklogTasks: vi.fn(),
			getCompletedSubtasks: vi.fn(),
			createTask: vi.fn().mockResolvedValue(undefined),
			updateTask: vi.fn(),
			batchUpdateLabels: vi.fn(),
			moveTask: vi.fn(),
			completeTask: vi.fn(),
			duplicateTask: vi.fn(),
			deleteTask: vi.fn(),
			getAppConfig: vi.fn(),
			patchState: vi.fn(),
			resetWeeklyLabel: vi.fn()
		};

		queue = {
			get pendingCount() { return pendingActions.length; },
			get failedCount() { return 0; },
			get items() { return []; },
			get flushing() { return false; },
			init: vi.fn(),
			enqueue: vi.fn((action: { type: string; payload: unknown }) => {
				pendingActions.push(action);
				return Promise.resolve();
			}),
			flush: vi.fn(() => {
				pendingActions.length = 0;
				return Promise.resolve();
			}),
			flushNow: vi.fn(() => {
				pendingActions.length = 0;
				return Promise.resolve();
			}),
			startAutoFlush: vi.fn(),
			stopAutoFlush: vi.fn(),
			clear: vi.fn(() => Promise.resolve()),
			retryFailed: vi.fn(),
			discard: vi.fn()
		} as unknown as ActionQueue;

		queuedBackend = new QueuedBackend(inner, queue);
	});

	it('getTask returns stale data when called without flushing first (the bug)', async () => {
		const data: CreateTaskRequest = {
			content: 'New subtask',
			description: '',
			labels: [],
			priority: 1,
			parent_id: 'parent-1'
		};

		// Step 1: createTask enqueues (does NOT send to backend)
		await queuedBackend.createTask(data);
		expect(pendingActions).toHaveLength(1);
		expect(inner.createTask).not.toHaveBeenCalled();

		// Step 2: getTask without flush → stale data (no children)
		const stale = await queuedBackend.getTask('parent-1');
		expect(stale.children).toHaveLength(0);
	});

	it('getTask returns fresh data after flushing (the fix)', async () => {
		const data: CreateTaskRequest = {
			content: 'New subtask',
			description: '',
			labels: [],
			priority: 1,
			parent_id: 'parent-1'
		};

		// Step 1: createTask enqueues
		await queuedBackend.createTask(data);
		expect(pendingActions).toHaveLength(1);

		// Step 2: flush the queue first
		await queue.flushNow();
		expect(pendingActions).toHaveLength(0);

		// Step 3: now getTask returns fresh data with the child
		const fresh = await queuedBackend.getTask('parent-1');
		expect(fresh.children).toHaveLength(1);
		expect(fresh.children[0].content).toBe('New subtask');
	});

	it('flush-before-read preserves correct call ordering', async () => {
		const callOrder: string[] = [];

		(queue.enqueue as ReturnType<typeof vi.fn>).mockImplementation(async () => {
			callOrder.push('enqueue');
			pendingActions.push({ type: 'createTask', payload: {} });
		});

		(queue.flushNow as ReturnType<typeof vi.fn>).mockImplementation(async () => {
			callOrder.push('flushNow');
			pendingActions.length = 0;
		});

		(inner.getTask as ReturnType<typeof vi.fn>).mockImplementation(async () => {
			callOrder.push('getTask');
			return makeTask();
		});

		// Simulate the fixed saveSubtask flow:
		// createTask() → then flushNow() → then getTask()
		await queuedBackend.createTask({
			content: 'sub',
			description: '',
			labels: [],
			priority: 1,
			parent_id: 'parent-1'
		});
		await queue.flushNow();
		await queuedBackend.getTask('parent-1');

		expect(callOrder).toEqual(['enqueue', 'flushNow', 'getTask']);
	});

	it('duplicateTask also requires flush before getTask', async () => {
		const callOrder: string[] = [];

		(queue.enqueue as ReturnType<typeof vi.fn>).mockImplementation(async () => {
			callOrder.push('enqueue');
			pendingActions.push({ type: 'duplicateTask', payload: {} });
		});

		(queue.flushNow as ReturnType<typeof vi.fn>).mockImplementation(async () => {
			callOrder.push('flushNow');
			pendingActions.length = 0;
		});

		(inner.getTask as ReturnType<typeof vi.fn>).mockImplementation(async () => {
			callOrder.push('getTask');
			return makeTask();
		});

		// Simulate the fixed duplicateSubtask flow
		await queuedBackend.duplicateTask('child-1');
		await queue.flushNow();
		await queuedBackend.getTask('parent-1');

		expect(callOrder).toEqual(['enqueue', 'flushNow', 'getTask']);
	});
});

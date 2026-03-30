import { describe, it, expect, vi, beforeEach } from 'vitest';
import { QueuedBackend } from './queued-backend';
import type { BackendConnector } from './backend';
import type { actionQueue as ActionQueueType } from '$lib/sync/action-queue.svelte';

vi.mock('$lib/stores/logger', () => ({
	logger: { log: vi.fn(), warn: vi.fn(), error: vi.fn() }
}));

type ActionQueue = typeof ActionQueueType;

function createMockBackend(): BackendConnector {
	return {
		login: vi.fn(),
		logout: vi.fn(),
		me: vi.fn(),
		getTasks: vi.fn(),
		getTask: vi.fn(),
		getInboxTasks: vi.fn(),
		getWeeklyTasks: vi.fn(),
		getNextWeekTasks: vi.fn(),
		getTodayTasks: vi.fn(),
		getTomorrowTasks: vi.fn(),
		getCompletedTasks: vi.fn(),
		getBacklogTasks: vi.fn(),
		getCompletedSubtasks: vi.fn(),
		createTask: vi.fn(),
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
}

function createMockQueue(): ActionQueue {
	return {
		get pendingCount() { return 0; },
		get failedCount() { return 0; },
		get items() { return []; },
		get flushing() { return false; },
		init: vi.fn(),
		enqueue: vi.fn(() => Promise.resolve()),
		flush: vi.fn(() => Promise.resolve()),
		flushNow: vi.fn(() => Promise.resolve()),
		startAutoFlush: vi.fn(),
		stopAutoFlush: vi.fn(),
		clear: vi.fn(() => Promise.resolve())
	} as unknown as ActionQueue;
}

describe('QueuedBackend', () => {
	let inner: BackendConnector;
	let queue: ActionQueue;
	let backend: QueuedBackend;

	beforeEach(() => {
		vi.clearAllMocks();
		inner = createMockBackend();
		queue = createMockQueue();
		backend = new QueuedBackend(inner, queue);
	});

	// --- All mutations always enqueue ---

	describe('createTask', () => {
		it('always enqueues regardless of connectivity', async () => {
			const data = { content: 'New task', description: '', labels: [], priority: 1 };

			await backend.createTask(data, 'ctx-1');

			expect(queue.enqueue).toHaveBeenCalledWith({
				type: 'createTask',
				payload: { data, context: 'ctx-1' }
			});
			expect(inner.createTask).not.toHaveBeenCalled();
		});
	});

	describe('updateTask', () => {
		it('always enqueues regardless of connectivity', async () => {
			await backend.updateTask('subtask-1', { priority: 4 });

			expect(queue.enqueue).toHaveBeenCalledWith({
				type: 'updateTask',
				payload: { id: 'subtask-1', data: { priority: 4 } }
			});
			expect(inner.updateTask).not.toHaveBeenCalled();
		});
	});

	describe('completeTask', () => {
		it('always enqueues regardless of connectivity', async () => {
			await backend.completeTask('task-1');

			expect(queue.enqueue).toHaveBeenCalledWith({
				type: 'completeTask',
				payload: { id: 'task-1' }
			});
			expect(inner.completeTask).not.toHaveBeenCalled();
		});
	});

	describe('deleteTask', () => {
		it('always enqueues regardless of connectivity', async () => {
			await backend.deleteTask('task-1');

			expect(queue.enqueue).toHaveBeenCalledWith({
				type: 'deleteTask',
				payload: { id: 'task-1' }
			});
			expect(inner.deleteTask).not.toHaveBeenCalled();
		});
	});

	describe('duplicateTask', () => {
		it('always enqueues regardless of connectivity', async () => {
			await backend.duplicateTask('task-1');

			expect(queue.enqueue).toHaveBeenCalledWith({
				type: 'duplicateTask',
				payload: { id: 'task-1' }
			});
			expect(inner.duplicateTask).not.toHaveBeenCalled();
		});
	});

	describe('resetWeeklyLabel', () => {
		it('always enqueues regardless of connectivity', async () => {
			await backend.resetWeeklyLabel();

			expect(queue.enqueue).toHaveBeenCalledWith({
				type: 'resetWeeklyLabel',
				payload: {}
			});
			expect(inner.resetWeeklyLabel).not.toHaveBeenCalled();
		});
	});

	describe('patchState', () => {
		it('always enqueues regardless of connectivity', async () => {
			const update = { collapsed_ids: ['task-1'] };

			await backend.patchState(update);

			expect(queue.enqueue).toHaveBeenCalledWith({
				type: 'patchState',
				payload: { update }
			});
			expect(inner.patchState).not.toHaveBeenCalled();
		});
	});

	// --- Read operations always pass through ---

	describe('read operations pass through', () => {
		it('getTasks passes through', async () => {
			(inner.getTasks as ReturnType<typeof vi.fn>).mockResolvedValue({ tasks: [], meta: {} });

			await backend.getTasks('ctx');

			expect(inner.getTasks).toHaveBeenCalledWith('ctx');
		});

		it('getTask passes through', async () => {
			(inner.getTask as ReturnType<typeof vi.fn>).mockResolvedValue({ id: '1' });

			await backend.getTask('1');

			expect(inner.getTask).toHaveBeenCalledWith('1');
		});

		it('getAppConfig passes through', async () => {
			(inner.getAppConfig as ReturnType<typeof vi.fn>).mockResolvedValue({});

			await backend.getAppConfig();

			expect(inner.getAppConfig).toHaveBeenCalled();
		});
	});
});

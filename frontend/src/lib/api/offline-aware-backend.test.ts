import { describe, it, expect, vi, beforeEach } from 'vitest';
import { OfflineAwareBackend } from './offline-aware-backend';
import type { BackendConnector } from './backend';
import type { actionQueue as ActionQueueType } from '$lib/sync/action-queue.svelte';

// Control wsClient.connected from tests
let mockConnected = false;

vi.mock('$lib/ws/client.svelte', () => ({
	wsClient: {
		get connected() {
			return mockConnected;
		}
	}
}));

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
		completeTask: vi.fn(),
		duplicateTask: vi.fn(() => Promise.resolve('new-id')),
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
		clear: vi.fn(() => Promise.resolve())
	} as unknown as ActionQueue;
}

describe('OfflineAwareBackend', () => {
	let inner: BackendConnector;
	let queue: ActionQueue;
	let backend: OfflineAwareBackend;

	beforeEach(() => {
		vi.clearAllMocks();
		inner = createMockBackend();
		queue = createMockQueue();
		backend = new OfflineAwareBackend(inner, queue);
	});

	// --- duplicateTask ---

	describe('duplicateTask', () => {
		it('throws offline:not-queueable when offline', async () => {
			mockConnected = false;

			await expect(backend.duplicateTask('task-1')).rejects.toThrow('offline:not-queueable');
			expect(inner.duplicateTask).not.toHaveBeenCalled();
			expect(queue.enqueue).not.toHaveBeenCalled();
		});

		it('delegates to inner backend when online', async () => {
			mockConnected = true;
			(inner.duplicateTask as ReturnType<typeof vi.fn>).mockResolvedValue('new-id');

			const result = await backend.duplicateTask('task-1');

			expect(result).toBe('new-id');
			expect(inner.duplicateTask).toHaveBeenCalledWith('task-1');
		});
	});

	// --- updateTask (used for subtask priority) ---

	describe('updateTask (subtask priority)', () => {
		it('queues priority update when offline', async () => {
			mockConnected = false;

			await backend.updateTask('subtask-1', { priority: 4 });

			expect(queue.enqueue).toHaveBeenCalledWith({
				type: 'updateTask',
				payload: { id: 'subtask-1', data: { priority: 4 } }
			});
			expect(inner.updateTask).not.toHaveBeenCalled();
		});

		it('delegates priority update to inner backend when online', async () => {
			mockConnected = true;

			await backend.updateTask('subtask-1', { priority: 4 });

			expect(inner.updateTask).toHaveBeenCalledWith('subtask-1', { priority: 4 });
			expect(queue.enqueue).not.toHaveBeenCalled();
		});
	});

	// --- createTask ---

	describe('createTask', () => {
		it('queues when offline', async () => {
			mockConnected = false;
			const data = { content: 'New task', description: '', labels: [], priority: 1 };

			await backend.createTask(data, 'ctx-1');

			expect(queue.enqueue).toHaveBeenCalledWith({
				type: 'createTask',
				payload: { data, context: 'ctx-1' }
			});
			expect(inner.createTask).not.toHaveBeenCalled();
		});

		it('delegates to inner backend when online', async () => {
			mockConnected = true;
			const data = { content: 'New task', description: '', labels: [], priority: 1 };

			await backend.createTask(data, 'ctx-1');

			expect(inner.createTask).toHaveBeenCalledWith(data, 'ctx-1');
		});
	});

	// --- completeTask ---

	describe('completeTask', () => {
		it('queues when offline', async () => {
			mockConnected = false;

			await backend.completeTask('task-1');

			expect(queue.enqueue).toHaveBeenCalledWith({
				type: 'completeTask',
				payload: { id: 'task-1' }
			});
			expect(inner.completeTask).not.toHaveBeenCalled();
		});

		it('delegates to inner backend when online', async () => {
			mockConnected = true;

			await backend.completeTask('task-1');

			expect(inner.completeTask).toHaveBeenCalledWith('task-1');
		});
	});

	// --- deleteTask ---

	describe('deleteTask', () => {
		it('queues when offline', async () => {
			mockConnected = false;

			await backend.deleteTask('task-1');

			expect(queue.enqueue).toHaveBeenCalledWith({
				type: 'deleteTask',
				payload: { id: 'task-1' }
			});
			expect(inner.deleteTask).not.toHaveBeenCalled();
		});

		it('delegates to inner backend when online', async () => {
			mockConnected = true;

			await backend.deleteTask('task-1');

			expect(inner.deleteTask).toHaveBeenCalledWith('task-1');
		});
	});

	// --- patchState ---

	describe('patchState', () => {
		it('queues when offline', async () => {
			mockConnected = false;
			const update = { collapsed_ids: ['task-1'] };

			await backend.patchState(update);

			expect(queue.enqueue).toHaveBeenCalledWith({
				type: 'patchState',
				payload: { update }
			});
			expect(inner.patchState).not.toHaveBeenCalled();
		});

		it('delegates to inner backend when online', async () => {
			mockConnected = true;
			const update = { collapsed_ids: ['task-1'] };

			await backend.patchState(update);

			expect(inner.patchState).toHaveBeenCalledWith(update);
		});
	});

	// --- resetWeeklyLabel ---

	describe('resetWeeklyLabel', () => {
		it('throws offline:not-queueable when offline', async () => {
			mockConnected = false;

			await expect(backend.resetWeeklyLabel()).rejects.toThrow('offline:not-queueable');
			expect(inner.resetWeeklyLabel).not.toHaveBeenCalled();
		});

		it('delegates to inner backend when online', async () => {
			mockConnected = true;
			(inner.resetWeeklyLabel as ReturnType<typeof vi.fn>).mockResolvedValue({ updated: 3 });

			const result = await backend.resetWeeklyLabel();

			expect(result).toEqual({ updated: 3 });
			expect(inner.resetWeeklyLabel).toHaveBeenCalled();
		});
	});

	// --- Read operations always pass through ---

	describe('read operations pass through regardless of connectivity', () => {
		it('getTasks passes through when offline', async () => {
			mockConnected = false;
			(inner.getTasks as ReturnType<typeof vi.fn>).mockResolvedValue({ tasks: [], meta: {} });

			await backend.getTasks('ctx');

			expect(inner.getTasks).toHaveBeenCalledWith('ctx');
		});

		it('getTask passes through when offline', async () => {
			mockConnected = false;
			(inner.getTask as ReturnType<typeof vi.fn>).mockResolvedValue({ id: '1' });

			await backend.getTask('1');

			expect(inner.getTask).toHaveBeenCalledWith('1');
		});

		it('getAppConfig passes through when offline', async () => {
			mockConnected = false;
			(inner.getAppConfig as ReturnType<typeof vi.fn>).mockResolvedValue({});

			await backend.getAppConfig();

			expect(inner.getAppConfig).toHaveBeenCalled();
		});
	});
});

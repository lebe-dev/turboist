import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import type { BackendConnector } from '$lib/api/backend';
import type { QueuedAction } from '$lib/sync/db';

// Mock dependencies
vi.mock('$lib/stores/logger', () => ({
	logger: { log: vi.fn(), warn: vi.fn(), error: vi.fn() }
}));

let nextId = 1;
const mockDb = {
	saveQueuedAction: vi.fn(() => Promise.resolve(nextId++)),
	loadPendingActions: vi.fn((): Promise<QueuedAction[]> => Promise.resolve([])),
	removeQueuedAction: vi.fn(() => Promise.resolve()),
	updateQueuedAction: vi.fn(() => Promise.resolve()),
	clearActionQueue: vi.fn(() => Promise.resolve())
};

vi.mock('$lib/sync/db', () => mockDb);

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
		createTask: vi.fn(() => Promise.resolve()),
		updateTask: vi.fn(() => Promise.resolve()),
		completeTask: vi.fn(() => Promise.resolve()),
		duplicateTask: vi.fn(() => Promise.resolve()),
		deleteTask: vi.fn(() => Promise.resolve()),
		getAppConfig: vi.fn(),
		patchState: vi.fn(() => Promise.resolve()),
		resetWeeklyLabel: vi.fn(() => Promise.resolve())
	};
}

function httpError(status: number, message = 'HTTP error'): Error & { status: number } {
	const err = new Error(message) as Error & { status: number };
	err.status = status;
	return err;
}

function makeAction(overrides: Partial<QueuedAction> = {}): QueuedAction {
	return {
		id: 1,
		type: 'updateTask',
		payload: { id: 'task-1', data: { content: 'test' } },
		createdAt: Date.now(),
		status: 'pending',
		...overrides
	};
}

async function freshQueue() {
	vi.resetModules();
	const mod = await import('./action-queue.svelte');
	return mod.actionQueue;
}

describe('action-queue', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		nextId = 1;
	});

	// ─── init ───

	describe('init', () => {
		it('loads pending actions from IDB and sets counts', async () => {
			const actions: QueuedAction[] = [
				makeAction({ id: 1, status: 'pending' }),
				makeAction({ id: 2, status: 'failed', error: 'oops' }),
				makeAction({ id: 3, status: 'pending' })
			];
			mockDb.loadPendingActions.mockResolvedValueOnce(actions);

			const queue = await freshQueue();
			await queue.init();

			expect(mockDb.loadPendingActions).toHaveBeenCalled();
			expect(queue.pendingCount).toBe(2);
			expect(queue.failedCount).toBe(1);
			expect(queue.items).toHaveLength(3);
		});

		it('handles IDB error gracefully', async () => {
			mockDb.loadPendingActions.mockRejectedValueOnce(new Error('IDB broken'));

			const queue = await freshQueue();
			await queue.init();

			expect(queue.pendingCount).toBe(0);
			expect(queue.failedCount).toBe(0);
			expect(queue.items).toHaveLength(0);
		});
	});

	// ─── enqueue — basic ───

	describe('enqueue', () => {
		it('enqueues action with pending status and increments pendingCount', async () => {
			const queue = await freshQueue();

			await queue.enqueue({
				type: 'createTask',
				payload: { data: { content: 'New', description: '', labels: [], priority: 1 } }
			});

			expect(mockDb.saveQueuedAction).toHaveBeenCalledWith(
				expect.objectContaining({ type: 'createTask', status: 'pending' })
			);
			expect(queue.pendingCount).toBe(1);
			expect(queue.items).toHaveLength(1);
		});

		it('does not crash on IDB save failure', async () => {
			mockDb.saveQueuedAction.mockRejectedValueOnce(new Error('IDB full'));

			const queue = await freshQueue();
			await queue.enqueue({ type: 'completeTask', payload: { id: 'task-1' } });

			expect(queue.pendingCount).toBe(0);
			expect(queue.items).toHaveLength(0);
		});
	});

	// ─── enqueue — $state proxy snapshot ───

	describe('enqueue — IDB receives plain snapshots, not proxy references', () => {
		it('saveQueuedAction receives a deep copy of the payload', async () => {
			const queue = await freshQueue();

			const labels = ['weekly', 'work'];
			const data = { labels };

			await queue.enqueue({ type: 'updateTask', payload: { id: 'task-1', data } });

			const savedArg = (mockDb.saveQueuedAction as ReturnType<typeof vi.fn>).mock.calls[0]?.[0] as QueuedAction | undefined;
			expect(savedArg).toBeDefined();
			const savedPayload = savedArg!.payload as { id: string; data: { labels: string[] } };

			// Must be a separate array, not the same reference
			expect(savedPayload.data.labels).toEqual(['weekly', 'work']);
			expect(savedPayload.data.labels).not.toBe(labels);
		});

		it('updateQueuedAction receives a deep copy when coalescing updateTask', async () => {
			const queue = await freshQueue();

			const labels1 = ['weekly'];
			await queue.enqueue({ type: 'updateTask', payload: { id: 'task-1', data: { labels: labels1 } } });

			const labels2 = ['monthly'];
			await queue.enqueue({ type: 'updateTask', payload: { id: 'task-1', data: { labels: labels2 } } });

			const updatedArg = (mockDb.updateQueuedAction as ReturnType<typeof vi.fn>).mock.calls[0]?.[0] as QueuedAction | undefined;
			expect(updatedArg).toBeDefined();
			const updatedPayload = updatedArg!.payload as { id: string; data: { labels: string[] } };

			// Coalesced value: last write wins for labels
			expect(updatedPayload.data.labels).toEqual(['monthly']);
			// Must not be the same reference as the input array
			expect(updatedPayload.data.labels).not.toBe(labels2);
		});

		it('updateQueuedAction receives a deep copy when coalescing patchState', async () => {
			const queue = await freshQueue();

			const ids = ['task-1'];
			await queue.enqueue({ type: 'patchState', payload: { update: { collapsed_ids: ids } } });
			await queue.enqueue({ type: 'patchState', payload: { update: { sidebar_collapsed: true } } });

			const updatedArg = (mockDb.updateQueuedAction as ReturnType<typeof vi.fn>).mock.calls[0]?.[0] as QueuedAction | undefined;
			expect(updatedArg).toBeDefined();
			const updatedPayload = updatedArg!.payload as { update: Record<string, unknown> };

			expect(updatedPayload.update.collapsed_ids).toEqual(['task-1']);
			expect(updatedPayload.update.collapsed_ids).not.toBe(ids);
		});
	});

	// ─── enqueue — coalescing updateTask ───

	describe('enqueue — coalescing updateTask', () => {
		it('merges second updateTask for same task ID', async () => {
			const queue = await freshQueue();

			await queue.enqueue({ type: 'updateTask', payload: { id: 'task-1', data: { content: 'a' } } });
			await queue.enqueue({ type: 'updateTask', payload: { id: 'task-1', data: { priority: 4 } } });

			expect(mockDb.saveQueuedAction).toHaveBeenCalledTimes(1);
			expect(mockDb.updateQueuedAction).toHaveBeenCalledTimes(1);
			expect(queue.pendingCount).toBe(1);
			expect(queue.items).toHaveLength(1);

			const merged = queue.items[0].payload as { id: string; data: Record<string, unknown> };
			expect(merged.data.content).toBe('a');
			expect(merged.data.priority).toBe(4);
		});

		it('does not coalesce different task IDs', async () => {
			const queue = await freshQueue();

			await queue.enqueue({ type: 'updateTask', payload: { id: 'task-1', data: { content: 'a' } } });
			await queue.enqueue({ type: 'updateTask', payload: { id: 'task-2', data: { content: 'b' } } });

			expect(mockDb.saveQueuedAction).toHaveBeenCalledTimes(2);
			expect(queue.pendingCount).toBe(2);
			expect(queue.items).toHaveLength(2);
		});

		it('does not coalesce if existing is failed', async () => {
			const failedAction = makeAction({ id: 1, status: 'failed', type: 'updateTask', payload: { id: 'task-1', data: { content: 'old' } } });
			mockDb.loadPendingActions.mockResolvedValueOnce([failedAction]);

			const queue = await freshQueue();
			await queue.init();
			await queue.enqueue({ type: 'updateTask', payload: { id: 'task-1', data: { content: 'new' } } });

			// Should save new action, not update existing
			expect(mockDb.saveQueuedAction).toHaveBeenCalledTimes(1);
			expect(queue.pendingCount).toBe(1);
			expect(queue.failedCount).toBe(1);
			expect(queue.items).toHaveLength(2);
		});

		it('later fields override earlier fields', async () => {
			const queue = await freshQueue();

			await queue.enqueue({ type: 'updateTask', payload: { id: 'task-1', data: { content: 'first' } } });
			await queue.enqueue({ type: 'updateTask', payload: { id: 'task-1', data: { content: 'second' } } });

			const merged = queue.items[0].payload as { id: string; data: Record<string, unknown> };
			expect(merged.data.content).toBe('second');
		});
	});

	// ─── enqueue — coalescing patchState ───

	describe('enqueue — coalescing patchState', () => {
		it('merges second patchState with existing pending', async () => {
			const queue = await freshQueue();

			await queue.enqueue({ type: 'patchState', payload: { update: { active_view: 'today' as const } } });
			await queue.enqueue({ type: 'patchState', payload: { update: { sidebar_collapsed: true } } });

			expect(mockDb.saveQueuedAction).toHaveBeenCalledTimes(1);
			expect(mockDb.updateQueuedAction).toHaveBeenCalledTimes(1);
			expect(queue.pendingCount).toBe(1);

			const merged = queue.items[0].payload as { update: Record<string, unknown> };
			expect(merged.update.active_view).toBe('today');
			expect(merged.update.sidebar_collapsed).toBe(true);
		});

		it('does not coalesce if no pending patchState exists', async () => {
			const failedPatch = makeAction({ id: 1, status: 'failed', type: 'patchState', payload: { update: { active_view: 'today' } } });
			mockDb.loadPendingActions.mockResolvedValueOnce([failedPatch]);

			const queue = await freshQueue();
			await queue.init();
			await queue.enqueue({ type: 'patchState', payload: { update: { sidebar_collapsed: true } } });

			expect(mockDb.saveQueuedAction).toHaveBeenCalledTimes(1);
			expect(queue.items).toHaveLength(2);
		});
	});

	// ─── flush — success ───

	describe('flush', () => {
		it('flushes single pending action and removes it', async () => {
			const queue = await freshQueue();
			await queue.enqueue({ type: 'completeTask', payload: { id: 'task-1' } });

			const backend = createMockBackend();
			await queue.flush(backend);

			expect(backend.completeTask).toHaveBeenCalledWith('task-1');
			expect(mockDb.removeQueuedAction).toHaveBeenCalledWith(1);
			expect(queue.pendingCount).toBe(0);
			expect(queue.items).toHaveLength(0);
		});

		it('flushes in FIFO order by createdAt', async () => {
			const queue = await freshQueue();

			// Enqueue in order; ids will be 1, 2
			await queue.enqueue({ type: 'completeTask', payload: { id: 'first' } });
			await queue.enqueue({ type: 'deleteTask', payload: { id: 'second' } });

			const order: string[] = [];
			const backend = createMockBackend();
			(backend.completeTask as ReturnType<typeof vi.fn>).mockImplementation(() => {
				order.push('complete');
				return Promise.resolve();
			});
			(backend.deleteTask as ReturnType<typeof vi.fn>).mockImplementation(() => {
				order.push('delete');
				return Promise.resolve();
			});

			await queue.flush(backend);

			expect(order).toEqual(['complete', 'delete']);
		});

		it('dispatches each action type to correct backend method', async () => {
			const queue = await freshQueue();
			const backend = createMockBackend();

			await queue.enqueue({ type: 'createTask', payload: { data: { content: 'x', description: '', labels: [], priority: 1 }, context: 'ctx' } });
			await queue.flush(backend);
			expect(backend.createTask).toHaveBeenCalledWith({ content: 'x', description: '', labels: [], priority: 1 }, 'ctx');

			vi.clearAllMocks();
			nextId = 10;
			const queue2 = await freshQueue();
			await queue2.enqueue({ type: 'updateTask', payload: { id: 't1', data: { content: 'y' } } });
			await queue2.flush(backend);
			expect(backend.updateTask).toHaveBeenCalledWith('t1', { content: 'y' });

			vi.clearAllMocks();
			nextId = 20;
			const queue3 = await freshQueue();
			await queue3.enqueue({ type: 'duplicateTask', payload: { id: 't2' } });
			await queue3.flush(backend);
			expect(backend.duplicateTask).toHaveBeenCalledWith('t2');

			vi.clearAllMocks();
			nextId = 30;
			const queue4 = await freshQueue();
			await queue4.enqueue({ type: 'resetWeeklyLabel', payload: {} });
			await queue4.flush(backend);
			expect(backend.resetWeeklyLabel).toHaveBeenCalled();

			vi.clearAllMocks();
			nextId = 40;
			const queue5 = await freshQueue();
			await queue5.enqueue({ type: 'patchState', payload: { update: { sidebar_collapsed: true } } });
			await queue5.flush(backend);
			expect(backend.patchState).toHaveBeenCalledWith({ sidebar_collapsed: true });
		});

		it('sets flushing=true during flush and false after', async () => {
			const queue = await freshQueue();
			await queue.enqueue({ type: 'completeTask', payload: { id: 'task-1' } });

			let flushingDuringExec = false;
			const backend = createMockBackend();
			(backend.completeTask as ReturnType<typeof vi.fn>).mockImplementation(() => {
				flushingDuringExec = queue.flushing;
				return Promise.resolve();
			});

			await queue.flush(backend);

			expect(flushingDuringExec).toBe(true);
			expect(queue.flushing).toBe(false);
		});

		it('reentrant guard: second concurrent flush is no-op', async () => {
			const queue = await freshQueue();
			await queue.enqueue({ type: 'completeTask', payload: { id: 'task-1' } });

			let resolveBackend!: () => void;
			const backendPromise = new Promise<void>((r) => { resolveBackend = r; });
			const backend = createMockBackend();
			(backend.completeTask as ReturnType<typeof vi.fn>).mockReturnValue(backendPromise);

			// updateQueuedAction is called before executeAction during flush,
			// so we need the backend call to block
			const p1 = queue.flush(backend);
			// Give flush time to reach the backend call
			await new Promise((r) => setTimeout(r, 0));
			const p2 = queue.flush(backend);

			resolveBackend();
			await Promise.all([p1, p2]);

			expect(backend.completeTask).toHaveBeenCalledTimes(1);
		});

		it('flushes failed actions too (not just pending)', async () => {
			const failedAction = makeAction({ id: 1, status: 'failed', type: 'completeTask', payload: { id: 'task-1' } });
			mockDb.loadPendingActions.mockResolvedValueOnce([failedAction]);

			const queue = await freshQueue();
			await queue.init();

			const backend = createMockBackend();
			await queue.flush(backend);

			expect(backend.completeTask).toHaveBeenCalledWith('task-1');
		});
	});

	// ─── flush — 401 ───

	describe('flush — 401', () => {
		it('stops flushing and resets action to pending', async () => {
			const queue = await freshQueue();
			await queue.enqueue({ type: 'completeTask', payload: { id: 'task-1' } });

			const backend = createMockBackend();
			(backend.completeTask as ReturnType<typeof vi.fn>).mockRejectedValue(httpError(401));

			await queue.flush(backend);

			expect(queue.pendingCount).toBe(1);
			expect(queue.items[0].status).toBe('pending');
		});

		it('does not process subsequent actions after 401', async () => {
			const queue = await freshQueue();
			await queue.enqueue({ type: 'completeTask', payload: { id: 'task-1' } });
			await queue.enqueue({ type: 'deleteTask', payload: { id: 'task-2' } });

			const backend = createMockBackend();
			(backend.completeTask as ReturnType<typeof vi.fn>).mockRejectedValue(httpError(401));

			await queue.flush(backend);

			expect(backend.completeTask).toHaveBeenCalledTimes(1);
			expect(backend.deleteTask).not.toHaveBeenCalled();
			expect(queue.pendingCount).toBe(2);
		});
	});

	// ─── flush — 404 ───

	describe('flush — 404 on mutations', () => {
		it.each([
			['completeTask', { id: 'task-1' }],
			['deleteTask', { id: 'task-1' }],
			['updateTask', { id: 'task-1', data: { content: 'x' } }],
			['duplicateTask', { id: 'task-1' }]
		] as const)('404 on %s treated as success', async (type, payload) => {
			const queue = await freshQueue();
			await queue.enqueue({ type, payload });

			const backend = createMockBackend();
			const methodName = type as keyof BackendConnector;
			(backend[methodName] as ReturnType<typeof vi.fn>).mockRejectedValue(httpError(404));

			await queue.flush(backend);

			expect(queue.pendingCount).toBe(0);
			expect(queue.items).toHaveLength(0);
		});

		it('404 on createTask is NOT treated as success', async () => {
			const queue = await freshQueue();
			await queue.enqueue({
				type: 'createTask',
				payload: { data: { content: 'x', description: '', labels: [], priority: 1 } }
			});

			const backend = createMockBackend();
			(backend.createTask as ReturnType<typeof vi.fn>).mockRejectedValue(httpError(404));

			await queue.flush(backend);

			expect(queue.pendingCount).toBe(0);
			expect(queue.failedCount).toBe(1);
			expect(queue.items[0].status).toBe('failed');
		});
	});

	// ─── flush — 5xx retry ───

	describe('flush — 5xx retry', () => {
		beforeEach(() => {
			vi.useFakeTimers();
		});

		afterEach(() => {
			vi.useRealTimers();
		});

		it('retries with exponential backoff and succeeds', async () => {
			const queue = await freshQueue();
			await queue.enqueue({ type: 'completeTask', payload: { id: 'task-1' } });

			const backend = createMockBackend();
			let calls = 0;
			(backend.completeTask as ReturnType<typeof vi.fn>).mockImplementation(() => {
				calls++;
				if (calls <= 2) return Promise.reject(httpError(500));
				return Promise.resolve();
			});

			const flushPromise = queue.flush(backend);

			// First retry after 1s backoff
			await vi.advanceTimersByTimeAsync(1000);
			// Second retry after 2s backoff
			await vi.advanceTimersByTimeAsync(2000);

			await flushPromise;

			expect(calls).toBe(3);
			expect(queue.pendingCount).toBe(0);
			expect(queue.items).toHaveLength(0);
		});

		it('marks as failed after exhausting retries', async () => {
			const queue = await freshQueue();
			await queue.enqueue({ type: 'completeTask', payload: { id: 'task-1' } });

			const backend = createMockBackend();
			(backend.completeTask as ReturnType<typeof vi.fn>).mockRejectedValue(httpError(500));

			const flushPromise = queue.flush(backend);

			// Advance through all backoff delays: 1s, 2s, 4s
			await vi.advanceTimersByTimeAsync(1000);
			await vi.advanceTimersByTimeAsync(2000);
			await vi.advanceTimersByTimeAsync(4000);

			await flushPromise;

			expect(queue.pendingCount).toBe(0);
			expect(queue.failedCount).toBe(1);
			expect(queue.items[0].status).toBe('failed');
		});
	});

	// ─── flush — other 4xx ───

	describe('flush — other 4xx', () => {
		it('marks action as failed with error message', async () => {
			const queue = await freshQueue();
			await queue.enqueue({ type: 'completeTask', payload: { id: 'task-1' } });

			const backend = createMockBackend();
			(backend.completeTask as ReturnType<typeof vi.fn>).mockRejectedValue(httpError(400, 'Bad request'));

			await queue.flush(backend);

			expect(queue.pendingCount).toBe(0);
			expect(queue.failedCount).toBe(1);
			expect(queue.items[0].status).toBe('failed');
			expect(queue.items[0].error).toBe('Bad request');
		});
	});

	// ─── clear ───

	describe('clear', () => {
		it('removes all actions and resets counts', async () => {
			const queue = await freshQueue();
			await queue.enqueue({ type: 'completeTask', payload: { id: 'task-1' } });
			await queue.enqueue({ type: 'deleteTask', payload: { id: 'task-2' } });

			await queue.clear();

			expect(mockDb.clearActionQueue).toHaveBeenCalled();
			expect(queue.pendingCount).toBe(0);
			expect(queue.failedCount).toBe(0);
			expect(queue.items).toHaveLength(0);
		});
	});

	// ─── retryFailed ───

	describe('retryFailed', () => {
		it('resets failed action to pending and updates counts', async () => {
			const failedAction = makeAction({ id: 5, status: 'failed', error: 'oops' });
			mockDb.loadPendingActions.mockResolvedValueOnce([failedAction]);

			const queue = await freshQueue();
			await queue.init();

			expect(queue.failedCount).toBe(1);
			expect(queue.pendingCount).toBe(0);

			await queue.retryFailed(5);

			expect(queue.failedCount).toBe(0);
			expect(queue.pendingCount).toBe(1);
			expect(mockDb.updateQueuedAction).toHaveBeenCalledWith(
				expect.objectContaining({ id: 5, status: 'pending', error: undefined })
			);
		});

		it('no-op if action not found', async () => {
			const queue = await freshQueue();
			await queue.retryFailed(999);

			expect(mockDb.updateQueuedAction).not.toHaveBeenCalled();
		});

		it('no-op if action is not failed', async () => {
			const queue = await freshQueue();
			await queue.enqueue({ type: 'completeTask', payload: { id: 'task-1' } });

			await queue.retryFailed(1);

			expect(mockDb.updateQueuedAction).not.toHaveBeenCalled();
			expect(queue.pendingCount).toBe(1);
		});
	});

	// ─── discard ───

	describe('discard', () => {
		it('removes pending action and decrements pendingCount', async () => {
			const queue = await freshQueue();
			await queue.enqueue({ type: 'completeTask', payload: { id: 'task-1' } });

			await queue.discard(1);

			expect(mockDb.removeQueuedAction).toHaveBeenCalledWith(1);
			expect(queue.pendingCount).toBe(0);
			expect(queue.items).toHaveLength(0);
		});

		it('removes failed action and decrements failedCount', async () => {
			const failedAction = makeAction({ id: 5, status: 'failed' });
			mockDb.loadPendingActions.mockResolvedValueOnce([failedAction]);

			const queue = await freshQueue();
			await queue.init();
			await queue.discard(5);

			expect(queue.failedCount).toBe(0);
		});

		it('no-op for non-existent id', async () => {
			const queue = await freshQueue();
			await queue.discard(999);

			expect(mockDb.removeQueuedAction).not.toHaveBeenCalled();
		});
	});

	// ─── auto-flush ───

	describe('auto-flush', () => {
		beforeEach(() => {
			vi.useFakeTimers();
		});

		afterEach(() => {
			vi.useRealTimers();
		});

		it('triggers flush on interval when pendingCount > 0', async () => {
			const queue = await freshQueue();
			await queue.enqueue({ type: 'completeTask', payload: { id: 'task-1' } });

			const backend = createMockBackend();
			queue.startAutoFlush(backend, 5000);

			await vi.advanceTimersByTimeAsync(5000);

			expect(backend.completeTask).toHaveBeenCalledTimes(1);

			queue.stopAutoFlush();
		});

		it('skips when no pending or failed actions', async () => {
			const queue = await freshQueue();
			const backend = createMockBackend();
			queue.startAutoFlush(backend, 5000);

			await vi.advanceTimersByTimeAsync(5000);

			// No backend methods should be called since queue is empty
			expect(backend.completeTask).not.toHaveBeenCalled();

			queue.stopAutoFlush();
		});

		it('stopAutoFlush prevents further flushes', async () => {
			const queue = await freshQueue();
			await queue.enqueue({ type: 'completeTask', payload: { id: 'task-1' } });

			const backend = createMockBackend();
			queue.startAutoFlush(backend, 5000);
			queue.stopAutoFlush();

			await vi.advanceTimersByTimeAsync(10000);

			expect(backend.completeTask).not.toHaveBeenCalled();
		});
	});

	// ─── flushNow ───

	describe('flushNow', () => {
		it('uses stored backend reference from startAutoFlush', async () => {
			const queue = await freshQueue();
			await queue.enqueue({ type: 'completeTask', payload: { id: 'task-1' } });

			const backend = createMockBackend();
			queue.startAutoFlush(backend, 60000);

			await queue.flushNow();

			expect(backend.completeTask).toHaveBeenCalledWith('task-1');

			queue.stopAutoFlush();
		});

		it('no-op without backend', async () => {
			const queue = await freshQueue();
			await queue.enqueue({ type: 'completeTask', payload: { id: 'task-1' } });

			// Should not throw
			await queue.flushNow();

			expect(queue.pendingCount).toBe(1);
		});
	});

	// ─── visibility flush ───

	describe('visibilitychange flush', () => {
		it('flushes when tab hidden and pending > 0', async () => {
			const queue = await freshQueue();
			await queue.enqueue({ type: 'completeTask', payload: { id: 'task-1' } });

			const backend = createMockBackend();
			queue.startAutoFlush(backend, 60000);

			Object.defineProperty(document, 'visibilityState', { value: 'hidden', configurable: true });
			document.dispatchEvent(new Event('visibilitychange'));

			// Wait for the async flush to complete
			await vi.dynamicImportSettled?.() ?? new Promise((r) => setTimeout(r, 0));

			expect(backend.completeTask).toHaveBeenCalledWith('task-1');

			Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
			queue.stopAutoFlush();
		});

		it('does not flush when no pending actions', async () => {
			const queue = await freshQueue();
			const backend = createMockBackend();
			queue.startAutoFlush(backend, 60000);

			Object.defineProperty(document, 'visibilityState', { value: 'hidden', configurable: true });
			document.dispatchEvent(new Event('visibilitychange'));

			await new Promise((r) => setTimeout(r, 0));

			// No backend calls expected
			expect(backend.completeTask).not.toHaveBeenCalled();

			Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
			queue.stopAutoFlush();
		});
	});
});

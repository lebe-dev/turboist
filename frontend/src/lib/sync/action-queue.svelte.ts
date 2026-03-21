import type { BackendConnector } from '$lib/api/backend';
import type { CreateTaskRequest, UpdateTaskRequest, UserState } from '$lib/api/types';
import {
	type QueuedAction,
	saveQueuedAction,
	loadPendingActions,
	removeQueuedAction,
	updateQueuedAction,
	clearActionQueue
} from '$lib/sync/db';
import { logger } from '$lib/stores/logger';

// Unwrap $state proxies before writing to IndexedDB — the structured clone
// algorithm used by IDB cannot serialise Svelte reactive proxies.
function idbSave(action: Omit<QueuedAction, 'id'>): Promise<number> {
	return saveQueuedAction($state.snapshot(action) as typeof action);
}

function idbUpdate(action: QueuedAction): Promise<void> {
	return updateQueuedAction($state.snapshot(action) as QueuedAction);
}

const TAG = 'action-queue';
const MAX_RETRIES = 3;

function delay(ms: number): Promise<void> {
	return new Promise((resolve) => setTimeout(resolve, ms));
}

// Check if an HTTP error response has a specific status code
function getStatusCode(err: unknown): number | undefined {
	if (err && typeof err === 'object' && 'status' in err) {
		return (err as { status: number }).status;
	}
	return undefined;
}

function createActionQueue() {
	let pendingCount = $state(0);
	let failedCount = $state(0);
	let items = $state<QueuedAction[]>([]);
	let flushing = $state(false);

	// Load pending actions from IDB and update reactive state
	async function init(): Promise<void> {
		try {
			const pending = await loadPendingActions();
			items = pending;
			pendingCount = pending.filter((a) => a.status === 'pending').length;
			failedCount = pending.filter((a) => a.status === 'failed').length;
			logger.log(TAG, `Loaded ${pending.length} queued actions (${pendingCount} pending, ${failedCount} failed)`);
		} catch (err) {
			logger.error(TAG, `Failed to load pending actions: ${err}`);
		}
	}

	// Enqueue a new action with coalescing for updateTask and patchState
	async function enqueue(action: Omit<QueuedAction, 'id' | 'createdAt' | 'status'>): Promise<void> {
		// Attempt coalescing before creating a new entry
		if (action.type === 'updateTask') {
			const { id: taskId, data } = action.payload as { id: string; data: UpdateTaskRequest };
			const existing = items.find(
				(a) =>
					a.type === 'updateTask' &&
					a.status === 'pending' &&
					(a.payload as { id: string }).id === taskId
			);
			if (existing) {
				// Merge payloads: keep latest values for each field
				const merged = {
					id: taskId,
					data: { ...(existing.payload as { id: string; data: UpdateTaskRequest }).data, ...data }
				};
				existing.payload = merged;
				existing.createdAt = Date.now();
				await idbUpdate(existing);
				logger.log(TAG, `Coalesced updateTask for task ${taskId}`);
				return;
			}
		}

		if (action.type === 'patchState') {
			const { update } = action.payload as { update: Partial<UserState> };
			const existing = items.find((a) => a.type === 'patchState' && a.status === 'pending');
			if (existing) {
				// Merge state patches
				const merged = {
					update: { ...(existing.payload as { update: Partial<UserState> }).update, ...update }
				};
				existing.payload = merged;
				existing.createdAt = Date.now();
				await idbUpdate(existing);
				logger.log(TAG, 'Coalesced patchState action');
				return;
			}
		}

		const full: Omit<QueuedAction, 'id'> = {
			...action,
			createdAt: Date.now(),
			status: 'pending'
		};

		try {
			const id = await idbSave(full);
			const queued: QueuedAction = { ...full, id };
			items = [...items, queued];
			pendingCount++;
			logger.log(TAG, `Enqueued ${action.type} (id=${id})`);
		} catch (err) {
			logger.error(TAG, `Failed to enqueue ${action.type}: ${err}`);
		}
	}

	// Execute a single action against the backend
	async function executeAction(action: QueuedAction, backend: BackendConnector): Promise<void> {
		switch (action.type) {
			case 'createTask': {
				const { data, context } = action.payload as {
					data: CreateTaskRequest;
					context?: string;
				};
				await backend.createTask(data, context);
				break;
			}
			case 'updateTask': {
				const { id, data } = action.payload as { id: string; data: UpdateTaskRequest };
				await backend.updateTask(id, data);
				break;
			}
			case 'completeTask': {
				const { id } = action.payload as { id: string };
				await backend.completeTask(id);
				break;
			}
			case 'deleteTask': {
				const { id } = action.payload as { id: string };
				await backend.deleteTask(id);
				break;
			}
			case 'duplicateTask': {
				const { id } = action.payload as { id: string };
				await backend.duplicateTask(id);
				break;
			}
			case 'resetWeeklyLabel': {
				await backend.resetWeeklyLabel();
				break;
			}
			case 'patchState': {
				const { update } = action.payload as { update: Partial<UserState> };
				await backend.patchState(update);
				break;
			}
		}
	}

	// Flush all pending actions to the backend in FIFO order
	async function flush(backend: BackendConnector): Promise<void> {
		if (flushing) return;
		flushing = true;

		try {
			// Process items in FIFO order (oldest first)
			const pending = items
				.filter((a) => a.status === 'pending' || a.status === 'failed')
				.sort((a, b) => a.createdAt - b.createdAt);

			for (const action of pending) {
				// Mark as processing
				action.status = 'processing';
				await idbUpdate(action);
				items = [...items];

				let succeeded = false;
				let retries = 0;

				while (!succeeded && retries <= MAX_RETRIES) {
					try {
						await executeAction(action, backend);
						succeeded = true;
					} catch (err) {
						const status = getStatusCode(err);

						// 401: abandon flush entirely, caller handles redirect
						if (status === 401) {
							action.status = 'pending';
							await idbUpdate(action);
							items = [...items];
							logger.warn(TAG, `Got 401 during flush, abandoning`);
							return;
						}

						// 404 for mutations on existing tasks: treat as success (task already gone)
						if (
							status === 404 &&
							(action.type === 'completeTask' ||
								action.type === 'deleteTask' ||
								action.type === 'updateTask' ||
								action.type === 'duplicateTask')
						) {
							logger.warn(
								TAG,
								`Got 404 for ${action.type}, treating as success (task already gone)`
							);
							succeeded = true;
							break;
						}

						// 5xx: retry with exponential backoff
						if (status !== undefined && status >= 500 && retries < MAX_RETRIES) {
							const backoffMs = 1000 * Math.pow(2, retries);
							logger.warn(
								TAG,
								`Got ${status} for ${action.type}, retrying in ${backoffMs}ms (attempt ${retries + 1}/${MAX_RETRIES})`
							);
							await delay(backoffMs);
							retries++;
							continue;
						}

						// Other 4xx or exhausted retries: mark as failed
						const message =
							err instanceof Error ? err.message : String(err);
						action.status = 'failed';
						action.error = message;
						await idbUpdate(action);
						items = [...items];
						pendingCount--;
						failedCount++;
						logger.error(TAG, `Action ${action.type} (id=${action.id}) failed: ${message}`);
						break;
					}
				}

				if (succeeded) {
					// Remove from IDB and reactive state
					await removeQueuedAction(action.id!);
					items = items.filter((a) => a.id !== action.id);
					pendingCount--;
					logger.log(TAG, `Flushed ${action.type} (id=${action.id})`);
				}
			}
		} finally {
			flushing = false;
		}
	}

	// Clear all queued actions
	async function clear(): Promise<void> {
		await clearActionQueue();
		items = [];
		pendingCount = 0;
		failedCount = 0;
		logger.log(TAG, 'Cleared action queue');
	}

	// Retry a failed action by setting its status back to pending
	async function retryFailed(id: number): Promise<void> {
		const action = items.find((a) => a.id === id);
		if (!action || action.status !== 'failed') return;

		action.status = 'pending';
		action.error = undefined;
		await idbUpdate(action);
		items = [...items];
		failedCount--;
		pendingCount++;
		logger.log(TAG, `Marked action ${id} for retry`);
	}

	// Discard a single action by id
	async function discard(id: number): Promise<void> {
		const action = items.find((a) => a.id === id);
		if (!action) return;

		await removeQueuedAction(id);
		const wasFailed = action.status === 'failed';
		const wasPending = action.status === 'pending';
		items = items.filter((a) => a.id !== id);
		if (wasFailed) failedCount--;
		if (wasPending) pendingCount--;
		logger.log(TAG, `Discarded action ${id} (${action.type})`);
	}

	// --- Auto-flush timer ---

	let flushTimer: ReturnType<typeof setInterval> | null = null;
	let flushBackend: BackendConnector | null = null;

	function startAutoFlush(backend: BackendConnector, intervalMs: number): void {
		stopAutoFlush();
		flushBackend = backend;
		flushTimer = setInterval(() => {
			if (pendingCount === 0 && failedCount === 0) return;
			flush(backend).catch((e) =>
				logger.error(TAG, `Auto-flush failed: ${e}`)
			);
		}, intervalMs);
		logger.log(TAG, `Auto-flush started: every ${intervalMs}ms`);
	}

	function stopAutoFlush(): void {
		if (flushTimer) {
			clearInterval(flushTimer);
			flushTimer = null;
		}
		flushBackend = null;
	}

	// Flush immediately using the stored backend reference
	async function flushNow(): Promise<void> {
		if (!flushBackend) {
			logger.warn(TAG, 'flushNow called but no backend set');
			return;
		}
		return flush(flushBackend);
	}

	// Flush when tab is hidden to avoid losing mutations
	if (typeof document !== 'undefined') {
		document.addEventListener('visibilitychange', () => {
			if (document.visibilityState === 'hidden' && pendingCount > 0 && flushBackend) {
				flush(flushBackend).catch((e) =>
					logger.error(TAG, `Visibility flush failed: ${e}`)
				);
			}
		});
	}

	return {
		get pendingCount() {
			return pendingCount;
		},
		get failedCount() {
			return failedCount;
		},
		get items() {
			return items;
		},
		get flushing() {
			return flushing;
		},
		enqueue,
		flush,
		flushNow,
		startAutoFlush,
		stopAutoFlush,
		clear,
		retryFailed,
		discard,
		init
	};
}

export const actionQueue = createActionQueue();

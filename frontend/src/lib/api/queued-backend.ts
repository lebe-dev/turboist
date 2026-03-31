import type { BackendConnector } from './backend';
import type {
	AppConfig,
	CreateTaskRequest,
	DecomposeTaskRequest,
	Task,
	TasksResponse,
	UpdateTaskRequest,
	UserState
} from './types';
import type { actionQueue as ActionQueueType } from '$lib/sync/action-queue.svelte';
import { logger } from '$lib/stores/logger';

type ActionQueue = typeof ActionQueueType;

// Decorator that always queues mutations and passes reads through.
// Mutations are flushed to the backend on a timer or on reconnect.
export class QueuedBackend implements BackendConnector {
	constructor(
		private inner: BackendConnector,
		private queue: ActionQueue
	) {}

	// --- Auth: always pass through (no point queuing) ---

	login(password: string): Promise<void> {
		return this.inner.login(password);
	}

	logout(): Promise<void> {
		return this.inner.logout();
	}

	me(): Promise<void> {
		return this.inner.me();
	}

	// --- Reads: always pass through (callers handle IDB fallback) ---

	getTasks(context?: string): Promise<TasksResponse> {
		return this.inner.getTasks(context);
	}

	getTask(id: string): Promise<Task> {
		return this.inner.getTask(id);
	}

	getInboxTasks(context?: string): Promise<TasksResponse> {
		return this.inner.getInboxTasks(context);
	}

	getWeeklyTasks(context?: string): Promise<TasksResponse> {
		return this.inner.getWeeklyTasks(context);
	}

	getNextWeekTasks(context?: string): Promise<TasksResponse> {
		return this.inner.getNextWeekTasks(context);
	}

	getTodayTasks(context?: string): Promise<TasksResponse> {
		return this.inner.getTodayTasks(context);
	}

	getTomorrowTasks(context?: string): Promise<TasksResponse> {
		return this.inner.getTomorrowTasks(context);
	}

	getCompletedTasks(context?: string): Promise<TasksResponse> {
		return this.inner.getCompletedTasks(context);
	}

	getBacklogTasks(context?: string): Promise<TasksResponse> {
		return this.inner.getBacklogTasks(context);
	}

	getCompletedSubtasks(id: string): Promise<Task[]> {
		return this.inner.getCompletedSubtasks(id);
	}

	getAppConfig(): Promise<AppConfig> {
		return this.inner.getAppConfig();
	}

	// --- Mutations: always enqueue, flushed by timer ---

	async createTask(data: CreateTaskRequest, context?: string): Promise<void> {
		logger.log('sync', 'Queuing createTask');
		await this.queue.enqueue({ type: 'createTask', payload: { data, context } });
	}

	async updateTask(id: string, data: UpdateTaskRequest): Promise<void> {
		logger.log('sync', 'Queuing updateTask');
		await this.queue.enqueue({ type: 'updateTask', payload: { id, data } });
	}

	async batchUpdateLabels(updates: Record<string, string[]>): Promise<void> {
		logger.log('sync', 'Queuing batchUpdateLabels');
		await this.queue.enqueue({ type: 'batchUpdateLabels', payload: { updates } });
	}

	async moveTask(id: string, parentId: string): Promise<void> {
		logger.log('sync', 'Queuing moveTask');
		await this.queue.enqueue({ type: 'moveTask', payload: { id, parentId } });
	}

	async completeTask(id: string): Promise<void> {
		logger.log('sync', 'Queuing completeTask');
		await this.queue.enqueue({ type: 'completeTask', payload: { id } });
	}

	async deleteTask(id: string): Promise<void> {
		logger.log('sync', 'Queuing deleteTask');
		await this.queue.enqueue({ type: 'deleteTask', payload: { id } });
	}

	async duplicateTask(id: string): Promise<void> {
		logger.log('sync', 'Queuing duplicateTask');
		await this.queue.enqueue({ type: 'duplicateTask', payload: { id } });
	}

	async decomposeTask(id: string, data: DecomposeTaskRequest): Promise<void> {
		logger.log('sync', 'Queuing decomposeTask');
		await this.queue.enqueue({ type: 'decomposeTask', payload: { id, data } });
	}

	async resetWeeklyLabel(): Promise<void> {
		logger.log('sync', 'Queuing resetWeeklyLabel');
		await this.queue.enqueue({ type: 'resetWeeklyLabel', payload: {} });
	}

	async patchState(update: Partial<UserState>): Promise<void> {
		logger.log('sync', 'Queuing patchState');
		await this.queue.enqueue({ type: 'patchState', payload: { update } });
	}
}

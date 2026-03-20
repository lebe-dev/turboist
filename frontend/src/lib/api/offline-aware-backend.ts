import type { BackendConnector } from './backend';
import type {
	AppConfig,
	CreateTaskRequest,
	Task,
	TasksResponse,
	UpdateTaskRequest,
	UserState
} from './types';
import type { actionQueue as ActionQueueType } from '$lib/sync/action-queue.svelte';
import { wsClient } from '$lib/ws/client.svelte';
import { logger } from '$lib/stores/logger';

type ActionQueue = typeof ActionQueueType;

// Decorator that queues mutations when offline and passes reads through.
// Uses WS connection state as the connectivity indicator.
export class OfflineAwareBackend implements BackendConnector {
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

	// --- Queueable mutations: enqueue when offline ---

	async createTask(data: CreateTaskRequest, context?: string): Promise<void> {
		if (!wsClient.connected) {
			logger.log('offline', 'Queuing createTask (offline)');
			await this.queue.enqueue({ type: 'createTask', payload: { data, context } });
			return;
		}
		return this.inner.createTask(data, context);
	}

	async updateTask(id: string, data: UpdateTaskRequest): Promise<void> {
		if (!wsClient.connected) {
			logger.log('offline', 'Queuing updateTask (offline)');
			await this.queue.enqueue({ type: 'updateTask', payload: { id, data } });
			return;
		}
		return this.inner.updateTask(id, data);
	}

	async completeTask(id: string): Promise<void> {
		if (!wsClient.connected) {
			logger.log('offline', 'Queuing completeTask (offline)');
			await this.queue.enqueue({ type: 'completeTask', payload: { id } });
			return;
		}
		return this.inner.completeTask(id);
	}

	async deleteTask(id: string): Promise<void> {
		if (!wsClient.connected) {
			logger.log('offline', 'Queuing deleteTask (offline)');
			await this.queue.enqueue({ type: 'deleteTask', payload: { id } });
			return;
		}
		return this.inner.deleteTask(id);
	}

	async patchState(update: Partial<UserState>): Promise<void> {
		if (!wsClient.connected) {
			logger.log('offline', 'Queuing patchState (offline)');
			await this.queue.enqueue({ type: 'patchState', payload: { update } });
			return;
		}
		return this.inner.patchState(update);
	}

	// --- Non-queueable: throw when offline ---

	async duplicateTask(id: string): Promise<string> {
		if (!wsClient.connected) {
			throw new Error('offline:not-queueable');
		}
		return this.inner.duplicateTask(id);
	}

	async resetWeeklyLabel(): Promise<{ updated: number }> {
		if (!wsClient.connected) {
			throw new Error('offline:not-queueable');
		}
		return this.inner.resetWeeklyLabel();
	}
}

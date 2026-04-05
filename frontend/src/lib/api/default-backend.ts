import { goto } from '$app/navigation';
import { logger } from '$lib/stores/logger';
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

// Default HTTP-based backend connector that talks to the Go API server.
export class DefaultBackendConnector implements BackendConnector {
	private async request<T>(path: string, options?: RequestInit): Promise<T> {
		const method = options?.method ?? 'GET';
		logger.log('api', `${method} ${path}`);

		const res = await fetch(path, {
			credentials: 'same-origin',
			cache: 'no-store',
			...options,
			headers: {
				'Content-Type': 'application/json',
				...options?.headers
			}
		});

		if (res.status === 401) {
			logger.warn('api', `${method} ${path} → 401 Unauthorized`);
			goto('/login');
			throw new Error('Unauthorized');
		}

		if (!res.ok) {
			const text = await res.text().catch(() => res.statusText);
			logger.error('api', `${method} ${path} → ${res.status} ${text}`);
			throw new Error(`${res.status}: ${text}`);
		}

		logger.log('api', `${method} ${path} → ${res.status}`);

		const contentType = res.headers.get('content-type');
		if (contentType?.includes('application/json')) {
			return res.json() as Promise<T>;
		}

		return undefined as unknown as T;
	}

	// Auth

	async login(password: string): Promise<void> {
		await this.request('/api/auth/login', {
			method: 'POST',
			body: JSON.stringify({ password })
		});
	}

	async logout(): Promise<void> {
		await this.request('/api/auth/logout', { method: 'POST' });
	}

	async me(): Promise<void> {
		await this.request('/api/auth/me');
	}

	// Task queries

	async getTasks(context?: string): Promise<TasksResponse> {
		const params = context ? `?context=${encodeURIComponent(context)}` : '';
		return this.request<TasksResponse>(`/api/tasks${params}`);
	}

	async getTask(id: string): Promise<Task> {
		return this.request<Task>(`/api/tasks/${encodeURIComponent(id)}`);
	}

	async getInboxTasks(context?: string): Promise<TasksResponse> {
		const params = context ? `?context=${encodeURIComponent(context)}` : '';
		return this.request<TasksResponse>(`/api/tasks/inbox${params}`);
	}

	async getWeeklyTasks(context?: string): Promise<TasksResponse> {
		const params = context ? `?context=${encodeURIComponent(context)}` : '';
		return this.request<TasksResponse>(`/api/tasks/weekly${params}`);
	}

	async getNextWeekTasks(context?: string): Promise<TasksResponse> {
		const params = context ? `?context=${encodeURIComponent(context)}` : '';
		return this.request<TasksResponse>(`/api/tasks/next-week${params}`);
	}

	async getTodayTasks(context?: string): Promise<TasksResponse> {
		const params = context ? `?context=${encodeURIComponent(context)}` : '';
		return this.request<TasksResponse>(`/api/tasks/today${params}`);
	}

	async getTomorrowTasks(context?: string): Promise<TasksResponse> {
		const params = context ? `?context=${encodeURIComponent(context)}` : '';
		return this.request<TasksResponse>(`/api/tasks/tomorrow${params}`);
	}

	async getCompletedTasks(_context?: string): Promise<TasksResponse> {
		return this.request<TasksResponse>('/api/tasks/completed');
	}

	async getBacklogTasks(context?: string): Promise<TasksResponse> {
		const params = context ? `?context=${encodeURIComponent(context)}` : '';
		return this.request<TasksResponse>(`/api/tasks/backlog${params}`);
	}

	async getProjectTasks(projectId: string): Promise<Task[]> {
		const res = await this.request<{ tasks: Task[] }>(
			`/api/tasks/project/${encodeURIComponent(projectId)}`
		);
		return res.tasks;
	}

	async getCompletedSubtasks(id: string): Promise<Task[]> {
		const res = await this.request<{ tasks: Task[] }>(
			`/api/tasks/${encodeURIComponent(id)}/completed-subtasks`
		);
		return res.tasks;
	}

	// Task mutations

	async createTask(data: CreateTaskRequest, context?: string): Promise<string> {
		const params = context ? `?context=${encodeURIComponent(context)}` : '';
		const res = await this.request<{ ok: boolean; id?: string }>(`/api/tasks${params}`, {
			method: 'POST',
			body: JSON.stringify(data)
		});
		return res?.id ?? '';
	}

	async updateTask(id: string, data: UpdateTaskRequest): Promise<void> {
		await this.request(`/api/tasks/${encodeURIComponent(id)}`, {
			method: 'PATCH',
			body: JSON.stringify(data)
		});
	}

	async batchUpdateLabels(updates: Record<string, string[]>): Promise<void> {
		await this.request('/api/tasks/batch-update-labels', {
			method: 'POST',
			body: JSON.stringify({ updates })
		});
	}

	async moveTask(id: string, parentId: string): Promise<void> {
		await this.request(`/api/tasks/${encodeURIComponent(id)}/move`, {
			method: 'POST',
			body: JSON.stringify({ parent_id: parentId })
		});
	}

	async completeTask(id: string): Promise<void> {
		await this.request(`/api/tasks/${encodeURIComponent(id)}/complete`, { method: 'POST' });
	}

	async duplicateTask(id: string): Promise<void> {
		await this.request(
			`/api/tasks/${encodeURIComponent(id)}/duplicate`,
			{ method: 'POST' }
		);
	}

	async deleteTask(id: string): Promise<void> {
		await this.request(`/api/tasks/${encodeURIComponent(id)}`, { method: 'DELETE' });
	}

	async decomposeTask(id: string, data: DecomposeTaskRequest): Promise<void> {
		await this.request(`/api/tasks/${encodeURIComponent(id)}/decompose`, {
			method: 'POST',
			body: JSON.stringify(data)
		});
	}

	// Config & state

	async getAppConfig(): Promise<AppConfig> {
		return this.request<AppConfig>('/api/config');
	}

	async patchState(update: Partial<UserState>): Promise<void> {
		logger.log('api', `patchState ${Object.keys(update).join(',')}`);
		await this.request('/api/state', {
			method: 'PATCH',
			body: JSON.stringify(update)
		});
	}

	async resetWeeklyLabel(): Promise<void> {
		await this.request('/api/tasks/reset-weekly', { method: 'POST' });
	}
}

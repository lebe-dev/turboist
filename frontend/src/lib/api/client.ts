import { goto } from '$app/navigation';
import { logger } from '$lib/stores/logger';
import type {
	AppConfig,
	CreateTaskRequest,
	Task,
	TasksResponse,
	UpdateTaskRequest,
	UserState
} from './types';

async function request<T>(path: string, options?: RequestInit): Promise<T> {
	const method = options?.method ?? 'GET';
	logger.log('api', `${method} ${path}`);

	const res = await fetch(path, {
		credentials: 'same-origin',
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

export async function login(password: string): Promise<void> {
	await request('/api/auth/login', {
		method: 'POST',
		body: JSON.stringify({ password })
	});
}

export async function logout(): Promise<void> {
	await request('/api/auth/logout', { method: 'POST' });
}

export async function me(): Promise<void> {
	await request('/api/auth/me');
}

export async function getTasks(context?: string): Promise<TasksResponse> {
	const params = context ? `?context=${encodeURIComponent(context)}` : '';
	return request<TasksResponse>(`/api/tasks${params}`);
}

export async function getTask(id: string): Promise<Task> {
	return request<Task>(`/api/tasks/${encodeURIComponent(id)}`);
}

export async function getInboxTasks(context?: string): Promise<TasksResponse> {
	const params = context ? `?context=${encodeURIComponent(context)}` : '';
	return request<TasksResponse>(`/api/tasks/inbox${params}`);
}

export async function getWeeklyTasks(context?: string): Promise<TasksResponse> {
	const params = context ? `?context=${encodeURIComponent(context)}` : '';
	return request<TasksResponse>(`/api/tasks/weekly${params}`);
}

export async function getNextWeekTasks(context?: string): Promise<TasksResponse> {
	const params = context ? `?context=${encodeURIComponent(context)}` : '';
	return request<TasksResponse>(`/api/tasks/next-week${params}`);
}

export async function getTodayTasks(context?: string): Promise<TasksResponse> {
	const params = context ? `?context=${encodeURIComponent(context)}` : '';
	return request<TasksResponse>(`/api/tasks/today${params}`);
}

export async function getTomorrowTasks(context?: string): Promise<TasksResponse> {
	const params = context ? `?context=${encodeURIComponent(context)}` : '';
	return request<TasksResponse>(`/api/tasks/tomorrow${params}`);
}

export async function getCompletedTasks(_context?: string): Promise<TasksResponse> {
	return request<TasksResponse>('/api/tasks/completed');
}

export async function getBacklogTasks(context?: string): Promise<TasksResponse> {
	const params = context ? `?context=${encodeURIComponent(context)}` : '';
	return request<TasksResponse>(`/api/tasks/backlog${params}`);
}

export async function resetWeeklyLabel(): Promise<{ updated: number }> {
	return request<{ updated: number }>('/api/tasks/reset-weekly', { method: 'POST' });
}

export async function getAppConfig(): Promise<AppConfig> {
	return request<AppConfig>('/api/config');
}

export async function patchState(update: Partial<UserState>): Promise<void> {
	logger.log('api', `patchState ${Object.keys(update).join(',')}`);
	await request('/api/state', {
		method: 'PATCH',
		body: JSON.stringify(update)
	});
}

export async function createTask(data: CreateTaskRequest, context?: string): Promise<void> {
	const params = context ? `?context=${encodeURIComponent(context)}` : '';
	await request(`/api/tasks${params}`, {
		method: 'POST',
		body: JSON.stringify(data)
	});
}

export async function updateTask(id: string, data: UpdateTaskRequest): Promise<void> {
	await request(`/api/tasks/${encodeURIComponent(id)}`, {
		method: 'PATCH',
		body: JSON.stringify(data)
	});
}

export async function completeTask(id: string): Promise<void> {
	await request(`/api/tasks/${encodeURIComponent(id)}/complete`, { method: 'POST' });
}

export async function duplicateTask(id: string): Promise<string> {
	const res = await request<{ task_id: string }>(
		`/api/tasks/${encodeURIComponent(id)}/duplicate`,
		{ method: 'POST' }
	);
	return res.task_id;
}

export async function deleteTask(id: string): Promise<void> {
	await request(`/api/tasks/${encodeURIComponent(id)}`, { method: 'DELETE' });
}

export async function getCompletedSubtasks(id: string): Promise<Task[]> {
	const res = await request<{ tasks: Task[] }>(
		`/api/tasks/${encodeURIComponent(id)}/completed-subtasks`
	);
	return res.tasks;
}

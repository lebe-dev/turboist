import { goto } from '$app/navigation';
import type { Config, Context, Label, Project, Task, TasksResponse } from './types';

async function request<T>(path: string, options?: RequestInit): Promise<T> {
	const res = await fetch(path, {
		credentials: 'same-origin',
		...options,
		headers: {
			'Content-Type': 'application/json',
			...options?.headers
		}
	});

	if (res.status === 401) {
		goto('/login');
		throw new Error('Unauthorized');
	}

	if (!res.ok) {
		const text = await res.text().catch(() => res.statusText);
		throw new Error(`${res.status}: ${text}`);
	}

	return res.json() as Promise<T>;
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

export async function getWeeklyTasks(context?: string): Promise<TasksResponse> {
	const params = context ? `?context=${encodeURIComponent(context)}` : '';
	return request<TasksResponse>(`/api/tasks/weekly${params}`);
}

export async function getNextWeekTasks(context?: string): Promise<TasksResponse> {
	const params = context ? `?context=${encodeURIComponent(context)}` : '';
	return request<TasksResponse>(`/api/tasks/next-week${params}`);
}

export async function getProjects(): Promise<Project[]> {
	return request<Project[]>('/api/projects');
}

export async function getLabels(): Promise<Label[]> {
	return request<Label[]>('/api/labels');
}

export async function getContexts(): Promise<Context[]> {
	return request<Context[]>('/api/contexts');
}

export async function getConfig(): Promise<Config> {
	return request<Config>('/api/config');
}

export async function completeTask(id: string): Promise<void> {
	await request(`/api/tasks/${encodeURIComponent(id)}/complete`, { method: 'POST' });
}

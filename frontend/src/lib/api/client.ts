import { backend } from './backend';
import type { AppConfig, CreateTaskRequest, DecomposeTaskRequest, Task, TasksResponse, UpdateTaskRequest, UserState } from './types';

// Thin wrapper layer: each function delegates to the active BackendConnector.
// All existing imports from '$lib/api/client' continue to work unchanged.

export async function login(password: string): Promise<void> {
	return backend.login(password);
}

export async function logout(): Promise<void> {
	return backend.logout();
}

export async function me(): Promise<void> {
	return backend.me();
}

export async function getTasks(context?: string): Promise<TasksResponse> {
	return backend.getTasks(context);
}

export async function getTask(id: string): Promise<Task> {
	return backend.getTask(id);
}

export async function getInboxTasks(context?: string): Promise<TasksResponse> {
	return backend.getInboxTasks(context);
}

export async function getWeeklyTasks(context?: string): Promise<TasksResponse> {
	return backend.getWeeklyTasks(context);
}

export async function getNextWeekTasks(context?: string): Promise<TasksResponse> {
	return backend.getNextWeekTasks(context);
}

export async function getTodayTasks(context?: string): Promise<TasksResponse> {
	return backend.getTodayTasks(context);
}

export async function getTomorrowTasks(context?: string): Promise<TasksResponse> {
	return backend.getTomorrowTasks(context);
}

export async function getCompletedTasks(_context?: string): Promise<TasksResponse> {
	return backend.getCompletedTasks(_context);
}

export async function getBacklogTasks(context?: string): Promise<TasksResponse> {
	return backend.getBacklogTasks(context);
}

export async function resetWeeklyLabel(): Promise<void> {
	return backend.resetWeeklyLabel();
}

export async function getAppConfig(): Promise<AppConfig> {
	return backend.getAppConfig();
}

export async function patchState(update: Partial<UserState>): Promise<void> {
	return backend.patchState(update);
}

export async function createTask(data: CreateTaskRequest, context?: string, tempId?: string): Promise<string> {
	return backend.createTask(data, context, tempId);
}

export async function updateTask(id: string, data: UpdateTaskRequest): Promise<void> {
	return backend.updateTask(id, data);
}

export async function batchUpdateLabels(updates: Record<string, string[]>): Promise<void> {
	return backend.batchUpdateLabels(updates);
}

export async function moveTask(id: string, parentId: string): Promise<void> {
	return backend.moveTask(id, parentId);
}

export async function completeTask(id: string): Promise<void> {
	return backend.completeTask(id);
}

export async function duplicateTask(id: string): Promise<void> {
	return backend.duplicateTask(id);
}

export async function deleteTask(id: string): Promise<void> {
	return backend.deleteTask(id);
}

export async function decomposeTask(id: string, data: DecomposeTaskRequest): Promise<void> {
	return backend.decomposeTask(id, data);
}

export async function getProjectTasks(projectId: string): Promise<Task[]> {
	return backend.getProjectTasks(projectId);
}

export async function getCompletedSubtasks(id: string): Promise<Task[]> {
	return backend.getCompletedSubtasks(id);
}

export async function resetCache(): Promise<void> {
	const res = await fetch('/api/cache/reset', { method: 'POST' });
	if (!res.ok) throw new Error(`resetCache: ${res.status}`);
}

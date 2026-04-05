import { getBackend } from './backend';
import type { AppConfig, CreateTaskRequest, DecomposeTaskRequest, Task, TasksResponse, UpdateTaskRequest, UserState } from './types';

// Thin wrapper layer: each function delegates to the active BackendConnector.
// All existing imports from '$lib/api/client' continue to work unchanged.

export async function login(password: string): Promise<void> {
	return getBackend().login(password);
}

export async function logout(): Promise<void> {
	return getBackend().logout();
}

export async function me(): Promise<void> {
	return getBackend().me();
}

export async function getTasks(context?: string): Promise<TasksResponse> {
	return getBackend().getTasks(context);
}

export async function getTask(id: string): Promise<Task> {
	return getBackend().getTask(id);
}

export async function getInboxTasks(context?: string): Promise<TasksResponse> {
	return getBackend().getInboxTasks(context);
}

export async function getWeeklyTasks(context?: string): Promise<TasksResponse> {
	return getBackend().getWeeklyTasks(context);
}

export async function getNextWeekTasks(context?: string): Promise<TasksResponse> {
	return getBackend().getNextWeekTasks(context);
}

export async function getTodayTasks(context?: string): Promise<TasksResponse> {
	return getBackend().getTodayTasks(context);
}

export async function getTomorrowTasks(context?: string): Promise<TasksResponse> {
	return getBackend().getTomorrowTasks(context);
}

export async function getCompletedTasks(_context?: string): Promise<TasksResponse> {
	return getBackend().getCompletedTasks(_context);
}

export async function getBacklogTasks(context?: string): Promise<TasksResponse> {
	return getBackend().getBacklogTasks(context);
}

export async function resetWeeklyLabel(): Promise<void> {
	return getBackend().resetWeeklyLabel();
}

export async function getAppConfig(): Promise<AppConfig> {
	return getBackend().getAppConfig();
}

export async function patchState(update: Partial<UserState>): Promise<void> {
	return getBackend().patchState(update);
}

export async function createTask(data: CreateTaskRequest, context?: string, tempId?: string): Promise<string> {
	return getBackend().createTask(data, context, tempId);
}

export async function updateTask(id: string, data: UpdateTaskRequest): Promise<void> {
	return getBackend().updateTask(id, data);
}

export async function batchUpdateLabels(updates: Record<string, string[]>): Promise<void> {
	return getBackend().batchUpdateLabels(updates);
}

export async function moveTask(id: string, parentId: string): Promise<void> {
	return getBackend().moveTask(id, parentId);
}

export async function completeTask(id: string): Promise<void> {
	return getBackend().completeTask(id);
}

export async function duplicateTask(id: string): Promise<void> {
	return getBackend().duplicateTask(id);
}

export async function deleteTask(id: string): Promise<void> {
	return getBackend().deleteTask(id);
}

export async function decomposeTask(id: string, data: DecomposeTaskRequest): Promise<void> {
	return getBackend().decomposeTask(id, data);
}

export async function getProjectTasks(projectId: string): Promise<Task[]> {
	return getBackend().getProjectTasks(projectId);
}

export async function getCompletedSubtasks(id: string): Promise<Task[]> {
	return getBackend().getCompletedSubtasks(id);
}

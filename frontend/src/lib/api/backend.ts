import type {
	AppConfig,
	CreateTaskRequest,
	DecomposeTaskRequest,
	Task,
	TasksResponse,
	UpdateTaskRequest,
	UserState
} from './types';
import { DefaultBackendConnector } from './default-backend';

// Interface that all backend connectors must implement.
// Mirrors the public API surface of client.ts.
export interface BackendConnector {
	// Auth
	login(password: string): Promise<void>;
	logout(): Promise<void>;
	me(): Promise<void>;

	// Task queries
	getTasks(context?: string): Promise<TasksResponse>;
	getTask(id: string): Promise<Task>;
	getInboxTasks(context?: string): Promise<TasksResponse>;
	getWeeklyTasks(context?: string): Promise<TasksResponse>;
	getNextWeekTasks(context?: string): Promise<TasksResponse>;
	getTodayTasks(context?: string): Promise<TasksResponse>;
	getTomorrowTasks(context?: string): Promise<TasksResponse>;
	getCompletedTasks(context?: string): Promise<TasksResponse>;
	getBacklogTasks(context?: string): Promise<TasksResponse>;
	getProjectTasks(projectId: string): Promise<Task[]>;
	getCompletedSubtasks(id: string): Promise<Task[]>;

	// Task mutations
	createTask(data: CreateTaskRequest, context?: string, tempId?: string): Promise<string>;
	updateTask(id: string, data: UpdateTaskRequest): Promise<void>;
	batchUpdateLabels(updates: Record<string, string[]>): Promise<void>;
	moveTask(id: string, parentId: string): Promise<void>;
	completeTask(id: string): Promise<void>;
	duplicateTask(id: string): Promise<void>;
	deleteTask(id: string): Promise<void>;
	decomposeTask(id: string, data: DecomposeTaskRequest): Promise<void>;

	// Config & state
	getAppConfig(): Promise<AppConfig>;
	patchState(update: Partial<UserState>): Promise<void>;
	resetWeeklyLabel(): Promise<void>;
}

// Start with DefaultBackendConnector so auth works before appStore.init()
let _backend: BackendConnector = new DefaultBackendConnector();

export function getBackend(): BackendConnector {
	return _backend;
}

export function setBackend(b: BackendConnector): void {
	_backend = b;
}

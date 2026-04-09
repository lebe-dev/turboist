// In-memory mock backend for tests.

import type { BackendConnector } from './backend';
import type {
	AppConfig,
	CreateTaskRequest,
	CreateTroikiTaskRequest,
	DecomposeTaskRequest,
	Meta,
	Task,
	TasksResponse,
	TroikiCompletedState,
	TroikiState,
	UpdateTaskRequest,
	UserState
} from './types';

interface Call {
	method: string;
	args: unknown[];
}

const defaultMeta: Meta = {
	context: '',
	weekly_limit: 0,
	weekly_count: 0,
	backlog_limit: 0,
	backlog_count: 0
};

function emptyTasksResponse(tasks: Task[]): TasksResponse {
	return { tasks, meta: { ...defaultMeta } };
}

export class MockBackendConnector implements BackendConnector {
	// Recorded calls for assertions.
	calls: Call[] = [];

	// Configurable return values.
	tasks: Task[] = [];
	config: AppConfig | null = null;

	// Clear all recorded calls and return values.
	reset(): void {
		this.calls = [];
		this.tasks = [];
		this.config = null;
	}

	private record(method: string, args: unknown[]): void {
		this.calls.push({ method, args });
	}

	// Auth

	async login(password: string): Promise<void> {
		this.record('login', [password]);
	}

	async logout(): Promise<void> {
		this.record('logout', []);
	}

	async me(): Promise<void> {
		this.record('me', []);
	}

	// Task queries

	async getTasks(context?: string): Promise<TasksResponse> {
		this.record('getTasks', [context]);
		return emptyTasksResponse(this.tasks);
	}

	async getTask(id: string): Promise<Task> {
		this.record('getTask', [id]);
		const found = this.tasks.find((t) => t.id === id);
		if (found) return found;
		throw new Error(`task not found: ${id}`);
	}

	async getInboxTasks(context?: string): Promise<TasksResponse> {
		this.record('getInboxTasks', [context]);
		return emptyTasksResponse(this.tasks);
	}

	async getWeeklyTasks(context?: string): Promise<TasksResponse> {
		this.record('getWeeklyTasks', [context]);
		return emptyTasksResponse(this.tasks);
	}

	async getNextWeekTasks(context?: string): Promise<TasksResponse> {
		this.record('getNextWeekTasks', [context]);
		return emptyTasksResponse(this.tasks);
	}

	async getTodayTasks(context?: string): Promise<TasksResponse> {
		this.record('getTodayTasks', [context]);
		return emptyTasksResponse(this.tasks);
	}

	async getTomorrowTasks(context?: string): Promise<TasksResponse> {
		this.record('getTomorrowTasks', [context]);
		return emptyTasksResponse(this.tasks);
	}

	async getCompletedTasks(context?: string): Promise<TasksResponse> {
		this.record('getCompletedTasks', [context]);
		return emptyTasksResponse(this.tasks);
	}

	async getBacklogTasks(context?: string): Promise<TasksResponse> {
		this.record('getBacklogTasks', [context]);
		return emptyTasksResponse(this.tasks);
	}

	async getProjectTasks(projectId: string): Promise<Task[]> {
		this.record('getProjectTasks', [projectId]);
		return this.tasks.filter((t) => t.project_id === projectId);
	}

	async getCompletedSubtasks(id: string): Promise<Task[]> {
		this.record('getCompletedSubtasks', [id]);
		return [];
	}

	// Task mutations

	async createTask(data: CreateTaskRequest, context?: string): Promise<string> {
		this.record('createTask', [data, context]);
		return `mock-${Date.now()}`;
	}

	async updateTask(id: string, data: UpdateTaskRequest): Promise<void> {
		this.record('updateTask', [id, data]);
	}

	async batchUpdateLabels(updates: Record<string, string[]>): Promise<void> {
		this.record('batchUpdateLabels', [updates]);
	}

	async completeTask(id: string): Promise<void> {
		this.record('completeTask', [id]);
	}

	async moveTask(id: string, parentId: string): Promise<void> {
		this.record('moveTask', [id, parentId]);
	}

	async duplicateTask(id: string): Promise<void> {
		this.record('duplicateTask', [id]);
	}

	async deleteTask(id: string): Promise<void> {
		this.record('deleteTask', [id]);
	}

	async decomposeTask(id: string, data: DecomposeTaskRequest): Promise<void> {
		this.record('decomposeTask', [id, data]);
	}

	// Troiki

	async getTroikiState(): Promise<TroikiState> {
		this.record('getTroikiState', []);
		return { project_id: '', sections: [] };
	}

	async getTroikiCompleted(): Promise<TroikiCompletedState> {
		this.record('getTroikiCompleted', []);
		return { sections: [] };
	}

	async createTroikiTask(data: CreateTroikiTaskRequest): Promise<string> {
		this.record('createTroikiTask', [data]);
		return `mock-${Date.now()}`;
	}

	// Config & state

	async getAppConfig(): Promise<AppConfig> {
		this.record('getAppConfig', []);
		if (this.config) return this.config;
		return {
			settings: {
				poll_interval: 30,
				sync_interval: 60,
				timezone: 'UTC',
				weekly_label: '',
				backlog_label: '',
				project_label: '',
				projects_label: '',
				weekly_limit: 0,
				backlog_limit: 0,
				completed_days: 7,
				max_pinned: 5,
				last_synced_at: null,
				day_parts: [],
				max_day_part_note_length: 200,
				inbox_project_id: '',
				inbox_limit: 10,
				inbox_overflow_task_content: 'Разобрать Входящие'
			},
			contexts: [],
			projects: [],
			labels: [],
			label_configs: [],
			auto_labels: [],
			quick_capture: null,
			project_tasks: [],
			label_project_map: { enabled: false, mappings: [] },
			auto_remove: { rules: [], paused: false },
			troiki: { enabled: false },
			state: {
				pinned_tasks: [],
				active_context_id: '',
				active_view: 'all',
				collapsed_ids: [],
				sidebar_collapsed: false,
				planning_open: false,
				day_part_notes: {},
				locale: '',
				all_filters: null
			}
		};
	}

	async patchState(update: Partial<UserState>): Promise<void> {
		this.record('patchState', [update]);
	}

	async resetWeeklyLabel(): Promise<void> {
		this.record('resetWeeklyLabel', []);
	}
}

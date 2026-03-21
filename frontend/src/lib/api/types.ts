export interface Due {
	date: string;
	recurring: boolean;
}

export interface Task {
	id: string;
	content: string;
	description: string;
	project_id: string;
	section_id: string | null;
	parent_id: string | null;
	labels: string[];
	priority: number;
	due: Due | null;
	sub_task_count: number;
	completed_sub_task_count: number;
	completed_at: string | null;
	added_at: string;
	is_project_task: boolean;
	children: Task[];
}

export interface Meta {
	context: string;
	weekly_limit: number;
	weekly_count: number;
	backlog_limit: number;
	backlog_count: number;
	last_synced_at?: string;
}

export interface TasksResponse {
	tasks: Task[];
	meta: Meta;
}

export interface Section {
	id: string;
	name: string;
	project_id: string;
	order: number;
}

export interface Project {
	id: string;
	name: string;
	color: string;
	sections: Section[];
}

export interface Label {
	id: string;
	name: string;
	color: string;
	order: number;
}

export interface ContextFilters {
	projects: string[];
	sections: string[];
	labels: string[];
}

export interface Context {
	id: string;
	display_name: string;
	color?: string;
	inherit_labels: boolean;
	filters: ContextFilters;
}

export interface CreateTaskRequest {
	content: string;
	description: string;
	labels: string[];
	priority: number;
	parent_id?: string;
	due_date?: string;
}

export interface UpdateTaskRequest {
	content?: string;
	description?: string;
	labels?: string[];
	priority?: number;
	due_date?: string;
}

export interface DayPart {
	label: string;
	start: number; // hour 0-23
	end: number; // hour 0-23
}

export interface PinnedTask {
	id: string;
	content: string;
}

export type View = 'all' | 'inbox' | 'today' | 'tomorrow' | 'weekly' | 'backlog' | 'completed';

export interface UserState {
	pinned_tasks: PinnedTask[];
	active_context_id: string;
	active_view: View;
	collapsed_ids: string[];
	sidebar_collapsed: boolean;
	planning_open: boolean;
}

export interface Settings {
	poll_interval: number; // seconds
	sync_interval: number; // seconds — how often the frontend flushes queued mutations
	timezone: string; // IANA timezone (e.g. "Europe/Moscow")
	weekly_label: string;
	backlog_label: string;
	weekly_limit: number;
	backlog_limit: number;
	completed_days: number;
	max_pinned: number;
	last_synced_at: string | null; // ISO 8601
	day_parts: DayPart[];
}

export interface QuickCaptureConfig {
	parent_task_id: string;
}

export interface LabelConfig {
	name: string;
	inherit_to_subtasks: boolean;
}

export interface AppConfig {
	settings: Settings;
	contexts: Context[];
	projects: Project[];
	labels: Label[];
	label_configs: LabelConfig[];
	quick_capture: QuickCaptureConfig | null;
	state: UserState;
}

// Legacy alias for backward compatibility within tasks/planning stores
export type Config = Settings;

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
	is_project_task: boolean;
	children: Task[];
}

export interface Meta {
	context: string;
	weekly_limit: number;
	weekly_count: number;
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
	filters: ContextFilters;
}

export interface CreateTaskRequest {
	content: string;
	description: string;
	labels: string[];
	priority: number;
}

export interface Config {
	poll_interval: number; // seconds
	weekly_limit: number;
	last_synced_at: string | null; // ISO 8601
}

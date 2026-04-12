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
	postpone_count: number;
	expires_at?: string;
	children: Task[];
}

export interface Meta {
	context: string;
	weekly_limit: number;
	weekly_count: number;
	backlog_limit: number;
	backlog_count: number;
	inbox_count?: number;
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
	project_id?: string;
	section_id?: string;
}

export interface UpdateTaskRequest {
	content?: string;
	description?: string;
	labels?: string[];
	priority?: number;
	due_date?: string;
	due_string?: string;
}

export interface DecomposeTaskRequest {
	tasks: string[];
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
	priority?: number;
}

export type View = 'all' | 'inbox' | 'today' | 'tomorrow' | 'weekly' | 'backlog' | 'completed';

export interface AllFiltersState {
	selected_priorities: number[];
	selected_labels: string[];
	links_only: boolean;
	filters_expanded: boolean;
}

export interface UserState {
	pinned_tasks: PinnedTask[];
	active_context_id: string;
	active_view: View;
	collapsed_ids: string[];
	sidebar_collapsed: boolean;
	planning_open: boolean;
	day_part_notes: Record<string, string>;
	locale: string;
	all_filters: AllFiltersState | null;
	banner_text: string;
	banner_dismissed_text: string;
	constraint_pool?: string[];
}

export interface Settings {
	poll_interval: number; // seconds
	sync_interval: number; // seconds — how often the frontend flushes queued mutations
	timezone: string; // IANA timezone (e.g. "Europe/Moscow")
	weekly_label: string;
	backlog_label: string;
	project_label: string;
	projects_label: string;
	weekly_limit: number;
	backlog_limit: number;
	completed_days: number;
	max_pinned: number;
	last_synced_at: string | null; // ISO 8601
	day_parts: DayPart[];
	max_day_part_note_length: number;
	inbox_project_id: string;
	inbox_limit: number;
	inbox_overflow_task_content: string;
}

export interface ProjectTask {
	id: string;
	content: string;
}

export interface QuickCaptureConfig {
	parent_task_id: string;
}

export interface LabelConfig {
	name: string;
	inherit_to_subtasks: boolean;
}

export interface AutoLabelMapping {
	mask: string;
	label: string;
	ignore_case: boolean;
}

export interface LabelProjectMapping {
	label: string;
	project: string;
	section?: string;
}

export interface LabelProjectMap {
	enabled: boolean;
	mappings: LabelProjectMapping[];
}

export interface AutoRemoveRule {
	label: string;
	ttl: number; // seconds
}

export interface AutoRemoveStatus {
	rules: AutoRemoveRule[];
	paused: boolean;
}

export interface AppConfig {
	settings: Settings;
	contexts: Context[];
	projects: Project[];
	labels: Label[];
	label_configs: LabelConfig[];
	auto_labels: AutoLabelMapping[];
	quick_capture: QuickCaptureConfig | null;
	project_tasks: ProjectTask[];
	label_project_map: LabelProjectMap;
	auto_remove: AutoRemoveStatus;
	troiki: TroikiConfig;
	constraints: ConstraintsConfig;
	state: UserState;
}

export type SectionClass = 'important' | 'medium' | 'rest';

export interface TroikiSectionState {
	class: SectionClass;
	section_id: string;
	name: string;
	tasks: Task[];
	root_count: number;
	max_tasks: number;
	capacity: number;
	can_add: boolean;
}

export interface TroikiState {
	project_id: string;
	sections: TroikiSectionState[];
}

export interface TroikiConfig {
	enabled: boolean;
	project_id?: string;
	project_name?: string;
	max_tasks_per_section?: number;
}

export interface CreateTroikiTaskRequest {
	section_class: SectionClass;
	content: string;
	description: string;
}

export interface TroikiCompletedSection {
	class: SectionClass;
	tasks: Task[];
}

export interface TroikiCompletedState {
	sections: TroikiCompletedSection[];
}

export interface LabelBlockStatus {
	label: string;
	remaining_seconds: number;
}

export interface DayPartCap {
	label: string;
	max_tasks: number;
}

export interface ConstraintsConfig {
	enabled: boolean;
	label_blocks: LabelBlockStatus[];
	day_part_caps: DayPartCap[];
	priority_floor: number;
	postpone_budget: number;
	postpone_budget_used: number;
}

export interface DailyConstraintsResponse {
	needs_selection: boolean;
	items: string[];
	rerolls_used: number;
	max_rerolls: number;
	pool_size: number;
	confirmed: boolean;
}

// Legacy alias for backward compatibility within tasks/planning stores
export type Config = Settings;

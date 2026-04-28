// DTO types mirroring backend (camelCase JSON, ISO-8601 UTC strings).

export type Priority = 'high' | 'medium' | 'low' | 'no-priority';
export type TaskStatus = 'open' | 'completed' | 'cancelled';
export type ProjectStatus = 'open' | 'completed' | 'archived' | 'cancelled';
export type DayPart = 'none' | 'morning' | 'afternoon' | 'evening';
export type PlanState = 'none' | 'week' | 'backlog';
export type ClientKind = 'web' | 'ios' | 'cli';

// Color palette is open-ended on the backend; alias for clarity.
export type ColorToken = string;

export interface User {
	id: number;
	username: string;
}

export interface AuthSetupRequiredResponse {
	required: boolean;
}

export interface AuthLoginResponse {
	access: string;
	refresh: string;
	user: User;
}

export interface AuthRefreshResponse {
	access: string;
	refresh: string;
}

export interface Label {
	id: number;
	name: string;
	color: ColorToken;
	isFavourite: boolean;
	createdAt: string;
	updatedAt: string;
}

export interface Context {
	id: number;
	name: string;
	color: ColorToken;
	isFavourite: boolean;
	createdAt: string;
	updatedAt: string;
}

export interface Project {
	id: number;
	contextId: number;
	title: string;
	description: string;
	color: ColorToken;
	status: ProjectStatus;
	isPinned: boolean;
	pinnedAt: string | null;
	labels: Label[];
	createdAt: string;
	updatedAt: string;
}

export interface ProjectSection {
	id: number;
	projectId: number;
	title: string;
	createdAt: string;
	updatedAt: string;
}

export interface Task {
	id: number;
	title: string;
	description: string;

	inboxId: number | null;
	contextId: number | null;
	projectId: number | null;
	sectionId: number | null;
	parentId: number | null;

	priority: Priority;
	status: TaskStatus;

	dueAt: string | null;
	dueHasTime: boolean;
	deadlineAt: string | null;
	deadlineHasTime: boolean;

	dayPart: DayPart;
	planState: PlanState;

	isPinned: boolean;
	pinnedAt: string | null;

	recurrenceRule: string | null;

	labels: Label[];

	url: string;
	createdAt: string;
	updatedAt: string;
}

export interface Page<T> {
	items: T[];
	total: number;
	limit: number;
	offset: number;
}

export interface ViewList<T> {
	items: T[];
	total: number;
}

export interface InboxResponse {
	count: number;
	warnThresholdExceeded: boolean;
	tasks: Task[];
}

export interface SearchResponse {
	tasks?: ViewList<Task>;
	projects?: ViewList<Project>;
}

export interface ConfigResponse {
	timezone: string;
	maxPinned: number;
	weekly: { limit: number };
	backlog: { limit: number };
	inbox: {
		warnThreshold: number;
		overflowTask: { title: string; priority: Priority };
	};
	dayParts: {
		morning: { start: number; end: number };
		afternoon: { start: number; end: number };
		evening: { start: number; end: number };
	};
	autoLabels: Array<{ mask: string; label: string; ignoreCase: boolean }>;
}

// Request payloads

export interface ContextInput {
	name?: string;
	color?: ColorToken;
	isFavourite?: boolean;
}

export interface ProjectInput {
	title?: string;
	description?: string | null;
	color?: ColorToken;
	contextId?: number;
	labels?: string[];
}

export interface SectionInput {
	title?: string;
}

export interface LabelInput {
	name?: string;
	color?: ColorToken;
	isFavourite?: boolean;
}

export interface TaskInput {
	title?: string;
	description?: string | null;
	priority?: Priority;
	dueAt?: string | null;
	dueHasTime?: boolean;
	deadlineAt?: string | null;
	deadlineHasTime?: boolean;
	dayPart?: DayPart;
	planState?: PlanState;
	recurrenceRule?: string | null;
	labels?: string[];
	removedAutoLabels?: string[];
}

export type TaskMoveInput =
	| { inboxId: number }
	| { contextId: number; projectId?: number; sectionId?: number }
	| { parentId: number };

export interface TaskPlanInput {
	state: PlanState;
}

export interface BulkResult {
	succeeded: number[];
	failed: Array<{ id: number; error: { code: string; message: string } }>;
}

export interface ListQuery {
	limit?: number;
	offset?: number;
}

export interface TasksQuery extends ListQuery {
	status?: TaskStatus;
	priority?: Priority;
	labelId?: number;
	q?: string;
}

export interface ViewQuery {
	contextId?: number;
	projectId?: number;
	labelId?: number;
	priority?: Priority;
}

export interface ViewPageQuery extends ViewQuery, ListQuery {}

export interface ProjectsQuery extends ListQuery {
	contextId?: number;
	status?: ProjectStatus;
}

export interface SearchQuery extends ListQuery {
	q: string;
	type?: 'tasks' | 'projects' | 'all';
}

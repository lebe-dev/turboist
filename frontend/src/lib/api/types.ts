// DTO types mirroring backend (camelCase JSON, ISO-8601 UTC strings).

export type Priority = 'high' | 'medium' | 'low' | 'no-priority';
export type TaskStatus = 'open' | 'completed' | 'cancelled';
export type ProjectStatus = 'open' | 'completed' | 'archived' | 'cancelled';
export type ProjectType = 'generic' | 'software';
export type DayPart = 'none' | 'morning' | 'afternoon' | 'evening';
// BannerDayPart scopes the Today banner to one day phase; '' means all day.
export type BannerDayPart = '' | 'morning' | 'afternoon' | 'evening';
export type PlanState = 'none' | 'week' | 'backlog';
export type ClientKind = 'web' | 'ios' | 'android' | 'cli';
export type TroikiCategory = 'important' | 'medium' | 'rest';

// Color palette is open-ended on the backend; alias for clarity.
export type ColorToken = string;

export interface User {
	id: number;
	username: string;
	totpEnabled: boolean;
}

export interface TOTPSetupResponse {
	secret: string;
	otpauthUrl: string;
	qrPngBase64: string;
}

export interface TOTPConfirmResponse {
	recoveryCodes: string[];
}

export interface AuthLoginSuccessResponse {
	access: string;
	refresh: string;
	user: User;
}

export interface AuthOTPChallengeResponse {
	otpRequired: true;
	ticket: string;
}

export type AuthLoginResponse = AuthLoginSuccessResponse | AuthOTPChallengeResponse;

export interface AuthOTPLoginRequest {
	ticket: string;
	code: string;
}

export interface AuthRefreshResponse {
	access: string;
	refresh: string;
	/**
	 * Present since v1.15 so boot does not need a follow-up `GET /auth/me`.
	 * Optional on the type: a freshly-deployed bundle can still be talking to a
	 * not-yet-restarted older server, and `AuthStore.bootstrap` falls back to
	 * `/auth/me` when it is missing.
	 */
	user?: User;
}

export interface Label {
	id: number;
	name: string;
	color: ColorToken;
	isFavourite: boolean;
	isPrivate: boolean;
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
	projectType: ProjectType;
	isPinned: boolean;
	pinnedAt: string | null;
	isPrivate: boolean;
	labels: Label[];
	troikiCategory: TroikiCategory | null;
	createdAt: string;
	updatedAt: string;
}

export interface ProjectSection {
	id: number;
	projectId: number;
	title: string;
	position: number;
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
	isPrivate: boolean;
	isComplex: boolean;
	completedAt: string | null;

	recurrenceRule: string | null;
	sourceTaskId: number | null;

	postponeCount: number;

	labels: Label[];

	url: string;
	createdAt: string;
	updatedAt: string;

	// How many still-open tasks block this one, and how many relations touch it in
	// total. Present on EVERY read path — single get, all list views, the /config
	// aggregate's pinnedTasks — because `blockedByCount > 0` is what disables the
	// checkbox, so a list without them would offer to complete a blocked task.
	blockedByCount: number;
	relationCount: number;

	// Populated only by GET /tasks/:id?subtasks=true so the task detail page
	// can fetch parent + children in one round-trip. Omitted otherwise.
	subtasks?: Page<Task>;

	// Populated only by GET /tasks/:id?relations=true and by the relation
	// mutations, which answer with the updated task. Omitted on list paths.
	relations?: TaskRelation[];

	// Populated only by the single-task GET handler when parentId is set.
	parentTitle?: string;
}

/** `related` is symmetric and informational; `blocks` is enforced on completion. */
export type RelationType = 'related' | 'blocks';

/**
 * Direction of a `blocks` relation as seen from the task it was loaded for:
 * `outgoing` = this task blocks the peer, `incoming` = the peer blocks this task.
 * Meaningless for `related`, which is symmetric.
 */
export type RelationDirection = 'outgoing' | 'incoming';

export interface TaskRelation {
	id: number;
	type: RelationType;
	direction: RelationDirection;
	/** The peer end — the task at the other side of the relation. */
	task: Task;
	createdAt: string;
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

export interface TodayBundle {
	today: ViewList<Task>;
	overdue: ViewList<Task>;
	completedToday: ViewList<Task>;
}

// ProjectBundle is the single-round-trip payload for the project page: the
// project, its sections and all its tasks (subtasks included, flattened — the
// client re-parents them via buildTree). Mirrors the backend
// projectBundleResponse behind GET /api/v1/projects/:id/bundle.
export interface ProjectBundle {
	project: Project;
	sections: Page<ProjectSection>;
	tasks: Page<Task>;
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

export interface PlanStatsResponse {
	week: number;
	backlog: number;
}

export interface SidebarStatsResponse {
	planStats: PlanStatsResponse;
	inboxStats: { count: number; warnThresholdExceeded: boolean };
	pinned: ViewList<Task>;
}

// WeekSummaryResponse backs the /week/summary review page. `completed` carries
// every task completed in the current week (incl. subtasks and recurrence
// snapshots); the page derives the by-priority/project/context breakdowns from
// it client-side. `stats.completedCount` is the authoritative total.
// WeekSummaryTroikiSlot is the per-category progress for one Troiki slot:
// how full it is (projects/capacity), how many open tasks remain, and how many
// of its tasks were completed during the current week.
export interface WeekSummaryTroikiSlot {
	category: TroikiCategory;
	capacity: number;
	projects: number;
	open: number;
	completed: number;
}

// WeekSummaryTroiki is present only when the Troiki system is enabled; `slots`
// is ordered important → medium → rest.
export interface WeekSummaryTroiki {
	started: boolean;
	slots: WeekSummaryTroikiSlot[];
}

export interface WeekSummaryResponse {
	range: { start: string; end: string };
	stats: {
		completedCount: number;
		plannedOpen: number;
		overdue: number;
	};
	completed: Task[];
	troiki: WeekSummaryTroiki | null;
}

export interface TroikiProject extends Project {
	tasks: Task[];
}

export interface TroikiSlot {
	capacity: number;
	projects: TroikiProject[];
}

export interface TroikiViewResponse {
	important: TroikiSlot;
	medium: TroikiSlot;
	rest: TroikiSlot;
	started: boolean;
}

export interface UserState {
	activeContextId?: number | null;
}

export interface UserSettings {
	weeklyUnplannedExcludedLabelIds: number[];
	bugLabelIds: number[];
	locale: string;
	publicView: boolean;
	bannerText: string;
	bannerPublished: boolean;
	bannerDayPart: BannerDayPart;
	calendarEnabled: boolean;
	calendarHidePastEvents: boolean;
	troikiEnabled: boolean;
}

export type HarpoonKind = 'task' | 'project';

export interface HarpoonSlot {
	kind: HarpoonKind;
	id: number;
	title: string;
}

export interface HarpoonState {
	slots: HarpoonSlot[];
}

export interface CalendarAccount {
	id: number;
	provider: string;
	email: string;
	displayName: string;
	createdAt: string;
	updatedAt: string;
}

export interface CalendarSource {
	id: number;
	accountId: number;
	provider: string;
	externalId: string;
	summary: string;
	color: string;
	selected: boolean;
	isPrimary: boolean;
}

export interface CalendarSettingsResponse {
	enabled: boolean;
	googleConfigured: boolean;
	googleClientIdConfigured: boolean;
	googleClientSecretConfigured: boolean;
	accounts: CalendarAccount[];
	sources: CalendarSource[];
}

export interface CalendarEvent {
	id: string;
	sourceId: number;
	sourceName: string;
	sourceColor: string;
	provider: string;
	externalId: string;
	title: string;
	description?: string;
	location: string;
	start: string;
	end: string;
	startDate?: string;
	endDate?: string;
	allDay: boolean;
	htmlLink: string;
}

export interface CalendarEventsResponse {
	items: CalendarEvent[];
}

export interface APIToken {
	id: number;
	name: string;
	scopes: string[];
	createdAt: string;
}

export interface APITokenWithSecret extends APIToken {
	token: string;
}

export const VALID_SCOPES = [
	'tasks:read',
	'tasks:write',
	'projects:read',
	'projects:write',
	'contexts:read',
	'contexts:write',
	'labels:read',
	'labels:write',
	'sections:read',
	'sections:write',
	'troiki:read',
	'troiki:write',
	'settings:read',
	'settings:write',
	'search:read',
	'calendars:read'
] as const;

export type Scope = (typeof VALID_SCOPES)[number] | '*';

export const SCOPE_RESOURCES = [
	{ resource: 'tasks', label: 'Задачи', hasWrite: true },
	{ resource: 'projects', label: 'Проекты', hasWrite: true },
	{ resource: 'contexts', label: 'Контексты', hasWrite: true },
	{ resource: 'labels', label: 'Метки', hasWrite: true },
	{ resource: 'sections', label: 'Секции', hasWrite: true },
	{ resource: 'troiki', label: 'Тройки', hasWrite: true },
	{ resource: 'settings', label: 'Настройки', hasWrite: true },
	{ resource: 'search', label: 'Поиск', hasWrite: false },
	{ resource: 'calendars', label: 'Календари', hasWrite: false }
] as const;

export interface Session {
	id: number;
	clientKind: ClientKind;
	userAgent: string;
	displayName: string;
	ipAddress: string;
	createdAt: string;
	lastUsedAt: string;
	isCurrent: boolean;
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
	totpAvailable: boolean;
	contexts: Context[];
	projects: Project[];
	labels: Label[];
	settings: UserSettings;
	appSettings: AppSettings;
	userState: UserState;
	troiki: TroikiViewResponse;
	planStats: PlanStatsResponse;
	inboxStats: { count: number; warnThresholdExceeded: boolean };
	pinnedTasks: Task[];
	harpoon: HarpoonState;
	/**
	 * A BARE array, unlike `GET /api/v1/task-templates` which returns a
	 * `Page<TaskTemplate>` envelope. Mirrors `configResp.TaskTemplates` in
	 * `internal/httpapi/handlers/meta.go` — do not reach for `.items` here.
	 */
	taskTemplates: TaskTemplate[];
}

export interface AutoLabelRule {
	mask: string;
	labelIds: number[];
	ignoreCase: boolean;
}

export interface ProjectSuggestionRule {
	mask: string;
	projectIds: number[];
	ignoreCase: boolean;
}

export interface AppSettings {
	autoLabels: AutoLabelRule[];
	projectSuggestions: ProjectSuggestionRule[];
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
	isPrivate?: boolean;
	projectType?: ProjectType;
}

export interface SectionInput {
	title?: string;
}

export interface LabelInput {
	name?: string;
	color?: ColorToken;
	isFavourite?: boolean;
	isPrivate?: boolean;
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
	isPrivate?: boolean;
	isComplex?: boolean;
}

export interface TaskTemplateSubtask {
	id: number;
	title: string;
	description: string;
	priority: Priority;
	dayPart: DayPart;
	labels: Label[];
}

export interface TaskTemplate {
	id: number;
	name: string;
	description: string;
	priority: Priority;
	dayPart: DayPart;
	position: number;
	labels: Label[];
	subtasks: TaskTemplateSubtask[];
	createdAt: string;
	updatedAt: string;
}

export interface TaskTemplateSubtaskInput {
	title: string;
	description?: string;
	priority?: Priority;
	dayPart?: DayPart;
	labelIds?: number[];
}

export interface TaskTemplateInput {
	name: string;
	description?: string;
	priority?: Priority;
	dayPart?: DayPart;
	labelIds?: number[];
	subtasks?: TaskTemplateSubtaskInput[];
}

export interface InstantiateTemplateResult {
	root: Task;
	subtasks: Task[];
}

export type TaskMoveInput =
	| { inboxId: number }
	| { contextId: number; projectId?: number; sectionId?: number; parentId?: number };

export interface TaskPlanInput {
	state: PlanState;
}

export interface BulkResult {
	succeeded: number[];
	failed: Array<{ id: number; error: { code: string; message: string } }>;
}

export interface GroupResult {
	parent: Task;
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

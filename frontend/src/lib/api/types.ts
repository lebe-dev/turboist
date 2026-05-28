// DTO types mirroring backend (camelCase JSON, ISO-8601 UTC strings).

export type Priority = 'high' | 'medium' | 'low' | 'no-priority';
export type TaskStatus = 'open' | 'completed' | 'cancelled';
export type ProjectStatus = 'open' | 'completed' | 'archived' | 'cancelled';
export type ProjectType = 'generic' | 'software';
export type DayPart = 'none' | 'morning' | 'afternoon' | 'evening';
export type PlanState = 'none' | 'week' | 'backlog';
export type ClientKind = 'web' | 'ios' | 'cli';
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
	completedAt: string | null;

	recurrenceRule: string | null;

	postponeCount: number;

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

export interface TodayBundle {
	today: ViewList<Task>;
	overdue: ViewList<Task>;
	completedToday: ViewList<Task>;
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
	calendarEnabled: boolean;
	calendarHidePastEvents: boolean;
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
}

export interface AutoLabelRule {
	mask: string;
	labelIds: number[];
	ignoreCase: boolean;
}

export interface AppSettings {
	autoLabels: AutoLabelRule[];
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

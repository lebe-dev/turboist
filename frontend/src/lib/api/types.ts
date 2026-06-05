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
	clientId: string;
	deletedAt: string | null;
	createdAt: string;
	updatedAt: string;
}

export interface Context {
	id: number;
	name: string;
	color: ColorToken;
	isFavourite: boolean;
	clientId: string;
	deletedAt: string | null;
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
	// isFederated reports whether this project has been enabled for federation
	// (Federation v1 F1.1). Mirrors ProjectDTO.IsFederated on the Go side.
	isFederated: boolean;
	// Federation surface (Federation v1 F2.4, US-2.4 AC1/AC2). Mirrors the
	// LEFT-JOIN-on-federated_projects fields on ProjectDTO. originInstance is the
	// owner instance a joined project mirrors; federationPermissions is this
	// instance's grant (read|write|admin); isOwner is true for the owner's own
	// federated project (controls stay enabled) and false for a joined peer copy.
	// All three are null/false for non-federated projects. The UI renders the
	// origin/role badges and disables editing when federationPermissions ===
	// 'read' — the backend 403 guard remains authoritative.
	originInstance: string | null;
	federationPermissions: FederationPermission | null;
	isOwner: boolean;
	// reBootstrappedAt is the wall-clock cutoff X of the most recent 410-stale
	// re-bootstrap of this joined project (Federation v1 F4.2, US-4.2 AC4), or null
	// if it has never been re-bootstrapped. When set, the joiner UI shows a
	// dismissible re-sync banner naming this timestamp ("your unsent changes from
	// before {X} were preserved but may have been overridden"). Null for the owner's
	// own project and for any project only ever initial-bootstrapped.
	reBootstrappedAt: string | null;
	// federationLost reports that this joined copy's trust link to the owner is
	// permanently gone (Federation v1 F5.4, US-6.2 AC3; shared with F5.5 / F5.6a).
	// federationLostReason disambiguates why (revoked|left|owner-dead). When lost
	// with a read-only reason (revoked/owner-dead) the UI renders the copy read-only
	// — the backend guard remains authoritative. Both are false/null for a healthy
	// or non-federated project. Mirrors ProjectDTO.FederationLost on the Go side.
	federationLost: boolean;
	federationLostReason: FederationLostReason | null;
	// ownerOffline reports that this JOINED copy's OWNER instance has been
	// unreachable past the owner-timeout window (Federation v1 F5.6a, US-6.5 AC1).
	// Unlike federationLost it is a DERIVED, transient signal: while true the joiner
	// keeps editing — edits queue and flush + LWW-resolve when the owner returns
	// (US-6.5 AC2/AC3) — and the UI shows a "pending — owner offline" badge WITHOUT
	// locking controls. False for the owner's own project, for non-federated
	// projects, and for a fresh-owner or already-lost copy. Mirrors
	// ProjectDTO.OwnerOffline on the Go side.
	ownerOffline: boolean;
	// peerInstances is the per-project named peer audience this project is visible
	// to (Federation v1 F6.4, US-7.1 AC3): the non-owner, non-revoked peers, each as
	// {instanceUrl, displayName}, resolved ONCE at bootstrap (no N+1). The new-task
	// editor renders the explicit instance hint ("will be visible to peers:
	// alice.example, bob.example") from this array, and the "visible to N peers" task
	// badge derives N from peerInstances.length — both read it locally without an
	// extra round-trip. It is an empty array for a non-federated project, the owner's
	// own project with no peers yet, and a joined copy (no outbound audience of its
	// own). Mirrors ProjectDTO.PeerInstances on the Go side.
	peerInstances: PeerInstance[];
	labels: Label[];
	troikiCategory: TroikiCategory | null;
	clientId: string;
	deletedAt: string | null;
	createdAt: string;
	updatedAt: string;
}

export interface ProjectSection {
	id: number;
	projectId: number;
	title: string;
	position: number;
	clientId: string;
	deletedAt: string | null;
	createdAt: string;
	updatedAt: string;
}

// FederationPermission is a peer's grant on a federated project (Federation v1).
// Mirrors model.FederationPermission. v1 invite UI offers read | write; admin is
// reserved for future use.
export type FederationPermission = 'read' | 'write' | 'admin';

// FederationLostReason disambiguates why a joined federated copy's trust link is
// permanently gone (Federation v1 F5.4, US-6.2; shared with F5.5 / F5.6a / F6.5).
// The empty/null case is the normal NOT-lost state. revoked/owner-dead/
// instance_url_changed render the copy read-only; left becomes a plain editable
// local project (F5.5). instance_url_changed is set when this instance was restored
// under a new BASE_URL (F6.5, US-8.5 AC2): the copy is read-only history until the
// user re-invites under the new URL. Mirrors model.FederationLostReason.
export type FederationLostReason = 'revoked' | 'left' | 'owner-dead' | 'instance_url_changed';

// CreateInviteRequest is the body for POST /api/v1/projects/:id/invites
// (Federation v1 F1.2, US-1.2). maxUses defaults to 1 and expiresAt defaults to
// now+7d server-side when omitted. Mirrors dto.CreateInviteRequest.
export interface CreateInviteRequest {
	permissions: FederationPermission;
	maxUses?: number;
	expiresAt?: string;
}

// CreateInviteResponse is the one-time invite-creation result (Federation v1
// F1.2). secret is the plaintext secret returned exactly once; link carries it
// in the URL fragment so it never reaches the server. Mirrors
// dto.CreateInviteResponse.
export interface CreateInviteResponse {
	inviteId: string;
	secret: string;
	link: string;
	permissions: FederationPermission;
	maxUses: number;
	expiresAt: string;
}

// InviteStatus is the server-derived lifecycle state of an invite (Federation v1
// F1.3, US-1.3 AC1): active|revoked|consumed|expired, with precedence
// revoked > consumed > expired > active. Mirrors model.InviteStatus.
export type InviteStatus = 'active' | 'revoked' | 'consumed' | 'expired';

// Invite is one row of the invite list (Federation v1 F1.3, US-1.3). It carries
// only id + metadata + derived status — there is intentionally NO secret field,
// so the secret is shown to the owner exactly once at creation and never
// re-served (US-1.3 AC5). Optional timestamps are empty strings when unset.
// Mirrors dto.InviteDTO.
export interface Invite {
	inviteId: string;
	permissions: FederationPermission;
	maxUses: number;
	usedCount: number;
	status: InviteStatus;
	expiresAt: string;
	revokedAt: string;
	consumedAt: string;
	createdAt: string;
}

// PeerStatus is the server-derived collaboration state of a federated peer
// (Federation v1 F1.4, US-1.4; "left" added F5.5, US-6.3 AC2) with precedence
// revoked > left > paused > stale(>24h) > active. "left" marks a peer that
// voluntarily left the project. Mirrors model.PeerStatus.
export type PeerStatus = 'active' | 'paused' | 'stale' | 'revoked' | 'left';

// Peer is one row of the project peers list (Federation v1 F1.4, US-1.4). It
// carries the peer's federation identity (instanceUrl) + handshake-supplied
// displayName so the UI can render "displayName @ instanceUrl" (US-1.4 AC2), the
// per-project permissions, the derived status (US-1.4 AC1/AC3), the last-sent HLC
// cursor, the last successful contact time, and pendingDelivery — the count of
// events queued for this peer (always 0 until the Phase-3 outbox lands, US-1.4
// AC4 partial). Optional timestamps are empty strings when unset. Mirrors
// dto.PeerDTO.
export interface Peer {
	instanceUrl: string;
	displayName: string;
	permissions: FederationPermission;
	status: PeerStatus;
	lastSentHlc: string;
	lastContactAt: string;
	joinedAt: string;
	pendingDelivery: number;
	// keyMismatchAt is the sticky timestamp of a detected peer key CHANGE
	// (Federation v1 F5.6b, US-6.4 AC2). Non-empty → the peer's inbound events are
	// being rejected (no auto-refetch, AC1) until an operator re-trusts the new key;
	// the PeersTable renders the "signature failed — possible key rotation or
	// compromise" incident alert + a "Trust new key" action. Empty in the healthy
	// case. Mirrors dto.PeerDTO.
	keyMismatchAt: string;
}

// PausePeerRequest is the body the owner UI POSTs to the pause/resume peer
// endpoints (Federation v1 F5.3, US-6.1). The peer's federation identity rides in
// the body (not the path) so a peer URL never needs URL-encoding into a route
// segment. Mirrors dto.PausePeerRequest.
export interface PausePeerRequest {
	instanceUrl: string;
}

// RevokePeerRequest is the body the owner UI sends to the DELETE peers endpoint
// (Federation v1 F5.4, US-6.2). Like PausePeerRequest the peer URL rides in the
// body. Revoke is irreversible — the UI confirms before sending. Mirrors
// dto.RevokePeerRequest.
export interface RevokePeerRequest {
	instanceUrl: string;
}

// TrustPeerKeyRequest is the body the owner UI POSTs to the trust-key endpoint
// (Federation v1 F5.6b, US-6.4 AC3). The peer URL rides in the body. Trusting a
// rotated key is a deliberate security action — the UI confirms before sending.
// Mirrors dto.TrustPeerKeyRequest.
export interface TrustPeerKeyRequest {
	instanceUrl: string;
}

// SyncState is the server-derived federation sync status of a single shared
// project (Federation v1 F4.3, US-4.3) with precedence key_mismatch > unreachable
// > pending > synced. Mirrors model.SyncStatus. The frontend SyncStatusBadge maps
// each state to a colour: synced=green, pending=yellow, unreachable=orange,
// key_mismatch=red.
export type SyncState = 'synced' | 'pending' | 'unreachable' | 'key_mismatch';

// SyncStatus is one federated project's sync status for the owner UI indicator
// (Federation v1 F4.3, US-4.3). projectId is the local int64 projects.id; status
// is the rolled-up state. The companion fields name the offending peer / count so
// the badge tooltip renders without a second round-trip: pendingCount is set only
// when status is pending, unreachablePeer only when unreachable, keyMismatchPeer
// only when key_mismatch. Mirrors dto.SyncStatusDTO.
export interface SyncStatus {
	projectId: number;
	status: SyncState;
	pendingCount: number;
	unreachablePeer: string;
	keyMismatchPeer: string;
}

// DeadLetterEntry is one parked, permanently-failed outbound event for the owner
// dead-letter diagnostics view (Federation v1 F4.4, US-4.4 AC3). It carries only
// metadata (never the event payload): the event id, the peer the delivery failed
// for, the originating local project, the HTTP status + federation error code
// that classified the failure, and when it was parked. Mirrors dto.DeadLetterDTO.
export interface DeadLetterEntry {
	eventId: string;
	peerInstanceUrl: string;
	projectId: number;
	statusCode: number;
	reason: string;
	failedAt: string;
}

// AuditKind is the security-relevant federation operation an audit row records
// (Federation v1 F6.3, US-7.4 AC1). Mirrors repo.AuditKind.
export type AuditKind =
	| 'handshake'
	| 'revoke'
	| 'trust_key'
	| 'signature_invalid'
	| 'digest_mismatch'
	| 'author_mismatch'
	| 'clock_skew'
	| 'replay'
	| 'timestamp_stale'
	| 'key_change';

// AuditOutcome records whether the operation succeeded or was refused (US-7.4
// AC1). Mirrors repo.AuditOutcome.
export type AuditOutcome = 'accepted' | 'rejected';

// AuditEntry is one federation audit-log row for the owner audit view (Federation
// v1 F6.3, US-7.4 AC1). detail is a short, NON-SENSITIVE coded reason — the server
// never persists secrets/signatures/tokens. Mirrors dto.AuditEntryDTO.
export interface AuditEntry {
	id: number;
	kind: AuditKind;
	outcome: AuditOutcome;
	peerInstanceUrl: string;
	detail: string;
	createdAt: string;
}

// SignatureFailureAlert flags a peer whose recent signature-failure count crossed
// the configured threshold (Federation v1 F6.3, US-7.4 AC3 — the "possible attack
// on peer X" banner). Mirrors dto.SignatureFailureAlertDTO.
export interface SignatureFailureAlert {
	peerInstanceUrl: string;
	count: number;
	threshold: number;
}

// AuditResponse is the paginated audit-log response (Federation v1 F6.3, US-7.4
// AC1/AC3): the rows plus the derived "possible attack" alerts. Mirrors
// dto.AuditResponseDTO.
export interface AuditResponse {
	entries: AuditEntry[];
	alerts: SignatureFailureAlert[];
}

// PeerInstance is one named peer instance a federated project is visible to
// (Federation v1 F6.4, US-7.1 AC3): the peer's federation identity (instanceUrl)
// plus its handshake-supplied displayName. It is the element type of the per-
// project Project.peerInstances array and the federation overview peer list.
// Mirrors dto.PeerInstanceDTO.
export interface PeerInstance {
	instanceUrl: string;
	displayName: string;
}

// FederationRole is this instance's coarse role on a federated project for the
// privacy/federation overview (Federation v1 F6.4, US-7.1 AC1): owner | peer |
// read-only. Mirrors model.FederationRole.
export type FederationRole = 'owner' | 'peer' | 'read-only';

// OverviewProject is one row of the privacy/federation overview (Federation v1
// F6.4, US-7.1 AC1): the local project id + title, this instance's role, and the
// named peer list it is visible to. Mirrors dto.OverviewProjectDTO.
export interface OverviewProject {
	projectId: number;
	title: string;
	role: FederationRole;
	peers: PeerInstance[];
}

// OverviewResponse is the GET /api/v1/federation/overview response (Federation v1
// F6.4, US-7.1 AC1): every federated project with its role + peer list. Non-
// federated projects are absent. Mirrors dto.OverviewResponseDTO.
export interface OverviewResponse {
	projects: OverviewProject[];
}

// FederationHealthStatus is the rolled-up federation liveness state (Federation v1
// F6.5, US-8.1): ok (nothing pending, all peers fresh), degraded (events pending),
// or peers_stale (a peer unreachable >24h). Mirrors fedsvc.HealthStatus.
export type FederationHealthStatus = 'ok' | 'degraded' | 'peers_stale';

// FederationHealthPeer is one peer's liveness line in the admin health view
// (Federation v1 F6.5, US-8.1). Mirrors dto.HealthPeerDTO.
export interface FederationHealthPeer {
	instanceUrl: string;
	displayName: string;
	status: string;
	lastContactAt?: string;
}

// FederationHealth is the GET /federation/health (public) or
// GET /api/v1/federation/health (admin, with peers) liveness report (Federation v1
// F6.5, US-8.1). The public probe omits peers. Mirrors dto.HealthResponse.
export interface FederationHealth {
	instanceUrl: string;
	protocolVersions: number[];
	uptimeS: number;
	outboxDepth: number;
	status: FederationHealthStatus;
	peers?: FederationHealthPeer[];
}

// RetentionSettings is the GET/PATCH /api/v1/federation/retention shape
// (Federation v1 F6.5, US-8.4). Each *Days field is the override in whole days (0
// = use the default); the effective* fields echo the resolved windows (post-clamp)
// so the UI shows what the GC will use; outboxHardcapDays surfaces the §16.3 30-day
// cap so the UI can warn when an over-cap outbox value is entered. Mirrors
// dto.RetentionSettingsDTO.
export interface RetentionSettings {
	tombstoneDays: number;
	outboxDays: number;
	inboxDays: number;
	outboxHardcapDays: number;
	effectiveTombstoneDays: number;
	effectiveOutboxDays: number;
	effectiveInboxDays: number;
}

// UpdateRetentionRequest is the PATCH body (Federation v1 F6.5, US-8.4). An omitted
// field keeps its current value; a sent 0 reverts that window to its default.
// Mirrors dto.UpdateRetentionRequest.
export interface UpdateRetentionRequest {
	tombstoneDays?: number;
	outboxDays?: number;
	inboxDays?: number;
}

// JoinInvite is the parsed (id, secret) pair the join flow sends server-side
// (Federation v1 F2.1, US-2.1). The secret rides in the request body to OUR own
// instance — never browser→owner — so it stays out of the CORS path and is
// resolved server-to-server. ownerInstanceUrl tells our instance WHERE to send
// the handshake (Federation v1 F2.2); the join page sources it from the link /
// page origin. Mirrors dto.JoinInviteRequest.
export interface JoinInvite {
	inviteId: string;
	secret: string;
	ownerInstanceUrl: string;
}

// JoinPreview is the read-only project summary the joiner instance fetches from
// the owner before accepting (Federation v1 F2.1, US-2.1 AC3). The owner
// identity renders as "ownerDisplayName @ ownerInstanceUrl" (US-1.4 AC2 shape).
// Mirrors the preview half of the F2.2 handshake response.
export interface JoinPreview {
	projectName: string;
	ownerInstanceUrl: string;
	ownerDisplayName: string;
	permissions: FederationPermission;
	protocolVersion: number;
}

// JoinResult is the outcome of accepting an invite (Federation v1 F2.1 stepper
// completion, US-2.1 AC4). projectId is the local int64 id of the newly mapped
// federated project. Mirrors the F2.2 join response.
export interface JoinResult {
	projectId: number;
	projectName: string;
	permissions: FederationPermission;
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
	// federated reports whether this task belongs to a federated project, and
	// visibleToPeers is the count of non-revoked peer instances that project is
	// shared with (Federation v1 F6.4, US-7.1 AC2). Together they back the
	// "federated, visible to N peers" task-header badge. The backend populates them
	// only where it resolved the project's federation surface (the task detail GET);
	// list endpoints leave them false / 0, so the badge prefers the project's
	// peerInstances array (which IS on every project) when rendering in a list.
	// Mirrors TaskDTO.Federated / TaskDTO.VisibleToPeers on the Go side.
	federated: boolean;
	visibleToPeers: number;
	// Offline-sync / federation overlay (Federation v1 F0.1): clientId is the
	// stable instance-portable id (UUIDv7); deletedAt is the soft-delete
	// tombstone — always null for entities the API returns, since tombstones are
	// filtered server-side, but present on the wire for the sync contract.
	clientId: string;
	deletedAt: string | null;
	createdAt: string;
	updatedAt: string;

	// Populated only by GET /tasks/:id?subtasks=true so the task detail page
	// can fetch parent + children in one round-trip. Omitted otherwise.
	subtasks?: Page<Task>;
}

// Comment is an immutable note on a task (Federation v1 F0.2). The body never
// changes after creation, so there is no patch shape. authorDisplayName /
// authorInstance render the federated author line "display_name @ origin"
// (US-3.5 AC4); both are null for locally-authored comments until federation is
// wired (F0.3). Mirrors internal/httpapi/dto/comments.go CommentDTO.
export interface Comment {
	id: number;
	taskId: number;
	body: string;
	authorDisplayName: string | null;
	authorInstance: string | null;
	clientId: string;
	deletedAt: string | null;
	createdAt: string;
	updatedAt: string;
}

export interface CreateCommentRequest {
	body: string;
}

// ChecklistItem is a small sub-todo on a task (Federation v1 F0.2). fracPosition
// is the fractional-index ordering key federation uses for conflict-free
// reorder; it is empty until the federated ordering path writes it. Mirrors
// internal/httpapi/dto/checklist_items.go ChecklistItemDTO.
export interface ChecklistItem {
	id: number;
	taskId: number;
	title: string;
	isCompleted: boolean;
	position: number;
	fracPosition: string;
	clientId: string;
	deletedAt: string | null;
	createdAt: string;
	updatedAt: string;
}

export interface CreateChecklistItemRequest {
	title: string;
}

export interface PatchChecklistItemRequest {
	title?: string;
	isCompleted?: boolean;
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
	troikiEnabled: boolean;
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

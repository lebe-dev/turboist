package dto

// Federation DTOs (Federation v1). F1.2 lands the invite-creation request /
// response; F1.3 adds the invite-list shape; later milestones extend this file
// with peer-list and handshake shapes. Field names mirror the frontend types in
// frontend/src/lib/api/types.ts (camelCase JSON tags); times are ISO-8601 UTC
// strings (model.FormatUTC).

// CreateInviteRequest is the body for POST /api/v1/projects/:id/invites
// (Federation v1 F1.2, US-1.2). Permissions is required (read|write|admin).
// MaxUses and ExpiresAt are optional: MaxUses defaults to 1 and ExpiresAt
// defaults to now+7d server-side when omitted (US-1.2 AC1, AC4). ExpiresAt is an
// ISO-8601 UTC string when provided.
type CreateInviteRequest struct {
	Permissions string  `json:"permissions"`
	MaxUses     int     `json:"maxUses"`
	ExpiresAt   *string `json:"expiresAt"`
}

// CreateInviteResponse is the one-time invite-creation result. Secret is the
// plaintext 256-bit secret returned to the owner UI exactly once (never stored
// in plaintext, never re-served); Link is the shareable join URL with the secret
// carried in the URL FRAGMENT — never the query string — so it never reaches the
// server in the request line or access logs (US-1.2 AC1, AC6).
type CreateInviteResponse struct {
	InviteID    string `json:"inviteId"`
	Secret      string `json:"secret"`
	Link        string `json:"link"`
	Permissions string `json:"permissions"`
	MaxUses     int    `json:"maxUses"`
	ExpiresAt   string `json:"expiresAt"`
}

// InviteDTO is one row of the invite list (Federation v1 F1.3, US-1.3). It
// carries only the invite id, metadata, and the server-derived lifecycle status
// — it intentionally has NO secret or secret-hash field, so the secret is shown
// to the owner exactly once at creation and is never re-served (US-1.3 AC5).
// Status is one of active|revoked|consumed|expired (US-1.3 AC1). Optional
// timestamps are empty strings when the underlying column is NULL.
type InviteDTO struct {
	InviteID    string `json:"inviteId"`
	Permissions string `json:"permissions"`
	MaxUses     int    `json:"maxUses"`
	UsedCount   int    `json:"usedCount"`
	Status      string `json:"status"`
	ExpiresAt   string `json:"expiresAt"`
	RevokedAt   string `json:"revokedAt"`
	ConsumedAt  string `json:"consumedAt"`
	CreatedAt   string `json:"createdAt"`
}

// PeerDTO is one row of the project peers list (Federation v1 F1.4, US-1.4). It
// carries the peer's federation identity (instanceUrl) + handshake-supplied
// displayName so the UI can render "displayName @ instanceUrl" (US-1.4 AC2), the
// per-project permissions, the server-derived collaboration status
// (active|paused|stale|revoked, US-1.4 AC1/AC3), the last-sent HLC cursor, the
// last successful contact time, and pendingDelivery — the count of events queued
// for this peer (always 0 until the Phase-3 outbox lands, US-1.4 AC4 partial).
// keyMismatchAt is the sticky timestamp of a detected peer key CHANGE (Federation
// v1 F5.6b, US-6.4 AC2): when non-empty the UI renders the "signature failed —
// possible key rotation or compromise" incident alert + a "Trust new key" action;
// empty in the healthy case. Optional timestamps are empty strings when the
// underlying value is unset.
type PeerDTO struct {
	InstanceURL     string `json:"instanceUrl"`
	DisplayName     string `json:"displayName"`
	Permissions     string `json:"permissions"`
	Status          string `json:"status"`
	LastSentHLC     string `json:"lastSentHlc"`
	LastContactAt   string `json:"lastContactAt"`
	JoinedAt        string `json:"joinedAt"`
	PendingDelivery int    `json:"pendingDelivery"`
	KeyMismatchAt   string `json:"keyMismatchAt"`
}

// TrustPeerKeyRequest is the body the owner UI POSTs to the trust-key endpoint
// (Federation v1 F5.6b, US-6.4 AC3). The peer's federation identity rides in the
// body (not the path) so a peer URL with a scheme/slashes never has to be
// URL-encoded into a route segment. Trusting a new key is a deliberate security
// action — the UI confirms before sending. Mirrors the frontend TrustPeerKeyRequest.
type TrustPeerKeyRequest struct {
	InstanceURL string `json:"instanceUrl"`
}

// PausePeerRequest is the body the owner UI POSTs to the pause/resume peer
// endpoints (Federation v1 F5.3, US-6.1). The peer's federation identity rides in
// the body (not the path) so a peer URL with a scheme/slashes never has to be
// URL-encoded into a route segment (R: "URL-encode peer or use body"). Mirrors the
// frontend PausePeerRequest type.
type PausePeerRequest struct {
	InstanceURL string `json:"instanceUrl"`
}

// RevokePeerRequest is the body the owner UI sends to the DELETE peers endpoint
// (Federation v1 F5.4, US-6.2). Like PausePeerRequest the peer's federation
// identity rides in the body (not the path) so a peer URL with a scheme/slashes
// never has to be URL-encoded into a route segment. Revoke is irreversible — the
// UI confirms before sending. Mirrors the frontend RevokePeerRequest type.
type RevokePeerRequest struct {
	InstanceURL string `json:"instanceUrl"`
}

// SyncStatusDTO is one federated project's sync-status for the owner UI indicator
// (Federation v1 F4.3, US-4.3). projectId is the local int64 projects.id; status
// is one of synced|pending|unreachable|key_mismatch (model.SyncStatus). The
// companion fields name the offending peer / count so the badge tooltip renders
// "N changes pending" / "peer X unreachable" / "peer X key mismatch" without a
// second round-trip: pendingCount is set only when status is pending,
// unreachablePeer only when unreachable, keyMismatchPeer only when key_mismatch.
// Mirrors the frontend SyncStatus type.
type SyncStatusDTO struct {
	ProjectId       int64  `json:"projectId"`
	Status          string `json:"status"`
	PendingCount    int    `json:"pendingCount"`
	UnreachablePeer string `json:"unreachablePeer"`
	KeyMismatchPeer string `json:"keyMismatchPeer"`
}

// DeadLetterDTO is one parked, permanently-failed (peer, event) delivery for the
// owner's dead-letter diagnostics view (Federation v1 F4.4, US-4.4 AC3). It
// carries only metadata (never the event payload bytes): the event id, the peer
// the delivery failed for, the originating local project, the HTTP status + the
// federation error code that classified the failure, and when it was parked.
// Mirrors the frontend DeadLetterEntry type.
type DeadLetterDTO struct {
	EventId         string `json:"eventId"`
	PeerInstanceUrl string `json:"peerInstanceUrl"`
	ProjectId       int64  `json:"projectId"`
	StatusCode      int    `json:"statusCode"`
	Reason          string `json:"reason"`
	FailedAt        string `json:"failedAt"`
}

// AuditEntryDTO is one federation audit-log row for the owner audit view
// (Federation v1 F6.3, US-7.4 AC1). It carries the timestamp, peer, kind, and
// outcome required by AC1 plus a short, NON-SENSITIVE coded detail — the writer
// never persists secrets/signatures/tokens. Mirrors the frontend AuditEntry type.
type AuditEntryDTO struct {
	Id              int64  `json:"id"`
	Kind            string `json:"kind"`
	Outcome         string `json:"outcome"`
	PeerInstanceUrl string `json:"peerInstanceUrl"`
	Detail          string `json:"detail"`
	CreatedAt       string `json:"createdAt"`
}

// AuditResponseDTO is the paginated audit-log response (Federation v1 F6.3,
// US-7.4 AC1/AC3): the rows plus the "possible attack on peer X" alerts derived
// from the recent signature-failure counts. Mirrors the frontend AuditResponse.
type AuditResponseDTO struct {
	Entries []AuditEntryDTO            `json:"entries"`
	Alerts  []SignatureFailureAlertDTO `json:"alerts"`
}

// SignatureFailureAlertDTO flags a peer whose recent signature-failure count
// crossed the configured threshold (Federation v1 F6.3, US-7.4 AC3 — the
// "possible attack on peer X" banner). Mirrors the frontend SignatureFailureAlert.
type SignatureFailureAlertDTO struct {
	PeerInstanceUrl string `json:"peerInstanceUrl"`
	Count           int    `json:"count"`
	Threshold       int    `json:"threshold"`
}

// PeerInstanceDTO is one named peer instance a federated project is visible to
// (Federation v1 F6.4, US-7.1 AC3): the peer's federation identity (instanceUrl)
// plus its handshake-supplied displayName. It is the element type of the per-
// project peerInstances array exposed on the project DTO — the new-task editor
// renders the explicit instance list ("visible to peers: alice.example,
// bob.example") and the "visible to N peers" task badge derives N from the array
// length. Mirrors the frontend PeerInstance type.
type PeerInstanceDTO struct {
	InstanceUrl string `json:"instanceUrl"`
	DisplayName string `json:"displayName"`
}

// OverviewProjectDTO is one row of the privacy/federation overview (Federation v1
// F6.4, US-7.1 AC1): the local project id + title, this instance's role
// (owner|peer|read-only), and the named peer list the project is visible to.
// Mirrors the frontend OverviewProject type.
type OverviewProjectDTO struct {
	ProjectId int64             `json:"projectId"`
	Title     string            `json:"title"`
	Role      string            `json:"role"`
	Peers     []PeerInstanceDTO `json:"peers"`
}

// OverviewResponseDTO is the GET /api/v1/federation/overview response (Federation
// v1 F6.4, US-7.1 AC1): every federated project with its role + peer list, newest
// projects sorted by title server-side. Non-federated projects are absent. Mirrors
// the frontend OverviewResponse type.
type OverviewResponseDTO struct {
	Projects []OverviewProjectDTO `json:"projects"`
}

// JoinInviteRequest is the body the joiner UI POSTs to OUR own instance's JWT
// preview/join endpoints (Federation v1 F2.2, US-2.2). The secret rides in the
// request body to our instance — never browser→owner — and our instance signs +
// sends the handshake server-to-server. OwnerInstanceURL is where to send the
// handshake; the join page sources it from the link/page origin. Mirrors the
// frontend JoinInvite type.
type JoinInviteRequest struct {
	InviteID         string `json:"inviteId"`
	Secret           string `json:"secret"`
	OwnerInstanceURL string `json:"ownerInstanceUrl"`
}

// JoinPreviewResponse is the read-only owner summary surfaced before accepting
// (Federation v1 F2.1 preview backing, US-2.1 AC3). The project name is empty
// until the handshake consumes the invite; the UI renders the owner identity as
// "ownerDisplayName @ ownerInstanceUrl". Mirrors the frontend JoinPreview type.
type JoinPreviewResponse struct {
	ProjectName      string `json:"projectName"`
	OwnerInstanceURL string `json:"ownerInstanceUrl"`
	OwnerDisplayName string `json:"ownerDisplayName"`
	Permissions      string `json:"permissions"`
	ProtocolVersion  int    `json:"protocolVersion"`
}

// JoinResultResponse is the outcome of accepting an invite (Federation v1 F2.2,
// US-2.2 AC3). ProjectID is the local int64 id the joiner maps the federated
// project to. Mirrors the frontend JoinResult type.
type JoinResultResponse struct {
	ProjectID   int64  `json:"projectId"`
	ProjectName string `json:"projectName"`
	Permissions string `json:"permissions"`
}

// HealthResponse is the GET /federation/health liveness report (Federation v1
// F6.5, US-8.1). status is one of ok|degraded|peers_stale. peers carries the
// per-peer detail only on the admin (JWT) route; the public liveness route omits
// it (an empty/nil array) so an unauthenticated probe leaks no peer directory.
type HealthResponse struct {
	InstanceUrl      string          `json:"instanceUrl"`
	ProtocolVersions []int           `json:"protocolVersions"`
	UptimeS          int64           `json:"uptimeS"`
	OutboxDepth      int             `json:"outboxDepth"`
	Status           string          `json:"status"`
	Peers            []HealthPeerDTO `json:"peers,omitempty"`
}

// HealthPeerDTO is one peer's liveness line (admin view only).
type HealthPeerDTO struct {
	InstanceUrl   string `json:"instanceUrl"`
	DisplayName   string `json:"displayName"`
	Status        string `json:"status"`
	LastContactAt string `json:"lastContactAt,omitempty"`
}

// RetentionSettingsDTO is the GET/PATCH /api/v1/federation/retention shape
// (Federation v1 F6.5, US-8.4). Each field is the override in whole DAYS; 0 means
// "use the default". outboxDays is hard-capped at 30 (§16.3) when applied —
// outboxHardcapDays surfaces the cap so the UI can warn. The effective* fields
// echo the RESOLVED windows (override-or-default, post-clamp) so the UI can render
// what the GC will actually use without re-deriving the defaults client-side.
type RetentionSettingsDTO struct {
	TombstoneDays          int `json:"tombstoneDays"`
	OutboxDays             int `json:"outboxDays"`
	InboxDays              int `json:"inboxDays"`
	OutboxHardcapDays      int `json:"outboxHardcapDays"`
	EffectiveTombstoneDays int `json:"effectiveTombstoneDays"`
	EffectiveOutboxDays    int `json:"effectiveOutboxDays"`
	EffectiveInboxDays     int `json:"effectiveInboxDays"`
}

// UpdateRetentionRequest is the PATCH body. Pointer fields so an omitted field
// keeps its current value; a sent 0 reverts that window to its default.
type UpdateRetentionRequest struct {
	TombstoneDays *int `json:"tombstoneDays"`
	OutboxDays    *int `json:"outboxDays"`
	InboxDays     *int `json:"inboxDays"`
}

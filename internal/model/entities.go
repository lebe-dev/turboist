package model

import (
	"strconv"
	"time"
)

// ClientID and DeletedAt are the offline-sync / federation overlay fields
// carried by every synchronized entity (Federation v1 F0.1):
//   - ClientID is a stable, instance-portable identifier (UUIDv7); empty for
//     legacy rows that predate the overlay and have not yet been backfilled.
//   - DeletedAt is the soft-delete tombstone; nil means live, a non-nil value
//     is a final tombstone (re-edit returns 410 Gone).

type Context struct {
	ID          int64
	Name        string
	Color       string
	IsFavourite bool
	ClientID    string
	DeletedAt   *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Label struct {
	ID          int64
	Name        string
	Color       string
	IsFavourite bool
	IsPrivate   bool
	ClientID    string
	DeletedAt   *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Project struct {
	ID          int64
	ContextID   int64
	Title       string
	Description string
	Color       string
	Status      ProjectStatus
	Type        ProjectType
	IsPinned    bool
	PinnedAt    *time.Time
	IsPrivate   bool
	// IsFederated reports whether this project has been enabled for federation
	// (Federation v1 F1.1). Enabling flips the flag and inserts the is_owner=1
	// self-row in federated_projects; the project is not actually syncable until
	// the Phase 3 sync core lands.
	IsFederated    bool
	TroikiCategory *TroikiCategory
	Labels         []Label
	ClientID       string
	DeletedAt      *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type ProjectSection struct {
	ID        int64
	ProjectID int64
	Title     string
	Position  int
	ClientID  string
	DeletedAt *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Task struct {
	ID          int64
	Title       string
	Description string

	InboxID   *int64
	ContextID *int64
	ProjectID *int64
	SectionID *int64
	ParentID  *int64

	Priority Priority
	Status   TaskStatus

	DueAt           *time.Time
	DueHasTime      bool
	DeadlineAt      *time.Time
	DeadlineHasTime bool

	DayPart   DayPart
	PlanState PlanState

	IsPinned bool
	PinnedAt *time.Time

	IsPrivate bool

	CompletedAt *time.Time

	RecurrenceRule *string

	// SourceTaskID points snapshot rows back to the parent recurring task they
	// were created from. Nil for non-snapshot rows.
	SourceTaskID *int64

	PostponeCount int

	TroikiCategory *TroikiCategory

	Labels []Label

	ClientID  string
	DeletedAt *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (t *Task) URL(baseURL string) string {
	return baseURL + "/task/" + strconv.FormatInt(t.ID, 10)
}

// Comment is an immutable note attached to a task (Federation v1 F0.2). The
// body is never updated — only create and (soft-)delete participate in
// federation sync — so cross-instance merge never has to reconcile a comment
// body. It carries the offline-sync overlay columns (ClientID, DeletedAt) like
// the other synchronized entities.
type Comment struct {
	ID        int64
	TaskID    int64
	Body      string
	ClientID  string
	DeletedAt *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ChecklistItem is a small sub-todo on a task (Federation v1 F0.2). Position is
// the local, renormalising integer order (like ProjectSection); FracPosition is
// the nullable fractional-index key federation will use for conflict-free
// ordering (§5.6 / R9) and is empty until the federated ordering path writes it.
type ChecklistItem struct {
	ID           int64
	TaskID       int64
	Title        string
	IsCompleted  bool
	Position     int
	FracPosition string
	ClientID     string
	DeletedAt    *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// FederationKeys is the single-row (id=1) federation trust-plane identity for
// this instance (Federation v1 F0.3). It holds the published Ed25519 public key,
// the encrypted private seed (TokenCipher at rest), a stable generated install
// node_id (the HLC tie-break id, R10 — never derived from BASE_URL), and the
// human-readable instance display_name carried by the handshake (the only source
// for "display_name @ instance.tld", since users has no display_name).
type FederationKeys struct {
	ID             int64
	PublicKey      string
	PrivateSeedEnc string
	NodeID         string
	DisplayName    string
	CreatedAt      time.Time
}

// FederationPermission is a peer's grant on a federated project (Federation v1
// F1.1). It maps to the federated_projects.permissions CHECK(read|write|admin).
type FederationPermission string

const (
	FederationPermissionRead  FederationPermission = "read"
	FederationPermissionWrite FederationPermission = "write"
	FederationPermissionAdmin FederationPermission = "admin"
)

// IsValid reports whether p is one of the three accepted permission grades.
func (p FederationPermission) IsValid() bool {
	switch p {
	case FederationPermissionRead, FederationPermissionWrite, FederationPermissionAdmin:
		return true
	default:
		return false
	}
}

// FederationLostReason disambiguates WHY a joined federated project copy became
// "lost" — its trust link to the owner is permanently gone (Federation v1 F5.4,
// US-6.2; shared with F5.5 US-6.3 leave and F5.6a US-6.5 owner-death). It maps to
// the federated_projects.lost_reason CHECK(”|revoked|left|owner-dead). The empty
// reason ("") is the normal, NOT-lost state. The reason drives whether the local
// copy is read-only (revoked / owner-dead) or becomes a plain editable local
// project (left, F5.5).
type FederationLostReason string

const (
	// FederationLostNone is the normal NOT-lost state (the trust link is intact).
	FederationLostNone FederationLostReason = ""
	// FederationLostRevoked — the owner permanently revoked this peer (F5.4,
	// US-6.2): the local copy stays READ-ONLY and re-collaboration needs a fresh
	// invite (irreversible, US-6.2 AC5).
	FederationLostRevoked FederationLostReason = "revoked"
	// FederationLostLeft — the joiner voluntarily left the project (F5.5, US-6.3):
	// the local copy becomes a plain editable local project (no outbound sync).
	FederationLostLeft FederationLostReason = "left"
	// FederationLostOwnerDead — the owner instance is permanently unreachable
	// (F5.6a, US-6.5): read-only fallback.
	FederationLostOwnerDead FederationLostReason = "owner-dead"
	// FederationLostInstanceURLChanged — this instance was restored under a NEW
	// BASE_URL / instance_url (Federation v1 F6.5, US-8.5 AC2, R27). The old
	// federation mappings are NOT destroyed: they are kept locally readable as
	// HISTORY (read-only) while outbound/inbound sync is halted, because peers will
	// reject this instance's new URL until the user re-invites. The keypair is
	// preserved (no key regen); only the URL identity changed.
	FederationLostInstanceURLChanged FederationLostReason = "instance_url_changed"
)

// IsValid reports whether r is one of the accepted lost reasons (including the
// empty NOT-lost reason). It is a drift guard for the wire/DB CHECK vocabulary.
func (r FederationLostReason) IsValid() bool {
	switch r {
	case FederationLostNone, FederationLostRevoked, FederationLostLeft, FederationLostOwnerDead, FederationLostInstanceURLChanged:
		return true
	default:
		return false
	}
}

// IsReadOnly reports whether a lost copy must be rendered read-only. A revoked or
// owner-dead copy is read-only (US-6.2 AC3 / US-6.5); an instance_url_changed copy
// is read-only history while the user re-invites (US-8.5 AC2, F6.5); a voluntarily-
// left copy becomes a plain editable local project (US-6.3 AC3, F5.5). The empty
// reason is not lost at all, so it is never read-only on this axis.
func (r FederationLostReason) IsReadOnly() bool {
	switch r {
	case FederationLostRevoked, FederationLostOwnerDead, FederationLostInstanceURLChanged:
		return true
	default:
		return false
	}
}

// FederatedProject is one (local project, peer) federation mapping row
// (Federation v1 F1.1). The owner's own instance has a self-row with
// IsOwner=true and PeerInstanceURL == OriginInstanceURL == this instance's URL.
// LocalProjectID is the int64 projects.id (deviation §3 — the design doc's TEXT
// id maps to RemoteProjectID/ClientID). LastSentHLC/LastReceivedHLC are the
// per-peer sync cursors populated by the Phase 3 sync workers.
type FederatedProject struct {
	LocalProjectID    int64
	PeerInstanceURL   string
	RemoteProjectID   string
	IsOwner           bool
	OriginInstanceURL string
	Permissions       FederationPermission
	Paused            bool
	Revoked           bool
	ProtocolVersion   int
	LastSentHLC       string
	LastReceivedHLC   string
	JoinedAt          time.Time
	// RebootstrapCutoffHLC / RebootstrappedAt record the cutoff X of the most
	// recent 410-stale re-bootstrap (Federation v1 F4.2, US-4.2 AC4). They are
	// empty on a row that has only ever been initial-bootstrapped (F2.3), so the
	// joiner UI distinguishes a first bootstrap from a re-bootstrap. CutoffHLC is
	// the snapshot's as_of_hlc (the causal cutoff); RebootstrappedAt is the
	// wall-clock TEXT (model.FormatUTC) the re-bootstrap committed at — the
	// human-readable X the re-sync banner renders.
	RebootstrapCutoffHLC string
	RebootstrappedAt     string
	// Lost / LostReason record that this (joined) copy's trust link to the owner is
	// permanently gone (Federation v1 F5.4, US-6.2; shared with F5.5 / F5.6a). Lost
	// is false on a healthy mapping; when true LostReason disambiguates why
	// (revoked|left|owner-dead) and drives whether the local copy is read-only
	// (LostReason.IsReadOnly). The marker is set on the JOINER's is_owner=0 row when
	// it applies a federation_revoke (or self-detects a revoke on the offline-return
	// 403, US-6.2 AC4); it is irreversible for revoked.
	Lost       bool
	LostReason FederationLostReason
}

// FederatedInstance is one peer in the trust directory (Federation v1 F1.4),
// keyed by its federation identity InstanceURL. DisplayName is the human-readable
// name the peer carried in its handshake (users has no display_name, R24) and is
// the source for the "display_name @ instance.tld" rendering (US-1.4 AC2). PublicKey
// is the peer's Ed25519 public key. LastContactAt is the most recent successful
// contact and drives the derived "stale" status (US-1.4 AC3); nil means never.
type FederatedInstance struct {
	InstanceURL   string
	PublicKey     string
	DisplayName   string
	LastContactAt *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// PeerInstance is the minimal named identity of a peer a federated project is
// visible to (Federation v1 F6.4, US-7.1 AC3): the peer's federation InstanceURL
// plus its handshake-supplied DisplayName. It backs the per-project peerInstances
// array exposed on the project DTO — the new-task editor renders the explicit
// instance list ("visible to peers: alice.example, bob.example"), and the
// "visible to N peers" task badge derives N from the array length. It carries the
// non-revoked, non-owner peer audience only; DisplayName falls back to the URL
// when the directory row has no name yet.
type PeerInstance struct {
	InstanceURL string
	DisplayName string
}

// PeerStatus is the derived collaboration state of a federated peer for a single
// project (Federation v1 F1.4, US-1.4). It is never stored; it is computed from
// the per-project row flags (revoked/paused) and contact recency by
// DerivePeerStatus with a fixed precedence so the peers table and any future
// fan-out gating agree.
type PeerStatus string

const (
	// PeerStatusActive — not revoked, not paused, contacted within PeerStaleAfter.
	PeerStatusActive PeerStatus = "active"
	// PeerStatusRevoked — the owner revoked this peer (highest precedence).
	PeerStatusRevoked PeerStatus = "revoked"
	// PeerStatusLeft — the peer voluntarily left the project (Federation v1 F5.5,
	// US-6.3 AC2). Like revoked it is terminal (the link is gone; the peer must
	// re-join with a fresh invite) but it is peer-initiated, not owner-initiated, so
	// the owner UI renders it distinctly as "left". Checked after revoked so a peer
	// the owner already revoked never silently re-labels as "left".
	PeerStatusLeft PeerStatus = "left"
	// PeerStatusPaused — sync to/from this peer is paused.
	PeerStatusPaused PeerStatus = "paused"
	// PeerStatusStale — last contact is older than PeerStaleAfter (or never).
	PeerStatusStale PeerStatus = "stale"
)

// PeerStaleAfter is the no-contact window after which a non-revoked, non-paused
// peer is flagged "stale" in the peers list (US-1.4 AC3). The boundary is strict:
// a peer last contacted exactly PeerStaleAfter ago is still active.
const PeerStaleAfter = 24 * time.Hour

// DerivePeerStatus computes a peer's collaboration status at the given instant
// with the canonical precedence revoked > left > paused > stale > active
// (Federation v1 F1.4, US-1.4; "left" added F5.5, US-6.3 AC2). Status is derived
// from the per-project row first (revoked/left/paused) and contact recency second
// (stale). A peer that voluntarily left is reported "left" — terminal like revoked
// but peer-initiated — and is recognised by a lost mapping whose reason is "left"
// (the owner's peer row, not a healthy contact-recency signal). A peer that has
// never been contacted (lastContact == nil) is stale. The window is strict:
// contact exactly PeerStaleAfter ago is still active, anything older is stale.
func DerivePeerStatus(revoked, paused bool, lostReason FederationLostReason, lastContact *time.Time, now time.Time) PeerStatus {
	if revoked {
		return PeerStatusRevoked
	}
	// A peer that voluntarily left is terminal but distinct from an owner revoke:
	// the owner's peer row is marked lost with reason "left" (US-6.3 AC2). It is
	// checked after revoked so an owner who revoked a peer never re-labels it "left".
	if lostReason == FederationLostLeft {
		return PeerStatusLeft
	}
	if paused {
		return PeerStatusPaused
	}
	if lastContact == nil {
		return PeerStatusStale
	}
	if now.Sub(*lastContact) > PeerStaleAfter {
		return PeerStatusStale
	}
	return PeerStatusActive
}

// FederationRole is the coarse role of THIS instance on a single federated project
// for the privacy/federation overview (Federation v1 F6.4, US-7.1 AC1). It is
// derived (never stored) from the resolved per-project row's is_owner flag + the
// granted permission, in ONE canonical place so the overview API and any future
// surface agree on the mapping.
type FederationRole string

const (
	// FederationRoleOwner — this instance owns the project (its is_owner=1 self-row).
	FederationRoleOwner FederationRole = "owner"
	// FederationRolePeer — a joined copy with a write/admin grant (US-7.1 AC1).
	FederationRolePeer FederationRole = "peer"
	// FederationRoleReadOnly — a joined copy with a read-only grant (US-7.1 AC1).
	FederationRoleReadOnly FederationRole = "read-only"
)

// DeriveFederationRole maps the resolved per-project federation row (is_owner +
// granted permission) to the coarse overview role with the canonical precedence
// owner > read-only > peer (Federation v1 F6.4, US-7.1 AC1). It is the SINGLE
// backend place the role mapping lives. An owner is always "owner" regardless of
// the self-row's stored permission; a joined copy with a bare read grant is
// "read-only"; any other joined grant (write/admin) is a collaborating "peer".
func DeriveFederationRole(isOwner bool, permissions FederationPermission) FederationRole {
	if isOwner {
		return FederationRoleOwner
	}
	if permissions == FederationPermissionRead {
		return FederationRoleReadOnly
	}
	return FederationRolePeer
}

// SyncStatus is the derived federation sync-status of a single federated project
// for the owner UI indicator (Federation v1 F4.3, US-4.3). It is never stored; it
// is rolled up from the project's per-peer health by DeriveSyncStatus with a
// fixed precedence so the badge and any future alerting agree on one state.
type SyncStatus string

const (
	// SyncStatusSynced — green: outbox is empty (or freshly delivered) and every
	// peer is fresh, no key mismatch (US-4.3 AC1).
	SyncStatusSynced SyncStatus = "synced"
	// SyncStatusPending — yellow: undelivered outbox events older than
	// SyncStatusPendingAfter (US-4.3 AC2).
	SyncStatusPending SyncStatus = "pending"
	// SyncStatusUnreachable — orange: a peer has not been contacted in over
	// PeerStaleAfter (US-4.3 AC3).
	SyncStatusUnreachable SyncStatus = "unreachable"
	// SyncStatusKeyMismatch — red, sticky: a peer's signature stopped validating
	// (its key changed) and its events are NOT applied until an operator re-trusts
	// the new key (US-4.3 AC4). Highest precedence.
	SyncStatusKeyMismatch SyncStatus = "key_mismatch"
)

// SyncStatusPendingAfter is the age an undelivered outbox event must exceed for
// the project to report "pending" rather than "synced" (US-4.3 AC2). A just-
// committed event still in flight (delivered well under the NFR-1.1 5s push
// budget) must NOT flip the badge yellow, so the boundary is a generous 5 minutes.
const SyncStatusPendingAfter = 5 * time.Minute

// IsValid reports whether s is one of the four canonical sync states. It is a
// drift guard for the wire contract the frontend SyncStatusBadge maps.
func (s SyncStatus) IsValid() bool {
	switch s {
	case SyncStatusSynced, SyncStatusPending, SyncStatusUnreachable, SyncStatusKeyMismatch:
		return true
	default:
		return false
	}
}

// DeriveSyncStatus rolls a federated project's per-peer health into a single
// status with the canonical precedence key_mismatch > unreachable > pending >
// synced (Federation v1 F4.3, US-4.3). The three risk flags are the WORST
// observed across the project's peers: one key-mismatched peer turns the project
// red, one unreachable peer turns it orange, one overdue-undelivered event turns
// it yellow; only an all-clear project is green. The precedence is deliberate —
// a key mismatch (events being silently dropped, US-4.3 AC4) is the most urgent,
// then an unreachable peer (US-4.3 AC3), then merely-pending delivery (AC2).
func DeriveSyncStatus(keyMismatch, anyUnreachable, anyPendingOverdue bool) SyncStatus {
	if keyMismatch {
		return SyncStatusKeyMismatch
	}
	if anyUnreachable {
		return SyncStatusUnreachable
	}
	if anyPendingOverdue {
		return SyncStatusPending
	}
	return SyncStatusSynced
}

// DeriveOwnerOffline reports whether a JOINER must treat its project's OWNER as
// dead/unreachable for the owner-death read-only/queued fallback (Federation v1
// F5.6a, US-6.5 AC1). The owner is offline once its last successful contact
// (push/pull/handshake touchpoint) is OLDER than the owner-timeout window, or it
// has never been contacted (ownerLastContact == nil). The boundary is strict —
// contact exactly ownerTimeout ago is still online — and the window is generous
// (default 30 days) so an owner being briefly unreachable does not falsely flip
// the joiner into the offline-fallback state.
//
// This is a DERIVED, transient signal, NOT the permanent federation_lost marker:
// while the owner is offline the joiner keeps editing (its edits queue in
// federation_outbox and flush + LWW-resolve when the owner returns, US-6.5
// AC2/AC3); only the UI surfaces a "pending — owner offline" badge. A non-positive
// ownerTimeout disables the check (fails safe to "online") so a misconfigured
// timeout never declares an owner dead.
func DeriveOwnerOffline(ownerLastContact *time.Time, ownerTimeout time.Duration, now time.Time) bool {
	if ownerTimeout <= 0 {
		return false
	}
	if ownerLastContact == nil {
		return true
	}
	return now.Sub(*ownerLastContact) > ownerTimeout
}

// FederationInvite is a per-project share invite (Federation v1 F1.2, US-1.2).
// The 256-bit secret is NEVER persisted in plaintext — only SecretHash =
// hex(SHA-256(secret)) is stored (US-1.2 AC2); the plaintext secret is returned
// to the owner UI exactly once at creation and travels in the join-link URL
// fragment. InviteID is a UUIDv7 (model.NewClientID). ExpiresAt defaults to
// now+7d and MaxUses defaults to 1 (US-1.2 AC1, AC4). RevokedAt/ConsumedAt drive
// the derived lifecycle status (active → consumed/revoked/expired), wired by
// later milestones (F1.3 list/revoke, F2.2 handshake-consume).
type FederationInvite struct {
	InviteID       string
	LocalProjectID int64
	SecretHash     string
	Permissions    FederationPermission
	MaxUses        int
	UsedCount      int
	ExpiresAt      *time.Time
	RevokedAt      *time.Time
	ConsumedAt     *time.Time
	CreatedAt      time.Time
}

// InviteStatus is the derived lifecycle state of a federation invite
// (Federation v1 F1.3, US-1.3 AC1). It is never stored; it is computed from
// RevokedAt / UsedCount / ExpiresAt by FederationInvite.Status with a fixed
// precedence so the list view and the handshake consume path agree.
type InviteStatus string

const (
	// InviteStatusActive — usable: not revoked, not fully consumed, not expired.
	InviteStatusActive InviteStatus = "active"
	// InviteStatusRevoked — the owner revoked it (highest precedence).
	InviteStatusRevoked InviteStatus = "revoked"
	// InviteStatusConsumed — UsedCount has reached MaxUses.
	InviteStatusConsumed InviteStatus = "consumed"
	// InviteStatusExpired — ExpiresAt is in the past.
	InviteStatusExpired InviteStatus = "expired"
)

// Status derives the invite's lifecycle state at the given instant with the
// canonical precedence revoked > consumed > expired > active (Federation v1
// F1.3, US-1.3 AC1). This is the single source of truth shared by the invite
// list (F1.3) and the handshake consume path (F2.2) so a revoked-and-expired
// invite never reports two states.
func (i FederationInvite) Status(now time.Time) InviteStatus {
	if i.RevokedAt != nil {
		return InviteStatusRevoked
	}
	if i.MaxUses > 0 && i.UsedCount >= i.MaxUses {
		return InviteStatusConsumed
	}
	if i.ExpiresAt != nil && !now.Before(*i.ExpiresAt) {
		return InviteStatusExpired
	}
	return InviteStatusActive
}

// IsConsumable reports whether the invite can still be consumed at the given
// instant — true exactly when its derived Status is active. The handshake
// consume path (F2.2) uses this so consumption agrees with the displayed status.
func (i FederationInvite) IsConsumable(now time.Time) bool {
	return i.Status(now) == InviteStatusActive
}

type User struct {
	ID                   int64
	Username             string
	PasswordHash         string
	TroikiMediumCapacity int
	TroikiRestCapacity   int
	TroikiStarted        bool
	TOTPSecret           string
	TOTPEnabled          bool
	TOTPEnabledAt        *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type Session struct {
	ID         int64
	UserID     int64
	TokenHash  string
	ClientKind ClientKind
	UserAgent  string
	IPAddress  string
	CreatedAt  time.Time
	LastUsedAt time.Time
	ExpiresAt  time.Time
	RevokedAt  *time.Time
}

func (s *Session) IsActive(now time.Time) bool {
	return s.RevokedAt == nil && now.Before(s.ExpiresAt)
}

type APIToken struct {
	ID        int64
	UserID    int64
	Name      string
	TokenHash string
	Scopes    []string
	CreatedAt time.Time
}

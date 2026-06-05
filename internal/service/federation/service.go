// Package federation owns the federation invariant logic that sits between the
// HTTP handlers and the raw federation repos (Federation v1). F1.1 lands the
// owner-local per-project enable flow; later phases extend the service with
// invites, handshake, status, and the transactional outbox emit.
package federation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/federation/peerkeys"
	"github.com/lebe-dev/turboist/internal/federation/protocol"
	"github.com/lebe-dev/turboist/internal/federation/store"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// ErrProjectNotFound is returned by EnableForProject when the target project
// does not exist (or is a tombstone). Handlers map it to 404.
var ErrProjectNotFound = errors.New("federation: project not found")

// ErrKeyMissing is returned when federation has not been configured on this
// instance (no FEDERATION_KEY / no cipher), so the trust-plane keypair cannot
// be generated. Handlers map it to CodeFederationKeyMissing (US-1.1 AC4 guard).
var ErrKeyMissing = errors.New("federation: keypair not available")

// ErrPeerNotFound is returned by PausePeer/ResumePeer/RevokePeer when the named
// peer is not joined to the project (no is_owner=0 mapping row). Handlers map it
// to 404.
var ErrPeerNotFound = errors.New("federation: peer not found")

// ErrPeerRevoked is returned by ResumePeer when the named peer has been
// permanently revoked (Federation v1 F5.4, US-6.2 AC5): a revoked peer cannot be
// un-paused or otherwise re-enabled — re-collaboration requires a fresh invite.
// Handlers map it to 403 federation_revoked.
var ErrPeerRevoked = errors.New("federation: peer access has been revoked")

// ErrNotJoined is returned by LeaveProject when the target project is not a joined
// federated copy (it is the owner's OWN project, or not federated at all): only a
// peer that joined another instance's project may leave it (Federation v1 F5.5,
// US-6.3). Handlers map it to 400/409 (the owner cannot "leave" their own project;
// they revoke peers instead).
var ErrNotJoined = errors.New("federation: project is not a joined federated copy")

// Service is the owner-local federation service. It is constructed only when
// federation is enabled (FEDERATION_KEY is set); cipher is the
// FEDERATION_KEY-derived TokenCipher used to lazily generate the instance
// keypair, and instanceURL is this instance's federation identity (BASE_URL).
//
// The handshake/join collaborators (sender, fetch, peerKeys, now) are wired
// additively by WithJoinDeps so the owner-enable path can be constructed without
// them. now is injectable for deterministic handshake-signature tests.
type Service struct {
	db           *sql.DB
	projects     *repo.ProjectRepo
	fedProjects  *repo.FederatedProjectRepo
	keys         *repo.FederationKeysRepo
	invites      *repo.FederationInviteRepo
	fedInstances *repo.FederatedInstanceRepo
	cipher       *crypto.TokenCipher
	instanceURL  string

	// Joiner-side collaborators (Federation v1 F2.2). sender delivers the signed
	// handshake to the owner; fetch resolves a peer's .well-known; peerKeys is the
	// shared peer-key cache warmed on a successful join (US-2.2 AC6).
	sender   HandshakeSender
	fetch    peerkeys.Fetcher
	peerKeys *peerkeys.Cache
	now      func() time.Time

	// Snapshot collaborators (Federation v1 F2.3). Wired additively by
	// WithSnapshotDeps; nil until the snapshot endpoint is mounted.
	snap *snapshotDeps

	// syncStore is the outbox/inbox store (Federation v1 F3.2). Wired additively
	// by WithSyncStore so ListPeers can report the real per-peer pending-delivery
	// count (US-3.2 AC4). nil → pending delivery is reported as 0.
	syncStore *store.Store

	// statusNotifier publishes a ScopeFederation SSE when a project's sync status
	// transitions (Federation v1 F4.3, US-4.3). Wired additively by
	// WithStatusNotifier; nil → no SSE on transition (headless / test).
	statusNotifier StatusNotifier

	// resumeFlush wakes the outbox publisher so a resumed peer's accumulated events
	// flush promptly rather than on the next safety-net tick (Federation v1 F5.3,
	// US-6.1 AC2). Wired additively by WithResumeFlush (the worker's Ping); nil → no
	// immediate flush (the next tick still drains, so correctness is unaffected).
	resumeFlush func()

	// revokeSender delivers the signed federation_revoke control event directly to
	// the peer being revoked (Federation v1 F5.4, US-6.2 AC1), special-cased past
	// the publisher's revoked-skip fan-out filter. Wired additively by
	// WithRevokeSender (the publisher's Push). nil → the revoke event is still
	// enqueued durably in the outbox (crash-safe) and the peer self-detects the
	// revoke on its next sync (the 403 federation_revoked, US-6.2 AC4).
	revokeSender RevokeSender

	// leaveSender delivers the signed federation_leave control event directly to the
	// OWNER when this instance voluntarily leaves a joined project (Federation v1
	// F5.5, US-6.3 AC1). It mirrors revokeSender: the project is marked lost in the
	// same tx so the fan-out (PeersForProject) skips it, and this direct push is the
	// point-to-point delivery to the owner. Wired additively by WithLeaveSender (the
	// publisher's Push). nil → the leave event is still enqueued durably in the
	// outbox; it flushes when delivery is next attempted (best-effort).
	leaveSender LeaveSender

	// incidents is the append-only key-change security-incident log (Federation v1
	// F5.6b, US-6.4 AC2/AC3). Wired additively by WithTrustKeyDeps; nil → the
	// manual trust-key action cannot run (TrustPeerKey reports ErrKeyMissing) and the
	// inbox path simply does not record incident history (the sticky key_mismatch
	// marker still flips the badge red). The cache + fetcher TrustPeerKey needs are
	// the shared join-deps collaborators (peerKeys / fetch).
	incidents *repo.FederationSecurityIncidentRepo

	// auditor records the security-relevant control-plane trust actions to the
	// federation audit log (Federation v1 F6.3, US-7.4 AC1): handshake accepted,
	// peer revoked, key manually trusted, and a detected key change. Wired additively
	// by WithAuditor; nil → those actions are not audited (the action itself is
	// unaffected). Recording is non-blocking.
	auditor AuditRecorder

	// auditLog is the read side of the audit log (Federation v1 F6.3, US-7.4 AC1/
	// AC3): the JWT owner audit view lists rows through it, and SignatureFailureAlerts
	// aggregates recent signature failures per peer. Wired additively by
	// WithAuditReader; nil → Audit returns an empty list and there are no alerts.
	auditLog            *repo.FederationAuditLogRepo
	auditAlertThreshold int
	auditAlertWindow    time.Duration
}

// AuditRecorder is the non-blocking audit sink the service records control-plane
// trust actions to (Federation v1 F6.3, US-7.4 AC1). It is satisfied by
// *audit.Writer; kept as a local interface taking repo.AuditEntry so the service
// holds no hard dependency on the audit package.
type AuditRecorder interface {
	Record(e repo.AuditEntry)
}

// WithAuditor wires the federation audit recorder so security-relevant trust
// actions (handshake/revoke/trust-key/key-change) write one audit row each
// (Federation v1 F6.3, US-7.4 AC1). It returns the service for chaining. A nil
// recorder leaves those actions un-audited without affecting their behaviour.
func (s *Service) WithAuditor(rec AuditRecorder) *Service {
	s.auditor = rec
	return s
}

// WithAuditReader wires the audit-log read side so the JWT owner audit view can
// list rows (US-7.4 AC1) and SignatureFailureAlerts can flag a peer under attack
// (US-7.4 AC3). threshold is the signature-failure count within window that trips
// an alert. It returns the service for chaining. A nil repo leaves Audit returning
// an empty list and produces no alerts.
func (s *Service) WithAuditReader(auditLog *repo.FederationAuditLogRepo, threshold int, window time.Duration) *Service {
	s.auditLog = auditLog
	s.auditAlertThreshold = threshold
	s.auditAlertWindow = window
	return s
}

// recordAudit emits one audit row for a control-plane trust action, stamping the
// service clock when none is provided. It is a no-op when no auditor is wired and
// must only ever carry a NON-SENSITIVE detail (no secrets/signatures/tokens).
func (s *Service) recordAudit(kind repo.AuditKind, outcome repo.AuditOutcome, peerURL, detail string) {
	if s.auditor == nil {
		return
	}
	s.auditor.Record(repo.AuditEntry{
		Kind:            kind,
		Outcome:         outcome,
		PeerInstanceURL: peerURL,
		Detail:          detail,
		CreatedAt:       s.now(),
	})
}

// RevokeSender delivers a batch of canonical signed event payloads to a peer's
// /federation/events endpoint (Federation v1 F5.4, US-6.2 AC1). It is satisfied
// by *Publisher.Push; kept as an interface alias so the revoke path holds no
// direct dependency on the worker wiring and stays unit-testable.
type RevokeSender func(ctx context.Context, peerURL string, payloads []string) error

// LeaveSender delivers a batch of canonical signed event payloads to the OWNER's
// /federation/events endpoint when this instance voluntarily leaves a joined
// project (Federation v1 F5.5, US-6.3 AC1). Like RevokeSender it is satisfied by
// *Publisher.Push and kept as an interface alias so the leave path stays
// unit-testable and holds no direct dependency on the worker wiring.
type LeaveSender func(ctx context.Context, peerURL string, payloads []string) error

// WithRevokeSender wires the direct revoke-event delivery hook so a revoked peer
// receives its federation_revoke point-to-point even though the fan-out skips
// revoked peers (Federation v1 F5.4, US-6.2 AC1). It returns the service for
// chaining. A nil sender is tolerated: the revoke event is still durably enqueued
// and the peer self-detects the revoke on its next sync (US-6.2 AC4).
func (s *Service) WithRevokeSender(send RevokeSender) *Service {
	s.revokeSender = send
	return s
}

// WithLeaveSender wires the direct leave-event delivery hook so the OWNER receives
// the federation_leave point-to-point even though the joiner has already marked its
// copy lost (Federation v1 F5.5, US-6.3 AC1). It returns the service for chaining.
// A nil sender is tolerated: the leave event is still durably enqueued in the outbox
// and flushes on the next delivery attempt.
func (s *Service) WithLeaveSender(send LeaveSender) *Service {
	s.leaveSender = send
	return s
}

// WithResumeFlush wires the outbox publisher's wake-up hook so resuming a paused
// peer flushes its accumulated events immediately rather than on the next tick
// (Federation v1 F5.3, US-6.1 AC2). It returns the service for chaining. A nil
// hook is tolerated (the next safety-net drain still delivers).
func (s *Service) WithResumeFlush(flush func()) *Service {
	s.resumeFlush = flush
	return s
}

// WithSyncStore wires the federation sync store so ListPeers can surface the
// real per-peer pending-delivery count (Federation v1 F3.2, US-3.2 AC4).
// Returns the service for chaining.
func (s *Service) WithSyncStore(st *store.Store) *Service {
	s.syncStore = st
	return s
}

// WithTrustKeyDeps wires the security-incident log so the manual "Trust new key"
// action (Federation v1 F5.6b, US-6.4 AC3) can resolve incidents and so the inbox
// path can record them on a key-change (AC2). It returns the service for chaining.
// The peer-key cache + .well-known fetcher TrustPeerKey also needs are the shared
// join-deps collaborators wired by WithJoinDeps. A nil repo leaves the trust-key
// action disabled (TrustPeerKey reports ErrKeyMissing) without affecting the
// sticky key_mismatch marker.
func (s *Service) WithTrustKeyDeps(incidents *repo.FederationSecurityIncidentRepo) *Service {
	s.incidents = incidents
	return s
}

// WithJoinDeps wires the joiner-side handshake collaborators onto the service
// (Federation v1 F2.2). It returns the same *Service for chaining. now may be
// nil (defaults to time.Now); the others are required for Join/Preview to run.
func (s *Service) WithJoinDeps(sender HandshakeSender, fetch peerkeys.Fetcher, cache *peerkeys.Cache, now func() time.Time) *Service {
	s.sender = sender
	s.fetch = fetch
	s.peerKeys = cache
	if now == nil {
		now = time.Now
	}
	s.now = now
	return s
}

// NewService constructs the federation service. cipher must be non-nil for the
// keypair to be generatable; callers that have not configured FEDERATION_KEY
// pass a nil cipher (or do not construct the service at all), in which case
// EnableForProject reports ErrKeyMissing.
func NewService(
	database *sql.DB,
	projects *repo.ProjectRepo,
	fedProjects *repo.FederatedProjectRepo,
	keys *repo.FederationKeysRepo,
	invites *repo.FederationInviteRepo,
	fedInstances *repo.FederatedInstanceRepo,
	cipher *crypto.TokenCipher,
	instanceURL string,
) *Service {
	return &Service{
		db:           database,
		projects:     projects,
		fedProjects:  fedProjects,
		keys:         keys,
		invites:      invites,
		fedInstances: fedInstances,
		cipher:       cipher,
		instanceURL:  instanceURL,
		now:          time.Now,
	}
}

// defaultProtocolVersion is the protocol version stamped on a freshly enabled
// project's self-row. The owner advertises the highest version it speaks; peers
// re-negotiate at handshake (F0.4 / F2.2).
func defaultProtocolVersion() int {
	best := 0
	for _, v := range protocol.SupportedProtocolVersions {
		if v > best {
			best = v
		}
	}
	if best == 0 {
		return 1
	}
	return best
}

// EnableForProject turns on federation for a single owner-local project
// (Federation v1 F1.1, US-1.1). It is idempotent: enabling an already-federated
// project re-runs cleanly without duplicating the self-row (US-1.1 AC1).
//
// Steps:
//  1. Ensure this instance has a federation keypair (US-1.1 AC4). Without a
//     configured cipher this returns ErrKeyMissing → CodeFederationKeyMissing.
//  2. In ONE transaction: flip projects.is_federated and upsert the owner's
//     is_owner=1 self-row in federated_projects, so the flag and the mapping can
//     never diverge. A missing/tombstoned project rolls back as ErrProjectNotFound.
//
// Enabling only flips the flag and records ownership; the project is not
// actually syncable until the Phase 3 sync core lands.
func (s *Service) EnableForProject(ctx context.Context, projectID int64) (*model.Project, error) {
	if s.cipher == nil {
		return nil, ErrKeyMissing
	}
	// Lazy-generate the keypair first (a one-shot INSERT OR IGNORE). This makes
	// "enabling federation guarantees the instance has keys" true (US-1.1 AC4)
	// and does not hold the connection across network I/O.
	if _, err := s.keys.Ensure(ctx, s.cipher, defaultInstanceDisplayName(s.instanceURL)); err != nil {
		return nil, fmt.Errorf("ensure federation keys: %w", err)
	}

	err := db.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		n, err := s.projects.SetFederatedTx(ctx, tx, projectID, true)
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrProjectNotFound
		}
		// Owner self-row: peer_instance_url == origin_instance_url == this
		// instance's URL, is_owner=1, admin permission over its own project.
		return s.fedProjects.UpsertSelfRowTx(ctx, tx, model.FederatedProject{
			LocalProjectID:    projectID,
			PeerInstanceURL:   s.instanceURL,
			RemoteProjectID:   "",
			IsOwner:           true,
			OriginInstanceURL: s.instanceURL,
			Permissions:       model.FederationPermissionAdmin,
			ProtocolVersion:   defaultProtocolVersion(),
			JoinedAt:          time.Now(),
		})
	})
	if err != nil {
		return nil, err
	}

	p, err := s.projects.Get(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("reload project: %w", err)
	}
	return p, nil
}

// EnsureKeys lazily generates this instance's federation keypair (a one-shot
// INSERT OR IGNORE, US-1.1 AC4) and returns its stable install node_id. It is the
// startup hook that lets main.go build the HLC store + Emitter eagerly (the HLC
// node_id must exist before the first federated mutation can be stamped), using
// the same default display name the enable path seeds so the .well-known and the
// keys row agree. Idempotent: an already-stored keypair is returned unchanged.
func (s *Service) EnsureKeys(ctx context.Context) (string, error) {
	if s.cipher == nil {
		return "", ErrKeyMissing
	}
	fk, err := s.keys.Ensure(ctx, s.cipher, defaultInstanceDisplayName(s.instanceURL))
	if err != nil {
		return "", fmt.Errorf("ensure federation keys: %w", err)
	}
	return fk.NodeID, nil
}

// trimSlash strips trailing slashes from a federation instance/base URL so a
// composed path is never double-slashed. Shared by the handshake snapshot URL
// and the join outbound URL builders.
func trimSlash(u string) string {
	return strings.TrimRight(u, "/")
}

// defaultInstanceDisplayName derives the fallback instance display name from the
// host of instanceURL (R24 — users has no display_name). It only seeds the
// federation_keys row on first creation; an already-stored name is never
// overwritten (FederationKeysRepo.Ensure is INSERT OR IGNORE). Falls back to the
// raw URL if it cannot be parsed. Mirrors handlers.defaultDisplayName so the
// enable path and the .well-known path agree on the seeded name.
func defaultInstanceDisplayName(instanceURL string) string {
	u, err := url.Parse(instanceURL)
	if err != nil || u.Host == "" {
		return strings.TrimSpace(instanceURL)
	}
	return u.Host
}

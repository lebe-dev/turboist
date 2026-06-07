package federation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// ProjectSyncStatus is the federation sync-status of one shared project for the
// owner UI indicator (Federation v1 F4.3, US-4.3). Status is the rolled-up
// SyncStatus (synced/pending/unreachable/key_mismatch); the companion fields name
// the offending peer / count so the badge tooltip can render "peer X unreachable"
// or "N changes pending" without a second round-trip.
type ProjectSyncStatus struct {
	LocalProjectID int64
	Status         model.SyncStatus
	// PendingCount is how many undelivered outbox EVENTS (changes) are overdue
	// (>SyncStatusPendingAfter, owed to at least one active peer); an event owed to
	// several peers counts once. 0 unless Status is pending. This counts changes, not
	// peers — matching the "N changes pending" badge / DTO / API.md / i18n wording.
	PendingCount int
	// UnreachablePeer is the instance_url of a peer not contacted in >24h; empty
	// unless Status is unreachable.
	UnreachablePeer string
	// KeyMismatchPeer is the instance_url of a peer whose signature stopped
	// validating; empty unless Status is key_mismatch.
	KeyMismatchPeer string
}

// StatusNotifier publishes a ScopeFederation SSE when a project's sync status
// transitions, so open tabs reload their badge (Federation v1 F4.3, US-4.3). It
// is satisfied by *HubNotifier (via NotifyFederation). nil → headless / test.
type StatusNotifier interface {
	NotifyFederation(ctx context.Context)
}

// WithStatusNotifier wires the SSE notifier the key-mismatch transition fires
// (Federation v1 F4.3). Returns the service for chaining. nil leaves the service
// headless (no SSE on transition).
func (s *Service) WithStatusNotifier(n StatusNotifier) *Service {
	s.statusNotifier = n
	return s
}

// Status computes the per-project federation sync status for every federation-
// enabled (owner self-row) project (Federation v1 F4.3, US-4.3). It is the
// JWT-only owner read backing GET /api/v1/federation/status. The status is purely
// SERVER-READ — there is no client outbox — derived from durable signals:
//
//   - key_mismatch (red, sticky): any peer has a key_mismatch_at marker (US-4.3
//     AC4) — its events are being dropped until an operator re-trusts the key;
//   - unreachable (orange): any peer not contacted in >PeerStaleAfter (US-4.3 AC3);
//   - pending (yellow): any undelivered outbox event older than
//     SyncStatusPendingAfter (US-4.3 AC2);
//   - synced (green): none of the above (US-4.3 AC1).
//
// Connection discipline: every read runs on the store/repo's own connection; no
// network I/O is performed here (R1).
func (s *Service) Status(ctx context.Context) ([]ProjectSyncStatus, error) {
	ids, err := s.fedProjects.ListOwnedFederatedProjectIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list owned federated projects: %w", err)
	}

	now := s.now()
	out := make([]ProjectSyncStatus, 0, len(ids))
	for _, pid := range ids {
		ps, err := s.projectStatus(ctx, pid, now)
		if err != nil {
			return nil, err
		}
		out = append(out, ps)
	}
	return out, nil
}

// projectStatus derives one project's sync status from its peer health + the
// overdue-pending outbox signal. Status precedence is enforced by
// model.DeriveSyncStatus (key_mismatch > unreachable > pending > synced).
func (s *Service) projectStatus(ctx context.Context, pid int64, now time.Time) (ProjectSyncStatus, error) {
	peers, err := s.fedProjects.ListPeerHealthByProject(ctx, pid)
	if err != nil {
		return ProjectSyncStatus{}, fmt.Errorf("peer health for %d: %w", pid, err)
	}

	out := ProjectSyncStatus{LocalProjectID: pid}

	var keyMismatch, unreachable bool
	// Only non-revoked, non-paused peers count toward unreachable/key_mismatch:
	// a revoked or paused peer is intentionally not synced and must not flip the
	// badge (it has its own peer-status surface in the peers table).
	activeURLs := make([]string, 0, len(peers))
	for _, p := range peers {
		if p.Revoked || p.Paused {
			continue
		}
		activeURLs = append(activeURLs, p.PeerInstanceURL)
		if p.KeyMismatchAt != "" && !keyMismatch {
			keyMismatch = true
			out.KeyMismatchPeer = p.PeerInstanceURL
		}
		if p.LastContactAt == nil || now.Sub(*p.LastContactAt) > model.PeerStaleAfter {
			if !unreachable {
				unreachable = true
				out.UnreachablePeer = p.PeerInstanceURL
			}
		}
	}

	// Overdue undelivered outbox events (>SyncStatusPendingAfter) for any active
	// peer mark the project pending. Without a sync store wired, pending is never
	// reported (the wire shape stays stable for a federation-off build).
	var pending bool
	if s.syncStore != nil && len(activeURLs) > 0 {
		cutoff := model.FormatUTC(now.Add(-model.SyncStatusPendingAfter))
		overdue, err := s.syncStore.OverduePendingCount(ctx, pid, cutoff, activeURLs)
		if err != nil {
			return ProjectSyncStatus{}, fmt.Errorf("overdue pending for %d: %w", pid, err)
		}
		out.PendingCount = overdue
		pending = overdue > 0
	}

	out.Status = model.DeriveSyncStatus(keyMismatch, unreachable, pending)
	return out, nil
}

// MarkKeyMismatchByRemote stamps the sticky key-mismatch marker when an inbound
// event from peerURL fails its per-event signature check (Federation v1 F4.3,
// US-4.3 AC4 — the inbox-signature-check writer). The event carries the cross-
// instance projectClientID; this resolves it to the local int64 project the peer
// is mapped to, then delegates to MarkPeerKeyMismatch (sticky, SSE-on-transition).
// It is best-effort and non-blocking: a rejected event is ALREADY dropped with
// zero rows by the validator, so a failure to record the marker (unknown project,
// DB hiccup) must NOT change the rejection — it is logged by the caller and the
// event stays rejected. An unresolvable projectClientID is a silent no-op (a
// probe for a project this instance does not hold has no peer row to mark).
func (s *Service) MarkKeyMismatchByRemote(ctx context.Context, peerURL, projectClientID string) error {
	if projectClientID == "" || peerURL == "" {
		return nil
	}
	var localID int64
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM projects WHERE client_id = ? AND is_federated = 1 AND deleted_at IS NULL`,
		projectClientID).Scan(&localID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // no local project for this remote id — nothing to mark.
	}
	if err != nil {
		return fmt.Errorf("resolve local project %q: %w", projectClientID, err)
	}
	if _, err := s.MarkPeerKeyMismatch(ctx, localID, peerURL); err != nil {
		return err
	}
	// Open a durable security incident alongside the sticky marker (Federation v1
	// F5.6b, US-6.4 AC2). The incident is the append-only history the trust-key audit
	// relies on and survives the marker's clear; recording it is idempotent while an
	// incident is open (the partial-unique index pins one open row per (project,
	// peer)), so a flood of rejected events under one rotation records a single row.
	// It is best-effort: a failure to record the incident must NOT change the
	// rejection (the event is already dropped with zero rows by the validator).
	if s.incidents != nil {
		oldKey := ""
		inst, gerr := s.fedInstances.Get(ctx, peerURL)
		switch {
		case gerr == nil && inst != nil:
			oldKey = inst.PublicKey
		case errors.Is(gerr, repo.ErrNotFound):
			// No directory row for this peer — the incident's old_key is legitimately
			// blank (e.g. a probe before handshake). Not an error.
		case gerr != nil:
			// A real DB fault dropped the incident's old_key audit field. Keep the
			// graceful blank-key fallback (the incident/marker must still be recorded —
			// the rejection already happened) but surface the loss at WARN so the
			// missing audit field is diagnosable rather than silently swallowed.
			logging.FromContext(ctx).WarnContext(ctx, "federation: read peer key for key-change incident failed, recording blank old_key",
				slog.String("op", "service.Federation.MarkKeyMismatchByRemote"),
				slog.Int64("project_id", localID),
				slog.String("peer", peerURL),
				slog.String("err", gerr.Error()),
			)
		}
		opened, rerr := s.incidents.RecordKeyChange(ctx, localID, peerURL, oldKey, s.now())
		if rerr != nil {
			return fmt.Errorf("record key-change incident: %w", rerr)
		}
		// Audit the key change once, on the transition that OPENS a fresh incident
		// (Federation v1 F6.3, US-7.4 AC1) — so a flood of rejected events under one
		// rotation records a single audit row, mirroring the incident's idempotency.
		if opened {
			s.recordAudit(repo.AuditKindKeyChange, repo.AuditOutcomeRejected, peerURL, "peer key changed — events rejected")
		}
	}
	return nil
}

// MarkPeerKeyMismatch stamps the sticky key-mismatch marker on a (project, peer)
// the first time that peer's inbound event signature stops validating (Federation
// v1 F4.3, US-4.3 AC4 — the signal the inbox signature check writes). It is the
// only writer of the red, sticky status; the marker is never auto-cleared here
// (manual trust-key clears it in F5.6b). On the TRANSITION (NULL → set) it
// publishes a ScopeFederation SSE so the owner's open tabs flip the badge red. It
// returns whether the row transitioned so callers (and tests) can assert the
// once-only semantics. A mismatch on a project/peer with no row is a silent no-op.
func (s *Service) MarkPeerKeyMismatch(ctx context.Context, localProjectID int64, peerInstanceURL string) (bool, error) {
	at := model.FormatUTC(s.now())
	transitioned, err := s.fedProjects.MarkKeyMismatch(ctx, localProjectID, peerInstanceURL, at)
	if err != nil {
		return false, fmt.Errorf("mark key mismatch: %w", err)
	}
	if transitioned && s.statusNotifier != nil {
		s.statusNotifier.NotifyFederation(ctx)
	}
	return transitioned, nil
}

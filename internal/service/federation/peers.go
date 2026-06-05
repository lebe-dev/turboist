package federation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// PeerView is one row of the peers list (Federation v1 F1.4, US-1.4). It carries
// the peer's federation identity (PeerInstanceURL) + handshake-supplied
// DisplayName (so the UI renders "display_name @ instance.tld", US-1.4 AC2), the
// per-project permissions, the last-sent HLC cursor, the last successful contact
// time, the derived collaboration Status (US-1.4 AC1/AC3), and PendingDelivery —
// the count of events queued for this peer.
//
// PendingDelivery is wired but always 0 in F1.4: the outbox does not land until
// Phase 3, so US-1.4 AC4 is satisfied only partially (the field is present and
// zero) until the publisher exists.
type PeerView struct {
	PeerInstanceURL string
	DisplayName     string
	Permissions     model.FederationPermission
	Status          model.PeerStatus
	LastSentHLC     string
	LastContactAt   *time.Time
	JoinedAt        time.Time
	PendingDelivery int
	// KeyMismatchAt is the sticky timestamp of a detected peer key CHANGE
	// (Federation v1 F5.6b, US-6.4 AC2). Non-empty → the UI renders the key-rotation
	// incident alert + a "Trust new key" action; empty in the healthy case.
	KeyMismatchAt string
}

// ListPeers returns every remote peer joined to a project, each with its
// handshake-supplied display_name and derived collaboration status (Federation v1
// F1.4, US-1.4). The owner self-row is excluded (US-1.4 AC1). The project must
// exist, else ErrProjectNotFound (→404). Status is derived via the single
// canonical model.DerivePeerStatus helper (revoked > paused > stale(>24h) >
// active, US-1.4 AC3). PendingDelivery is 0 until the Phase-3 outbox lands
// (US-1.4 AC4 partial).
//
// A genuinely-missing project maps to ErrProjectNotFound (→404); any other
// Get failure (DB unavailable, scan/hydration error) is wrapped and surfaced so
// the handler returns 500 instead of masking infrastructure faults as 404.
func (s *Service) ListPeers(ctx context.Context, projectID int64) ([]PeerView, error) {
	if _, err := s.projects.Get(ctx, projectID); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("load project: %w", err)
	}

	rows, err := s.fedProjects.ListPeersByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list peers: %w", err)
	}

	now := time.Now()
	out := make([]PeerView, 0, len(rows))
	for _, p := range rows {
		// PendingDelivery is the count of outbox events not yet delivered to this
		// peer (US-3.2 AC4 — the overdue signal). It is 0 when no sync store is
		// wired (the wire shape stays stable for a federation-off build).
		pending := 0
		if s.syncStore != nil {
			n, err := s.syncStore.PendingDeliveryCount(ctx, projectID, p.PeerInstanceURL)
			if err != nil {
				return nil, fmt.Errorf("pending delivery count for %q: %w", p.PeerInstanceURL, err)
			}
			pending = n
		}
		out = append(out, PeerView{
			PeerInstanceURL: p.PeerInstanceURL,
			DisplayName:     p.DisplayName,
			Permissions:     p.Permissions,
			Status:          model.DerivePeerStatus(p.Revoked, p.Paused, lostReasonOf(p), p.LastContactAt, now),
			LastSentHLC:     p.LastSentHLC,
			LastContactAt:   p.LastContactAt,
			JoinedAt:        p.JoinedAt,
			PendingDelivery: pending,
			KeyMismatchAt:   p.KeyMismatchAt,
		})
	}
	return out, nil
}

// lostReasonOf returns the peer mapping's lost-reason ONLY when the row is
// actually lost, so a stale lost_reason left over from a cleared marker can never
// be read as a live status (Federation v1 F5.5, US-6.3 AC2). The owner's peer row
// for a voluntarily-left peer carries lost=1, reason="left" → PeerStatusLeft.
func lostReasonOf(p repo.FederatedPeer) model.FederationLostReason {
	if !p.Lost {
		return model.FederationLostNone
	}
	return p.LostReason
}

// PausePeer temporarily pauses exchange with one peer of a project without
// breaking the trust link (Federation v1 F5.3, US-6.1 AC1). It flips paused=true
// on the (project, peer) mapping; the outbox worker then skips the peer
// (PeersForProject), so its events accumulate in federation_outbox, and the
// signed event/pull endpoints reject the peer's inbound traffic with 403
// federation_paused. It is non-destructive (distinct from revoke) and idempotent.
// A genuinely-missing project → ErrProjectNotFound (404); an unknown peer →
// ErrPeerNotFound (404).
func (s *Service) PausePeer(ctx context.Context, projectID int64, peerInstanceURL string) error {
	return s.setPeerPaused(ctx, projectID, peerInstanceURL, true)
}

// ResumePeer un-pauses a previously paused peer (Federation v1 F5.3, US-6.1 AC2).
// It flips paused=false on the (project, peer) mapping, then fires the resume-
// flush hook (the outbox worker's wake-up) so the events that accumulated while
// paused are pushed promptly rather than on the next safety-net tick. The hook is
// best-effort: a nil hook still converges on the next drain. Same not-found
// mapping as PausePeer.
//
// A REVOKED peer cannot be resumed (Federation v1 F5.4, US-6.2 AC5): revoke is
// terminal and irreversible, so a resume on a revoked peer returns ErrPeerRevoked
// (→403) WITHOUT clearing paused — re-collaboration requires a fresh invite.
func (s *Service) ResumePeer(ctx context.Context, projectID int64, peerInstanceURL string) error {
	// Guard the irreversible-revoke invariant BEFORE flipping paused: load the peer
	// row; a revoked peer is rejected so a resume can never silently re-enable a
	// revoked link (US-6.2 AC5).
	fp, err := s.fedProjects.Get(ctx, projectID, peerInstanceURL)
	if errors.Is(err, repo.ErrNotFound) {
		// Defer the not-found classification to setPeerPaused (it also checks the
		// project exists, so an unknown project is a 404 not a misleading peer-404).
	} else if err != nil {
		return fmt.Errorf("load peer: %w", err)
	} else if fp.Revoked {
		return ErrPeerRevoked
	}
	if err := s.setPeerPaused(ctx, projectID, peerInstanceURL, false); err != nil {
		return err
	}
	// Wake the publisher so the accumulated batch flushes immediately (US-6.1 AC2).
	// The drain reads the peer's still-undelivered events (delivered_to was never
	// stamped while paused) from last_sent_hlc forward and pushes them.
	if s.resumeFlush != nil {
		s.resumeFlush()
	}
	return nil
}

// RevokePeer permanently revokes one peer's access to a project (Federation v1
// F5.4, US-6.2). In ONE transaction it: (1) verifies the project exists, (2)
// flips revoked=1 on the (project, peer) mapping (US-6.2 AC1), and (3) enqueues a
// signed federation_revoke control event into federation_outbox, pre-stamped
// delivered to every OTHER peer (point-to-point to the revoked peer). After commit
// it delivers the event DIRECTLY to the now-revoked peer once (special-cased past
// the publisher's revoked-skip fan-out filter, US-6.2 AC1) and stamps it delivered
// on success. Revoke is IRREVERSIBLE (US-6.2 AC5) — there is no un-revoke; the
// peer self-marks federation_lost on receipt (US-6.2 AC3) or on its next rejected
// sync if it was offline (US-6.2 AC4). A missing project → ErrProjectNotFound
// (404); an unknown peer → ErrPeerNotFound (404). It is idempotent: re-revoking an
// already-revoked peer re-sends the same (deduped) event.
func (s *Service) RevokePeer(ctx context.Context, projectID int64, peerInstanceURL string) error {
	proj, err := s.projects.Get(ctx, projectID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return ErrProjectNotFound
		}
		return fmt.Errorf("load project: %w", err)
	}
	if _, err := s.fedProjects.Get(ctx, projectID, peerInstanceURL); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return ErrPeerNotFound
		}
		return fmt.Errorf("load peer: %w", err)
	}

	// Build + sign the federation_revoke control event up front (no DB access): it
	// targets the project (entity_id = project client_id), carries no per-field LWW,
	// and is signed by this instance like any event so the joiner verifies it
	// end-to-end (F3.2a). A missing keypair is ErrKeyMissing.
	revokeEvt, payload, err := s.buildRevokeEvent(ctx, proj.ClientID)
	if err != nil {
		return err
	}

	// Resolve the OTHER peers so the control event is pre-stamped delivered to them
	// — it is point-to-point to the revoked peer and must never fan out elsewhere.
	others, err := s.otherPeerURLs(ctx, projectID, peerInstanceURL)
	if err != nil {
		return err
	}

	var outboxID int64
	if s.syncStore != nil {
		nowStr := model.FormatUTC(s.now())
		err = db.WithTx(ctx, s.db, func(tx *sql.Tx) error {
			n, ferr := s.fedProjects.RevokeTx(ctx, tx, projectID, peerInstanceURL)
			if ferr != nil {
				return ferr
			}
			if n == 0 {
				return ErrPeerNotFound
			}
			id, ierr := s.syncStore.InsertControlOutboxTx(ctx, tx, revokeEvt.EventID, projectID, payload, others, nowStr)
			if ierr != nil {
				return ierr
			}
			outboxID = id
			return nil
		})
	} else {
		// No sync store wired (a federation-off build / unit harness): flip the flag
		// only. The revoke still takes effect (the peer is rejected on any inbound).
		var n int
		n, err = s.fedProjects.Revoke(ctx, projectID, peerInstanceURL)
		if err == nil && n == 0 {
			err = ErrPeerNotFound
		}
	}
	if err != nil {
		return err
	}

	// Audit the revoke (Federation v1 F6.3, US-7.4 AC1): a control-plane trust
	// action that succeeded. Recorded once after the revoke has taken effect,
	// regardless of whether the direct delivery below reaches the peer.
	s.recordAudit(repo.AuditKindRevoke, repo.AuditOutcomeAccepted, peerInstanceURL, "peer revoked")

	// Deliver the revoke directly to the now-revoked peer once (US-6.2 AC1). The
	// peer is revoked in the same tx above, so the normal fan-out (PeersForProject)
	// will never reach it — this direct push is the special-case. Best-effort: on
	// failure (peer offline, US-6.2 AC4) the event stays pending in the outbox and
	// the peer self-detects the revoke on its next sync (the 403 federation_revoked).
	if s.revokeSender != nil {
		if perr := s.revokeSender(ctx, peerInstanceURL, []string{payload}); perr != nil {
			logging.FromContext(ctx).WarnContext(ctx, "federation: direct revoke delivery failed, peer will self-detect on next sync",
				slog.String("op", "federation.RevokePeer"),
				slog.Int64("project_id", projectID),
				slog.String("peer", peerInstanceURL),
				slog.String("err", perr.Error()),
			)
			return nil
		}
		if s.syncStore != nil && outboxID != 0 {
			if merr := s.syncStore.MarkDelivered(ctx, outboxID, peerInstanceURL); merr != nil {
				// The push succeeded; failing to stamp delivered only leaves the row
				// pending (harmless — the peer is revoked and never re-probed by fan-out).
				logging.FromContext(ctx).WarnContext(ctx, "federation: mark revoke delivered failed",
					slog.String("op", "federation.RevokePeer"),
					slog.String("peer", peerInstanceURL),
					slog.String("err", merr.Error()),
				)
			}
		}
	}
	return nil
}

// buildRevokeEvent assembles + signs the federation_revoke control event for a
// project (Federation v1 F5.4, US-6.2 AC1). It targets the project (entity_type =
// project, entity_id = the project's cross-instance client_id), carries no fields,
// and is authored/originated by this instance. A missing keypair → ErrKeyMissing.
func (s *Service) buildRevokeEvent(ctx context.Context, projectClientID string) (events.Event, string, error) {
	if s.cipher == nil {
		return events.Event{}, "", ErrKeyMissing
	}
	fk, err := s.keys.Ensure(ctx, s.cipher, defaultInstanceDisplayName(s.instanceURL))
	if err != nil {
		return events.Event{}, "", fmt.Errorf("ensure federation keys: %w", err)
	}
	priv, _, err := crypto.LoadInstanceKeypair(s.cipher, fk.PublicKey, fk.PrivateSeedEnc)
	if err != nil {
		return events.Event{}, "", fmt.Errorf("load instance keypair: %w", err)
	}
	evt := events.Event{
		EventID:         model.NewClientID(),
		Op:              events.OpRevoke,
		EntityType:      events.EntityProject,
		EntityID:        projectClientID,
		ProjectClientID: projectClientID,
		Author:          s.instanceURL,
		OriginInstance:  s.instanceURL,
		CreatedAt:       model.FormatUTC(s.now()),
		Fields:          map[string]events.Field{},
	}
	signed, err := events.Sign(evt, priv)
	if err != nil {
		return events.Event{}, "", fmt.Errorf("sign revoke event: %w", err)
	}
	payload, err := events.Marshal(signed)
	if err != nil {
		return events.Event{}, "", fmt.Errorf("marshal revoke event: %w", err)
	}
	return signed, string(payload), nil
}

// otherPeerURLs returns the non-owner peer URLs of a project EXCLUDING the given
// peer — the set the federation_revoke control event is pre-stamped delivered to,
// so the point-to-point revoke never fans out to anyone else.
func (s *Service) otherPeerURLs(ctx context.Context, projectID int64, exclude string) ([]string, error) {
	rows, err := s.fedProjects.ListByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list peers for revoke stamp: %w", err)
	}
	out := make([]string, 0, len(rows))
	for _, fp := range rows {
		if fp.IsOwner || fp.PeerInstanceURL == exclude || fp.PeerInstanceURL == s.instanceURL {
			continue
		}
		out = append(out, fp.PeerInstanceURL)
	}
	return out, nil
}

// setPeerPaused is the shared pause/resume body: it verifies the project exists
// (so an unknown project is a 404, not a silent no-op), flips the peer's paused
// flag, and maps a zero affected-row count to ErrPeerNotFound.
func (s *Service) setPeerPaused(ctx context.Context, projectID int64, peerInstanceURL string, paused bool) error {
	if _, err := s.projects.Get(ctx, projectID); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return ErrProjectNotFound
		}
		return fmt.Errorf("load project: %w", err)
	}
	n, err := s.fedProjects.SetPaused(ctx, projectID, peerInstanceURL, paused)
	if err != nil {
		return fmt.Errorf("set peer paused: %w", err)
	}
	if n == 0 {
		return ErrPeerNotFound
	}
	return nil
}

package federation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// LeaveProject voluntarily leaves a JOINED federated project (Federation v1 F5.5,
// US-6.3). It is the joiner-side, symmetric counterpart of RevokePeer: in ONE
// transaction it (1) marks the local copy federation_lost with reason="left" — a
// plain, editable local project with no further outbound sync (US-6.3 AC1/AC3) —
// and (2) enqueues a signed federation_leave control event into federation_outbox,
// pre-stamped delivered to every OTHER peer (in v1 a joiner has none, so the owner
// is the only pending target). After commit it delivers the event DIRECTLY to the
// owner once via the wired LeaveSender (the fan-out skips the now-lost project, so
// this direct push is how the owner learns, US-6.3 AC2). It is IDEMPOTENT and a
// no-op when the copy is already lost (leave-after-revoke does nothing, US-6.3):
// the reason is not overwritten and no second leave is enqueued. A missing project
// → ErrProjectNotFound; the owner's OWN project (or a non-federated project) →
// ErrNotJoined.
//
// When deleteLocal is true the local copy is ALSO soft-deleted (cascading to its
// tasks + sections) in the SAME transaction that marks it left — the user chose
// "delete" instead of "keep locally" when ending the link. The delete is purely
// LOCAL: it does NOT emit an op=delete tombstone to the owner (a member can never
// delete the owner's project — the owner only learns we LEFT, via the leave event
// which is still sent). This is the reason leave+delete is one server operation
// rather than a leave followed by the ordinary federated DELETE (which would emit).
func (s *Service) LeaveProject(ctx context.Context, projectID int64, deleteLocal bool) error {
	proj, err := s.projects.Get(ctx, projectID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return ErrProjectNotFound
		}
		return fmt.Errorf("load project: %w", err)
	}
	_ = proj // existence + tombstone check above; the leave targets the owner's id.

	// Resolve the joiner mapping (is_owner=0). A missing joiner row means this is the
	// owner's own project or a non-federated project — only a joined copy can be left.
	joiner, err := s.fedProjects.JoinerRow(ctx, projectID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return ErrNotJoined
		}
		return fmt.Errorf("load joiner mapping: %w", err)
	}

	// Already lost (e.g. already left, or the owner revoked us): leaving is a no-op.
	// Leave-after-revoke must not overwrite the terminal reason nor re-enqueue a leave
	// (US-6.3 — idempotent).
	if joiner.Lost {
		return nil
	}

	ownerURL := joiner.OriginInstanceURL
	// The leave targets the OWNER's project client_id (RemoteProjectID) so the owner
	// resolves it against its own projects.client_id; if the joiner never recorded the
	// remote id, fall back to the local project's own client_id (best-effort — a v1
	// joiner row always carries the remote id from the snapshot bootstrap).
	ownerProjectCID := joiner.RemoteProjectID
	if ownerProjectCID == "" {
		ownerProjectCID = proj.ClientID
	}

	// Build + sign the federation_leave control event up front (no DB access): it
	// targets the owner's project, carries no per-field LWW, and is authored/
	// originated by THIS instance (the leaver). A missing keypair is ErrKeyMissing.
	leaveEvt, payload, err := s.buildLeaveEvent(ctx, ownerProjectCID)
	if err != nil {
		return err
	}

	var outboxID int64
	if s.syncStore != nil {
		nowStr := model.FormatUTC(s.now())
		err = db.WithTx(ctx, s.db, func(tx *sql.Tx) error {
			transitioned, ferr := s.fedProjects.MarkLostTx(ctx, tx, projectID, ownerURL, model.FederationLostLeft)
			if ferr != nil {
				return ferr
			}
			if !transitioned {
				// A concurrent mark (already lost) — nothing more to do; the enqueue is
				// skipped so a redelivered leave is not produced. ErrAlreadyLost is mapped
				// to a clean no-op by leaving outboxID at 0.
				return errAlreadyLost
			}
			// "Delete" instead of "keep locally": soft-delete the local copy + cascade to
			// its tasks/sections in this same tx (atomic with the leave). Crucially this is
			// the repo delete (local-only) — NOT the federated emitter — so no op=delete
			// event is produced; the owner only ever learns we left.
			if deleteLocal {
				if derr := s.projects.DeleteTx(ctx, tx, projectID); derr != nil {
					return derr
				}
			}
			// Pre-stamp delivered to every OTHER peer so the leave never fans out beyond
			// the owner. In v1 a joiner has no other peers, so this is empty and only the
			// owner stays pending.
			id, ierr := s.syncStore.InsertControlOutboxTx(ctx, tx, leaveEvt.EventID, projectID, payload, nil, nowStr)
			if ierr != nil {
				return ierr
			}
			outboxID = id
			return nil
		})
		if errors.Is(err, errAlreadyLost) {
			return nil
		}
	} else if deleteLocal {
		// No sync store wired (a federation-off build / unit harness) AND the user chose
		// delete: mark left + local soft-delete cascade in one tx (still no emit).
		err = db.WithTx(ctx, s.db, func(tx *sql.Tx) error {
			transitioned, ferr := s.fedProjects.MarkLostTx(ctx, tx, projectID, ownerURL, model.FederationLostLeft)
			if ferr != nil {
				return ferr
			}
			if !transitioned {
				return errAlreadyLost
			}
			return s.projects.DeleteTx(ctx, tx, projectID)
		})
		if errors.Is(err, errAlreadyLost) {
			return nil
		}
	} else {
		// No sync store wired: flip the marker only. The local copy still becomes an
		// editable local project (keep-locally).
		var transitioned bool
		transitioned, err = s.fedProjects.MarkLost(ctx, projectID, ownerURL, model.FederationLostLeft)
		if err == nil && !transitioned {
			return nil
		}
	}
	if err != nil {
		return err
	}

	// Deliver the leave directly to the owner once (US-6.3 AC2). The project is marked
	// lost in the same tx above, so the normal fan-out (PeersForProject) will never
	// reach the owner — this direct push is how the owner learns. Best-effort: on
	// failure (owner offline) the event stays pending in the outbox and flushes on the
	// next delivery attempt.
	if s.leaveSender != nil {
		if perr := s.leaveSender(ctx, ownerURL, []string{payload}); perr != nil {
			logging.FromContext(ctx).WarnContext(ctx, "federation: direct leave delivery failed, will flush on next attempt",
				slog.String("op", "federation.LeaveProject"),
				slog.Int64("project_id", projectID),
				slog.String("owner", ownerURL),
				slog.String("err", perr.Error()),
			)
			return nil
		}
		if s.syncStore != nil && outboxID != 0 {
			if merr := s.syncStore.MarkDelivered(ctx, outboxID, ownerURL); merr != nil {
				logging.FromContext(ctx).WarnContext(ctx, "federation: mark leave delivered failed",
					slog.String("op", "federation.LeaveProject"),
					slog.String("owner", ownerURL),
					slog.String("err", merr.Error()),
				)
			}
		}
	}
	return nil
}

// errAlreadyLost is an internal sentinel used to roll back the leave tx cleanly
// when a concurrent caller already marked the copy lost (so no duplicate leave is
// enqueued). It never escapes LeaveProject (mapped to a nil no-op).
var errAlreadyLost = errors.New("federation: project already lost")

// buildLeaveEvent assembles + signs the federation_leave control event for a
// joined project (Federation v1 F5.5, US-6.3 AC1). It targets the OWNER's project
// (entity_type = project, entity_id = the owner's project client_id) and is
// authored/originated by THIS instance (the leaving joiner). A missing keypair →
// ErrKeyMissing.
func (s *Service) buildLeaveEvent(ctx context.Context, ownerProjectCID string) (events.Event, string, error) {
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
		Op:              events.OpLeave,
		EntityType:      events.EntityProject,
		EntityID:        ownerProjectCID,
		ProjectClientID: ownerProjectCID,
		Author:          s.instanceURL,
		OriginInstance:  s.instanceURL,
		CreatedAt:       model.FormatUTC(s.now()),
		Fields:          map[string]events.Field{},
	}
	signed, err := events.Sign(evt, priv)
	if err != nil {
		return events.Event{}, "", fmt.Errorf("sign leave event: %w", err)
	}
	payload, err := events.Marshal(signed)
	if err != nil {
		return events.Event{}, "", fmt.Errorf("marshal leave event: %w", err)
	}
	return signed, string(payload), nil
}

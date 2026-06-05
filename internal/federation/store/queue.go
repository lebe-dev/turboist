package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lebe-dev/turboist/internal/logging"
)

// normInstance normalizes a federation instance/base URL for the delivered_to
// echo-loop guard by trimming trailing slashes, mirroring the service layer's
// trimSlash. Without it, an origin stamped as "https://a.example/" would not
// match a peer mapping URL of "https://a.example" (or vice-versa), so the
// publisher would re-push a relayed event back to its origin — an echo loop.
func normInstance(u string) string {
	return strings.TrimRight(u, "/")
}

// ReBroadcastOutboxTx re-enqueues a relayed peer event into the transactional
// outbox so the owner can fan it out to the OTHER peers (Federation v1 F5.1,
// US-5.2 AC2 — owner-hub re-broadcast). It runs INSIDE the inbox-apply tx so the
// re-broadcast row commits or rolls back atomically with the merge (§3 risk:
// "re-enqueue inside inbox-apply tx (single connection)").
//
// The echo-loop guard is the PRE-STAMPED delivered_to: the row is recorded as
// already delivered to originInstance, so the publisher's ListUndeliveredForPeer
// skips the origin and never pushes the event back to where it came from. A
// non-empty originInstance is stamped; an empty origin stamps nobody (the row is
// then pending for every peer — used only defensively).
//
// event_id is the cross-instance dedup key (UNIQUE): re-broadcasting the same
// event_id twice — an at-least-once redelivery — is a no-op on the second write
// (ON CONFLICT DO NOTHING), preserving the first row's delivered_to stamp so a
// peer is never spammed by a duplicate relay (NFR-2 dedup).
func (s *Store) ReBroadcastOutboxTx(ctx context.Context, tx Querier, eventID string, localProjectID int64, payload, originInstance, createdAt string) error {
	deliveredTo := ""
	// Normalize the stamped origin (trailing-slash trim) so it compares equal to a
	// peer mapping URL regardless of slash form — closing the echo loop where a
	// trailing-slash mismatch would let the publisher re-push the relayed event back
	// to its origin.
	if origin := normInstance(originInstance); origin != "" {
		b, err := json.Marshal([]string{origin})
		if err != nil {
			return fmt.Errorf("rebroadcast stamp origin: %w", err)
		}
		deliveredTo = string(b)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO federation_outbox (event_id, local_project_id, payload, delivered_to, created_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(event_id) DO NOTHING`,
		eventID, localProjectID, payload, deliveredTo, createdAt); err != nil {
		return fmt.Errorf("rebroadcast outbox: %w", err)
	}
	return nil
}

// InsertControlOutboxTx writes a point-to-point CONTROL event (e.g. the
// federation_revoke of Federation v1 F5.4, US-6.2 AC1) to the transactional
// outbox and returns the new row id. Unlike InsertOutboxTx it pre-stamps
// delivered_to with everyPeerExceptTarget so the normal fan-out NEVER delivers it
// to anyone but the single target peer — the event is point-to-point. The target
// is deliberately LEFT OUT of delivered_to so it stays "pending" for the target;
// the revoke flow then delivers it directly (special-cased past the fan-out's
// revoked-skip, since the target is revoked in the same tx). event_id is the
// cross-instance dedup key (UNIQUE); a duplicate insert is a no-op and returns the
// existing row id so a retried revoke does not duplicate the control event.
func (s *Store) InsertControlOutboxTx(ctx context.Context, tx Querier, eventID string, localProjectID int64, payload string, deliveredToOthers []string, createdAt string) (int64, error) {
	deliveredTo := ""
	if len(deliveredToOthers) > 0 {
		b, err := json.Marshal(deliveredToOthers)
		if err != nil {
			return 0, fmt.Errorf("control outbox stamp peers: %w", err)
		}
		deliveredTo = string(b)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO federation_outbox (event_id, local_project_id, payload, delivered_to, created_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(event_id) DO NOTHING`,
		eventID, localProjectID, payload, deliveredTo, createdAt); err != nil {
		return 0, fmt.Errorf("insert control outbox: %w", err)
	}
	var id int64
	if err := tx.QueryRowContext(ctx,
		`SELECT id FROM federation_outbox WHERE event_id = ?`, eventID).Scan(&id); err != nil {
		return 0, fmt.Errorf("control outbox id: %w", err)
	}
	return id, nil
}

// InsertOutboxTx writes a canonical signed event to the transactional outbox in
// the SAME tx as the domain write (NFR-2 crash-safety: domain + outbox + field
// HLC commit or roll back together). delivered_to starts empty; the Phase-3
// publisher worker drains it. event_id is the cross-instance dedup key (UNIQUE);
// a duplicate insert is a no-op (the publisher is at-least-once with dedup).
//
// protocolVersion records the wire protocol version the payload was serialised at
// (Federation v1 F6.1 dual-write seam, migration 038). In v1 it is always 1; it
// persists so a future build can down-convert per peer without a schema change. A
// value < 1 falls back to 1 so a caller that does not yet pass the version keeps
// the v1 default the column carries.
func (s *Store) InsertOutboxTx(ctx context.Context, tx Querier, eventID string, localProjectID int64, payload string, protocolVersion int, createdAt string) error {
	if protocolVersion < 1 {
		protocolVersion = 1
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO federation_outbox (event_id, local_project_id, payload, delivered_to, protocol_version, created_at)
		 VALUES (?, ?, ?, '', ?, ?)
		 ON CONFLICT(event_id) DO NOTHING`,
		eventID, localProjectID, payload, protocolVersion, createdAt)
	if err != nil {
		return fmt.Errorf("insert outbox: %w", err)
	}
	return nil
}

// InsertInbox records a received event for dedup + apply (NFR-2). It is
// ON CONFLICT(event_id) DO NOTHING so a duplicate delivery (push + pull, or a
// retried POST) is a no-op (idempotent). The boolean reports whether the row was
// newly inserted (true) or a duplicate (false) so the caller can skip re-apply.
func (s *Store) InsertInbox(ctx context.Context, eventID, peerInstanceURL string, localProjectID int64, payload, receivedAt string) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO federation_inbox (event_id, peer_instance_url, local_project_id, payload, received_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(event_id) DO NOTHING`,
		eventID, peerInstanceURL, nullInt64(localProjectID), payload, receivedAt)
	if err != nil {
		return false, fmt.Errorf("insert inbox: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("insert inbox rows: %w", err)
	}
	return n > 0, nil
}

// MarkInboxAppliedTx stamps applied_at on an inbox row inside the apply tx so the
// dedup log records that the event has reached a TERMINAL state — either merged
// (success) or permanently rejected (poison). A row whose applied_at stays NULL
// is a still-pending event the inbox queue re-drives (transient failure / crash
// before apply); stamping it here is what stops a successfully-applied or
// permanently-rejected event from being re-driven forever (idx_federation_inbox_pending).
func (s *Store) MarkInboxAppliedTx(ctx context.Context, tx Querier, eventID, appliedAt string) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE federation_inbox SET applied_at = ? WHERE event_id = ?`,
		appliedAt, eventID)
	if err != nil {
		return fmt.Errorf("mark inbox applied: %w", err)
	}
	return nil
}

// MarkInboxApplied is MarkInboxAppliedTx on the store's own connection, for the
// terminal-but-not-merged poison case where there is no surrounding apply tx (the
// merge tx rolled back, but the event is permanent and must not be re-driven).
func (s *Store) MarkInboxApplied(ctx context.Context, eventID, appliedAt string) error {
	return s.MarkInboxAppliedTx(ctx, s.db, eventID, appliedAt)
}

// PendingInboxEvent is one received-but-not-yet-applied inbox row (applied_at IS
// NULL) the queue re-drives on startup and on its periodic re-scan tick.
type PendingInboxEvent struct {
	EventID         string
	PeerInstanceURL string
	Payload         string
}

// ListUnappliedInbox returns up to limit inbox rows whose applied_at is still
// NULL, oldest first (received_at asc). It is the recovery read that re-drives
// events whose apply was lost to a transient failure or a crash between the
// dedup INSERT and a successful merge (NFR-2 at-least-once): the POST handler
// records the event, then enqueues it, so an in-memory drop or restart between
// those two would otherwise strand the event forever (a redelivery is deduped
// and not re-enqueued). It rides the partial idx_federation_inbox_pending index.
func (s *Store) ListUnappliedInbox(ctx context.Context, limit int) ([]PendingInboxEvent, error) {
	const op = "store.ListUnappliedInbox"
	if limit <= 0 {
		limit = 500
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT event_id, peer_instance_url, payload
		   FROM federation_inbox
		  WHERE applied_at IS NULL
		  ORDER BY received_at ASC, id ASC
		  LIMIT ?`,
		limit)
	if err != nil {
		return nil, fmt.Errorf("%s query: %w", op, err)
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	out := make([]PendingInboxEvent, 0, limit)
	for rows.Next() {
		var ev PendingInboxEvent
		if err := rows.Scan(&ev.EventID, &ev.PeerInstanceURL, &ev.Payload); err != nil {
			return nil, fmt.Errorf("%s scan: %w", op, err)
		}
		out = append(out, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s rows: %w", op, err)
	}
	return out, nil
}

func nullInt64(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

// Verify Store satisfies the Querier-compatible call sites at compile time.
var _ = func() Querier { return (*sql.Tx)(nil) }

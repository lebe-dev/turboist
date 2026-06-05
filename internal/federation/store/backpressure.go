package store

import (
	"context"
	"fmt"

	"github.com/lebe-dev/turboist/internal/logging"
)

// Outbox backpressure persistence (Federation v1 F4.4, US-4.4 / US-8.3). Two
// durable tables back the publisher's per-peer backpressure so a restart neither
// re-hammers a down/rejecting peer nor loses the record of a permanent failure:
//
//   - federation_dead_letter parks events whose delivery to a specific peer
//     failed PERMANENTLY (a 4xx ≠ 429). They are NOT retried automatically
//     (US-4.4 AC3); they surface in the owner's dead-letter diagnostics view and
//     are EXCLUDED from the per-peer pending-delivery count.
//   - federation_peer_retry persists the per-peer retry gate (not_before /
//     attempt / permanent) so exponential backoff survives a restart.
//
// All reads/writes run on the store's own connection so the publisher can
// release it before any peer network I/O (R1).

// DeadLetterRow is one parked, permanently-failed (peer, event) delivery.
type DeadLetterRow struct {
	// ID is the autoincrement federation_dead_letter.id (newest = highest).
	ID int64
	// EventID is the cross-instance dedup key.
	EventID string
	// PeerURL is the peer the delivery permanently failed for.
	PeerURL string
	// LocalProjectID is the int64 projects.id the event belongs to (0 if unknown).
	LocalProjectID int64
	// Payload is the verbatim canonical signed event bytes that failed.
	Payload string
	// StatusCode is the HTTP status the peer returned (0 for a non-HTTP failure).
	StatusCode int
	// Reason is the federation error code (or a short message) classified.
	Reason string
	// FailedAt is the wall-clock the failure was parked (model.FormatUTC).
	FailedAt string
}

// InsertDeadLetter parks a permanently-failed event for a peer. It is idempotent
// per (peer_instance_url, event_id) (ON CONFLICT DO NOTHING) so re-classifying the
// same event for the same peer never duplicates the row (US-4.4 AC3).
func (s *Store) InsertDeadLetter(ctx context.Context, dl DeadLetterRow) error {
	const op = "store.InsertDeadLetter"
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO federation_dead_letter
		   (event_id, peer_instance_url, local_project_id, payload, status_code, reason, failed_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(peer_instance_url, event_id) DO NOTHING`,
		dl.EventID, dl.PeerURL, nullInt64(dl.LocalProjectID), dl.Payload, dl.StatusCode, dl.Reason, dl.FailedAt)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

// ListDeadLetter returns parked dead-letter rows newest-first (most recent
// failure on top, the diagnostics-view ordering). A non-positive limit defaults
// to a sane cap so the admin endpoint never streams an unbounded list.
func (s *Store) ListDeadLetter(ctx context.Context, limit int) ([]DeadLetterRow, error) {
	const op = "store.ListDeadLetter"
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, event_id, peer_instance_url, COALESCE(local_project_id, 0), payload, status_code, reason, failed_at
		   FROM federation_dead_letter
		  ORDER BY failed_at DESC, id DESC
		  LIMIT ?`,
		limit)
	if err != nil {
		return nil, fmt.Errorf("%s query: %w", op, err)
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	out := make([]DeadLetterRow, 0, limit)
	for rows.Next() {
		var dl DeadLetterRow
		if err := rows.Scan(&dl.ID, &dl.EventID, &dl.PeerURL, &dl.LocalProjectID, &dl.Payload, &dl.StatusCode, &dl.Reason, &dl.FailedAt); err != nil {
			return nil, fmt.Errorf("%s scan: %w", op, err)
		}
		out = append(out, dl)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s rows: %w", op, err)
	}
	return out, nil
}

// PeerRetryRow is the durable per-peer retry gate (Federation v1 F4.4). NotBefore
// is the earliest wall-clock the peer may be re-POSTed; Attempt is the
// consecutive-transient failure count driving the 1s..1h exponential window;
// Permanent marks a peer that has been fully dead-lettered and must not be
// re-probed until an operator re-enables it.
type PeerRetryRow struct {
	PeerURL   string
	NotBefore string
	Attempt   int
	Permanent bool
	UpdatedAt string
}

// SavePeerRetry upserts the per-peer retry gate (one row per peer). It is the
// durable mirror of the in-memory backoff state so a restart restores it rather
// than re-hammering a down/rejecting peer (US-4.4 AC1/AC2, §7 F4.4 risk).
func (s *Store) SavePeerRetry(ctx context.Context, r PeerRetryRow) error {
	const op = "store.SavePeerRetry"
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO federation_peer_retry (peer_instance_url, not_before, attempt, permanent, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(peer_instance_url) DO UPDATE SET
		   not_before = excluded.not_before,
		   attempt    = excluded.attempt,
		   permanent  = excluded.permanent,
		   updated_at = excluded.updated_at`,
		r.PeerURL, r.NotBefore, r.Attempt, boolToInt(r.Permanent), r.UpdatedAt)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

// LoadPeerRetry returns every persisted per-peer retry gate. The worker loads it
// once on startup so an exponential backoff / permanent-failure gate survives a
// restart (the cross-restart persistence F4.4 requires).
func (s *Store) LoadPeerRetry(ctx context.Context) ([]PeerRetryRow, error) {
	const op = "store.LoadPeerRetry"
	rows, err := s.db.QueryContext(ctx,
		`SELECT peer_instance_url, not_before, attempt, permanent, updated_at FROM federation_peer_retry`)
	if err != nil {
		return nil, fmt.Errorf("%s query: %w", op, err)
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	out := make([]PeerRetryRow, 0)
	for rows.Next() {
		var r PeerRetryRow
		var permanent int
		if err := rows.Scan(&r.PeerURL, &r.NotBefore, &r.Attempt, &permanent, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("%s scan: %w", op, err)
		}
		r.Permanent = permanent != 0
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s rows: %w", op, err)
	}
	return out, nil
}

// DeletePeerRetry clears a peer's retry gate (a successful delivery resets the
// backoff). A missing row is a no-op.
func (s *Store) DeletePeerRetry(ctx context.Context, peerURL string) error {
	const op = "store.DeletePeerRetry"
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM federation_peer_retry WHERE peer_instance_url = ?`, peerURL); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

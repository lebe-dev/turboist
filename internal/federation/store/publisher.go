package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/federation/hlc"
	"github.com/lebe-dev/turboist/internal/logging"
)

// Publisher / pull reads over federation_outbox (Federation v1 F3.2, US-3.1/
// US-3.2). The publisher worker (internal/federation/outbox) batch-reads
// undelivered events per (project, peer), POSTs them, and stamps delivery; the
// pull handler replays events to a peer by HLC cursor. All reads here run on the
// store's own connection so the worker can release it before any network I/O
// (R1 — never hold the lone connection across a peer POST).
//
// delivered_to is a JSON array of peer instance_urls a given outbox row has been
// successfully delivered to. An event is "pending" for a peer when its url is NOT
// in that array. The owner self-row peer is never a delivery target (the worker
// filters it out by is_owner), so its url never appears here.

// ListProjectsWithOutbox returns the distinct local project ids that have at
// least one outbox event (the publisher's drain scope). Projects with an empty
// outbox are skipped so a drain pass touches only projects with pending work.
func (s *Store) ListProjectsWithOutbox(ctx context.Context) ([]int64, error) {
	const op = "store.ListProjectsWithOutbox"
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT local_project_id FROM federation_outbox ORDER BY local_project_id ASC`)
	if err != nil {
		return nil, fmt.Errorf("%s query: %w", op, err)
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	out := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("%s scan: %w", op, err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s rows: %w", op, err)
	}
	return out, nil
}

// OutboxEvent is one undelivered outbox row for a peer (the publisher batch).
type OutboxEvent struct {
	// ID is the autoincrement federation_outbox.id (the MarkDelivered key).
	ID int64
	// EventID is the cross-instance dedup key carried in the payload.
	EventID string
	// Payload is the canonical signed event JSON, POSTed verbatim to the peer
	// (the worker never re-serialises it — the per-event signature is over these
	// exact bytes).
	Payload string
}

// ListUndeliveredForPeer returns up to limit outbox events for a project that
// have NOT yet been delivered to peerURL, in id (chronological) order. It is the
// publisher's batch read (US-3.2 AC2): the worker resolves it, releases the
// connection, then POSTs the batch.
//
// A dead-lettered event for this peer is SKIPPED (Federation v1 F4.4, US-4.4 AC3)
// so a permanently-failed event is not re-read and re-POSTed forever — it stays
// parked in federation_dead_letter until an operator intervention.
func (s *Store) ListUndeliveredForPeer(ctx context.Context, localProjectID int64, peerURL string, limit int) ([]OutboxEvent, error) {
	const op = "store.ListUndeliveredForPeer"
	if limit <= 0 {
		limit = 500
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT o.id, o.event_id, o.payload, o.delivered_to
		   FROM federation_outbox o
		  WHERE o.local_project_id = ?
		    AND NOT EXISTS (
		      SELECT 1 FROM federation_dead_letter dl
		       WHERE dl.peer_instance_url = ? AND dl.event_id = o.event_id
		    )
		  ORDER BY o.id ASC`,
		localProjectID, peerURL)
	if err != nil {
		return nil, fmt.Errorf("%s query: %w", op, err)
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	out := make([]OutboxEvent, 0, limit)
	for rows.Next() {
		var ev OutboxEvent
		var deliveredTo string
		if err := rows.Scan(&ev.ID, &ev.EventID, &ev.Payload, &deliveredTo); err != nil {
			return nil, fmt.Errorf("%s scan: %w", op, err)
		}
		delivered, decodeErr := parseDeliveredErr(deliveredTo)
		if decodeErr != nil {
			// A corrupt delivered_to decodes to the empty "delivered to nobody" set,
			// so the row is (re-)queued for this peer. Forward progress is safe
			// (at-least-once + receiver event_id dedup), but surface the corruption at
			// WARN so it is diagnosable rather than silently re-broadcasting forever.
			logging.FromContext(ctx).WarnContext(ctx, "federation: undecodable outbox delivered_to, treating as delivered to nobody",
				slog.String("op", op),
				slog.Int64("outbox_id", ev.ID),
				slog.String("event_id", ev.EventID),
				slog.String("peer", peerURL),
				slog.String("err", decodeErr.Error()),
			)
		}
		if deliveredHas(delivered, peerURL) {
			continue
		}
		out = append(out, ev)
		if len(out) >= limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s rows: %w", op, err)
	}
	return out, nil
}

// PendingDeliveryCount returns how many outbox events for a project have not yet
// been delivered to peerURL (US-1.4 AC4 / US-3.2 AC4 — the peer delivery-overdue
// signal). It is a count companion to ListUndeliveredForPeer.
//
// A permanently-failed (dead-lettered) event for this peer is EXCLUDED from the
// count (Federation v1 F4.4, US-4.4 AC3 / §7 F4.4 risk: "dead-letter excluded
// from pending count") — a 4xx-rejected event must not keep the sync status stuck
// "pending" forever once it has been parked in federation_dead_letter.
func (s *Store) PendingDeliveryCount(ctx context.Context, localProjectID int64, peerURL string) (int, error) {
	const op = "store.PendingDeliveryCount"
	rows, err := s.db.QueryContext(ctx,
		`SELECT o.delivered_to FROM federation_outbox o
		  WHERE o.local_project_id = ?
		    AND NOT EXISTS (
		      SELECT 1 FROM federation_dead_letter dl
		       WHERE dl.peer_instance_url = ? AND dl.event_id = o.event_id
		    )`,
		localProjectID, peerURL)
	if err != nil {
		return 0, fmt.Errorf("%s query: %w", op, err)
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	n := 0
	for rows.Next() {
		var deliveredTo string
		if err := rows.Scan(&deliveredTo); err != nil {
			return 0, fmt.Errorf("%s scan: %w", op, err)
		}
		if !deliveredToHas(deliveredTo, peerURL) {
			n++
		}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("%s rows: %w", op, err)
	}
	return n, nil
}

// MarkDelivered appends peerURL to an outbox row's delivered_to set, idempotently
// (a peer already present is a no-op). It runs in its own SHORT transaction so
// the worker only holds the lone connection briefly after the network I/O
// completes (R1: read-batch → release → network → short tx to mark delivered).
func (s *Store) MarkDelivered(ctx context.Context, outboxID int64, peerURL string) error {
	const op = "store.MarkDelivered"
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%s begin: %w", op, err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := MarkDeliveredTx(ctx, tx, outboxID, peerURL); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%s commit: %w", op, err)
	}
	return nil
}

// MarkDeliveredTx is MarkDelivered inside a caller transaction (e.g. the inbox
// hub-and-spoke re-broadcast path in a later phase). It reads the current
// delivered_to, appends peerURL if absent, and writes it back.
func MarkDeliveredTx(ctx context.Context, tx Querier, outboxID int64, peerURL string) error {
	var deliveredTo string
	err := tx.QueryRowContext(ctx,
		`SELECT delivered_to FROM federation_outbox WHERE id = ?`, outboxID).Scan(&deliveredTo)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("mark delivered read: %w", err)
	}
	next, changed := appendDelivered(deliveredTo, peerURL)
	if !changed {
		return nil
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE federation_outbox SET delivered_to = ? WHERE id = ?`, next, outboxID); err != nil {
		return fmt.Errorf("mark delivered update: %w", err)
	}
	return nil
}

// PullEvent is one event returned by the pull read, carrying its decoded
// max-field HLC so the caller can advance its cursor.
type PullEvent struct {
	EventID string
	Payload string
	MaxHLC  string
}

// ListEventsSinceHLC returns up to limit outbox events for a project whose
// greatest per-field HLC is STRICTLY GREATER than sinceHLC, ordered by that HLC
// ascending (US-3.2 AC3 pull replay; US-4.1 cursor catch-up). An empty sinceHLC
// returns every event (a fresh peer). The HLC compare is the same lexical total
// order the per-field CAS uses (hlc.CompareString), so a pull replays to exactly
// the same state a push would converge to.
func (s *Store) ListEventsSinceHLC(ctx context.Context, localProjectID int64, sinceHLC string, limit int) ([]PullEvent, error) {
	const op = "store.ListEventsSinceHLC"
	if limit <= 0 {
		limit = 500
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT event_id, payload FROM federation_outbox WHERE local_project_id = ? ORDER BY id ASC`,
		localProjectID)
	if err != nil {
		return nil, fmt.Errorf("%s query: %w", op, err)
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	out := make([]PullEvent, 0, limit)
	for rows.Next() {
		var eventID, payload string
		if err := rows.Scan(&eventID, &payload); err != nil {
			return nil, fmt.Errorf("%s scan: %w", op, err)
		}
		maxHLC := payloadMaxHLC(payload)
		if hlc.CompareString(maxHLC, sinceHLC) <= 0 {
			continue // not strictly greater than the cursor — already seen.
		}
		out = append(out, PullEvent{EventID: eventID, Payload: payload, MaxHLC: maxHLC})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s rows: %w", op, err)
	}

	sortByHLC(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// payloadMaxHLC decodes the event payload's per-field HLCs and returns the
// lexically-greatest. A payload that fails to decode contributes an empty HLC
// (it sorts first and is harmless to the cursor).
func payloadMaxHLC(payload string) string {
	maxHLC, _ := payloadMaxHLCErr(payload)
	return maxHLC
}

// payloadMaxHLCErr is payloadMaxHLC but surfaces the decode error so callers on
// the retention/cursor path can WARN on an undecodable stored payload rather than
// silently treating it as an empty HLC. The empty-HLC fallback is preserved on
// error so the cursor math stays harmless.
func payloadMaxHLCErr(payload string) (string, error) {
	var e events.Event
	if err := json.Unmarshal([]byte(payload), &e); err != nil {
		return "", err
	}
	return e.MaxFieldHLC(), nil
}

// sortByHLC sorts pull events by their max field HLC ascending (insertion sort —
// the v1 batches are small and bounded by the 500 limit). A stable order on
// equal HLCs is fine: the receiver applies idempotently and per-field LWW is
// order-independent (US-3.3 AC3).
func sortByHLC(evs []PullEvent) {
	for i := 1; i < len(evs); i++ {
		j := i
		for j > 0 && hlc.CompareString(evs[j-1].MaxHLC, evs[j].MaxHLC) > 0 {
			evs[j-1], evs[j] = evs[j], evs[j-1]
			j--
		}
	}
}

// deliveredToHas reports whether peerURL is recorded in the delivered_to JSON
// array. A non-JSON / empty value means "delivered to nobody".
func deliveredToHas(deliveredTo, peerURL string) bool {
	return deliveredHas(parseDelivered(deliveredTo), peerURL)
}

// deliveredHas reports whether peerURL is in an already-decoded delivered set.
// Both sides are normalized (trailing-slash trim) so a slash-form mismatch
// between a stamped origin and a peer mapping URL cannot reopen the echo loop.
func deliveredHas(delivered []string, peerURL string) bool {
	want := normInstance(peerURL)
	for _, u := range delivered {
		if normInstance(u) == want {
			return true
		}
	}
	return false
}

// appendDelivered returns delivered_to with peerURL appended (as a JSON array),
// and whether the set actually changed. A peer already present is unchanged.
func appendDelivered(deliveredTo, peerURL string) (string, bool) {
	cur := parseDelivered(deliveredTo)
	for _, u := range cur {
		if u == peerURL {
			return deliveredTo, false
		}
	}
	cur = append(cur, peerURL)
	b, err := json.Marshal(cur)
	if err != nil {
		return deliveredTo, false
	}
	return string(b), true
}

// parseDelivered decodes the delivered_to JSON array, tolerating the legacy
// empty-string default (” from the migration) as an empty set.
func parseDelivered(deliveredTo string) []string {
	urls, _ := parseDeliveredErr(deliveredTo)
	return urls
}

// parseDeliveredErr is parseDelivered but surfaces the decode error so the
// publisher's batch read can WARN (with the outbox row id) on a corrupt
// delivered_to value instead of silently degrading it to "delivered to nobody"
// — which would otherwise cause the event to be re-delivered to every peer. The
// nil ("nobody") fallback is preserved on error so delivery still makes forward
// progress (at-least-once is idempotent on the receiver via event_id dedup).
func parseDeliveredErr(deliveredTo string) ([]string, error) {
	if deliveredTo == "" {
		return nil, nil
	}
	var urls []string
	if err := json.Unmarshal([]byte(deliveredTo), &urls); err != nil {
		return nil, err
	}
	return urls, nil
}

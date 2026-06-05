package store_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/federation/store"
)

// withTx runs fn inside a transaction and commits, mirroring the inbox-apply
// path that re-broadcasts INSIDE the merge tx (Federation v1 F5.1, §3 risk:
// "re-enqueue inside inbox-apply tx (single connection)").
func withTx(t *testing.T, raw *sql.DB, fn func(tx *sql.Tx) error) {
	t.Helper()
	tx, err := raw.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		t.Fatalf("tx fn: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

// openMigratedRaw opens a fresh migrated SQLite DB and returns both the store and
// the raw *sql.DB so a test can drive a transaction directly.
func openMigratedRaw(t *testing.T) (*store.Store, *sql.DB) {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(dir + "/store.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.RunMigrations(context.Background(), d); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return store.New(d), d
}

// seedProject inserts a context + a minimal local project so the outbox FK is
// satisfied, and returns the project id.
func seedProject(t *testing.T, raw *sql.DB) int64 {
	t.Helper()
	ctxRes, err := raw.Exec(
		`INSERT INTO contexts (name, color, created_at, updated_at)
		 VALUES ('Work', 'blue', '2026-06-01T10:00:00.000Z', '2026-06-01T10:00:00.000Z')`)
	if err != nil {
		t.Fatalf("seed context: %v", err)
	}
	ctxID, _ := ctxRes.LastInsertId()
	res, err := raw.Exec(
		`INSERT INTO projects (context_id, title, color, created_at, updated_at, client_id)
		 VALUES (?, 'Shared', 'blue', '2026-06-01T10:00:00.000Z', '2026-06-01T10:00:00.000Z', 'proj-client-1')`,
		ctxID)
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// deliveredOf reads the delivered_to JSON array recorded on an outbox row.
func deliveredOf(t *testing.T, raw *sql.DB, eventID string) []string {
	t.Helper()
	var deliveredTo string
	if err := raw.QueryRow(`SELECT delivered_to FROM federation_outbox WHERE event_id = ?`, eventID).Scan(&deliveredTo); err != nil {
		t.Fatalf("read delivered_to: %v", err)
	}
	if deliveredTo == "" {
		return nil
	}
	var urls []string
	if err := json.Unmarshal([]byte(deliveredTo), &urls); err != nil {
		t.Fatalf("decode delivered_to %q: %v", deliveredTo, err)
	}
	return urls
}

// TestReBroadcastOutboxTx_PreStampsOrigin asserts the owner re-broadcast writes
// the relayed event to federation_outbox with delivered_to PRE-STAMPED to the
// origin instance, so the publisher never pushes the event back to where it came
// from (Federation v1 F5.1, US-5.2 AC2 echo-loop guard). The row is then pending
// for every OTHER non-revoked peer (hub-and-spoke fan-out).
func TestReBroadcastOutboxTx_PreStampsOrigin(t *testing.T) {
	s, raw := openMigratedRaw(t)
	ctx := context.Background()
	pid := seedProject(t, raw)

	const eventID = "evt-relay-1"
	const origin = "https://bob.example"
	payload := `{"event_id":"evt-relay-1","op":"update","entity_type":"task","origin_instance":"https://bob.example"}`

	withTx(t, raw, func(tx *sql.Tx) error {
		return s.ReBroadcastOutboxTx(ctx, tx, eventID, pid, payload, origin, "2026-06-01T10:00:00.000Z")
	})

	got := deliveredOf(t, raw, eventID)
	if len(got) != 1 || got[0] != origin {
		t.Fatalf("delivered_to: got %v, want pre-stamped origin [%s]", got, origin)
	}

	// The publisher must treat the origin as already delivered (skip it) but the row
	// is pending for any other peer.
	undelivered, err := s.ListUndeliveredForPeer(ctx, pid, origin, 100)
	if err != nil {
		t.Fatalf("list for origin: %v", err)
	}
	if len(undelivered) != 0 {
		t.Errorf("origin peer must NOT receive its own event back: got %d pending", len(undelivered))
	}
	other, err := s.ListUndeliveredForPeer(ctx, pid, "https://carol.example", 100)
	if err != nil {
		t.Fatalf("list for other: %v", err)
	}
	if len(other) != 1 || other[0].EventID != eventID {
		t.Errorf("other peer must receive the re-broadcast: got %+v", other)
	}
}

// TestReBroadcastOutboxTx_DedupOnEventID asserts re-broadcasting the same event_id
// twice is a no-op on the second write (ON CONFLICT(event_id) DO NOTHING), so an
// at-least-once redelivery the owner re-broadcasts cannot duplicate the relayed
// row (NFR-2 dedup; US-5.2 AC2 convergence under redelivery). The first write's
// delivered_to stamp is preserved.
func TestReBroadcastOutboxTx_DedupOnEventID(t *testing.T) {
	s, raw := openMigratedRaw(t)
	ctx := context.Background()
	pid := seedProject(t, raw)

	const eventID = "evt-relay-dup"
	const origin = "https://bob.example"
	payload := `{"event_id":"evt-relay-dup","op":"update"}`

	withTx(t, raw, func(tx *sql.Tx) error {
		return s.ReBroadcastOutboxTx(ctx, tx, eventID, pid, payload, origin, "2026-06-01T10:00:00.000Z")
	})
	withTx(t, raw, func(tx *sql.Tx) error {
		return s.ReBroadcastOutboxTx(ctx, tx, eventID, pid, payload, "https://other-origin.example", "2026-06-01T11:00:00.000Z")
	})

	var count int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM federation_outbox WHERE event_id = ?`, eventID).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("duplicate re-broadcast must be a no-op: got %d rows, want 1", count)
	}
	// The first stamp survives the conflict (DO NOTHING leaves the original row).
	got := deliveredOf(t, raw, eventID)
	if len(got) != 1 || got[0] != origin {
		t.Errorf("delivered_to after dedup: got %v, want first origin [%s]", got, origin)
	}
}

// TestReBroadcastOutboxTx_TrailingSlashOriginNotReBroadcast pins the echo-loop
// guard's slash normalization (item 11): an origin supplied WITH a trailing slash
// must still be recognized as the origin and NOT re-pushed back to it, regardless
// of whether the publisher asks with or without the slash. Without normalization
// the exact-match comparison would treat the two forms as different peers and
// re-broadcast the relayed event to its own origin (an echo loop).
func TestReBroadcastOutboxTx_TrailingSlashOriginNotReBroadcast(t *testing.T) {
	s, raw := openMigratedRaw(t)
	ctx := context.Background()
	pid := seedProject(t, raw)

	const eventID = "evt-relay-slash"
	const originWithSlash = "https://bob.example/"
	const originNoSlash = "https://bob.example"

	withTx(t, raw, func(tx *sql.Tx) error {
		return s.ReBroadcastOutboxTx(ctx, tx, eventID, pid, `{"event_id":"evt-relay-slash"}`, originWithSlash, "2026-06-01T10:00:00.000Z")
	})

	// The stamp is normalized (slash trimmed) on write.
	got := deliveredOf(t, raw, eventID)
	if len(got) != 1 || got[0] != originNoSlash {
		t.Fatalf("delivered_to: got %v, want normalized origin [%s]", got, originNoSlash)
	}

	// The publisher must skip the origin whether it asks with OR without the slash.
	for _, peer := range []string{originWithSlash, originNoSlash} {
		undelivered, err := s.ListUndeliveredForPeer(ctx, pid, peer, 100)
		if err != nil {
			t.Fatalf("list for origin %q: %v", peer, err)
		}
		if len(undelivered) != 0 {
			t.Errorf("origin %q must NOT receive its own event back: got %d pending", peer, len(undelivered))
		}
	}

	// A genuinely different peer still receives the re-broadcast.
	other, err := s.ListUndeliveredForPeer(ctx, pid, "https://carol.example", 100)
	if err != nil {
		t.Fatalf("list for other: %v", err)
	}
	if len(other) != 1 || other[0].EventID != eventID {
		t.Errorf("other peer must receive the re-broadcast: got %+v", other)
	}
}

// TestReBroadcastOutboxTx_EmptyOriginStampsNobody asserts that when no origin is
// supplied (defensive — a locally-authored event would not use this path), the
// re-broadcast row is pending for every peer (delivered_to empty). This documents
// that the origin-skip is the ONLY echo guard and is not applied for an empty
// origin.
func TestReBroadcastOutboxTx_EmptyOriginStampsNobody(t *testing.T) {
	s, raw := openMigratedRaw(t)
	ctx := context.Background()
	pid := seedProject(t, raw)

	const eventID = "evt-relay-noorigin"
	withTx(t, raw, func(tx *sql.Tx) error {
		return s.ReBroadcastOutboxTx(ctx, tx, eventID, pid, `{"event_id":"evt-relay-noorigin"}`, "", "2026-06-01T10:00:00.000Z")
	})

	got := deliveredOf(t, raw, eventID)
	if len(got) != 0 {
		t.Errorf("empty origin must stamp nobody: got %v", got)
	}
}

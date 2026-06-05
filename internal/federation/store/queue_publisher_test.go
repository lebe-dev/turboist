package store_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/federation/store"
	"github.com/lebe-dev/turboist/internal/model"
)

// openMigratedDB opens a fresh migrated DB and returns both the raw *sql.DB (for
// seeding fixtures + transactions) and a Store.
func openMigratedDB(t *testing.T) (*sql.DB, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "queue.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.RunMigrations(context.Background(), d); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d, store.New(d)
}

// seedProjectRow inserts a minimal federated project so the outbox FK is
// satisfied, returning its int64 id.
func seedProjectRow(t *testing.T, d *sql.DB, title string) int64 {
	t.Helper()
	if _, err := d.Exec(
		`INSERT OR IGNORE INTO contexts (id, name, color, client_id, created_at, updated_at)
		 VALUES (1, 'c', 'blue', 'q-ctx-1', '2024-01-01T00:00:00.000Z', '2024-01-01T00:00:00.000Z')`,
	); err != nil {
		t.Fatalf("seed context: %v", err)
	}
	res, err := d.Exec(
		`INSERT INTO projects (context_id, title, color, status, is_federated, client_id, created_at, updated_at)
		 VALUES (1, ?, 'blue', 'open', 1, ?, '2024-01-01T00:00:00.000Z', '2024-01-01T00:00:00.000Z')`,
		title, model.NewClientID())
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("last insert id: %v", err)
	}
	return id
}

// TestListUndeliveredForPeer_NewEventsReturned asserts a freshly written outbox
// event (delivered_to empty) is returned for a peer that has not received it yet,
// in id order (US-3.2 AC2 backbone — the publisher batch read).
func TestListUndeliveredForPeer_NewEventsReturned(t *testing.T) {
	d, s := openMigratedDB(t)
	ctx := context.Background()
	pid := seedProjectRow(t, d, "Shared")

	if err := writeOutbox(ctx, d, s, "evt-1", pid, `{"event_id":"evt-1"}`); err != nil {
		t.Fatalf("write outbox: %v", err)
	}
	if err := writeOutbox(ctx, d, s, "evt-2", pid, `{"event_id":"evt-2"}`); err != nil {
		t.Fatalf("write outbox: %v", err)
	}

	got, err := s.ListUndeliveredForPeer(ctx, pid, "https://peer.example", 100)
	if err != nil {
		t.Fatalf("list undelivered: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("undelivered count: got %d, want 2", len(got))
	}
	if got[0].EventID != "evt-1" || got[1].EventID != "evt-2" {
		t.Errorf("order: got %q,%q want evt-1,evt-2", got[0].EventID, got[1].EventID)
	}
}

// TestMarkDelivered_PeerExcludedAfterStamp asserts that after a peer is stamped
// into delivered_to, that event is no longer returned for THAT peer but is still
// pending for a different peer (US-3.2 AC2 — per-peer delivery isolation).
func TestMarkDelivered_PeerExcludedAfterStamp(t *testing.T) {
	d, s := openMigratedDB(t)
	ctx := context.Background()
	pid := seedProjectRow(t, d, "Shared")

	if err := writeOutbox(ctx, d, s, "evt-1", pid, `{"event_id":"evt-1"}`); err != nil {
		t.Fatalf("write outbox: %v", err)
	}
	got, err := s.ListUndeliveredForPeer(ctx, pid, "https://a.example", 100)
	if err != nil || len(got) != 1 {
		t.Fatalf("pre-mark list: got %d err %v", len(got), err)
	}

	if err := s.MarkDelivered(ctx, got[0].ID, "https://a.example"); err != nil {
		t.Fatalf("mark delivered: %v", err)
	}

	// Same peer: no longer pending.
	gotA, err := s.ListUndeliveredForPeer(ctx, pid, "https://a.example", 100)
	if err != nil {
		t.Fatalf("list a: %v", err)
	}
	if len(gotA) != 0 {
		t.Errorf("peer a should have nothing pending after mark: got %d", len(gotA))
	}

	// Different peer: still pending (per-peer fan-out, US-3.2 AC3 isolation).
	gotB, err := s.ListUndeliveredForPeer(ctx, pid, "https://b.example", 100)
	if err != nil {
		t.Fatalf("list b: %v", err)
	}
	if len(gotB) != 1 {
		t.Errorf("peer b should still have it pending: got %d", len(gotB))
	}
}

// TestMarkDelivered_Idempotent asserts marking the same (event, peer) twice does
// not corrupt delivered_to and the event stays delivered for that peer.
func TestMarkDelivered_Idempotent(t *testing.T) {
	d, s := openMigratedDB(t)
	ctx := context.Background()
	pid := seedProjectRow(t, d, "Shared")
	if err := writeOutbox(ctx, d, s, "evt-1", pid, `{"event_id":"evt-1"}`); err != nil {
		t.Fatalf("write outbox: %v", err)
	}
	got, _ := s.ListUndeliveredForPeer(ctx, pid, "https://a.example", 100)
	if err := s.MarkDelivered(ctx, got[0].ID, "https://a.example"); err != nil {
		t.Fatalf("mark 1: %v", err)
	}
	if err := s.MarkDelivered(ctx, got[0].ID, "https://a.example"); err != nil {
		t.Fatalf("mark 2: %v", err)
	}
	gotA, _ := s.ListUndeliveredForPeer(ctx, pid, "https://a.example", 100)
	if len(gotA) != 0 {
		t.Errorf("double-mark should leave nothing pending: got %d", len(gotA))
	}
}

// TestPendingDeliveryCount_CountsUndelivered asserts the pending-delivery counter
// (US-1.4 AC4 / US-3.2 AC4) reflects events not yet delivered to a peer.
func TestPendingDeliveryCount_CountsUndelivered(t *testing.T) {
	d, s := openMigratedDB(t)
	ctx := context.Background()
	pid := seedProjectRow(t, d, "Shared")
	for _, id := range []string{"e1", "e2", "e3"} {
		if err := writeOutbox(ctx, d, s, id, pid, `{}`); err != nil {
			t.Fatalf("write %s: %v", id, err)
		}
	}
	got, _ := s.ListUndeliveredForPeer(ctx, pid, "https://a.example", 100)
	if err := s.MarkDelivered(ctx, got[0].ID, "https://a.example"); err != nil {
		t.Fatalf("mark: %v", err)
	}
	n, err := s.PendingDeliveryCount(ctx, pid, "https://a.example")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 2 {
		t.Errorf("pending count: got %d, want 2", n)
	}
}

// TestListEventsSinceHLC_OrdersByHLCAsc asserts the pull read returns events with
// a max field HLC strictly greater than since_hlc, in ascending HLC order
// (US-3.2 AC3 pull replay; US-4.1 cursor advance).
func TestListEventsSinceHLC_OrdersByHLCAsc(t *testing.T) {
	d, s := openMigratedDB(t)
	ctx := context.Background()
	pid := seedProjectRow(t, d, "Shared")

	// Three events with ascending field HLCs.
	mustOutboxWithHLC(t, ctx, d, s, "e-low", pid, "00000000000100-0000-nodeA")
	mustOutboxWithHLC(t, ctx, d, s, "e-mid", pid, "00000000000200-0000-nodeA")
	mustOutboxWithHLC(t, ctx, d, s, "e-high", pid, "00000000000300-0000-nodeA")

	got, err := s.ListEventsSinceHLC(ctx, pid, "00000000000100-0000-nodeA", 100)
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	// since == e-low's HLC -> strictly-greater excludes e-low, returns mid+high asc.
	if len(got) != 2 {
		t.Fatalf("pull count: got %d, want 2", len(got))
	}
	if got[0].EventID != "e-mid" || got[1].EventID != "e-high" {
		t.Errorf("pull order: got %q,%q want e-mid,e-high", got[0].EventID, got[1].EventID)
	}
}

// TestListEventsSinceHLC_EmptySinceReturnsAll asserts an empty since_hlc cursor
// (a fresh peer) returns every event in HLC-ascending order.
func TestListEventsSinceHLC_EmptySinceReturnsAll(t *testing.T) {
	d, s := openMigratedDB(t)
	ctx := context.Background()
	pid := seedProjectRow(t, d, "Shared")
	mustOutboxWithHLC(t, ctx, d, s, "e1", pid, "00000000000100-0000-nodeA")
	mustOutboxWithHLC(t, ctx, d, s, "e2", pid, "00000000000200-0000-nodeA")

	got, err := s.ListEventsSinceHLC(ctx, pid, "", 100)
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("empty-since pull count: got %d, want 2", len(got))
	}
}

// TestListUnappliedInbox_ReturnsOnlyNullApplied asserts the inbox recovery read
// returns rows whose applied_at is still NULL (received but not merged) oldest
// first, and excludes rows already stamped terminal — the at-least-once recovery
// scan that re-drives an event whose apply was lost to a transient failure / a
// crash (NFR-2; finding fix).
func TestListUnappliedInbox_ReturnsOnlyNullApplied(t *testing.T) {
	d, s := openMigratedDB(t)
	ctx := context.Background()
	pid := seedProjectRow(t, d, "Shared")

	// Two received events, oldest first by received_at.
	if _, err := s.InsertInbox(ctx, "evt-old", "https://peer.example", pid, `{"event_id":"evt-old"}`, "2024-01-01T00:00:01.000Z"); err != nil {
		t.Fatalf("insert inbox old: %v", err)
	}
	if _, err := s.InsertInbox(ctx, "evt-new", "https://peer.example", pid, `{"event_id":"evt-new"}`, "2024-01-01T00:00:02.000Z"); err != nil {
		t.Fatalf("insert inbox new: %v", err)
	}

	got, err := s.ListUnappliedInbox(ctx, 100)
	if err != nil {
		t.Fatalf("list unapplied: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("unapplied count: got %d, want 2", len(got))
	}
	if got[0].EventID != "evt-old" || got[1].EventID != "evt-new" {
		t.Errorf("order (received_at asc): got %q,%q want evt-old,evt-new", got[0].EventID, got[1].EventID)
	}
	if got[0].PeerInstanceURL != "https://peer.example" || got[0].Payload != `{"event_id":"evt-old"}` {
		t.Errorf("row fields: got peer=%q payload=%q", got[0].PeerInstanceURL, got[0].Payload)
	}

	// Stamp one terminal: it must drop out of the unapplied scan.
	if err := s.MarkInboxApplied(ctx, "evt-old", "2024-01-01T00:00:03.000Z"); err != nil {
		t.Fatalf("mark applied: %v", err)
	}
	got, err = s.ListUnappliedInbox(ctx, 100)
	if err != nil {
		t.Fatalf("list unapplied after stamp: %v", err)
	}
	if len(got) != 1 || got[0].EventID != "evt-new" {
		t.Errorf("after stamp: got %d rows (first %q), want 1 (evt-new)", len(got), firstEventID(got))
	}
}

func firstEventID(rows []store.PendingInboxEvent) string {
	if len(rows) == 0 {
		return ""
	}
	return rows[0].EventID
}

func writeOutbox(ctx context.Context, d *sql.DB, s *store.Store, eventID string, pid int64, payload string) error {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.InsertOutboxTx(ctx, tx, eventID, pid, payload, 1, "2024-01-01T00:00:00.000Z"); err != nil {
		return err
	}
	return tx.Commit()
}

func mustOutboxWithHLC(t *testing.T, ctx context.Context, d *sql.DB, s *store.Store, eventID string, pid int64, hlc string) {
	t.Helper()
	payload := `{"event_id":"` + eventID + `","fields":{"title":{"value":"x","hlc":"` + hlc + `"}}}`
	if err := writeOutbox(ctx, d, s, eventID, pid, payload); err != nil {
		t.Fatalf("write outbox %s: %v", eventID, err)
	}
}

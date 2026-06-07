package store_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/federation/store"
	"github.com/lebe-dev/turboist/internal/model"
)

// insertOutboxRow commits one federation_outbox row for pid at createdAt and
// returns its id (test helper for the pending-count cases).
func insertOutboxRow(t *testing.T, ctx context.Context, d *sql.DB, s *store.Store, eventID string, pid int64, createdAt string) int64 {
	t.Helper()
	tx, _ := d.BeginTx(ctx, nil)
	if err := s.InsertOutboxTx(ctx, tx, eventID, pid, `{}`, 1, createdAt); err != nil {
		t.Fatalf("insert outbox %s: %v", eventID, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit %s: %v", eventID, err)
	}
	var id int64
	if err := d.QueryRow(`SELECT id FROM federation_outbox WHERE event_id = ?`, eventID).Scan(&id); err != nil {
		t.Fatalf("read id %s: %v", eventID, err)
	}
	return id
}

// TestOverduePendingCount_CountsEventsNotPeers is the F4.3 semantics regression: the
// pending count is the number of overdue EVENTS (changes), NOT the number of overdue
// peers — matching the "N changes pending" badge / DTO / API.md / i18n wording. Two
// overdue events undelivered to two peers count as 2 (the events), never 2-per-peer
// or 1-per-peer, and an event owed to more than one peer is counted once.
func TestOverduePendingCount_CountsEventsNotPeers(t *testing.T) {
	d, s := openMigratedDB(t)
	ctx := context.Background()
	pid := seedProjectRow(t, d, "Shared")

	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	old := model.FormatUTC(now.Add(-10 * time.Minute)) // older than the 5min cutoff
	peers := []string{"https://bob.example", "https://dave.example"}

	// e1 is undelivered to BOTH peers; e2 is delivered to bob but not dave. Each is
	// ONE overdue change → count 2 (not 3 peer-deliveries, not 1 peer).
	insertOutboxRow(t, ctx, d, s, "e1", pid, old)
	e2 := insertOutboxRow(t, ctx, d, s, "e2", pid, old)
	if err := s.MarkDelivered(ctx, e2, "https://bob.example"); err != nil {
		t.Fatalf("mark delivered: %v", err)
	}

	cutoff := model.FormatUTC(now.Add(-model.SyncStatusPendingAfter))
	count, err := s.OverduePendingCount(ctx, pid, cutoff, peers)
	if err != nil {
		t.Fatalf("OverduePendingCount: %v", err)
	}
	if count != 2 {
		t.Errorf("pending count: got %d, want 2 (two overdue events, counted once each)", count)
	}
}

// TestOverduePendingCount_FullyDeliveredIsZero asserts an event delivered to EVERY
// active peer is not counted, and that a single overdue event owed to one peer
// counts exactly once (US-4.3 AC2).
func TestOverduePendingCount_FullyDeliveredIsZero(t *testing.T) {
	d, s := openMigratedDB(t)
	ctx := context.Background()
	pid := seedProjectRow(t, d, "Shared")

	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	old := model.FormatUTC(now.Add(-10 * time.Minute))
	peers := []string{"https://bob.example", "https://dave.example"}

	id := insertOutboxRow(t, ctx, d, s, "e-old", pid, old)
	if err := s.MarkDelivered(ctx, id, "https://bob.example"); err != nil {
		t.Fatalf("mark delivered bob: %v", err)
	}
	cutoff := model.FormatUTC(now.Add(-model.SyncStatusPendingAfter))

	// Still owed to dave → one overdue change.
	if count, _ := s.OverduePendingCount(ctx, pid, cutoff, peers); count != 1 {
		t.Errorf("one overdue event owed to dave: got %d, want 1", count)
	}
	// Delivered to every active peer → zero.
	if err := s.MarkDelivered(ctx, id, "https://dave.example"); err != nil {
		t.Fatalf("mark delivered dave: %v", err)
	}
	if count, _ := s.OverduePendingCount(ctx, pid, cutoff, peers); count != 0 {
		t.Errorf("fully-delivered event must not be pending: got %d, want 0", count)
	}
}

// TestOverduePendingCount_FreshNotOverdue asserts an undelivered event WITHIN the
// cutoff window is not counted — a just-committed event still in flight (under the
// NFR-1.1 5s push budget) must not flip the badge (US-4.3 AC2).
func TestOverduePendingCount_FreshNotOverdue(t *testing.T) {
	d, s := openMigratedDB(t)
	ctx := context.Background()
	pid := seedProjectRow(t, d, "Shared")

	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	fresh := model.FormatUTC(now.Add(-1 * time.Minute)) // within the 5min window
	insertOutboxRow(t, ctx, d, s, "e-fresh", pid, fresh)

	cutoff := model.FormatUTC(now.Add(-model.SyncStatusPendingAfter))
	count, err := s.OverduePendingCount(ctx, pid, cutoff, []string{"https://bob.example"})
	if err != nil {
		t.Fatalf("OverduePendingCount: %v", err)
	}
	if count != 0 {
		t.Errorf("fresh event must not be overdue: got %d, want 0", count)
	}
}

// TestOverduePendingCount_EmptyOutbox asserts a project with no outbox rows reports
// zero pending (US-4.3 AC1 — the synced baseline).
func TestOverduePendingCount_EmptyOutbox(t *testing.T) {
	d, s := openMigratedDB(t)
	ctx := context.Background()
	pid := seedProjectRow(t, d, "Shared")

	cutoff := model.FormatUTC(time.Now().Add(-model.SyncStatusPendingAfter))
	count, err := s.OverduePendingCount(ctx, pid, cutoff, []string{"https://bob.example"})
	if err != nil {
		t.Fatalf("OverduePendingCount: %v", err)
	}
	if count != 0 {
		t.Errorf("pending on empty outbox: got %d, want 0", count)
	}
}

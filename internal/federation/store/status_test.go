package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

// TestOverduePendingPeers_ReportsOverdueUndelivered asserts the status query
// reports a peer as overdue exactly when the project has an undelivered outbox
// event OLDER than the cutoff that this peer has not received (US-4.3 AC2 — the
// >5min pending signal). Outbox events are project-wide (not peer-targeted);
// delivered_to tracks per-peer delivery, so a peer that has received every overdue
// event is NOT pending, while a peer missing one is.
func TestOverduePendingPeers_ReportsOverdueUndelivered(t *testing.T) {
	d, s := openMigratedDB(t)
	ctx := context.Background()
	pid := seedProjectRow(t, d, "Shared")

	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	old := model.FormatUTC(now.Add(-10 * time.Minute)) // older than the 5min cutoff

	insertOutbox := func(eventID, createdAt string) int64 {
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

	// One overdue (10min) event for the project. Deliver it to dave only.
	id := insertOutbox("e-old", old)
	if err := s.MarkDelivered(ctx, id, "https://dave.example"); err != nil {
		t.Fatalf("mark delivered: %v", err)
	}

	cutoff := model.FormatUTC(now.Add(-model.SyncStatusPendingAfter))
	overdue, err := s.OverduePendingPeers(ctx, pid, cutoff, []string{
		"https://bob.example", "https://dave.example",
	})
	if err != nil {
		t.Fatalf("OverduePendingPeers: %v", err)
	}

	// Bob never received the overdue event → overdue.
	if !overdue["https://bob.example"] {
		t.Errorf("bob: got not-overdue, want overdue (overdue event undelivered to bob)")
	}
	// Dave received the only overdue event → not pending.
	if overdue["https://dave.example"] {
		t.Errorf("dave: got overdue, want not (received the overdue event)")
	}
}

// TestOverduePendingPeers_FreshNotOverdue asserts an undelivered event WITHIN the
// cutoff window does not make a peer pending — a just-committed event still in
// flight (under the NFR-1.1 5s push budget) must not flip the badge (US-4.3 AC2).
func TestOverduePendingPeers_FreshNotOverdue(t *testing.T) {
	d, s := openMigratedDB(t)
	ctx := context.Background()
	pid := seedProjectRow(t, d, "Shared")

	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	fresh := model.FormatUTC(now.Add(-1 * time.Minute)) // within the 5min window
	tx, _ := d.BeginTx(ctx, nil)
	if err := s.InsertOutboxTx(ctx, tx, "e-fresh", pid, `{}`, 1, fresh); err != nil {
		t.Fatalf("insert outbox: %v", err)
	}
	_ = tx.Commit()

	cutoff := model.FormatUTC(now.Add(-model.SyncStatusPendingAfter))
	overdue, err := s.OverduePendingPeers(ctx, pid, cutoff, []string{"https://bob.example"})
	if err != nil {
		t.Fatalf("OverduePendingPeers: %v", err)
	}
	if overdue["https://bob.example"] {
		t.Errorf("bob: got overdue, want not (only a fresh 1min event)")
	}
}

// TestOverduePendingPeers_EmptyOutbox asserts a project with no outbox rows
// reports no overdue peers (US-4.3 AC1 — the synced baseline).
func TestOverduePendingPeers_EmptyOutbox(t *testing.T) {
	d, s := openMigratedDB(t)
	ctx := context.Background()
	pid := seedProjectRow(t, d, "Shared")

	cutoff := model.FormatUTC(time.Now().Add(-model.SyncStatusPendingAfter))
	overdue, err := s.OverduePendingPeers(ctx, pid, cutoff, []string{"https://bob.example"})
	if err != nil {
		t.Fatalf("OverduePendingPeers: %v", err)
	}
	if len(overdue) != 0 {
		t.Errorf("overdue peers on empty outbox: got %v, want none", overdue)
	}
}

package store_test

import (
	"context"
	"testing"

	"github.com/lebe-dev/turboist/internal/federation/store"
)

// TestInsertDeadLetter_RoundTripsAndDedups asserts a permanently-failed event is
// parked in the dead-letter table and that re-parking the same (peer, event) is
// idempotent (Federation v1 F4.4, US-4.4 AC3 — permanent failure logged, not
// retried).
func TestInsertDeadLetter_RoundTripsAndDedups(t *testing.T) {
	d, s := openMigratedDB(t)
	ctx := context.Background()
	pid := seedProjectRow(t, d, "Shared")

	dl := store.DeadLetterRow{
		EventID:        "e1",
		PeerURL:        "https://peer.example",
		LocalProjectID: pid,
		Payload:        `{"event_id":"e1"}`,
		StatusCode:     403,
		Reason:         "federation_read_only",
		FailedAt:       "2026-06-03T10:00:00.000Z",
	}
	if err := s.InsertDeadLetter(ctx, dl); err != nil {
		t.Fatalf("insert dead-letter: %v", err)
	}
	// Re-parking the same (peer, event) is a no-op (ON CONFLICT DO NOTHING).
	if err := s.InsertDeadLetter(ctx, dl); err != nil {
		t.Fatalf("re-insert dead-letter: %v", err)
	}

	rows, err := s.ListDeadLetter(ctx, 0)
	if err != nil {
		t.Fatalf("list dead-letter: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("dead-letter rows: got %d, want 1", len(rows))
	}
	got := rows[0]
	if got.EventID != "e1" || got.PeerURL != "https://peer.example" || got.StatusCode != 403 || got.Reason != "federation_read_only" {
		t.Errorf("dead-letter row: got %+v", got)
	}
	if got.LocalProjectID != pid {
		t.Errorf("dead-letter local project: got %d, want %d", got.LocalProjectID, pid)
	}
}

// TestListDeadLetter_NewestFirstAndLimited asserts the dead-letter list returns
// rows newest-first (most recent failure on top, the diagnostics view ordering)
// and honors the limit.
func TestListDeadLetter_NewestFirstAndLimited(t *testing.T) {
	d, s := openMigratedDB(t)
	ctx := context.Background()
	pid := seedProjectRow(t, d, "Shared")

	for _, ev := range []struct{ id, at string }{
		{"e1", "2026-06-03T10:00:00.000Z"},
		{"e2", "2026-06-03T10:00:05.000Z"},
		{"e3", "2026-06-03T10:00:10.000Z"},
	} {
		if err := s.InsertDeadLetter(ctx, store.DeadLetterRow{
			EventID: ev.id, PeerURL: "https://peer.example", LocalProjectID: pid,
			Payload: "{}", StatusCode: 400, Reason: "x", FailedAt: ev.at,
		}); err != nil {
			t.Fatalf("insert %s: %v", ev.id, err)
		}
	}

	rows, err := s.ListDeadLetter(ctx, 2)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("limited rows: got %d, want 2", len(rows))
	}
	if rows[0].EventID != "e3" || rows[1].EventID != "e2" {
		t.Errorf("dead-letter order: got [%s, %s], want [e3, e2]", rows[0].EventID, rows[1].EventID)
	}
}

// TestPendingDeliveryCount_ExcludesDeadLettered asserts a dead-lettered event is
// EXCLUDED from the per-peer pending-delivery count (Federation v1 F4.4 risk:
// "dead-letter excluded from pending count") — a permanently-failed event must
// not keep the sync status stuck "pending" forever.
func TestPendingDeliveryCount_ExcludesDeadLettered(t *testing.T) {
	d, s := openMigratedDB(t)
	ctx := context.Background()
	pid := seedProjectRow(t, d, "Shared")
	peer := "https://peer.example"

	// Two undelivered events; one of them will be dead-lettered.
	insertOutbox(t, d, "e1", pid, "2024-01-01T00:00:00.000Z")
	insertOutbox(t, d, "e2", pid, "2024-01-01T00:00:00.000Z")

	before, err := s.PendingDeliveryCount(ctx, pid, peer)
	if err != nil {
		t.Fatalf("count before: %v", err)
	}
	if before != 2 {
		t.Fatalf("pending before dead-letter: got %d, want 2", before)
	}

	if err := s.InsertDeadLetter(ctx, store.DeadLetterRow{
		EventID: "e2", PeerURL: peer, LocalProjectID: pid,
		Payload: "{}", StatusCode: 403, Reason: "federation_read_only", FailedAt: "2026-06-03T10:00:00.000Z",
	}); err != nil {
		t.Fatalf("dead-letter e2: %v", err)
	}

	after, err := s.PendingDeliveryCount(ctx, pid, peer)
	if err != nil {
		t.Fatalf("count after: %v", err)
	}
	if after != 1 {
		t.Errorf("pending after dead-letter excludes the parked event: got %d, want 1", after)
	}
}

// TestPeerRetry_RoundTripAcrossRestart asserts the per-peer retry gate persists
// (not_before / attempt / permanent) so a restart restores backoff rather than
// re-hammering a down/rejecting peer (Federation v1 F4.4 risk: "persist retry-
// not-before across restart"). A clear DELETEs the row.
func TestPeerRetry_RoundTripAcrossRestart(t *testing.T) {
	_, s := openMigratedDB(t)
	ctx := context.Background()
	peer := "https://peer.example"

	if err := s.SavePeerRetry(ctx, store.PeerRetryRow{
		PeerURL:   peer,
		NotBefore: "2026-06-03T10:00:05.000Z",
		Attempt:   3,
		Permanent: false,
		UpdatedAt: "2026-06-03T10:00:00.000Z",
	}); err != nil {
		t.Fatalf("save peer-retry: %v", err)
	}

	// Upsert: a second save for the same peer overwrites in place.
	if err := s.SavePeerRetry(ctx, store.PeerRetryRow{
		PeerURL:   peer,
		NotBefore: "2026-06-03T11:00:00.000Z",
		Attempt:   1,
		Permanent: true,
		UpdatedAt: "2026-06-03T10:30:00.000Z",
	}); err != nil {
		t.Fatalf("upsert peer-retry: %v", err)
	}

	rows, err := s.LoadPeerRetry(ctx)
	if err != nil {
		t.Fatalf("load peer-retry: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("peer-retry rows: got %d, want 1", len(rows))
	}
	got := rows[0]
	if got.PeerURL != peer || got.NotBefore != "2026-06-03T11:00:00.000Z" || got.Attempt != 1 || !got.Permanent {
		t.Errorf("peer-retry round-trip: got %+v", got)
	}

	if err := s.DeletePeerRetry(ctx, peer); err != nil {
		t.Fatalf("delete peer-retry: %v", err)
	}
	rows, err = s.LoadPeerRetry(ctx)
	if err != nil {
		t.Fatalf("load after delete: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("peer-retry rows after clear: got %d, want 0", len(rows))
	}
}

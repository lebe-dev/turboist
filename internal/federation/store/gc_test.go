package store_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

// seedFederatedProject creates a context + a federated project and returns its
// int64 id, so the GC outbox/inbox FKs (REFERENCES projects(id)) are satisfiable.
func seedFederatedProject(t *testing.T, d *sql.DB) int64 {
	t.Helper()
	ctx := context.Background()
	now := model.FormatUTC(time.Now())
	res, err := d.ExecContext(ctx,
		`INSERT INTO contexts (name, color, is_favourite, created_at, updated_at) VALUES ('Work', 'blue', 0, ?, ?)`,
		now, now)
	if err != nil {
		t.Fatalf("seed context: %v", err)
	}
	cxID, _ := res.LastInsertId()
	res, err = d.ExecContext(ctx,
		`INSERT INTO projects (context_id, title, description, color, status, project_type, is_pinned, is_federated, client_id, created_at, updated_at)
		 VALUES (?, 'Shared', '', 'blue', 'open', 'generic', 0, 1, 'proj-client-1', ?, ?)`,
		cxID, now, now)
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// seedTombstoneTask inserts a soft-deleted task whose deleted_at is at the given
// wall clock, returning its int64 id. ageDays<0 = older than now.
func seedTombstoneTask(t *testing.T, d *sql.DB, projectID int64, clientID string, deletedAt time.Time) int64 {
	t.Helper()
	ctx := context.Background()
	created := model.FormatUTC(deletedAt.Add(-time.Hour))
	del := model.FormatUTC(deletedAt)
	var cxID int64
	if err := d.QueryRowContext(ctx, `SELECT context_id FROM projects WHERE id = ?`, projectID).Scan(&cxID); err != nil {
		t.Fatalf("project context: %v", err)
	}
	res, err := d.ExecContext(ctx,
		`INSERT INTO tasks (title, description, context_id, project_id, priority, status, day_part, plan_state, is_pinned, client_id, deleted_at, created_at, updated_at)
		 VALUES ('gone', '', ?, ?, 'no-priority', 'open', 'none', 'none', 0, ?, ?, ?, ?)`,
		cxID, projectID, clientID, del, created, del)
	if err != nil {
		t.Fatalf("seed tombstone task: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// TestDeleteTombstonesOlderThan_RemovesOldKeepsFresh asserts the retention GC
// hard-DELETEs a tombstone whose deleted_at predates the cutoff and KEEPS a
// fresh tombstone (US-3.7 AC5) plus a live (non-deleted) row.
func TestDeleteTombstonesOlderThan_RemovesOldKeepsFresh(t *testing.T) {
	d, s := openMigratedDB(t)
	ctx := context.Background()
	pid := seedFederatedProject(t, d)

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	oldID := seedTombstoneTask(t, d, pid, "task-old", now.Add(-100*24*time.Hour))    // 100d ago > 90d retention
	freshID := seedTombstoneTask(t, d, pid, "task-fresh", now.Add(-10*24*time.Hour)) // 10d ago < retention

	// A live (non-deleted) task must never be touched by tombstone GC.
	var cxID int64
	if err := d.QueryRow(`SELECT context_id FROM projects WHERE id = ?`, pid).Scan(&cxID); err != nil {
		t.Fatalf("ctx: %v", err)
	}
	res, err := d.Exec(
		`INSERT INTO tasks (title, description, context_id, project_id, priority, status, day_part, plan_state, is_pinned, client_id, created_at, updated_at)
		 VALUES ('live', '', ?, ?, 'no-priority', 'open', 'none', 'none', 0, 'task-live', ?, ?)`,
		cxID, pid, model.FormatUTC(now), model.FormatUTC(now))
	if err != nil {
		t.Fatalf("seed live: %v", err)
	}
	liveID, _ := res.LastInsertId()

	// Seed a stale field_hlc row for the old tombstone so the GC also prunes the
	// per-field HLC sidecar (no dangling resurrection guard for a hard-deleted row).
	if _, err := s.CASFieldHLC(ctx, "task", "task-old", "_deleted", "00000000000100-0000-nodeA"); err != nil {
		t.Fatalf("seed field hlc: %v", err)
	}

	cutoff := model.FormatUTC(now.Add(-90 * 24 * time.Hour))
	n, err := s.DeleteTombstonesOlderThan(ctx, "tasks", cutoff)
	if err != nil {
		t.Fatalf("gc: %v", err)
	}
	if n != 1 {
		t.Errorf("deleted count: got %d, want 1 (only the >90d tombstone)", n)
	}

	if rowExists(t, d, "tasks", oldID) {
		t.Errorf("old tombstone must be hard-deleted")
	}
	if !rowExists(t, d, "tasks", freshID) {
		t.Errorf("fresh tombstone must be kept (within retention)")
	}
	if !rowExists(t, d, "tasks", liveID) {
		t.Errorf("live row must never be GC'd")
	}

	// The pruned tombstone's field_hlc sidecar is also gone (no orphan guard).
	got, err := s.GetFieldHLC(ctx, "task", "task-old", "_deleted")
	if err != nil {
		t.Fatalf("get field hlc: %v", err)
	}
	if got != "" {
		t.Errorf("field_hlc for hard-deleted entity must be pruned: got %q", got)
	}
}

// TestPurgeDeliveredOutbox_RemovesAgedKeepsRecent asserts the GC purges outbox
// rows older than the outbox retention window and keeps recent ones (US-3.7 AC5).
func TestPurgeDeliveredOutbox_RemovesAgedKeepsRecent(t *testing.T) {
	d, s := openMigratedDB(t)
	ctx := context.Background()
	pid := seedFederatedProject(t, d)

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	insertOutbox(t, d, "ev-old", pid, model.FormatUTC(now.Add(-40*24*time.Hour))) // 40d > 30d outbox retention
	insertOutbox(t, d, "ev-recent", pid, model.FormatUTC(now.Add(-5*24*time.Hour)))

	cutoff := model.FormatUTC(now.Add(-30 * 24 * time.Hour))
	n, err := s.PurgeOutboxOlderThan(ctx, cutoff)
	if err != nil {
		t.Fatalf("purge outbox: %v", err)
	}
	if n != 1 {
		t.Errorf("purged outbox: got %d, want 1", n)
	}
	if outboxExists(t, d, "ev-old") {
		t.Errorf("aged outbox row must be purged")
	}
	if !outboxExists(t, d, "ev-recent") {
		t.Errorf("recent outbox row must be kept")
	}
}

// TestPurgeAppliedInbox_RemovesAppliedAgedKeepsPending asserts the GC purges only
// APPLIED inbox rows older than retention (dedup is no longer needed once aged),
// never an un-applied row (a pending event the queue still re-drives).
func TestPurgeAppliedInbox_RemovesAppliedAgedKeepsPending(t *testing.T) {
	d, s := openMigratedDB(t)
	ctx := context.Background()
	pid := seedFederatedProject(t, d)

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	old := model.FormatUTC(now.Add(-40 * 24 * time.Hour))
	insertInbox(t, d, "in-applied-old", pid, old, &old) // applied + aged → purge
	insertInbox(t, d, "in-pending-old", pid, old, nil)  // aged but NOT applied → keep
	recent := model.FormatUTC(now.Add(-1 * 24 * time.Hour))
	insertInbox(t, d, "in-applied-recent", pid, recent, &recent) // applied but recent → keep

	cutoff := model.FormatUTC(now.Add(-30 * 24 * time.Hour))
	n, err := s.PurgeAppliedInboxOlderThan(ctx, cutoff)
	if err != nil {
		t.Fatalf("purge inbox: %v", err)
	}
	if n != 1 {
		t.Errorf("purged inbox: got %d, want 1", n)
	}
	if inboxExists(t, d, "in-applied-old") {
		t.Errorf("applied+aged inbox row must be purged")
	}
	if !inboxExists(t, d, "in-pending-old") {
		t.Errorf("un-applied inbox row must be kept (queue still re-drives it)")
	}
	if !inboxExists(t, d, "in-applied-recent") {
		t.Errorf("recent applied inbox row must be kept (dedup window)")
	}
}

// TestOldestRetainedHLC_ReturnsMinMaxFieldHLC asserts OldestRetainedHLC returns
// the smallest per-event max-field HLC across a project's retained outbox events
// — the boundary the pull handler compares since_hlc against for the 410 emit
// (US-3.7 AC4). With no retained events it returns the empty string.
func TestOldestRetainedHLC_ReturnsMinMaxFieldHLC(t *testing.T) {
	d, s := openMigratedDB(t)
	ctx := context.Background()
	pid := seedFederatedProject(t, d)

	empty, err := s.OldestRetainedHLC(ctx, pid)
	if err != nil {
		t.Fatalf("oldest (empty): %v", err)
	}
	if empty != "" {
		t.Errorf("oldest with no events: got %q, want empty", empty)
	}

	insertOutboxWithHLC(t, d, "ev-a", pid, "00000000000500-0000-nodeA")
	insertOutboxWithHLC(t, d, "ev-b", pid, "00000000000300-0000-nodeA")
	insertOutboxWithHLC(t, d, "ev-c", pid, "00000000000900-0000-nodeA")

	got, err := s.OldestRetainedHLC(ctx, pid)
	if err != nil {
		t.Fatalf("oldest: %v", err)
	}
	if got != "00000000000300-0000-nodeA" {
		t.Errorf("oldest retained HLC: got %q, want the minimum (00000000000300-0000-nodeA)", got)
	}
}

// TestPurgeOutboxAdvancingFloor_RecordsPrunedFloor asserts the GC outbox purge
// advances the per-project durable pruned-floor HLC to the MAX event HLC of the
// rows it removes, and leaves the floor untouched (monotonic) for events it keeps
// (US-3.7 AC4 review fix). The floor is what the stale-pull 410 gate keys off once
// the outbox is GC'd to empty.
func TestPurgeOutboxAdvancingFloor_RecordsPrunedFloor(t *testing.T) {
	d, s := openMigratedDB(t)
	ctx := context.Background()
	pid := seedFederatedProject(t, d)

	// Two aged events (purged) and one recent event (kept). The pruned floor must
	// advance to the MAX HLC among the purged rows, not the kept one.
	insertOutboxWithHLCAt(t, d, "ev-aged-low", pid, "00000000000200-0000-nodeA", "2026-04-01T00:00:00.000Z")
	insertOutboxWithHLCAt(t, d, "ev-aged-high", pid, "00000000000700-0000-nodeA", "2026-04-02T00:00:00.000Z")
	insertOutboxWithHLCAt(t, d, "ev-recent", pid, "00000000000900-0000-nodeA", "2026-05-31T00:00:00.000Z")

	cutoff := "2026-05-01T00:00:00.000Z"
	now := "2026-06-01T12:00:00.000Z"
	n, err := s.PurgeOutboxOlderThanAdvancingFloor(ctx, cutoff, now)
	if err != nil {
		t.Fatalf("purge advancing floor: %v", err)
	}
	if n != 2 {
		t.Errorf("purged outbox: got %d, want 2 (the two aged rows)", n)
	}
	if outboxExists(t, d, "ev-aged-low") || outboxExists(t, d, "ev-aged-high") {
		t.Errorf("aged outbox rows must be purged")
	}
	if !outboxExists(t, d, "ev-recent") {
		t.Errorf("recent outbox row must be kept")
	}

	floor, err := s.PrunedFloorHLC(ctx, pid)
	if err != nil {
		t.Fatalf("pruned floor: %v", err)
	}
	if floor != "00000000000700-0000-nodeA" {
		t.Errorf("pruned floor: got %q, want the max purged HLC (00000000000700-0000-nodeA)", floor)
	}

	// A subsequent purge that removes a LOWER-HLC straggler must not move the floor
	// backwards (monotonic).
	insertOutboxWithHLCAt(t, d, "ev-aged-late", pid, "00000000000300-0000-nodeA", "2026-04-03T00:00:00.000Z")
	if _, err := s.PurgeOutboxOlderThanAdvancingFloor(ctx, cutoff, now); err != nil {
		t.Fatalf("second purge: %v", err)
	}
	floor2, err := s.PrunedFloorHLC(ctx, pid)
	if err != nil {
		t.Fatalf("pruned floor 2: %v", err)
	}
	if floor2 != "00000000000700-0000-nodeA" {
		t.Errorf("pruned floor must be monotonic: got %q, want unchanged 00000000000700-0000-nodeA", floor2)
	}
}

// TestPrunedFloorHLC_EmptyWhenNothingPruned asserts the floor is the empty string
// for a project the GC has never pruned (no false 410 for a never-GC'd project).
func TestPrunedFloorHLC_EmptyWhenNothingPruned(t *testing.T) {
	d, s := openMigratedDB(t)
	ctx := context.Background()
	pid := seedFederatedProject(t, d)

	floor, err := s.PrunedFloorHLC(ctx, pid)
	if err != nil {
		t.Fatalf("pruned floor: %v", err)
	}
	if floor != "" {
		t.Errorf("pruned floor with nothing pruned: got %q, want empty", floor)
	}
}

func insertOutboxWithHLCAt(t *testing.T, d *sql.DB, eventID string, projectID int64, hlc, createdAt string) {
	t.Helper()
	payload := `{"event_id":"` + eventID + `","op":"update","entity_type":"task","entity_id":"x","fields":{"title":{"value":"v","hlc":"` + hlc + `"}}}`
	_, err := d.Exec(
		`INSERT INTO federation_outbox (event_id, local_project_id, payload, delivered_to, created_at) VALUES (?, ?, ?, '', ?)`,
		eventID, projectID, payload, createdAt)
	if err != nil {
		t.Fatalf("insert outbox hlc at: %v", err)
	}
}

func insertOutbox(t *testing.T, d *sql.DB, eventID string, projectID int64, createdAt string) {
	t.Helper()
	_, err := d.Exec(
		`INSERT INTO federation_outbox (event_id, local_project_id, payload, delivered_to, created_at) VALUES (?, ?, '{}', '', ?)`,
		eventID, projectID, createdAt)
	if err != nil {
		t.Fatalf("insert outbox: %v", err)
	}
}

func insertOutboxWithHLC(t *testing.T, d *sql.DB, eventID string, projectID int64, hlc string) {
	t.Helper()
	payload := `{"event_id":"` + eventID + `","op":"update","entity_type":"task","entity_id":"x","fields":{"title":{"value":"v","hlc":"` + hlc + `"}}}`
	_, err := d.Exec(
		`INSERT INTO federation_outbox (event_id, local_project_id, payload, delivered_to, created_at) VALUES (?, ?, ?, '', '2026-06-01T00:00:00.000Z')`,
		eventID, projectID, payload)
	if err != nil {
		t.Fatalf("insert outbox hlc: %v", err)
	}
}

func insertInbox(t *testing.T, d *sql.DB, eventID string, projectID int64, receivedAt string, appliedAt *string) {
	t.Helper()
	_, err := d.Exec(
		`INSERT INTO federation_inbox (event_id, peer_instance_url, local_project_id, payload, applied_at, received_at) VALUES (?, 'https://alice.example', ?, '{}', ?, ?)`,
		eventID, projectID, appliedAt, receivedAt)
	if err != nil {
		t.Fatalf("insert inbox: %v", err)
	}
}

func rowExists(t *testing.T, d *sql.DB, table string, id int64) bool {
	t.Helper()
	var n int
	if err := d.QueryRow(`SELECT COUNT(1) FROM `+table+` WHERE id = ?`, id).Scan(&n); err != nil {
		t.Fatalf("row exists %s/%d: %v", table, id, err)
	}
	return n > 0
}

func outboxExists(t *testing.T, d *sql.DB, eventID string) bool {
	t.Helper()
	var n int
	if err := d.QueryRow(`SELECT COUNT(1) FROM federation_outbox WHERE event_id = ?`, eventID).Scan(&n); err != nil {
		t.Fatalf("outbox exists %s: %v", eventID, err)
	}
	return n > 0
}

func inboxExists(t *testing.T, d *sql.DB, eventID string) bool {
	t.Helper()
	var n int
	if err := d.QueryRow(`SELECT COUNT(1) FROM federation_inbox WHERE event_id = ?`, eventID).Scan(&n); err != nil {
		t.Fatalf("inbox exists %s: %v", eventID, err)
	}
	return n > 0
}

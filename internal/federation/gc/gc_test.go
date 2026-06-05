package gc_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/federation/gc"
	"github.com/lebe-dev/turboist/internal/federation/store"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

func openMigrated(t *testing.T) (*sql.DB, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "gc.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.RunMigrations(context.Background(), d); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d, store.New(d)
}

func seedProject(t *testing.T, d *sql.DB) int64 {
	t.Helper()
	now := model.FormatUTC(time.Now())
	res, err := d.Exec(`INSERT INTO contexts (name, color, is_favourite, created_at, updated_at) VALUES ('W', 'blue', 0, ?, ?)`, now, now)
	if err != nil {
		t.Fatalf("ctx: %v", err)
	}
	cx, _ := res.LastInsertId()
	res, err = d.Exec(
		`INSERT INTO projects (context_id, title, description, color, status, project_type, is_pinned, is_federated, client_id, created_at, updated_at)
		 VALUES (?, 'P', '', 'blue', 'open', 'generic', 0, 1, 'p-1', ?, ?)`, cx, now, now)
	if err != nil {
		t.Fatalf("proj: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func seedTombstone(t *testing.T, d *sql.DB, projectID int64, clientID string, deletedAt time.Time) {
	t.Helper()
	var cx int64
	if err := d.QueryRow(`SELECT context_id FROM projects WHERE id = ?`, projectID).Scan(&cx); err != nil {
		t.Fatalf("ctx: %v", err)
	}
	del := model.FormatUTC(deletedAt)
	if _, err := d.Exec(
		`INSERT INTO tasks (title, description, context_id, project_id, priority, status, day_part, plan_state, is_pinned, client_id, deleted_at, created_at, updated_at)
		 VALUES ('g', '', ?, ?, 'no-priority', 'open', 'none', 'none', 0, ?, ?, ?, ?)`,
		cx, projectID, clientID, del, del, del); err != nil {
		t.Fatalf("tombstone: %v", err)
	}
}

// TestRunOnce_PrunesAgedTombstonesAndQueues asserts a single GC pass hard-deletes
// tombstones beyond the configured retention while keeping fresh ones, and purges
// aged outbox/applied-inbox rows (US-3.7 AC5). The Collector is driven via the
// injectable clock so the test is deterministic.
func TestRunOnce_PrunesAgedTombstonesAndQueues(t *testing.T) {
	d, s := openMigrated(t)
	pid := seedProject(t, d)
	ctx := context.Background()

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	seedTombstone(t, d, pid, "t-old", now.Add(-100*24*time.Hour))
	seedTombstone(t, d, pid, "t-fresh", now.Add(-10*24*time.Hour))

	old := model.FormatUTC(now.Add(-40 * 24 * time.Hour))
	if _, err := d.Exec(`INSERT INTO federation_outbox (event_id, local_project_id, payload, delivered_to, created_at) VALUES ('o-old', ?, '{}', '', ?)`, pid, old); err != nil {
		t.Fatalf("outbox: %v", err)
	}
	if _, err := d.Exec(`INSERT INTO federation_inbox (event_id, peer_instance_url, local_project_id, payload, applied_at, received_at) VALUES ('i-old', 'https://a', ?, '{}', ?, ?)`, pid, old, old); err != nil {
		t.Fatalf("inbox: %v", err)
	}

	collector := gc.NewCollector(s, gc.Config{
		TombstoneRetention: 90 * 24 * time.Hour,
		OutboxRetention:    30 * 24 * time.Hour,
		InboxRetention:     30 * 24 * time.Hour,
	}, nil).WithClock(func() time.Time { return now })

	if err := collector.RunOnce(ctx); err != nil {
		t.Fatalf("run once: %v", err)
	}

	if count(t, d, "SELECT COUNT(1) FROM tasks WHERE client_id = 't-old'") != 0 {
		t.Errorf("aged tombstone must be hard-deleted")
	}
	if count(t, d, "SELECT COUNT(1) FROM tasks WHERE client_id = 't-fresh'") != 1 {
		t.Errorf("fresh tombstone must be kept")
	}
	if count(t, d, "SELECT COUNT(1) FROM federation_outbox WHERE event_id = 'o-old'") != 0 {
		t.Errorf("aged outbox row must be purged")
	}
	if count(t, d, "SELECT COUNT(1) FROM federation_inbox WHERE event_id = 'i-old'") != 0 {
		t.Errorf("aged applied inbox row must be purged")
	}
}

// TestRunOnce_ConfigSourceOverridesStatic asserts the live config source (US-8.4)
// overrides the static construction config on each pass: a tombstone that survives
// under the static 90d window is pruned once the live source shortens the window to
// 30d, without reconstructing the collector (Federation v1 F6.5).
func TestRunOnce_ConfigSourceOverridesStatic(t *testing.T) {
	d, s := openMigrated(t)
	pid := seedProject(t, d)
	ctx := context.Background()

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	// 50 days old: kept under the static 90d window, pruned under a live 30d window.
	seedTombstone(t, d, pid, "t-50d", now.Add(-50*24*time.Hour))

	live := gc.Config{TombstoneRetention: 90 * 24 * time.Hour}
	collector := gc.NewCollector(s, gc.Config{TombstoneRetention: 90 * 24 * time.Hour}, nil).
		WithClock(func() time.Time { return now }).
		WithConfigSource(func() gc.Config { return live })

	if err := collector.RunOnce(ctx); err != nil {
		t.Fatalf("run once (90d): %v", err)
	}
	if count(t, d, "SELECT COUNT(1) FROM tasks WHERE client_id = 't-50d'") != 1 {
		t.Errorf("50d tombstone must survive the 90d window")
	}

	// Shorten the live window — the next pass prunes the 50d tombstone.
	live = gc.Config{TombstoneRetention: 30 * 24 * time.Hour}
	if err := collector.RunOnce(ctx); err != nil {
		t.Fatalf("run once (30d): %v", err)
	}
	if count(t, d, "SELECT COUNT(1) FROM tasks WHERE client_id = 't-50d'") != 0 {
		t.Errorf("50d tombstone must be pruned after the live window shortened to 30d (US-8.4)")
	}
}

// TestRunOnce_PrunesAgedAuditLog asserts a GC pass hard-deletes audit rows beyond
// the configured 1-year retention while keeping fresh ones (Federation v1 F6.3,
// US-7.4 AC2), when the audit pruner is wired via WithAudit.
func TestRunOnce_PrunesAgedAuditLog(t *testing.T) {
	d, s := openMigrated(t)
	ctx := context.Background()
	auditRepo := repo.NewFederationAuditLogRepo(d)

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	if err := auditRepo.Insert(ctx, repo.AuditEntry{Kind: repo.AuditKindReplay, Outcome: repo.AuditOutcomeRejected, CreatedAt: now.Add(-400 * 24 * time.Hour)}); err != nil {
		t.Fatalf("insert old: %v", err)
	}
	if err := auditRepo.Insert(ctx, repo.AuditEntry{Kind: repo.AuditKindReplay, Outcome: repo.AuditOutcomeRejected, CreatedAt: now.Add(-10 * 24 * time.Hour)}); err != nil {
		t.Fatalf("insert fresh: %v", err)
	}

	collector := gc.NewCollector(s, gc.Config{}, nil).
		WithClock(func() time.Time { return now }).
		WithAudit(auditRepo, 365*24*time.Hour)

	if err := collector.RunOnce(ctx); err != nil {
		t.Fatalf("run once: %v", err)
	}

	if count(t, d, "SELECT COUNT(1) FROM federation_audit_log") != 1 {
		t.Errorf("aged audit row must be pruned, fresh row kept: got %d, want 1", count(t, d, "SELECT COUNT(1) FROM federation_audit_log"))
	}
}

func count(t *testing.T, d *sql.DB, q string) int {
	t.Helper()
	var n int
	if err := d.QueryRow(q).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", q, err)
	}
	return n
}

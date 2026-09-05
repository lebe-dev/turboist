package main

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

func setupChangeLogDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "prune.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.RunMigrations(context.Background(), d); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d
}

// seedChange writes one log row directly. The prune only ever reads seq and
// changed_at, so the entity behind the row is irrelevant here.
func seedChange(t *testing.T, d *sql.DB, entityID int64, changedAt time.Time) {
	t.Helper()
	if _, err := d.Exec(
		`INSERT INTO change_log (entity, entity_id, op, changed_at) VALUES ('label', ?, 'upsert', ?)`,
		entityID, model.FormatUTC(changedAt)); err != nil {
		t.Fatalf("seed change: %v", err)
	}
}

func countChanges(t *testing.T, d *sql.DB) int {
	t.Helper()
	var n int
	if err := d.QueryRow(`SELECT COUNT(*) FROM change_log`).Scan(&n); err != nil {
		t.Fatalf("count changes: %v", err)
	}
	return n
}

func pruneDoneRecord(cap *cleanupCaptureHandler) (lvl slog.Level, removed int64, found bool) {
	for _, r := range cap.snapshot() {
		if r.Message != "change log prune done" {
			continue
		}
		found = true
		lvl = r.Level
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == "removed" {
				removed = a.Value.Int64()
			}
			return true
		})
	}
	return lvl, removed, found
}

func TestPruneChangeLogOnce_RemovesHistoryPastTheWindow(t *testing.T) {
	d := setupChangeLogDB(t)
	seedChange(t, d, 1, time.Now().Add(-repo.SyncHistoryWindow-24*time.Hour))
	seedChange(t, d, 2, time.Now())

	cap := newCleanupCaptureHandler()
	pruneChangeLogOnce(context.Background(), repo.NewChangeLogRepo(d), slog.New(cap))

	if got := countChanges(t, d); got != 1 {
		t.Errorf("retained changes: got %d, want 1", got)
	}
	lvl, removed, found := pruneDoneRecord(cap)
	if !found {
		t.Fatal("no 'change log prune done' record")
	}
	if lvl != slog.LevelInfo {
		t.Errorf("level: got %v, want %v (rows removed)", lvl, slog.LevelInfo)
	}
	if removed != 1 {
		t.Errorf("removed: got %d, want 1", removed)
	}
}

func TestPruneChangeLogOnce_LogsDebugWhenNoneRemoved(t *testing.T) {
	d := setupChangeLogDB(t)
	seedChange(t, d, 1, time.Now())

	cap := newCleanupCaptureHandler()
	pruneChangeLogOnce(context.Background(), repo.NewChangeLogRepo(d), slog.New(cap))

	if got := countChanges(t, d); got != 1 {
		t.Errorf("retained changes: got %d, want 1", got)
	}
	lvl, removed, found := pruneDoneRecord(cap)
	if !found {
		t.Fatal("no 'change log prune done' record")
	}
	if lvl != slog.LevelDebug {
		t.Errorf("level: got %v, want %v (no rows removed)", lvl, slog.LevelDebug)
	}
	if removed != 0 {
		t.Errorf("removed: got %d, want 0", removed)
	}
}

// The job has to work on startup, not only on the first tick a day later: an
// install that restarts nightly would otherwise never prune anything.
func TestRunChangeLogPrune_RunsImmediatelyAndStopsOnCancel(t *testing.T) {
	d := setupChangeLogDB(t)
	seedChange(t, d, 1, time.Now().Add(-repo.SyncHistoryWindow-24*time.Hour))

	ctx, cancel := context.WithCancel(context.Background())
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	done := make(chan struct{})
	go func() {
		// A long interval guarantees only the immediate prune runs during the test.
		runChangeLogPrune(ctx, repo.NewChangeLogRepo(d), log, time.Hour)
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && countChanges(t, d) > 0 {
		time.Sleep(20 * time.Millisecond)
	}
	if got := countChanges(t, d); got != 0 {
		cancel()
		t.Fatalf("stale change was not pruned within 2s: %d rows left", got)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("prune goroutine did not stop after context cancel")
	}
}

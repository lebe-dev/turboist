package main

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

func setupIdempotencyDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "cleanup.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.RunMigrations(context.Background(), d); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// idempotency_keys.user_id has an FK to users(id); migration 002 does not
	// seed a user, so create id=1 for the inserts below.
	if _, err := repo.NewUserRepo(d).Create(context.Background(), "admin", "h"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	return d
}

func seedIdempotencyKey(t *testing.T, d *sql.DB, key string, createdAt time.Time) {
	t.Helper()
	_, err := d.ExecContext(context.Background(),
		`INSERT INTO idempotency_keys (key, user_id, method, path, status, response, created_at)
		 VALUES (?, 1, 'POST', '/api/v1/tasks/1/complete', 200, '{}', ?)`,
		key, model.FormatUTC(createdAt))
	if err != nil {
		t.Fatalf("seed key %s: %v", key, err)
	}
}

func idempotencyKeyExists(t *testing.T, d *sql.DB, key string) bool {
	t.Helper()
	var n int
	if err := d.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM idempotency_keys WHERE key = ?`, key).Scan(&n); err != nil {
		t.Fatalf("count key: %v", err)
	}
	return n > 0
}

func TestPruneIdempotencyOnce_RemovesExpiredKeepsFresh(t *testing.T) {
	d := setupIdempotencyDB(t)
	keys := repo.NewIdempotencyRepo(d)

	seedIdempotencyKey(t, d, "expired-key", time.Now().Add(-72*time.Hour)) // older than TTL
	seedIdempotencyKey(t, d, "fresh-key", time.Now())                      // within TTL

	cap := newCleanupCaptureHandler()
	pruneIdempotencyOnce(context.Background(), keys, slog.New(cap))

	if idempotencyKeyExists(t, d, "expired-key") {
		t.Errorf("expired-key: got present, want pruned")
	}
	if !idempotencyKeyExists(t, d, "fresh-key") {
		t.Errorf("fresh-key: got pruned, want present")
	}

	lvl, removed, found := cleanupDoneRecord(cap)
	if !found {
		t.Fatal("no 'idempotency cleanup done' record")
	}
	if lvl != slog.LevelInfo {
		t.Errorf("level: got %v, want %v (rows removed)", lvl, slog.LevelInfo)
	}
	if removed < 1 {
		t.Errorf("removed: got %d, want >= 1", removed)
	}
}

func TestPruneIdempotencyOnce_LogsDebugWhenNoneRemoved(t *testing.T) {
	d := setupIdempotencyDB(t)
	keys := repo.NewIdempotencyRepo(d)

	cap := newCleanupCaptureHandler()
	pruneIdempotencyOnce(context.Background(), keys, slog.New(cap))

	lvl, removed, found := cleanupDoneRecord(cap)
	if !found {
		t.Fatal("no 'idempotency cleanup done' record")
	}
	if lvl != slog.LevelDebug {
		t.Errorf("level: got %v, want %v (no rows removed)", lvl, slog.LevelDebug)
	}
	if removed != 0 {
		t.Errorf("removed: got %d, want 0", removed)
	}
}

func TestStartIdempotencyCleanup_RunsImmediatelyAndStopsOnCancel(t *testing.T) {
	d := setupIdempotencyDB(t)
	keys := repo.NewIdempotencyRepo(d)
	seedIdempotencyKey(t, d, "expired-key", time.Now().Add(-72*time.Hour))

	ctx, cancel := context.WithCancel(context.Background())
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	done := make(chan struct{})
	go func() {
		// A long interval guarantees only the immediate prune runs during the test.
		runIdempotencyCleanup(ctx, keys, log, time.Hour)
		close(done)
	}()

	// Poll briefly for the immediate prune to take effect.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && idempotencyKeyExists(t, d, "expired-key") {
		time.Sleep(20 * time.Millisecond)
	}
	if idempotencyKeyExists(t, d, "expired-key") {
		cancel()
		t.Fatalf("expired key was not pruned within 2s")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("cleanup goroutine did not stop after context cancel")
	}
}

// cleanupDoneRecord returns the level and "removed" count of the
// "idempotency cleanup done" record, if present.
func cleanupDoneRecord(cap *cleanupCaptureHandler) (lvl slog.Level, removed int64, found bool) {
	for _, r := range cap.snapshot() {
		if r.Message != "idempotency cleanup done" {
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

// cleanupCaptureHandler is a minimal slog.Handler that records emitted records
// so tests can inspect the log level and attributes of cleanup output.
type cleanupCaptureHandler struct {
	mu      *sync.Mutex
	records *[]slog.Record
}

func newCleanupCaptureHandler() *cleanupCaptureHandler {
	return &cleanupCaptureHandler{mu: &sync.Mutex{}, records: &[]slog.Record{}}
}

func (h *cleanupCaptureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *cleanupCaptureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	*h.records = append(*h.records, r.Clone())
	return nil
}

func (h *cleanupCaptureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *cleanupCaptureHandler) WithGroup(string) slog.Handler      { return h }

func (h *cleanupCaptureHandler) snapshot() []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]slog.Record, len(*h.records))
	copy(out, *h.records)
	return out
}

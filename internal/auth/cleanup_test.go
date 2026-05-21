package auth

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

func setupCleanupDB(t *testing.T) *sql.DB {
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
	if _, err := repo.NewUserRepo(d).Create(context.Background(), "admin", "h"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	return d
}

func TestStartSessionCleanup_RunsImmediatelyAndStopsOnCancel(t *testing.T) {
	d := setupCleanupDB(t)
	sessions := repo.NewSessionRepo(d)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create one expired session that should disappear after cleanup.
	s, err := sessions.Create(ctx, repo.CreateSessionParams{
		UserID: 1, TokenHash: "expired-tok", ClientKind: model.ClientWeb,
		ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := d.ExecContext(ctx, `UPDATE sessions SET expires_at = ? WHERE id = ?`,
		model.FormatUTC(time.Now().Add(-time.Hour)), s.ID); err != nil {
		t.Fatalf("backdate: %v", err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	go runSessionCleanup(ctx, sessions, log, time.Hour)

	// Poll briefly for the immediate cleanup to take effect.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_, err := sessions.GetByTokenHash(ctx, "expired-tok")
		if err != nil {
			return // ErrNotFound — cleanup happened.
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("expired session was not cleaned up within 2s")
}

func TestCleanupOnce_LogsDebugWhenNoneRemoved(t *testing.T) {
	d := setupCleanupDB(t)
	sessions := repo.NewSessionRepo(d)
	ctx := context.Background()

	cap := newCaptureHandler()
	log := slog.New(cap)
	cleanupOnce(ctx, sessions, log)

	var lvl slog.Level
	var removed int64
	var found bool
	for _, r := range cap.snapshot() {
		if r.Message == "session cleanup done" {
			lvl = r.Level
			found = true
			r.Attrs(func(a slog.Attr) bool {
				if a.Key == "removed" {
					removed = a.Value.Int64()
				}
				return true
			})
		}
	}
	if !found {
		t.Fatal("no 'session cleanup done' record")
	}
	if lvl != slog.LevelDebug {
		t.Errorf("level: got %v, want %v (no rows removed)", lvl, slog.LevelDebug)
	}
	if removed != 0 {
		t.Errorf("removed: got %d, want 0", removed)
	}
}

func TestCleanupOnce_LogsInfoWhenRowsRemoved(t *testing.T) {
	d := setupCleanupDB(t)
	sessions := repo.NewSessionRepo(d)
	ctx := context.Background()

	// Create an expired session.
	s, err := sessions.Create(ctx, repo.CreateSessionParams{
		UserID: 1, TokenHash: "old-tok", ClientKind: model.ClientWeb,
		ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := d.ExecContext(ctx, `UPDATE sessions SET expires_at = ? WHERE id = ?`,
		model.FormatUTC(time.Now().Add(-time.Hour)), s.ID); err != nil {
		t.Fatalf("backdate: %v", err)
	}

	cap := newCaptureHandler()
	log := slog.New(cap)
	cleanupOnce(ctx, sessions, log)

	var lvl slog.Level
	var removed int64
	for _, r := range cap.snapshot() {
		if r.Message == "session cleanup done" {
			lvl = r.Level
			r.Attrs(func(a slog.Attr) bool {
				if a.Key == "removed" {
					removed = a.Value.Int64()
				}
				return true
			})
		}
	}
	if lvl != slog.LevelInfo {
		t.Errorf("level: got %v, want %v", lvl, slog.LevelInfo)
	}
	if removed < 1 {
		t.Errorf("removed: got %d, want >= 1", removed)
	}
}

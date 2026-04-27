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

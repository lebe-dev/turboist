package repo

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

func mustCreateUser(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := NewUserRepo(db).Create(context.Background(), "admin", "h"); err != nil {
		t.Fatalf("create user: %v", err)
	}
}

func TestSessionRepo_CreateAndGet(t *testing.T) {
	db := setupTestDB(t)
	mustCreateUser(t, db)
	r := NewSessionRepo(db)
	ctx := context.Background()

	exp := time.Now().Add(30 * 24 * time.Hour)
	s, err := r.Create(ctx, CreateSessionParams{
		UserID: 1, TokenHash: "h1", ClientKind: model.ClientWeb,
		UserAgent: "ua/1", ExpiresAt: exp,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if s.ID == 0 {
		t.Errorf("id: got 0, want non-zero")
	}
	if s.ClientKind != model.ClientWeb {
		t.Errorf("client kind: got %q, want web", s.ClientKind)
	}
	if s.RevokedAt != nil {
		t.Errorf("revoked_at: got non-nil, want nil")
	}

	got, err := r.GetByTokenHash(ctx, "h1")
	if err != nil {
		t.Fatalf("get-by-hash: %v", err)
	}
	if got.ID != s.ID {
		t.Errorf("id: got %d, want %d", got.ID, s.ID)
	}
}

func TestSessionRepo_Rotate(t *testing.T) {
	db := setupTestDB(t)
	mustCreateUser(t, db)
	r := NewSessionRepo(db)
	ctx := context.Background()

	exp := time.Now().Add(30 * 24 * time.Hour)
	s, err := r.Create(ctx, CreateSessionParams{
		UserID: 1, TokenHash: "old", ClientKind: model.ClientCLI, ExpiresAt: exp,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	newExp := time.Now().Add(30 * 24 * time.Hour)
	if err := r.Rotate(ctx, s.ID, "new", newExp); err != nil {
		t.Fatalf("rotate: %v", err)
	}

	if _, err := r.GetByTokenHash(ctx, "old"); !errors.Is(err, ErrNotFound) {
		t.Errorf("old hash lookup: got %v, want ErrNotFound", err)
	}
	got, err := r.GetByTokenHash(ctx, "new")
	if err != nil {
		t.Fatalf("get new: %v", err)
	}
	if got.ID != s.ID {
		t.Errorf("rotation must keep id")
	}
}

func TestSessionRepo_Rotate_RevokedSessionFails(t *testing.T) {
	db := setupTestDB(t)
	mustCreateUser(t, db)
	r := NewSessionRepo(db)
	ctx := context.Background()
	exp := time.Now().Add(30 * 24 * time.Hour)
	s, _ := r.Create(ctx, CreateSessionParams{UserID: 1, TokenHash: "h", ClientKind: model.ClientWeb, ExpiresAt: exp})
	if err := r.Revoke(ctx, s.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	err := r.Rotate(ctx, s.ID, "new", exp)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("rotate revoked: got %v, want ErrNotFound", err)
	}
}

func TestSessionRepo_RevokeAllForUser(t *testing.T) {
	db := setupTestDB(t)
	mustCreateUser(t, db)
	r := NewSessionRepo(db)
	ctx := context.Background()
	exp := time.Now().Add(30 * 24 * time.Hour)
	for i, kind := range []model.ClientKind{model.ClientWeb, model.ClientIOS, model.ClientCLI} {
		_, err := r.Create(ctx, CreateSessionParams{
			UserID: 1, TokenHash: string(rune('a' + i)), ClientKind: kind, ExpiresAt: exp,
		})
		if err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}
	if err := r.RevokeAllForUser(ctx, 1); err != nil {
		t.Fatalf("revoke all: %v", err)
	}
	active, err := r.ListActiveForUser(ctx, 1)
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	if len(active) != 0 {
		t.Errorf("active sessions after revoke-all: got %d, want 0", len(active))
	}
}

func TestSessionRepo_EnforceLimit_KeepsNewest(t *testing.T) {
	db := setupTestDB(t)
	mustCreateUser(t, db)
	r := NewSessionRepo(db)
	ctx := context.Background()
	exp := time.Now().Add(30 * 24 * time.Hour)

	// Create 6 web sessions, each with progressively newer last_used_at.
	ids := make([]int64, 0, 6)
	for i := range 6 {
		s, err := r.Create(ctx, CreateSessionParams{
			UserID: 1, TokenHash: "tok-" + string(rune('a'+i)),
			ClientKind: model.ClientWeb, ExpiresAt: exp,
		})
		if err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
		ids = append(ids, s.ID)
		// Manually shift last_used_at to enforce ordering.
		bumped := time.Now().Add(time.Duration(i) * time.Second)
		if _, err := db.ExecContext(ctx,
			`UPDATE sessions SET last_used_at = ? WHERE id = ?`,
			model.FormatUTC(bumped), s.ID); err != nil {
			t.Fatalf("bump: %v", err)
		}
	}

	// Add a different client_kind session to ensure it isn't touched.
	cli, err := r.Create(ctx, CreateSessionParams{
		UserID: 1, TokenHash: "cli-1", ClientKind: model.ClientCLI, ExpiresAt: exp,
	})
	if err != nil {
		t.Fatalf("create cli: %v", err)
	}

	if err := r.EnforceLimit(ctx, 1, model.ClientWeb, 5); err != nil {
		t.Fatalf("enforce: %v", err)
	}

	// Oldest web session (ids[0]) must be gone; ids[1..5] retained.
	if _, err := r.Get(ctx, ids[0]); !errors.Is(err, ErrNotFound) {
		t.Errorf("oldest web session: got %v, want ErrNotFound", err)
	}
	for _, id := range ids[1:] {
		if _, err := r.Get(ctx, id); err != nil {
			t.Errorf("session %d should remain: %v", id, err)
		}
	}
	// CLI session must remain.
	if _, err := r.Get(ctx, cli.ID); err != nil {
		t.Errorf("cli session should remain: %v", err)
	}
}

func TestSessionRepo_Cleanup_RemovesExpiredAndOldRevoked(t *testing.T) {
	db := setupTestDB(t)
	mustCreateUser(t, db)
	r := NewSessionRepo(db)
	ctx := context.Background()

	// Active session — must remain.
	active, _ := r.Create(ctx, CreateSessionParams{
		UserID: 1, TokenHash: "active", ClientKind: model.ClientWeb,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	// Expired (past) session — must be removed.
	expired, _ := r.Create(ctx, CreateSessionParams{
		UserID: 1, TokenHash: "expired", ClientKind: model.ClientIOS,
		ExpiresAt: time.Now().Add(time.Hour),
	})
	if _, err := db.ExecContext(ctx, `UPDATE sessions SET expires_at = ? WHERE id = ?`,
		model.FormatUTC(time.Now().Add(-time.Hour)), expired.ID); err != nil {
		t.Fatalf("backdate expired: %v", err)
	}

	// Recently revoked (≤ 7d) — must remain.
	recent, _ := r.Create(ctx, CreateSessionParams{
		UserID: 1, TokenHash: "recent-revoked", ClientKind: model.ClientCLI,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})
	if _, err := db.ExecContext(ctx, `UPDATE sessions SET revoked_at = ? WHERE id = ?`,
		model.FormatUTC(time.Now().Add(-time.Hour)), recent.ID); err != nil {
		t.Fatalf("revoke recent: %v", err)
	}

	// Old revoked (> 7d) — must be removed.
	old, _ := r.Create(ctx, CreateSessionParams{
		UserID: 1, TokenHash: "old-revoked", ClientKind: model.ClientWeb,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})
	if _, err := db.ExecContext(ctx, `UPDATE sessions SET revoked_at = ? WHERE id = ?`,
		model.FormatUTC(time.Now().Add(-8*24*time.Hour)), old.ID); err != nil {
		t.Fatalf("revoke old: %v", err)
	}

	n, err := r.Cleanup(ctx)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if n != 2 {
		t.Errorf("removed: got %d, want 2", n)
	}

	if _, err := r.Get(ctx, active.ID); err != nil {
		t.Errorf("active session removed: %v", err)
	}
	if _, err := r.Get(ctx, expired.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("expired must be removed: got %v", err)
	}
	if _, err := r.Get(ctx, old.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("old revoked must be removed: got %v", err)
	}
	if _, err := r.Get(ctx, recent.ID); err != nil {
		t.Errorf("recent revoked must remain: %v", err)
	}
}

func TestSessionRepo_FK_CascadeOnUserDelete(t *testing.T) {
	db := setupTestDB(t)
	mustCreateUser(t, db)
	r := NewSessionRepo(db)
	ctx := context.Background()
	exp := time.Now().Add(time.Hour)
	if _, err := r.Create(ctx, CreateSessionParams{
		UserID: 1, TokenHash: "h", ClientKind: model.ClientWeb, ExpiresAt: exp,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM users WHERE id = 1`); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	if _, err := r.GetByTokenHash(ctx, "h"); !errors.Is(err, ErrNotFound) {
		t.Errorf("session not cascaded: got %v, want ErrNotFound", err)
	}
}

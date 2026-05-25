package repo

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"
)

func TestAPITokenRepo_CRUD(t *testing.T) {
	db := setupTestDB(t)
	if _, err := NewUserRepo(db).Create(context.Background(), "admin", "h"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	r := NewAPITokenRepo(db)
	ctx := context.Background()

	scopes := []string{"tasks:read", "tasks:write", "projects:read"}
	created, err := r.Create(ctx, 1, "n8n", "hash-1", scopes)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == 0 || created.UserID != 1 || created.Name != "n8n" || created.TokenHash != "hash-1" {
		t.Fatalf("unexpected created token: %+v", created)
	}
	if !reflect.DeepEqual(created.Scopes, scopes) {
		t.Fatalf("scopes after create: got %v, want %v", created.Scopes, scopes)
	}
	if created.CreatedAt.IsZero() {
		t.Fatalf("created_at must be set")
	}

	got, err := r.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != created.ID || got.Name != "n8n" {
		t.Fatalf("get mismatch: %+v", got)
	}
	if !reflect.DeepEqual(got.Scopes, scopes) {
		t.Errorf("scopes after get: got %v, want %v", got.Scopes, scopes)
	}

	byHash, err := r.GetByTokenHash(ctx, "hash-1")
	if err != nil {
		t.Fatalf("get by hash: %v", err)
	}
	if byHash.ID != created.ID {
		t.Fatalf("get by hash mismatch: %+v", byHash)
	}
	if !reflect.DeepEqual(byHash.Scopes, scopes) {
		t.Errorf("scopes after GetByTokenHash: got %v, want %v", byHash.Scopes, scopes)
	}

	if _, err := r.Create(ctx, 1, "other", "hash-2", []string{"*"}); err != nil {
		t.Fatalf("create second: %v", err)
	}

	list, err := r.ListByUser(ctx, 1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list length: got %d, want 2", len(list))
	}
	for _, tok := range list {
		if len(tok.Scopes) == 0 {
			t.Errorf("listed token %d has empty scopes", tok.ID)
		}
	}

	if err := r.Delete(ctx, created.ID, 1); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := r.Get(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestAPITokenRepo_GetByTokenHash_NotFound(t *testing.T) {
	db := setupTestDB(t)
	r := NewAPITokenRepo(db)
	if _, err := r.GetByTokenHash(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestAPITokenRepo_Create_HashConflict(t *testing.T) {
	db := setupTestDB(t)
	if _, err := NewUserRepo(db).Create(context.Background(), "admin", "h"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	r := NewAPITokenRepo(db)
	ctx := context.Background()
	if _, err := r.Create(ctx, 1, "a", "same-hash", []string{"*"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := r.Create(ctx, 1, "b", "same-hash", []string{"*"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestAPITokenRepo_Delete_WrongUser(t *testing.T) {
	db := setupTestDB(t)
	if _, err := NewUserRepo(db).Create(context.Background(), "admin", "h"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	r := NewAPITokenRepo(db)
	ctx := context.Background()
	created, err := r.Create(ctx, 1, "n8n", "h", []string{"*"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := r.Delete(ctx, created.ID, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete with wrong user must return ErrNotFound, got %v", err)
	}
}

// Existing rows inserted before the 023 migration default to scopes='["*"]',
// so reading a freshly-migrated row with the column default must yield ["*"].
func TestAPITokenRepo_BackwardCompat_DefaultWildcard(t *testing.T) {
	db := setupTestDB(t)
	if _, err := NewUserRepo(db).Create(context.Background(), "admin", "h"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	// Bypass the repo's Create so we exercise the column DEFAULT, which is
	// what existing rows from before the migration effectively rely on.
	ctx := context.Background()
	insertWithDefaultScopes(t, db, ctx, 1, "legacy", "legacy-hash")

	r := NewAPITokenRepo(db)
	tok, err := r.GetByTokenHash(ctx, "legacy-hash")
	if err != nil {
		t.Fatalf("get by hash: %v", err)
	}
	want := []string{"*"}
	if !reflect.DeepEqual(tok.Scopes, want) {
		t.Errorf("legacy scopes default: got %v, want %v", tok.Scopes, want)
	}
}

func insertWithDefaultScopes(t *testing.T, db *sql.DB, ctx context.Context, userID int64, name, hash string) {
	t.Helper()
	_, err := db.ExecContext(ctx,
		`INSERT INTO api_tokens (user_id, name, token_hash, created_at) VALUES (?, ?, ?, ?)`,
		userID, name, hash, "2024-01-01T00:00:00.000Z")
	if err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
}

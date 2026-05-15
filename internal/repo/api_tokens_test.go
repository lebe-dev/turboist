package repo

import (
	"context"
	"errors"
	"testing"
)

func TestAPITokenRepo_CRUD(t *testing.T) {
	db := setupTestDB(t)
	if _, err := NewUserRepo(db).Create(context.Background(), "admin", "h"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	r := NewAPITokenRepo(db)
	ctx := context.Background()

	created, err := r.Create(ctx, 1, "n8n", "hash-1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == 0 || created.UserID != 1 || created.Name != "n8n" || created.TokenHash != "hash-1" {
		t.Fatalf("unexpected created token: %+v", created)
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

	byHash, err := r.GetByTokenHash(ctx, "hash-1")
	if err != nil {
		t.Fatalf("get by hash: %v", err)
	}
	if byHash.ID != created.ID {
		t.Fatalf("get by hash mismatch: %+v", byHash)
	}

	if _, err := r.Create(ctx, 1, "other", "hash-2"); err != nil {
		t.Fatalf("create second: %v", err)
	}

	list, err := r.ListByUser(ctx, 1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list length: got %d, want 2", len(list))
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
	if _, err := r.Create(ctx, 1, "a", "same-hash"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := r.Create(ctx, 1, "b", "same-hash"); !errors.Is(err, ErrConflict) {
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
	created, err := r.Create(ctx, 1, "n8n", "h")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := r.Delete(ctx, created.ID, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete with wrong user must return ErrNotFound, got %v", err)
	}
}

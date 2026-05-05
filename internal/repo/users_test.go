package repo

import (
	"context"
	"errors"
	"testing"

	"github.com/lebe-dev/turboist/internal/model"
)

func TestUserRepo_CreateAndGet(t *testing.T) {
	db := setupTestDB(t)
	r := NewUserRepo(db)
	ctx := context.Background()

	u, err := r.Create(ctx, "admin", "hash1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if u.ID != 1 {
		t.Errorf("id: got %d, want 1", u.ID)
	}
	if u.Username != "admin" {
		t.Errorf("username: got %q, want admin", u.Username)
	}

	got, err := r.Get(ctx, 1)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.PasswordHash != "hash1" {
		t.Errorf("hash: got %q, want hash1", got.PasswordHash)
	}
}

func TestUserRepo_Exists(t *testing.T) {
	db := setupTestDB(t)
	r := NewUserRepo(db)
	ctx := context.Background()

	exists, err := r.Exists(ctx)
	if err != nil {
		t.Fatalf("exists: %v", err)
	}
	if exists {
		t.Errorf("exists before create: got true, want false")
	}

	if _, err := r.Create(ctx, "admin", "h"); err != nil {
		t.Fatalf("create: %v", err)
	}
	exists, err = r.Exists(ctx)
	if err != nil {
		t.Fatalf("exists: %v", err)
	}
	if !exists {
		t.Errorf("exists after create: got false, want true")
	}
}

func TestUserRepo_Create_SecondUserBlockedByCheck(t *testing.T) {
	db := setupTestDB(t)
	r := NewUserRepo(db)
	ctx := context.Background()

	if _, err := r.Create(ctx, "admin", "h"); err != nil {
		t.Fatalf("first create: %v", err)
	}
	// Second create with id=1 must conflict (PK collision).
	_, err := r.Create(ctx, "other", "h2")
	if err == nil {
		t.Fatalf("expected error on second user create")
	}
	if !errors.Is(err, ErrConflict) {
		t.Errorf("err: got %v, want ErrConflict", err)
	}
}

func TestUserRepo_GetByUsername(t *testing.T) {
	db := setupTestDB(t)
	r := NewUserRepo(db)
	ctx := context.Background()

	if _, err := r.Create(ctx, "admin", "h"); err != nil {
		t.Fatalf("create: %v", err)
	}
	u, err := r.GetByUsername(ctx, "admin")
	if err != nil {
		t.Fatalf("get-by-username: %v", err)
	}
	if u.ID != 1 {
		t.Errorf("id: got %d, want 1", u.ID)
	}

	_, err = r.GetByUsername(ctx, "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("missing user: got %v, want ErrNotFound", err)
	}
}

func TestUserRepo_TroikiCapacity_DefaultsToZero(t *testing.T) {
	db := setupTestDB(t)
	r := NewUserRepo(db)
	ctx := context.Background()
	if _, err := r.Create(ctx, "admin", "h"); err != nil {
		t.Fatalf("create: %v", err)
	}
	c, err := r.GetTroikiCapacity(ctx, 1)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if c.Medium != 0 || c.Rest != 0 {
		t.Errorf("defaults: got %+v, want {0,0}", c)
	}
}

func TestUserRepo_IncTroikiCapacity_Medium(t *testing.T) {
	db := setupTestDB(t)
	r := NewUserRepo(db)
	ctx := context.Background()
	if _, err := r.Create(ctx, "admin", "h"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := r.IncTroikiCapacity(ctx, 1, model.TroikiCategoryMedium); err != nil {
		t.Fatalf("inc: %v", err)
	}
	if err := r.IncTroikiCapacity(ctx, 1, model.TroikiCategoryMedium); err != nil {
		t.Fatalf("inc2: %v", err)
	}
	if err := r.IncTroikiCapacity(ctx, 1, model.TroikiCategoryRest); err != nil {
		t.Fatalf("inc rest: %v", err)
	}
	c, _ := r.GetTroikiCapacity(ctx, 1)
	if c.Medium != 2 {
		t.Errorf("medium: got %d, want 2", c.Medium)
	}
	if c.Rest != 1 {
		t.Errorf("rest: got %d, want 1", c.Rest)
	}
}

func TestUserRepo_IncTroikiCapacity_RejectsImportant(t *testing.T) {
	db := setupTestDB(t)
	r := NewUserRepo(db)
	ctx := context.Background()
	if _, err := r.Create(ctx, "admin", "h"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := r.IncTroikiCapacity(ctx, 1, model.TroikiCategoryImportant); err == nil {
		t.Error("expected error for important")
	}
}

func TestUserRepo_GetTroikiCapacity_NotFound(t *testing.T) {
	db := setupTestDB(t)
	r := NewUserRepo(db)
	ctx := context.Background()
	if _, err := r.GetTroikiCapacity(ctx, 99); !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

func TestUserRepo_UpdatePasswordHash(t *testing.T) {
	db := setupTestDB(t)
	r := NewUserRepo(db)
	ctx := context.Background()
	if _, err := r.Create(ctx, "admin", "h1"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := r.UpdatePasswordHash(ctx, 1, "h2"); err != nil {
		t.Fatalf("update: %v", err)
	}
	u, err := r.Get(ctx, 1)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if u.PasswordHash != "h2" {
		t.Errorf("hash: got %q, want h2", u.PasswordHash)
	}
}

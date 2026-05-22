package repo

import (
	"context"
	"errors"
	"testing"
)

func TestTOTPRecoveryRepo_ReplaceAndListUnused(t *testing.T) {
	db := setupTestDB(t)
	users := NewUserRepo(db)
	rec := NewTOTPRecoveryRepo(db)
	ctx := context.Background()
	if _, err := users.Create(ctx, "admin", "h"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	hashes := []string{"h1", "h2", "h3"}
	if err := rec.Replace(ctx, 1, hashes); err != nil {
		t.Fatalf("replace: %v", err)
	}
	got, err := rec.ListUnused(ctx, 1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len: got %d, want 3", len(got))
	}
	for i, c := range got {
		if c.CodeHash != hashes[i] {
			t.Errorf("hash[%d]: got %q, want %q", i, c.CodeHash, hashes[i])
		}
		if c.UsedAt != nil {
			t.Errorf("used_at[%d]: got %v, want nil", i, c.UsedAt)
		}
	}
}

func TestTOTPRecoveryRepo_Replace_OverwritesExisting(t *testing.T) {
	db := setupTestDB(t)
	users := NewUserRepo(db)
	rec := NewTOTPRecoveryRepo(db)
	ctx := context.Background()
	if _, err := users.Create(ctx, "admin", "h"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := rec.Replace(ctx, 1, []string{"a", "b"}); err != nil {
		t.Fatalf("replace1: %v", err)
	}
	if err := rec.Replace(ctx, 1, []string{"x"}); err != nil {
		t.Fatalf("replace2: %v", err)
	}
	got, _ := rec.ListUnused(ctx, 1)
	if len(got) != 1 || got[0].CodeHash != "x" {
		t.Errorf("after second replace: got %+v, want [x]", got)
	}
}

func TestTOTPRecoveryRepo_MarkUsed(t *testing.T) {
	db := setupTestDB(t)
	users := NewUserRepo(db)
	rec := NewTOTPRecoveryRepo(db)
	ctx := context.Background()
	if _, err := users.Create(ctx, "admin", "h"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := rec.Replace(ctx, 1, []string{"a", "b"}); err != nil {
		t.Fatalf("replace: %v", err)
	}
	all, _ := rec.ListUnused(ctx, 1)
	if err := rec.MarkUsed(ctx, all[0].ID); err != nil {
		t.Fatalf("mark used: %v", err)
	}
	// Second time is a no-op (already used) -> ErrNotFound.
	if err := rec.MarkUsed(ctx, all[0].ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second mark used: got %v, want ErrNotFound", err)
	}
	unused, _ := rec.ListUnused(ctx, 1)
	if len(unused) != 1 || unused[0].ID != all[1].ID {
		t.Errorf("unused after consume: got %+v, want only [b]", unused)
	}
}

func TestTOTPRecoveryRepo_DeleteAll(t *testing.T) {
	db := setupTestDB(t)
	users := NewUserRepo(db)
	rec := NewTOTPRecoveryRepo(db)
	ctx := context.Background()
	if _, err := users.Create(ctx, "admin", "h"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := rec.Replace(ctx, 1, []string{"a", "b"}); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if err := rec.DeleteAll(ctx, 1); err != nil {
		t.Fatalf("delete: %v", err)
	}
	got, _ := rec.ListUnused(ctx, 1)
	if len(got) != 0 {
		t.Errorf("after delete: got %d, want 0", len(got))
	}
}

func TestTOTPRecoveryRepo_MarkUsed_Missing(t *testing.T) {
	db := setupTestDB(t)
	rec := NewTOTPRecoveryRepo(db)
	ctx := context.Background()
	if err := rec.MarkUsed(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

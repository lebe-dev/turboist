package repo

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

// setupWebAuthnDB returns a migrated database with the single account seeded,
// since every credential row is FK-bound to a user.
func setupWebAuthnDB(t *testing.T) *sql.DB {
	t.Helper()
	d := setupTestDB(t)
	if _, err := NewUserRepo(d).Create(context.Background(), "admin", "h"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return d
}

func TestWebAuthnRepo_EnsureHandle_StableAcrossCalls(t *testing.T) {
	r := NewWebAuthnRepo(setupWebAuthnDB(t))
	ctx := context.Background()

	first, err := r.EnsureHandle(ctx, 1)
	if err != nil {
		t.Fatalf("ensure handle: %v", err)
	}
	if first == "" {
		t.Fatal("handle: got empty string, want generated value")
	}
	second, err := r.EnsureHandle(ctx, 1)
	if err != nil {
		t.Fatalf("ensure handle again: %v", err)
	}
	if second != first {
		t.Errorf("handle: got %q, want %q", second, first)
	}
}

func TestWebAuthnRepo_UserIDByHandle(t *testing.T) {
	r := NewWebAuthnRepo(setupWebAuthnDB(t))
	ctx := context.Background()

	handle, err := r.EnsureHandle(ctx, 1)
	if err != nil {
		t.Fatalf("ensure handle: %v", err)
	}
	userID, err := r.UserIDByHandle(ctx, handle)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if userID != 1 {
		t.Errorf("user id: got %d, want 1", userID)
	}
	if _, err := r.UserIDByHandle(ctx, "unknown"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown handle: got %v, want ErrNotFound", err)
	}
}

func TestWebAuthnRepo_CreateAndList(t *testing.T) {
	r := NewWebAuthnRepo(setupWebAuthnDB(t))
	ctx := context.Background()

	created, err := r.Create(ctx, CreateCredentialParams{
		UserID:       1,
		CredentialID: "cred-1",
		Name:         "MacBook",
		Credential:   []byte(`{"id":"AQ"}`),
		SignCount:    3,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Name != "MacBook" || created.SignCount != 3 {
		t.Errorf("created: got name=%q signCount=%d, want name=MacBook signCount=3", created.Name, created.SignCount)
	}
	if created.LastUsedAt != nil {
		t.Errorf("lastUsedAt: got %v, want nil for a fresh credential", created.LastUsedAt)
	}

	rows, err := r.ListByUser(ctx, 1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("list: got %d rows, want 1", len(rows))
	}

	n, err := r.CountAll(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("count: got %d, want 1", n)
	}
}

func TestWebAuthnRepo_Create_DuplicateCredentialID(t *testing.T) {
	r := NewWebAuthnRepo(setupWebAuthnDB(t))
	ctx := context.Background()
	params := CreateCredentialParams{UserID: 1, CredentialID: "cred-1", Name: "Key", Credential: []byte(`{}`)}

	if _, err := r.Create(ctx, params); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := r.Create(ctx, params); !errors.Is(err, ErrConflict) {
		t.Errorf("duplicate create: got %v, want ErrConflict", err)
	}
}

func TestWebAuthnRepo_TouchAfterLogin(t *testing.T) {
	r := NewWebAuthnRepo(setupWebAuthnDB(t))
	ctx := context.Background()

	created, err := r.Create(ctx, CreateCredentialParams{
		UserID: 1, CredentialID: "cred-1", Name: "Key", Credential: []byte(`{}`), SignCount: 1,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := r.TouchAfterLogin(ctx, "cred-1", []byte(`{"updated":true}`), 9); err != nil {
		t.Fatalf("touch: %v", err)
	}

	got, err := r.Get(ctx, created.ID, 1)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.SignCount != 9 {
		t.Errorf("signCount: got %d, want 9", got.SignCount)
	}
	if string(got.Credential) != `{"updated":true}` {
		t.Errorf("credential: got %s, want the rewritten blob", got.Credential)
	}
	if got.LastUsedAt == nil {
		t.Error("lastUsedAt: got nil, want a timestamp after a successful assertion")
	}

	if err := r.TouchAfterLogin(ctx, "missing", []byte(`{}`), 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("touch unknown: got %v, want ErrNotFound", err)
	}
}

func TestWebAuthnRepo_RenameAndDelete(t *testing.T) {
	r := NewWebAuthnRepo(setupWebAuthnDB(t))
	ctx := context.Background()

	created, err := r.Create(ctx, CreateCredentialParams{
		UserID: 1, CredentialID: "cred-1", Name: "Key", Credential: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := r.Rename(ctx, created.ID, 1, "YubiKey"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	got, err := r.Get(ctx, created.ID, 1)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "YubiKey" {
		t.Errorf("name: got %q, want YubiKey", got.Name)
	}

	// A credential belonging to another user must be invisible to this one.
	if err := r.Rename(ctx, created.ID, 2, "Nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("rename foreign: got %v, want ErrNotFound", err)
	}
	if err := r.Delete(ctx, created.ID, 2); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete foreign: got %v, want ErrNotFound", err)
	}

	if err := r.Delete(ctx, created.ID, 1); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := r.Get(ctx, created.ID, 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("get after delete: got %v, want ErrNotFound", err)
	}
}

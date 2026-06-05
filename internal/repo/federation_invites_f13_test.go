package repo

import (
	"context"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

// seedInvite creates an invite row for the seeded project (id=1).
func seedInvite(t *testing.T, r *FederationInviteRepo, id string) {
	t.Helper()
	exp := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := r.Create(context.Background(), model.FederationInvite{
		InviteID:       id,
		LocalProjectID: 1,
		SecretHash:     "hash-" + id,
		Permissions:    model.FederationPermissionWrite,
		MaxUses:        1,
		ExpiresAt:      &exp,
		CreatedAt:      time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("seed invite %q: %v", id, err)
	}
}

// TestFederationInviteRepo_Revoke_SetsRevokedAtIdempotent asserts Revoke stamps
// revoked_at once and is idempotent — a second revoke does not move the stored
// timestamp (US-1.3 AC2).
func TestFederationInviteRepo_Revoke_SetsRevokedAtIdempotent(t *testing.T) {
	d := setupTestDB(t)
	seedFederatedProjectRow(t, d)
	r := NewFederationInviteRepo(d)
	seedInvite(t, r, "inv-revoke")

	first := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := r.Revoke(context.Background(), "inv-revoke", first); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	got, err := r.Get(context.Background(), "inv-revoke")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.RevokedAt == nil || !got.RevokedAt.Equal(first) {
		t.Fatalf("revoked_at: got %v, want %v", got.RevokedAt, first)
	}

	// Idempotent: a later revoke leaves the original timestamp untouched.
	if err := r.Revoke(context.Background(), "inv-revoke", first.Add(time.Hour)); err != nil {
		t.Fatalf("second revoke: %v", err)
	}
	got2, err := r.Get(context.Background(), "inv-revoke")
	if err != nil {
		t.Fatalf("get after second revoke: %v", err)
	}
	if got2.RevokedAt == nil || !got2.RevokedAt.Equal(first) {
		t.Errorf("revoked_at moved on second revoke: got %v, want %v (idempotent)", got2.RevokedAt, first)
	}
}

// TestFederationInviteRepo_Revoke_NotFound asserts revoking an unknown id is
// reported as ErrNotFound so the handler can map it to 404.
func TestFederationInviteRepo_Revoke_NotFound(t *testing.T) {
	d := setupTestDB(t)
	seedFederatedProjectRow(t, d)
	r := NewFederationInviteRepo(d)
	if err := r.Revoke(context.Background(), "missing", time.Now()); err != ErrNotFound {
		t.Fatalf("revoke missing: got %v, want ErrNotFound", err)
	}
}

// TestFederationInviteRepo_Delete_HardDeletes asserts Delete hard-removes the
// invite row (US-1.3 AC3). Peer rows live in a separate table and are untouched.
func TestFederationInviteRepo_Delete_HardDeletes(t *testing.T) {
	d := setupTestDB(t)
	seedFederatedProjectRow(t, d)
	r := NewFederationInviteRepo(d)
	seedInvite(t, r, "inv-del")

	if err := r.Delete(context.Background(), "inv-del"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := r.Get(context.Background(), "inv-del"); err != ErrNotFound {
		t.Errorf("after delete: got %v, want ErrNotFound", err)
	}
}

// TestFederationInviteRepo_Delete_NotFound asserts deleting an unknown id is
// reported as ErrNotFound.
func TestFederationInviteRepo_Delete_NotFound(t *testing.T) {
	d := setupTestDB(t)
	seedFederatedProjectRow(t, d)
	r := NewFederationInviteRepo(d)
	if err := r.Delete(context.Background(), "missing"); err != ErrNotFound {
		t.Fatalf("delete missing: got %v, want ErrNotFound", err)
	}
}

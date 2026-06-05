package repo

import (
	"context"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

// TestTouchLastContact_UpdatesExisting asserts the freshness touchpoint advances
// an existing peer's last_contact_at without disturbing its public_key or
// display_name (Federation v1 F5.6a, US-6.5 AC1/AC3 — the owner-returns signal).
func TestTouchLastContact_UpdatesExisting(t *testing.T) {
	d := setupTestDB(t)
	r := NewFederatedInstanceRepo(d)
	ctx := context.Background()
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := r.Upsert(ctx, model.FederatedInstance{
		InstanceURL: "https://owner.example", PublicKey: "pk-1", DisplayName: "Owner",
		LastContactAt: &created, CreatedAt: created, UpdatedAt: created,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	touch := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	if err := r.TouchLastContact(ctx, "https://owner.example", touch); err != nil {
		t.Fatalf("touch: %v", err)
	}

	got, err := r.Get(ctx, "https://owner.example")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.LastContactAt == nil || !got.LastContactAt.Equal(touch) {
		t.Errorf("last_contact_at: got %v, want %s", got.LastContactAt, touch)
	}
	if got.PublicKey != "pk-1" {
		t.Errorf("public_key: got %q, want pk-1 (touch must not clobber the key)", got.PublicKey)
	}
	if got.DisplayName != "Owner" {
		t.Errorf("display_name: got %q, want Owner (touch must not clobber the name)", got.DisplayName)
	}
	if !got.CreatedAt.Equal(created) {
		t.Errorf("created_at: got %s, want %s (preserved)", got.CreatedAt, created)
	}
}

// TestTouchLastContact_UnknownPeerNoOp asserts touching a peer not in the
// directory neither errors nor CREATES a row (Federation v1 F5.6a): only the
// handshake/join may insert a trust-directory row, never a contact touch.
func TestTouchLastContact_UnknownPeerNoOp(t *testing.T) {
	d := setupTestDB(t)
	r := NewFederatedInstanceRepo(d)
	ctx := context.Background()

	if err := r.TouchLastContact(ctx, "https://stranger.example", time.Now()); err != nil {
		t.Fatalf("touch unknown: %v", err)
	}
	if _, err := r.Get(ctx, "https://stranger.example"); err != ErrNotFound {
		t.Errorf("get unknown after touch: got %v, want ErrNotFound (no row created)", err)
	}
}

// TestUpdatePublicKey_OverwritesKeyOnly asserts the trust-key path overwrites a
// peer's pinned public_key (Federation v1 F5.6b, US-6.4 AC3) without disturbing its
// display_name, last_contact_at, or created_at — only the key and updated_at move.
func TestUpdatePublicKey_OverwritesKeyOnly(t *testing.T) {
	d := setupTestDB(t)
	r := NewFederatedInstanceRepo(d)
	ctx := context.Background()
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	contact := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	if err := r.Upsert(ctx, model.FederatedInstance{
		InstanceURL: "https://peer.example", PublicKey: "old-key", DisplayName: "Peer",
		LastContactAt: &contact, CreatedAt: created, UpdatedAt: created,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	n, err := r.UpdatePublicKey(ctx, "https://peer.example", "new-key", time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("update key: %v", err)
	}
	if n != 1 {
		t.Fatalf("update key rows: got %d, want 1", n)
	}

	got, err := r.Get(ctx, "https://peer.example")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.PublicKey != "new-key" {
		t.Errorf("public_key: got %q, want new-key", got.PublicKey)
	}
	if got.DisplayName != "Peer" {
		t.Errorf("display_name: got %q, want Peer (untouched)", got.DisplayName)
	}
	if got.LastContactAt == nil || !got.LastContactAt.Equal(contact) {
		t.Errorf("last_contact_at: got %v, want %s (untouched)", got.LastContactAt, contact)
	}
	if !got.CreatedAt.Equal(created) {
		t.Errorf("created_at: got %s, want %s (preserved)", got.CreatedAt, created)
	}
}

// TestUpdatePublicKey_UnknownPeerZeroRows asserts updating an unknown peer's key
// is a no-op (0 rows, nil error) and never CREATES a directory row.
func TestUpdatePublicKey_UnknownPeerZeroRows(t *testing.T) {
	d := setupTestDB(t)
	r := NewFederatedInstanceRepo(d)
	ctx := context.Background()

	n, err := r.UpdatePublicKey(ctx, "https://stranger.example", "k", time.Now())
	if err != nil {
		t.Fatalf("update unknown: %v", err)
	}
	if n != 0 {
		t.Errorf("update unknown rows: got %d, want 0", n)
	}
}

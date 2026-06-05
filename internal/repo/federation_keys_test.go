package repo

import (
	"context"
	"testing"

	"github.com/lebe-dev/turboist/internal/crypto"
)

const fedTestKey = "federation-cipher-key-32-bytes-min-padding!"

func TestFederationKeysRepo_EnsureGeneratesOnce(t *testing.T) {
	d := setupTestDB(t)
	r := NewFederationKeysRepo(d)
	cipher := crypto.NewTokenCipher(fedTestKey)
	ctx := context.Background()

	first, err := r.Ensure(ctx, cipher, "alice.example")
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if first.PublicKey == "" {
		t.Fatal("public key empty after ensure")
	}
	if !crypto.IsEncrypted(first.PrivateSeedEnc) {
		t.Fatalf("private seed not encrypted: %q", first.PrivateSeedEnc)
	}
	if first.NodeID == "" {
		t.Fatal("node_id empty after ensure")
	}
	if first.DisplayName != "alice.example" {
		t.Errorf("display_name: got %q, want %q", first.DisplayName, "alice.example")
	}

	// Lazy-gen is idempotent (INSERT OR IGNORE): a second Ensure returns the
	// same keypair and node_id, never regenerating.
	second, err := r.Ensure(ctx, cipher, "different.example")
	if err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if second.PublicKey != first.PublicKey {
		t.Errorf("public key changed on second ensure: %q vs %q", second.PublicKey, first.PublicKey)
	}
	if second.NodeID != first.NodeID {
		t.Errorf("node_id changed on second ensure: %q vs %q", second.NodeID, first.NodeID)
	}
	if second.PrivateSeedEnc != first.PrivateSeedEnc {
		t.Error("private seed changed on second ensure")
	}
	// The display name from the first ensure is sticky (not overwritten).
	if second.DisplayName != "alice.example" {
		t.Errorf("display_name overwritten: got %q, want %q", second.DisplayName, "alice.example")
	}
}

func TestFederationKeysRepo_GetMissing(t *testing.T) {
	d := setupTestDB(t)
	r := NewFederationKeysRepo(d)

	_, err := r.Get(context.Background())
	if err != ErrNotFound {
		t.Fatalf("Get before Ensure: got %v, want ErrNotFound", err)
	}
}

func TestFederationKeysRepo_GetAfterEnsure(t *testing.T) {
	d := setupTestDB(t)
	r := NewFederationKeysRepo(d)
	cipher := crypto.NewTokenCipher(fedTestKey)
	ctx := context.Background()

	ensured, err := r.Ensure(ctx, cipher, "host.example")
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	got, err := r.Get(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.PublicKey != ensured.PublicKey {
		t.Errorf("public key: got %q, want %q", got.PublicKey, ensured.PublicKey)
	}
	if got.ID != 1 {
		t.Errorf("id: got %d, want 1", got.ID)
	}
	// The stored seed decrypts to a usable private key matching the public key.
	if _, _, err := crypto.LoadInstanceKeypair(cipher, got.PublicKey, got.PrivateSeedEnc); err != nil {
		t.Errorf("loaded keypair does not round-trip: %v", err)
	}
}

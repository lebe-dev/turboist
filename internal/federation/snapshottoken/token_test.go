package snapshottoken

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"
	"time"
)

func mustKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	return pub, priv
}

// TestMintVerify_RoundTrip asserts a freshly minted token verifies and returns
// the embedded project id while inside its 15-min window.
func TestMintVerify_RoundTrip(t *testing.T) {
	pub, priv := mustKey(t)
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	tok, err := Mint(priv, 42, now)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	pid, err := Verify(pub, tok, now.Add(5*time.Minute))
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if pid != 42 {
		t.Errorf("project id: got %d, want 42", pid)
	}
}

// TestVerify_Expired asserts a token past its TTL is rejected with ErrExpired
// (US-2.3 AC4 — expired snapshot token → 401).
func TestVerify_Expired(t *testing.T) {
	pub, priv := mustKey(t)
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	tok, err := Mint(priv, 7, now)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if _, err := Verify(pub, tok, now.Add(TTL+time.Second)); !errors.Is(err, ErrExpired) {
		t.Errorf("verify expired: got %v, want ErrExpired", err)
	}
}

// TestVerify_WrongKey asserts a token signed by one instance does not verify
// under a different instance's key.
func TestVerify_WrongKey(t *testing.T) {
	_, priv := mustKey(t)
	otherPub, _ := mustKey(t)
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	tok, err := Mint(priv, 1, now)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if _, err := Verify(otherPub, tok, now); !errors.Is(err, ErrInvalid) {
		t.Errorf("verify wrong key: got %v, want ErrInvalid", err)
	}
}

// TestVerify_Malformed asserts garbage and tampered tokens are rejected.
func TestVerify_Malformed(t *testing.T) {
	pub, priv := mustKey(t)
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	tok, _ := Mint(priv, 1, now)

	for _, bad := range []string{"", "no-dot", ".", "a.b", tok + "tamper"} {
		if _, err := Verify(pub, bad, now); !errors.Is(err, ErrInvalid) {
			t.Errorf("verify(%q): got %v, want ErrInvalid", bad, err)
		}
	}
}

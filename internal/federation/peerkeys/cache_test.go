package peerkeys

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync/atomic"
	"testing"
)

func newTestKeyB64(t *testing.T) string {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	return base64.StdEncoding.EncodeToString(pub)
}

func TestCache_FetchesOnceThenServesCached(t *testing.T) {
	keyB64 := newTestKeyB64(t)
	var calls int32
	fetcher := func(ctx context.Context, instanceURL string) (*Instance, error) {
		atomic.AddInt32(&calls, 1)
		return &Instance{InstanceURL: instanceURL, PublicKey: keyB64, DisplayName: "Alice"}, nil
	}
	c := NewCache(fetcher)
	ctx := context.Background()

	first, err := c.Resolve(ctx, "https://alice.example")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if first.DisplayName != "Alice" {
		t.Errorf("display name: got %q, want Alice", first.DisplayName)
	}
	// The decoded ed25519 key is cached and usable.
	if len(first.Key) != ed25519.PublicKeySize {
		t.Errorf("decoded key size: got %d, want %d", len(first.Key), ed25519.PublicKeySize)
	}

	// A second Resolve for the same instance must NOT re-fetch.
	if _, err := c.Resolve(ctx, "https://alice.example"); err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("fetcher calls: got %d, want 1 (must fetch once)", got)
	}
}

// TestCache_PinnedKeyMismatchDoesNotRefetch is the F5.6b R5 / US-6.4 AC1 guard:
// once a peer's key is PINNED (warmed from the durable directory, or Put after a
// handshake), a later inbound event signed with a DIFFERENT (rotated) key must NOT
// trigger a silent .well-known re-fetch. The signature middleware / per-event
// validator resolves the PINNED key, the rotated signature fails to verify against
// it → 401, and the event is rejected — auto-refetch would defeat key-change
// detection entirely. So Resolve always serves the pinned key from cache and the
// fetcher is never called for a pinned peer.
func TestCache_PinnedKeyMismatchDoesNotRefetch(t *testing.T) {
	pinnedKey := newTestKeyB64(t)
	fetcher := func(ctx context.Context, instanceURL string) (*Instance, error) {
		t.Fatalf("fetcher must NOT be called for a pinned peer (%s) — auto-refetch would defeat key-change detection (US-6.4 AC1)", instanceURL)
		return nil, nil
	}
	c := NewCache(fetcher)

	// Warm pins the peer's published key (the startup directory warm).
	if got := c.Warm([]Instance{{InstanceURL: "https://peer.example", PublicKey: pinnedKey, DisplayName: "Peer"}}); got != 1 {
		t.Fatalf("warm count: got %d, want 1", got)
	}

	ctx := context.Background()
	// Every Resolve serves the pinned key — never a fetch. A caller that then
	// verifies a rotated-key signature against this key gets a mismatch (the 401).
	for i := 0; i < 3; i++ {
		rk, err := c.Resolve(ctx, "https://peer.example")
		if err != nil {
			t.Fatalf("resolve pinned (iter %d): %v", i, err)
		}
		if rk.DisplayName != "Peer" {
			t.Errorf("resolved pinned display name: got %q, want Peer", rk.DisplayName)
		}
	}
}

// TestCache_PinnedMissDoesNotFetch asserts a peer that is KNOWN-pinned (registered
// via Pin) is never re-fetched on a cache miss even if its decoded entry is not
// present: a pinned peer's key can only change through the explicit trust-key path,
// never a silent fetch (F5.6b R5). An unknown (un-pinned) peer still fetches on
// miss so the cold join flow works.
func TestCache_PinnedMissDoesNotFetch(t *testing.T) {
	var calls int32
	unknownKey := newTestKeyB64(t)
	fetcher := func(ctx context.Context, instanceURL string) (*Instance, error) {
		atomic.AddInt32(&calls, 1)
		return &Instance{InstanceURL: instanceURL, PublicKey: unknownKey}, nil
	}
	c := NewCache(fetcher)
	ctx := context.Background()

	// Register a peer as pinned WITHOUT seeding a decoded entry (simulating a pin
	// the cache knows about but whose in-memory entry was not warmed).
	c.Pin("https://pinned.example")
	if _, err := c.Resolve(ctx, "https://pinned.example"); err == nil {
		t.Errorf("expected Resolve of a pinned-but-uncached peer to error rather than fetch")
	}
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Errorf("fetcher calls for pinned peer: got %d, want 0 (no auto-refetch, US-6.4 AC1)", got)
	}

	// An un-pinned, unknown peer still fetches on miss (cold join flow unaffected).
	if _, err := c.Resolve(ctx, "https://fresh.example"); err != nil {
		t.Fatalf("resolve unknown peer: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("fetcher calls for unknown peer: got %d, want 1", got)
	}
}

// TestCache_TrustReplacesPinnedKey asserts the explicit trust path (Trust) is the
// ONLY way a pinned key changes (F5.6b US-6.4 AC3): after Trust overwrites the
// pinned key, Resolve serves the NEW key (so a peer signing with the new key now
// verifies) — still without any fetch.
func TestCache_TrustReplacesPinnedKey(t *testing.T) {
	oldKey := newTestKeyB64(t)
	newKey := newTestKeyB64(t)
	fetcher := func(ctx context.Context, instanceURL string) (*Instance, error) {
		t.Fatalf("fetcher must not be called")
		return nil, nil
	}
	c := NewCache(fetcher)
	if err := c.Put("https://peer.example", oldKey, "Peer"); err != nil {
		t.Fatalf("put old: %v", err)
	}

	// Trust overwrites the pinned key (what the trust-key service calls after it
	// fetches the new .well-known key out-of-band).
	if err := c.Trust("https://peer.example", newKey, "Peer"); err != nil {
		t.Fatalf("trust new: %v", err)
	}
	rk, err := c.Resolve(context.Background(), "https://peer.example")
	if err != nil {
		t.Fatalf("resolve after trust: %v", err)
	}
	gotB64 := base64.StdEncoding.EncodeToString(rk.Key)
	if gotB64 != newKey {
		t.Errorf("resolved key after trust: got %q, want the new key %q", gotB64, newKey)
	}
}

func TestCache_FetchErrorNotCached(t *testing.T) {
	var calls int32
	keyB64 := newTestKeyB64(t)
	fetcher := func(ctx context.Context, instanceURL string) (*Instance, error) {
		if atomic.AddInt32(&calls, 1) == 1 {
			return nil, errors.New("network down")
		}
		return &Instance{InstanceURL: instanceURL, PublicKey: keyB64}, nil
	}
	c := NewCache(fetcher)
	ctx := context.Background()

	if _, err := c.Resolve(ctx, "https://bob.example"); err == nil {
		t.Fatal("expected first resolve to error")
	}
	// A failed fetch must not be cached — the retry succeeds.
	if _, err := c.Resolve(ctx, "https://bob.example"); err != nil {
		t.Fatalf("retry resolve: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("fetcher calls: got %d, want 2 (error must not cache)", got)
	}
}

func TestCache_PutSeedsWithoutFetch(t *testing.T) {
	keyB64 := newTestKeyB64(t)
	fetcher := func(ctx context.Context, instanceURL string) (*Instance, error) {
		t.Fatal("fetcher must not be called after Put")
		return nil, nil
	}
	c := NewCache(fetcher)
	ctx := context.Background()

	// Put warms the cache (e.g. from a handshake response) so the signature
	// middleware can verify a peer it just established trust with without a
	// round-trip (US-2.2 AC6).
	if err := c.Put("https://carol.example", keyB64, "Carol"); err != nil {
		t.Fatalf("put: %v", err)
	}
	got, err := c.Resolve(ctx, "https://carol.example")
	if err != nil {
		t.Fatalf("resolve after put: %v", err)
	}
	if got.DisplayName != "Carol" {
		t.Errorf("display name: got %q, want Carol", got.DisplayName)
	}
}

func TestCache_PutRejectsBadKey(t *testing.T) {
	c := NewCache(nil)
	if err := c.Put("https://x.example", "not-base64!!", "X"); err == nil {
		t.Fatal("expected Put to reject an invalid public key")
	}
}

// TestCache_WarmServesPinnedKeysWithoutFetch asserts Warm pre-loads pinned peer
// keys so a subsequent Resolve is served from cache WITHOUT a .well-known fetch
// (Federation v1 F4.3 review fix). This is the startup warm that makes a real
// signature mismatch a genuine key rotation rather than a cold-cache fetch error.
// A row whose stored key fails to decode is skipped; the count reflects only the
// keys actually warmed.
func TestCache_WarmServesPinnedKeysWithoutFetch(t *testing.T) {
	goodA := newTestKeyB64(t)
	goodB := newTestKeyB64(t)
	fetcher := func(ctx context.Context, instanceURL string) (*Instance, error) {
		t.Fatalf("fetcher must not be called for a warmed peer (%s)", instanceURL)
		return nil, nil
	}
	c := NewCache(fetcher)

	warmed := c.Warm([]Instance{
		{InstanceURL: "https://alice.example", PublicKey: goodA, DisplayName: "Alice"},
		{InstanceURL: "https://bob.example", PublicKey: goodB, DisplayName: "Bob"},
		{InstanceURL: "https://broken.example", PublicKey: "not-base64!!", DisplayName: "Broken"},
		{InstanceURL: "https://empty.example", PublicKey: "", DisplayName: "Empty"},
	})
	if warmed != 2 {
		t.Fatalf("warmed count: got %d, want 2 (broken + empty skipped)", warmed)
	}

	ctx := context.Background()
	rk, err := c.Resolve(ctx, "https://alice.example")
	if err != nil {
		t.Fatalf("resolve warmed alice: %v", err)
	}
	if rk.DisplayName != "Alice" {
		t.Errorf("display name: got %q, want Alice", rk.DisplayName)
	}
	if _, err := c.Resolve(ctx, "https://bob.example"); err != nil {
		t.Fatalf("resolve warmed bob: %v", err)
	}
}

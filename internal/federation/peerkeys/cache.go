// Package peerkeys resolves and caches a federation peer's published Ed25519
// public key and display name from its .well-known/instance document
// (Federation v1 F0.3). The signature middleware uses it to verify a peer's
// transport signature, and the join/handshake path uses Put to warm the cache
// with a key it just exchanged (US-2.2 AC6).
//
// R5 / US-6.4 AC1: F0.3 fetches a peer key on cache miss. Once a key is PINNED
// (a peer relationship exists with a stored key — every key added via Warm or Put
// is pinned), F5.6b DISABLES fetch-on-miss for that instance, so a signature
// signed with a rotated key fails to verify against the pinned key (a 401
// incident) rather than being silently accepted after an auto-refetch. The ONLY
// way a pinned key changes is the explicit operator-driven Trust path (the manual
// "Trust new key" action, US-6.4 AC3). An un-pinned, unknown instance still
// fetches on miss so the cold join/handshake flow works.
package peerkeys

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"sync"

	"github.com/lebe-dev/turboist/internal/crypto"
)

// Instance is a peer's published identity (the .well-known/instance payload
// subset the trust plane needs).
type Instance struct {
	InstanceURL string
	PublicKey   string // base64-std encoded Ed25519 public key
	DisplayName string
}

// ResolvedKey is a cached, decoded peer identity.
type ResolvedKey struct {
	InstanceURL string
	Key         ed25519.PublicKey
	DisplayName string
}

// Fetcher retrieves a peer's published Instance document over the network. It is
// injectable so tests do not perform real HTTP and so the caller controls the
// HTTP client (timeouts, proxy). It must NOT hold any DB connection (R1).
type Fetcher func(ctx context.Context, instanceURL string) (*Instance, error)

// ErrPinnedKeyUnavailable is returned by Resolve when an instance is known-pinned
// but its decoded key is not in the cache: a pinned peer must NEVER trigger an
// auto-refetch (US-6.4 AC1), so a pinned miss is a hard error rather than a fetch.
// In practice this is unreachable on a warmed cache (Pin is always called with a
// stored entry); it defends the invariant that a pinned key only changes via Trust.
var ErrPinnedKeyUnavailable = errors.New("peerkeys: pinned peer key unavailable (refetch disabled)")

// Cache is a concurrency-safe peer-key resolver with fetch-once semantics and a
// pinned-key set (F5.6b R5). A pinned instance never fetches on miss.
type Cache struct {
	mu      sync.RWMutex
	entries map[string]*ResolvedKey
	pinned  map[string]struct{}
	fetch   Fetcher
}

// NewCache returns a Cache backed by fetch (used on cache miss for un-pinned
// instances only).
func NewCache(fetch Fetcher) *Cache {
	return &Cache{
		entries: make(map[string]*ResolvedKey),
		pinned:  make(map[string]struct{}),
		fetch:   fetch,
	}
}

// Resolve returns the cached peer key for instanceURL. For a PINNED instance it
// NEVER fetches on miss (US-6.4 AC1): a cached pinned key is returned; a pinned
// instance with no cached key is ErrPinnedKeyUnavailable. For an un-pinned, unknown
// instance it fetches once via the configured Fetcher (the cold join/handshake
// flow); a failed fetch is NOT cached so a transient network error can be retried.
// A fetched key is pinned thereafter. Concurrent Resolves may each fetch on the
// very first miss; the last write wins and subsequent calls are served from cache.
func (c *Cache) Resolve(ctx context.Context, instanceURL string) (*ResolvedKey, error) {
	c.mu.RLock()
	rk, cached := c.entries[instanceURL]
	_, isPinned := c.pinned[instanceURL]
	c.mu.RUnlock()
	if cached {
		return rk, nil
	}
	// A pinned instance must never auto-refetch — that would silently accept a
	// rotated key and defeat key-change detection (US-6.4 AC1, R5).
	if isPinned {
		return nil, ErrPinnedKeyUnavailable
	}

	if c.fetch == nil {
		return nil, fmt.Errorf("peerkeys: no fetcher configured for %q", instanceURL)
	}
	inst, err := c.fetch(ctx, instanceURL)
	if err != nil {
		return nil, fmt.Errorf("fetch peer key %q: %w", instanceURL, err)
	}
	// A key resolved over .well-known for a peer we exchange events with is pinned
	// thereafter: the next mismatch is a rotation incident, not a silent refetch.
	return c.storePinned(instanceURL, inst.PublicKey, inst.DisplayName)
}

// Put warms the cache with a key obtained out-of-band (e.g. from a handshake
// response, US-2.2 AC6). It validates and decodes the key before storing and PINS
// the instance, so a later signature mismatch never triggers an auto-refetch.
func (c *Cache) Put(instanceURL, publicKeyB64, displayName string) error {
	_, err := c.storePinned(instanceURL, publicKeyB64, displayName)
	return err
}

// Pin marks an instance as pinned WITHOUT seeding a key, so a subsequent Resolve
// miss errors rather than fetching (US-6.4 AC1). It is a defensive marker for a
// known peer whose decoded entry is not (yet) cached; the normal warm/Put/Resolve
// paths pin implicitly.
func (c *Cache) Pin(instanceURL string) {
	c.mu.Lock()
	c.pinned[instanceURL] = struct{}{}
	c.mu.Unlock()
}

// Trust overwrites the pinned key for an instance with an operator-trusted new key
// (Federation v1 F5.6b, US-6.4 AC3 — the manual "Trust new key" action). It is the
// ONLY way a pinned key changes: the trust-key service fetches the new .well-known
// key out-of-band, persists it, then calls Trust so the in-memory cache the
// signature middleware / validator read agrees with the durable directory. It
// validates and decodes the key before replacing it (and (re)pins the instance).
func (c *Cache) Trust(instanceURL, publicKeyB64, displayName string) error {
	_, err := c.storePinned(instanceURL, publicKeyB64, displayName)
	return err
}

// Warm pre-loads the cache with already-pinned peer keys read from the durable
// federated_instances directory (Federation v1 F4.3 review fix). The cache is
// in-memory only and cold after every restart; warming it at startup means the
// first inbound event from a joined peer verifies against the pinned key instead
// of triggering a cold-cache .well-known fetch-on-miss — so a genuine signature
// mismatch is a real key rotation, not a transient cold-start fetch error that
// would otherwise stamp the sticky, irreversible key_mismatch marker. A peer row
// whose stored key fails to decode is skipped (best-effort warm); the count of
// successfully warmed entries is returned so the caller can log it.
func (c *Cache) Warm(instances []Instance) int {
	warmed := 0
	for _, inst := range instances {
		if inst.InstanceURL == "" || inst.PublicKey == "" {
			continue
		}
		if _, err := c.storePinned(inst.InstanceURL, inst.PublicKey, inst.DisplayName); err == nil {
			warmed++
		}
	}
	return warmed
}

// storePinned decodes + stores the key and marks the instance pinned, so a later
// signature mismatch is a rotation incident rather than a silent auto-refetch
// (US-6.4 AC1, R5). All write paths (Warm/Put/Trust/Resolve-fetch) go through it.
func (c *Cache) storePinned(instanceURL, publicKeyB64, displayName string) (*ResolvedKey, error) {
	key, err := crypto.DecodePublicKey(publicKeyB64)
	if err != nil {
		return nil, fmt.Errorf("decode peer key %q: %w", instanceURL, err)
	}
	rk := &ResolvedKey{InstanceURL: instanceURL, Key: key, DisplayName: displayName}
	c.mu.Lock()
	c.entries[instanceURL] = rk
	c.pinned[instanceURL] = struct{}{}
	c.mu.Unlock()
	return rk, nil
}

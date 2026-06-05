// Package nonce provides an in-memory, TTL-bounded anti-replay cache for
// federation HTTP-signature requests (Federation v1 F0.3, US-7.3 AC1). It is a
// close clone of events.TicketStore's sweep-on-access design: a small map under
// a mutex, with expired entries swept lazily on each access.
//
// The cache only needs to remember a nonce for as long as the request
// timestamp window stays valid (±5min, enforced separately and BEFORE the nonce
// check). In-memory state resets on restart — a documented v1 gap (R18): a
// single replay is possible within the window immediately after a restart.
package nonce

import (
	"sync"
	"time"
)

// DefaultTTL is how long a seen nonce is remembered. It is set comfortably
// above the ±5min request timestamp window so a nonce cannot be replayed while
// its timestamp still validates.
const DefaultTTL = 10 * time.Minute

// Cache is a concurrency-safe anti-replay nonce store.
type Cache struct {
	mu   sync.Mutex
	seen map[string]time.Time
	now  func() time.Time
	ttl  time.Duration
	// disabled turns Check into a no-op that accepts every non-empty nonce. It is
	// NEVER set in production wiring (cmd/turboist/main.go uses NewCache); it exists
	// only for the in-process integration harness, whose app.Test() transport can
	// re-serve the SAME signed request and trip a spurious replay. See
	// NewDisabledCache.
	disabled bool
}

// NewCache returns a Cache using the default TTL and the system clock.
func NewCache() *Cache {
	return &Cache{
		seen: make(map[string]time.Time),
		now:  time.Now,
		ttl:  DefaultTTL,
	}
}

// NewDisabledCache returns a Cache whose Check accepts every non-empty nonce (no
// anti-replay). It is for the in-process integration harness ONLY: Fiber's
// app.Test() can re-serve the identical signed request, which the real cache would
// reject as a replay even though no genuine replay occurred. Production code uses
// NewCache; the transport anti-replay itself is covered by the dedicated
// HTTPSignatureMiddleware tests (single-request, deterministic). An empty nonce is
// still rejected so the "missing nonce" contract holds.
func NewDisabledCache() *Cache {
	return &Cache{seen: make(map[string]time.Time), now: time.Now, ttl: DefaultTTL, disabled: true}
}

// Check records nonce as seen and reports whether it is fresh (i.e. the request
// is NOT a replay). It returns true the first time a nonce is presented and
// false on any subsequent presentation within the TTL. An empty nonce is always
// rejected (returns false). Expired entries are swept on each call.
func (c *Cache) Check(nonce string) bool {
	if nonce == "" {
		return false
	}
	if c.disabled {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	c.sweepLocked(now)
	if _, ok := c.seen[nonce]; ok {
		return false
	}
	c.seen[nonce] = now.Add(c.ttl)
	return true
}

func (c *Cache) sweepLocked(now time.Time) {
	for k, expiresAt := range c.seen {
		if now.After(expiresAt) {
			delete(c.seen, k)
		}
	}
}

// Package ratelimit implements the inbound per-peer token-bucket limiter for the
// federation event endpoint (Federation v1 F4.4, US-8.3). A peer sending more
// than the configured rate (default 600 events/min) is rejected with 429 + a
// Retry-After hint so it backs off (the symmetric inbound half of the outbox
// worker's outbound 429 honoring).
//
// It is a clone of internal/auth.IPLimiter keyed by peer instance_url instead of
// IP, with a GC sweep that evicts idle peers so the map cannot grow unbounded.
// The limiter is in-memory and resets on restart — an accepted v1 gap (R18): a
// burst is briefly possible in the window after a restart, recorded in the threat
// model. It holds NO DB connection (R1).
package ratelimit

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// PeerLimiter is a per-peer token-bucket rate limiter. Each peer gets its own
// bucket so one noisy peer's 429s never throttle a healthy peer.
type PeerLimiter struct {
	mu    sync.Mutex
	peers map[string]*peerBucket

	rps   rate.Limit
	burst int
	ttl   time.Duration
	now   func() time.Time

	stopCh   chan struct{}
	stopOnce sync.Once
}

type peerBucket struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewPeerLimiter constructs a per-peer limiter and starts its idle-eviction GC.
// eventsPerMinute is the steady inbound rate (US-8.3 default 600); burst is the
// bucket size (a short spike a peer may send before throttling). A non-positive
// eventsPerMinute disables limiting (Allow always permits) so a misconfigured
// value never locks out all peers.
func NewPeerLimiter(eventsPerMinute, burst int, ttl time.Duration) *PeerLimiter {
	rps := rate.Limit(0)
	if eventsPerMinute > 0 {
		rps = rate.Limit(float64(eventsPerMinute) / 60.0)
	}
	if burst <= 0 {
		burst = eventsPerMinute
	}
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	l := &PeerLimiter{
		peers:  make(map[string]*peerBucket),
		rps:    rps,
		burst:  burst,
		ttl:    ttl,
		now:    time.Now,
		stopCh: make(chan struct{}),
	}
	go l.gc()
	return l
}

// disabled reports whether limiting is off (a non-positive configured rate).
func (l *PeerLimiter) disabled() bool { return l.rps == 0 }

// AllowN reports whether a peer may send n more events now. When throttled it
// returns ok=false plus the Retry-After window the peer should wait before its
// next attempt (US-8.3 AC1 — 429 + Retry-After). A disabled limiter always
// permits. n<=0 is treated as 1 (a body always carries at least one event).
func (l *PeerLimiter) AllowN(peerURL string, n int) (ok bool, retryAfter time.Duration) {
	if l.disabled() {
		return true, 0
	}
	if n <= 0 {
		n = 1
	}

	l.mu.Lock()
	b, found := l.peers[peerURL]
	if !found {
		b = &peerBucket{limiter: rate.NewLimiter(l.rps, l.burst)}
		l.peers[peerURL] = b
	}
	now := l.now()
	b.lastSeen = now
	res := b.limiter.ReserveN(now, n)
	l.mu.Unlock()

	if !res.OK() {
		// The request can never be satisfied within the bucket (n > burst). Reject
		// without consuming tokens and signal a conservative 1s wait.
		return false, time.Second
	}
	delay := res.DelayFrom(now)
	if delay <= 0 {
		return true, 0
	}
	// Over the steady rate: cancel the reservation (do not consume future capacity
	// for a rejected request) and tell the peer how long to wait.
	res.CancelAt(now)
	return false, roundUpSeconds(delay)
}

// Stop halts the GC goroutine (teardown).
func (l *PeerLimiter) Stop() {
	l.stopOnce.Do(func() { close(l.stopCh) })
}

func (l *PeerLimiter) gc() {
	t := time.NewTicker(l.ttl)
	defer t.Stop()
	for {
		select {
		case <-l.stopCh:
			return
		case <-t.C:
			l.sweep()
		}
	}
}

func (l *PeerLimiter) sweep() {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	for url, b := range l.peers {
		if now.Sub(b.lastSeen) > l.ttl {
			delete(l.peers, url)
		}
	}
}

// roundUpSeconds rounds a sub-second delay up to a whole second so the
// Retry-After header (which carries integer seconds) never undershoots the real
// wait — a peer told "0" would retry immediately and be rejected again.
func roundUpSeconds(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	secs := d / time.Second
	if d%time.Second != 0 {
		secs++
	}
	if secs < 1 {
		secs = 1
	}
	return secs * time.Second
}

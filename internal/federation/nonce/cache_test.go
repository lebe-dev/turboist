package nonce

import (
	"testing"
	"time"
)

func TestCache_FirstSeenAccepts(t *testing.T) {
	c := NewCache()
	if !c.Check("abc") {
		t.Fatal("first sighting of a nonce must be accepted")
	}
}

func TestCache_ReplayRejected(t *testing.T) {
	c := NewCache()
	if !c.Check("nonce-1") {
		t.Fatal("first Check should accept")
	}
	if c.Check("nonce-1") {
		t.Fatal("second Check of the same nonce must reject (replay)")
	}
}

func TestCache_ExpiredNonceForgotten(t *testing.T) {
	c := NewCache()
	now := time.Now()
	c.now = func() time.Time { return now }
	c.ttl = time.Minute

	if !c.Check("n") {
		t.Fatal("first Check should accept")
	}
	// Within TTL: still remembered (replay rejected).
	now = now.Add(30 * time.Second)
	if c.Check("n") {
		t.Fatal("replay within TTL must reject")
	}
	// After TTL: the entry is swept and the same nonce is accepted again. (In
	// practice the timestamp window prevents this from being a real replay
	// vector; the cache only needs to hold nonces for the window duration.)
	now = now.Add(2 * time.Minute)
	if !c.Check("n") {
		t.Fatal("nonce older than TTL should be forgotten and re-accepted")
	}
}

func TestCache_EmptyNonceRejected(t *testing.T) {
	c := NewCache()
	if c.Check("") {
		t.Fatal("empty nonce must be rejected")
	}
}

func TestCache_DistinctNoncesIndependent(t *testing.T) {
	c := NewCache()
	if !c.Check("a") || !c.Check("b") {
		t.Fatal("distinct nonces must both be accepted")
	}
	if c.Check("a") || c.Check("b") {
		t.Fatal("each nonce must reject its own replay independently")
	}
}

// TestDisabledCache_AcceptsRepeats covers the harness-only NewDisabledCache: it
// accepts EVERY non-empty nonce (no anti-replay), so a repeated nonce is not
// rejected — neutralising the in-process app.Test() re-serve flake the F7.3
// owner-hub convergence tests hit. The empty-nonce contract still holds.
func TestDisabledCache_AcceptsRepeats(t *testing.T) {
	c := NewDisabledCache()
	if !c.Check("dup") {
		t.Fatal("disabled cache must accept a first nonce")
	}
	if !c.Check("dup") {
		t.Fatal("disabled cache must accept a REPEATED nonce (anti-replay off)")
	}
	if c.Check("") {
		t.Fatal("disabled cache must still reject an empty nonce")
	}
}

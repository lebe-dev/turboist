package ratelimit_test

import (
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/federation/ratelimit"
)

// TestPeerLimiter_ThrottlesOverRate asserts a peer exceeding its burst is
// throttled with a positive Retry-After (Federation v1 F4.4, US-8.3 AC1).
func TestPeerLimiter_ThrottlesOverRate(t *testing.T) {
	// 60 events/min = 1/s, burst 2: the third event in a tight window is rejected.
	l := ratelimit.NewPeerLimiter(60, 2, time.Minute)
	t.Cleanup(l.Stop)

	if ok, _ := l.AllowN("https://peer.example", 1); !ok {
		t.Fatalf("first event should be allowed")
	}
	if ok, _ := l.AllowN("https://peer.example", 1); !ok {
		t.Fatalf("second event (within burst) should be allowed")
	}
	ok, retryAfter := l.AllowN("https://peer.example", 1)
	if ok {
		t.Errorf("third event over burst should be throttled")
	}
	if retryAfter <= 0 {
		t.Errorf("throttled request must carry a positive Retry-After, got %v", retryAfter)
	}
}

// TestPeerLimiter_IsolatedPerPeer asserts one throttled peer does not throttle a
// different peer (per-peer buckets — US-8.3 isolation).
func TestPeerLimiter_IsolatedPerPeer(t *testing.T) {
	l := ratelimit.NewPeerLimiter(60, 1, time.Minute)
	t.Cleanup(l.Stop)

	if ok, _ := l.AllowN("https://a.example", 1); !ok {
		t.Fatalf("peer A first event should be allowed")
	}
	if ok, _ := l.AllowN("https://a.example", 1); ok {
		t.Errorf("peer A second event over burst should be throttled")
	}
	// Peer B has its own untouched bucket.
	if ok, _ := l.AllowN("https://b.example", 1); !ok {
		t.Errorf("peer B should be allowed despite peer A being throttled")
	}
}

// TestPeerLimiter_DisabledAlwaysAllows asserts a non-positive configured rate
// disables limiting so a misconfiguration never locks out every peer.
func TestPeerLimiter_DisabledAlwaysAllows(t *testing.T) {
	l := ratelimit.NewPeerLimiter(0, 0, time.Minute)
	t.Cleanup(l.Stop)
	for i := 0; i < 1000; i++ {
		if ok, _ := l.AllowN("https://peer.example", 1); !ok {
			t.Fatalf("disabled limiter should always allow, rejected at i=%d", i)
		}
	}
}

// TestPeerLimiter_BatchExceedingBurstRejected asserts a batch larger than the
// burst is rejected outright rather than partially consuming the bucket.
func TestPeerLimiter_BatchExceedingBurstRejected(t *testing.T) {
	l := ratelimit.NewPeerLimiter(60, 5, time.Minute)
	t.Cleanup(l.Stop)
	ok, retryAfter := l.AllowN("https://peer.example", 10)
	if ok {
		t.Errorf("a batch of 10 over a burst of 5 should be rejected")
	}
	if retryAfter <= 0 {
		t.Errorf("rejected over-burst batch must carry a Retry-After, got %v", retryAfter)
	}
}

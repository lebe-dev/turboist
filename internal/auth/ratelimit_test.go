package auth

import (
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestIPLimiter_AllowsBurstThenBlocks(t *testing.T) {
	l := NewIPLimiter(rate.Every(6*time.Second), 10, 10*time.Minute)
	defer l.Stop()
	allowed := 0
	for range 15 {
		if l.Allow("1.2.3.4") {
			allowed++
		}
	}
	if allowed != 10 {
		t.Errorf("allowed: got %d, want 10 (burst)", allowed)
	}
}

func TestIPLimiter_PerIPIsolation(t *testing.T) {
	l := NewIPLimiter(rate.Every(6*time.Second), 10, 10*time.Minute)
	defer l.Stop()
	for range 10 {
		if !l.Allow("a") {
			t.Fatalf("a: drained too early")
		}
	}
	if !l.Allow("b") {
		t.Errorf("b should still have full burst")
	}
}

func TestIPLimiter_GCSweep(t *testing.T) {
	l := NewIPLimiter(rate.Every(time.Second), 1, 10*time.Minute)
	defer l.Stop()
	now := time.Now()
	l.now = func() time.Time { return now }

	if !l.Allow("ip") {
		t.Fatalf("first allow")
	}
	if _, ok := l.visitors["ip"]; !ok {
		t.Errorf("visitor not registered")
	}

	// Advance the clock past TTL and run a sweep manually.
	l.now = func() time.Time { return now.Add(2 * 10 * time.Minute) }
	l.sweep()

	if _, ok := l.visitors["ip"]; ok {
		t.Errorf("idle visitor should be evicted")
	}
}

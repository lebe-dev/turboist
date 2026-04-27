package auth

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type IPLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	rps      rate.Limit
	burst    int
	ttl      time.Duration
	now      func() time.Time
	stopCh   chan struct{}
	stopOnce sync.Once
}

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewIPLimiter creates a per-IP token-bucket limiter and starts a GC goroutine.
// rps is the steady rate; burst is the bucket size; ttl is how long an idle visitor is kept.
func NewIPLimiter(rps rate.Limit, burst int, ttl time.Duration) *IPLimiter {
	l := &IPLimiter{
		visitors: make(map[string]*visitor),
		rps:      rps,
		burst:    burst,
		ttl:      ttl,
		now:      time.Now,
		stopCh:   make(chan struct{}),
	}
	go l.gc()
	return l
}

func (l *IPLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	v, ok := l.visitors[ip]
	if !ok {
		v = &visitor{limiter: rate.NewLimiter(l.rps, l.burst)}
		l.visitors[ip] = v
	}
	v.lastSeen = l.now()
	return v.limiter.Allow()
}

func (l *IPLimiter) Stop() {
	l.stopOnce.Do(func() { close(l.stopCh) })
}

func (l *IPLimiter) gc() {
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

func (l *IPLimiter) sweep() {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	for ip, v := range l.visitors {
		if now.Sub(v.lastSeen) > l.ttl {
			delete(l.visitors, ip)
		}
	}
}

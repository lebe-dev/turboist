package auth

import (
	"context"
	"log/slog"
	"sync"
	"testing"
)

// captureHandler is a minimal slog.Handler used in auth tests to inspect
// records emitted via slog.Default() (used by jwt.go, ratelimit.go, cleanup.go).
type captureHandler struct {
	mu      *sync.Mutex
	records *[]slog.Record
	attrs   []slog.Attr
}

func newCaptureHandler() *captureHandler {
	return &captureHandler{mu: &sync.Mutex{}, records: &[]slog.Record{}}
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	rec := r.Clone()
	if len(h.attrs) > 0 {
		rec.AddAttrs(h.attrs...)
	}
	*h.records = append(*h.records, rec)
	return nil
}

func (h *captureHandler) WithAttrs(a []slog.Attr) slog.Handler {
	merged := make([]slog.Attr, 0, len(h.attrs)+len(a))
	merged = append(merged, h.attrs...)
	merged = append(merged, a...)
	return &captureHandler{mu: h.mu, records: h.records, attrs: merged}
}

func (h *captureHandler) WithGroup(_ string) slog.Handler { return h }

// snapshot returns a copy of recorded records, safe for read after test end.
func (h *captureHandler) snapshot() []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]slog.Record, len(*h.records))
	copy(out, *h.records)
	return out
}

// swapDefault replaces slog.Default and restores it on test cleanup.
// Tests using it should not run in parallel since slog.Default is process-global.
func swapDefault(t *testing.T, h slog.Handler) {
	t.Helper()
	prev := slog.Default()
	slog.SetDefault(slog.New(h))
	t.Cleanup(func() { slog.SetDefault(prev) })
}

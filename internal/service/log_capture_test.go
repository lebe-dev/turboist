package service_test

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"github.com/lebe-dev/turboist/internal/logging"
)

// captureHandler is a minimal slog.Handler used in service tests to assert
// that DEBUG/INFO/WARN/ERROR records are emitted by service operations.
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

func (h *captureHandler) snapshot() []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]slog.Record, len(*h.records))
	copy(out, *h.records)
	return out
}

// ctxWithCapture returns a context carrying a capture logger so that service
// methods reading the logger from context emit records into the returned
// handler.
func ctxWithCapture(t *testing.T) (context.Context, *captureHandler) {
	t.Helper()
	h := newCaptureHandler()
	log := slog.New(h)
	ctx := logging.WithLogger(context.Background(), log)
	return ctx, h
}

// hasMessageAtLevel reports whether any captured record has the given message
// at the given level.
func hasMessageAtLevel(records []slog.Record, msg string, lvl slog.Level) bool {
	for _, r := range records {
		if r.Level == lvl && r.Message == msg {
			return true
		}
	}
	return false
}

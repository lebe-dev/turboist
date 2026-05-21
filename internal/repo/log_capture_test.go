package repo

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"github.com/lebe-dev/turboist/internal/logging"
)

// captureHandler is a minimal slog.Handler used in repo tests to assert
// that DEBUG/ERROR records are emitted by logQuery / logErr.
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

// ctxWithCapture returns a context with the capture logger attached, so any
// repo method reading the logger from context will emit records into the
// returned handler.
func ctxWithCapture(t *testing.T) (context.Context, *captureHandler) {
	t.Helper()
	h := newCaptureHandler()
	log := slog.New(h)
	ctx := logging.WithLogger(context.Background(), log)
	return ctx, h
}

// findOp returns the first record where attr "op" equals the given value.
func findOp(records []slog.Record, op string) (slog.Record, bool) {
	for _, r := range records {
		var match bool
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == "op" && a.Value.String() == op {
				match = true
				return false
			}
			return true
		})
		if match {
			return r, true
		}
	}
	return slog.Record{}, false
}

// countLevel returns the number of records emitted at the given level.
func countLevel(records []slog.Record, lvl slog.Level) int {
	n := 0
	for _, r := range records {
		if r.Level == lvl {
			n++
		}
	}
	return n
}

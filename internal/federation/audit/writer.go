// Package audit owns the federation audit-log async writer (Federation v1 F6.3,
// US-7.4). Every security-relevant federation operation — the transport and
// per-event rejections produced by the two signature planes, plus the owner
// control-plane trust actions (handshake accepted, peer revoked, key trusted) —
// is Recorded here. Recording is NON-BLOCKING: Record hands the entry to a
// buffered channel and returns immediately, and a single background goroutine
// drains the channel into the audit repo. This keeps logging off the request
// path so an attacker flooding rejections can never stall the rejection itself
// (§7 F6.3 "async writer, failure-spam is worst-case write load"). When the
// buffer saturates, Record DROPS the entry (and counts the drop) rather than
// blocking — the rejection still happens; only the audit trail of the flood is
// lossy, which is acceptable for a Could-grade investigation aid.
//
// The writer NEVER persists secrets, raw signatures, private seeds, or invite
// tokens: callers pass only a short coded reason as the entry Detail, and the
// repo stores it verbatim (the redaction discipline lives at the call sites).
package audit

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lebe-dev/turboist/internal/repo"
)

// defaultBuffer is the channel depth. It is generous enough to absorb a brief
// rejection burst without dropping, but bounded so a sustained flood degrades by
// dropping audit rows (the rejections themselves are unaffected).
const defaultBuffer = 1024

// Recorder is the non-blocking audit sink the federation trust planes depend on
// (the signature middleware, the events handler, and the service trust actions).
// It is satisfied by *Writer. A nil Recorder means audit logging is off (a
// federation-disabled / partially-wired build), so callers guard nil before
// Record.
type Recorder interface {
	Record(e repo.AuditEntry)
}

// Sink is the durable persistence the writer drains into — satisfied by
// *repo.FederationAuditLogRepo. Kept as an interface so the writer is unit-testable
// without a DB.
type Sink interface {
	Insert(ctx context.Context, e repo.AuditEntry) error
}

// Writer is the buffered, single-goroutine audit writer. Construct with NewWriter,
// then Start it on the cleanup context and Stop it on shutdown.
type Writer struct {
	sink    Sink
	log     *slog.Logger
	entries chan repo.AuditEntry
	doneCh  chan struct{}
	stopCh  chan struct{}
	stopper sync.Once
	now     func() time.Time
	buffer  int
	dropped atomicCounter
}

// NewWriter constructs the audit writer over a sink. A nil log uses slog.Default.
func NewWriter(sink Sink, log *slog.Logger) *Writer {
	if log == nil {
		log = slog.Default()
	}
	return &Writer{
		sink:   sink,
		log:    log,
		doneCh: make(chan struct{}),
		stopCh: make(chan struct{}),
		now:    time.Now,
		buffer: defaultBuffer,
	}
}

// WithBuffer overrides the channel depth (mainly for tests that want a tiny buffer
// to exercise the drop path). It must be called before Start.
func (w *Writer) WithBuffer(n int) *Writer {
	if n > 0 {
		w.buffer = n
	}
	return w
}

// WithClock overrides the wall clock used to stamp an entry that carries no
// CreatedAt (deterministic tests). It must be called before Start.
func (w *Writer) WithClock(now func() time.Time) *Writer {
	if now != nil {
		w.now = now
	}
	return w
}

// Start launches the single drain goroutine; it runs until ctx is cancelled. The
// channel is created here so a WithBuffer set before Start takes effect.
func (w *Writer) Start(ctx context.Context) {
	w.entries = make(chan repo.AuditEntry, w.buffer)
	go w.run(ctx)
}

// Record hands an entry to the drain goroutine WITHOUT blocking. If the entry
// carries no CreatedAt the writer stamps the current time so callers need not.
// When the buffer is full the entry is dropped (and counted) so a rejection is
// never stalled by audit logging. A Writer not yet Started silently no-ops (the
// channel is nil) so wiring order can never panic a rejection path.
func (w *Writer) Record(e repo.AuditEntry) {
	if w.entries == nil {
		return
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = w.now()
	}
	select {
	case w.entries <- e:
	default:
		// Buffer saturated — drop rather than block the caller (the rejection still
		// happens; only the audit trail of the flood is lossy).
		w.dropped.inc()
	}
}

// Dropped reports how many entries were dropped because the buffer was full —
// surfaced for the saturation test and operational visibility.
func (w *Writer) Dropped() int64 {
	return w.dropped.load()
}

// Stop signals the drain goroutine to flush its buffer and return, then blocks
// until it has. It is idempotent (safe to call more than once) and does NOT depend
// on the producing ctx being cancelled first — closing stopCh wakes the run loop
// directly. A Writer that was never Started returns immediately.
func (w *Writer) Stop() {
	if w.entries == nil {
		return
	}
	w.stopper.Do(func() { close(w.stopCh) })
	<-w.doneCh
}

func (w *Writer) run(ctx context.Context) {
	defer close(w.doneCh)
	for {
		select {
		case <-ctx.Done():
			// Best-effort final drain so entries recorded just before shutdown are
			// persisted (the audit trail of a teardown-time rejection is not lost).
			w.drainBuffered(context.WithoutCancel(ctx))
			return
		case <-w.stopCh:
			// Explicit Stop (idempotent): drain and exit regardless of the ctx state.
			w.drainBuffered(context.WithoutCancel(ctx))
			return
		case e := <-w.entries:
			w.write(ctx, e)
		}
	}
}

// drainBuffered persists every entry already buffered, then returns. It never
// waits for new entries.
func (w *Writer) drainBuffered(ctx context.Context) {
	for {
		select {
		case e := <-w.entries:
			w.write(ctx, e)
		default:
			return
		}
	}
}

// write persists one entry, swallowing (logging) a sink error so a transient DB
// failure never kills the goroutine — the next entry is still drained.
func (w *Writer) write(ctx context.Context, e repo.AuditEntry) {
	if err := w.sink.Insert(ctx, e); err != nil {
		w.log.ErrorContext(ctx, "federation: audit write failed",
			slog.String("op", "federation.audit.Write"),
			slog.String("kind", string(e.Kind)),
			slog.String("peer", e.PeerInstanceURL),
			slog.String("err", err.Error()),
		)
	}
}

// atomicCounter is a tiny lock-free counter for the drop tally.
type atomicCounter struct{ n atomic.Int64 }

func (c *atomicCounter) inc()        { c.n.Add(1) }
func (c *atomicCounter) load() int64 { return c.n.Load() }

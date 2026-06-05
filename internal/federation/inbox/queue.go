package inbox

import (
	"context"
	"log/slog"
	"time"

	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/federation/store"
	"github.com/lebe-dev/turboist/internal/model"
)

// The single inbox-apply goroutine (Federation v1 F3.2). POST /federation/events
// records the event to federation_inbox (dedup) and ENQUEUES it here, then
// returns fast — apply runs OFF the HTTP path on one goroutine so a slow merge
// never blocks the signed endpoint, and the lone DB connection is serialised by
// a single applier rather than contended by concurrent request handlers.
//
// After a successful apply that actually CHANGED something, the queue fires a
// federation-origin notification (Notifier) so the SSE hub publishes the change
// to the local UI WITHOUT echo-suppression (US-3.1 AC2): a remote edit is not the
// user's own mutation, so it must reach every open tab. A no-op merge (every
// field stale) fires nothing — a redundant refresh is exactly the flicker the
// self-refresh work removed.
//
// At-least-once recovery (NFR-2). The POST handler durably records the event
// (federation_inbox, ON CONFLICT DO NOTHING) BEFORE enqueue, so a redelivery of
// the same event_id is deduped and NOT re-enqueued. A successful apply stamps
// applied_at in the same tx as the merge (terminal); a poison (permanent) reject
// stamps applied_at too so it is never retried. A TRANSIENT apply failure (DB
// busy, lock contention) leaves applied_at NULL — and because nothing in-process
// re-drives the dropped in-memory job, the queue itself periodically re-scans
// rows whose applied_at is still NULL (Recoverer) and re-enqueues them, on
// startup and on a recovery tick. On shutdown the buffered jobs channel is
// best-effort drained (mirroring the outbox worker) so a queued-but-unapplied
// event is given a final apply attempt; anything still NULL after teardown is
// recovered on the next startup re-scan. Without this, a single SQLITE_BUSY blip
// or a restart would acknowledge (202) and durably record an event whose merge
// is then silently skipped until the F4.1 pull loop — which does not exist yet.

// EntityApplier is the apply surface the queue drives (satisfied by *Applier).
// Kept as an interface so the queue goroutine logic is unit-testable without a
// DB.
type EntityApplier interface {
	Apply(ctx context.Context, e events.Event, peerURL string) (*ApplyResult, error)
}

// Job is one unit of work for the apply goroutine: the decoded event plus the
// transport-verified peer it arrived from.
type Job struct {
	Event   events.Event
	PeerURL string
}

// Applied describes a successfully-applied event whose merge changed local state,
// handed to the Notifier so it can publish a federation-origin SSE refresh.
type Applied struct {
	Event   events.Event
	Result  *ApplyResult
	PeerURL string
}

// Notifier is called after an apply that changed something so the SSE hub can
// publish the change to local subscribers (federation-origin, not echo-
// suppressed). A nil notifier disables notifications (tests / federation-off).
type Notifier interface {
	Notify(ctx context.Context, ev Applied)
}

// Recoverer re-drives events whose apply was lost to a transient failure or a
// crash between the durable inbox INSERT and a successful merge (satisfied by
// *store.Store). ListUnapplied returns inbox rows whose applied_at is still NULL;
// MarkApplied stamps a permanently-rejected (poison) event terminal so it is not
// re-driven forever. A nil recoverer disables recovery (tests / federation-off).
type Recoverer interface {
	ListUnapplied(ctx context.Context, limit int) ([]store.PendingInboxEvent, error)
	MarkApplied(ctx context.Context, eventID, appliedAt string) error
}

// recoverInterval is the inbox recovery re-scan cadence: how often the queue
// re-scans federation_inbox for rows whose applied_at is still NULL (a transient
// apply failure left the durable row pending) and re-enqueues them. A startup
// re-scan covers events stranded by a restart; the tick covers a transient blip.
const recoverInterval = time.Minute

// recoverLimit caps how many pending rows one re-scan pass re-enqueues so a
// large backlog never floods the bounded jobs buffer in a single pass.
const recoverLimit = 256

// Queue is the single-writer inbox-apply pump.
type Queue struct {
	applier EntityApplier
	notify  Notifier
	recover Recoverer
	log     *slog.Logger

	recoverEvery time.Duration

	jobs   chan Job
	doneCh chan struct{}
}

// queueBuffer bounds the in-flight enqueue backlog. POST /federation/events
// blocks (briefly) if the buffer is full, which is the desired backpressure: the
// inbound rate-limit / 413 cap lands in F4.4, but a bounded channel already keeps
// memory from growing unboundedly under a flood.
const queueBuffer = 1024

// NewQueue constructs the apply queue. notify, recover, and log may be nil; a nil
// recover disables the transient-failure / crash recovery re-scan (the queue then
// only processes live enqueues).
func NewQueue(applier EntityApplier, notify Notifier, recover Recoverer, log *slog.Logger) *Queue {
	if log == nil {
		log = slog.Default()
	}
	return &Queue{
		applier:      applier,
		notify:       notify,
		recover:      recover,
		log:          log,
		recoverEvery: recoverInterval,
		jobs:         make(chan Job, queueBuffer),
		doneCh:       make(chan struct{}),
	}
}

// WithRecoverInterval overrides the recovery re-scan cadence (default
// recoverInterval). It must be set before Start; a non-positive value is ignored.
// The deterministic-test path uses a short interval so a transient-failure
// re-drive is observable without waiting a minute.
func (q *Queue) WithRecoverInterval(d time.Duration) *Queue {
	if d > 0 {
		q.recoverEvery = d
	}
	return q
}

// StoreRecoverer adapts *store.Store to the Recoverer interface so the queue can
// re-drive un-applied inbox rows without importing the store's method names into
// its own contract.
type StoreRecoverer struct {
	store *store.Store
}

// NewStoreRecoverer wraps st as a Recoverer for the inbox queue.
func NewStoreRecoverer(st *store.Store) *StoreRecoverer {
	return &StoreRecoverer{store: st}
}

// ListUnapplied returns up to limit inbox rows whose applied_at is still NULL.
func (r *StoreRecoverer) ListUnapplied(ctx context.Context, limit int) ([]store.PendingInboxEvent, error) {
	return r.store.ListUnappliedInbox(ctx, limit)
}

// MarkApplied stamps applied_at on a terminal (poison) inbox row.
func (r *StoreRecoverer) MarkApplied(ctx context.Context, eventID, appliedAt string) error {
	return r.store.MarkInboxApplied(ctx, eventID, appliedAt)
}

// Start launches the single apply goroutine; it runs until ctx is cancelled.
func (q *Queue) Start(ctx context.Context) {
	go q.run(ctx)
}

// Enqueue hands a job to the apply goroutine. It blocks only if the buffer is
// full (bounded backpressure). The POST handler calls this after the inbox dedup
// insert and returns immediately.
func (q *Queue) Enqueue(job Job) {
	q.jobs <- job
}

// Stop blocks until the apply goroutine has returned (post-cancel teardown).
func (q *Queue) Stop() {
	<-q.doneCh
}

func (q *Queue) run(ctx context.Context) {
	defer close(q.doneCh)

	ticker := time.NewTicker(q.recoverEvery)
	defer ticker.Stop()

	// Startup re-scan: re-drive events durably recorded but never applied (a
	// transient failure or a crash between the inbox INSERT and a successful merge
	// in a previous run). A redelivery of these is deduped and not re-enqueued, so
	// this is the only path that recovers them before the F4.1 pull loop exists.
	q.recoverPending(ctx)

	for {
		select {
		case <-ctx.Done():
			// Best-effort final drain of buffered jobs on shutdown so a queued-but-
			// unapplied event gets one more apply attempt before teardown (mirrors the
			// outbox worker). Anything still un-applied after this is recovered by the
			// next startup re-scan (applied_at stays NULL — durably recorded).
			q.drainBuffered(context.WithoutCancel(ctx))
			return
		case <-ticker.C:
			q.recoverPending(ctx)
		case job := <-q.jobs:
			q.apply(ctx, job)
		}
	}
}

// drainBuffered applies every job already buffered in the channel, then returns.
// It never blocks waiting for new jobs: it drains only what is already queued.
func (q *Queue) drainBuffered(ctx context.Context) {
	for {
		select {
		case job := <-q.jobs:
			q.apply(ctx, job)
		default:
			return
		}
	}
}

// recoverPending re-enqueues inbox rows whose applied_at is still NULL — events
// whose apply was lost to a transient failure (DB busy) or a process restart. The
// dedup INSERT already happened, so a redelivery would NOT re-enqueue them; this
// re-scan is what re-drives them. Re-enqueue (not direct apply) keeps the single
// applier the only writer and reuses the same poison/notify handling.
func (q *Queue) recoverPending(ctx context.Context) {
	if q.recover == nil {
		return
	}
	pending, err := q.recover.ListUnapplied(ctx, recoverLimit)
	if err != nil {
		q.log.ErrorContext(ctx, "federation: inbox recovery scan failed",
			slog.String("op", "federation.inbox.Recover"),
			slog.String("err", err.Error()),
		)
		return
	}
	for _, p := range pending {
		var e events.Event
		if err := events.Unmarshal([]byte(p.Payload), &e); err != nil {
			// A stored event that no longer decodes can never apply — log and leave it
			// (a manual / GC cleanup handles a corrupt row); skipping avoids a tight
			// re-scan loop on an undecodable payload.
			q.log.WarnContext(ctx, "federation: skip undecodable pending inbox event",
				slog.String("op", "federation.inbox.Recover"),
				slog.String("event_id", p.EventID),
			)
			continue
		}
		select {
		case q.jobs <- Job{Event: e, PeerURL: p.PeerInstanceURL}:
		case <-ctx.Done():
			return
		default:
			// Buffer momentarily full (a live-enqueue burst). The row stays applied_at
			// NULL, so the next re-scan tick picks it up — never block the run loop here
			// (it is the sole consumer of q.jobs; blocking would deadlock).
			return
		}
	}
}

func (q *Queue) apply(ctx context.Context, job Job) {
	res, err := q.applier.Apply(ctx, job.Event, job.PeerURL)
	if err != nil {
		// A poison event (out-of-domain value / malformed HLC) is PERMANENT — stamp
		// it applied (terminal) so it never head-of-line blocks the queue and is never
		// re-driven by the recovery re-scan (errors.go classifies it). A TRANSIENT
		// error (DB busy, lock contention) leaves applied_at NULL so the recovery
		// re-scan re-enqueues it rather than silently losing the durably-recorded
		// event (NFR-2 at-least-once).
		_, poison := IsPoison(err)
		level := slog.LevelError
		if poison {
			level = slog.LevelWarn
			q.markPoisonTerminal(ctx, job.Event.EventID)
		}
		q.log.Log(ctx, level, "federation: inbox apply failed",
			slog.String("op", "federation.inbox.Apply"),
			slog.String("event_id", job.Event.EventID),
			slog.String("peer", job.PeerURL),
			slog.Bool("poison", poison),
			slog.String("err", err.Error()),
		)
		return
	}
	if q.notify == nil || !changed(res) {
		return
	}
	q.notify.Notify(ctx, Applied{Event: job.Event, Result: res, PeerURL: job.PeerURL})
}

// markPoisonTerminal stamps a permanently-rejected event's applied_at so it is
// not re-driven by the recovery re-scan. The apply tx itself rolled back (the
// poison error aborted the merge), so this is a separate short write on the
// store's own connection. A nil recoverer (tests) is a no-op.
func (q *Queue) markPoisonTerminal(ctx context.Context, eventID string) {
	if q.recover == nil {
		return
	}
	if err := q.recover.MarkApplied(ctx, eventID, model.FormatUTC(time.Now())); err != nil {
		// If the stamp fails the row stays NULL and the recovery re-scan re-drives it;
		// the apply will poison again and re-attempt the stamp — bounded, not a loss.
		q.log.ErrorContext(ctx, "federation: mark poison terminal failed",
			slog.String("op", "federation.inbox.Apply"),
			slog.String("event_id", eventID),
			slog.String("err", err.Error()),
		)
	}
}

// changed reports whether an apply result altered local state (a field won LWW,
// or the entity was created/deleted). A pure no-op merge returns false so no
// redundant refresh is published.
func changed(res *ApplyResult) bool {
	if res == nil {
		return false
	}
	if res.EntityCreated || res.EntityDeleted || res.ProjectLost {
		return true
	}
	for _, applied := range res.AppliedFields {
		if applied {
			return true
		}
	}
	return false
}

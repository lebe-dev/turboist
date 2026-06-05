package inbox_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/federation/inbox"
	"github.com/lebe-dev/turboist/internal/federation/store"
)

// applierStub records the events handed to it by the queue goroutine and lets a
// test signal completion. It stands in for the real Applier so the queue's
// off-the-HTTP-path dispatch + notify behaviour can be asserted without a DB.
type applierStub struct {
	mu       sync.Mutex
	applied  []string
	attempts map[string]int
	results  map[string]*inbox.ApplyResult
	// errFor[eventID] returns the error a given attempt index should fail with (nil
	// = succeed). It is consulted per call so a test can fail the first attempt
	// transiently and succeed on the recovery re-drive.
	errFor func(eventID string, attempt int) error
	done   chan string
}

func newApplierStub() *applierStub {
	return &applierStub{
		attempts: map[string]int{},
		results:  map[string]*inbox.ApplyResult{},
		done:     make(chan string, 16),
	}
}

func (a *applierStub) Apply(_ context.Context, e events.Event, _ string) (*inbox.ApplyResult, error) {
	a.mu.Lock()
	a.applied = append(a.applied, e.EventID)
	a.attempts[e.EventID]++
	attempt := a.attempts[e.EventID]
	res := a.results[e.EventID]
	errFor := a.errFor
	a.mu.Unlock()
	if errFor != nil {
		if err := errFor(e.EventID, attempt); err != nil {
			a.done <- e.EventID
			return nil, err
		}
	}
	if res == nil {
		res = &inbox.ApplyResult{AppliedFields: map[string]bool{}}
	}
	a.done <- e.EventID
	return res, nil
}

func (a *applierStub) count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.applied)
}

func (a *applierStub) attemptCount(eventID string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.attempts[eventID]
}

// recovererStub stands in for the store-backed Recoverer: ListUnapplied returns
// the pending rows a test seeds, and MarkApplied records the terminal stamps so a
// test can assert a poison event was stamped (and dropped from re-drive).
type recovererStub struct {
	mu      sync.Mutex
	pending []store.PendingInboxEvent
	marked  map[string]bool
}

func newRecovererStub() *recovererStub {
	return &recovererStub{marked: map[string]bool{}}
}

func (r *recovererStub) ListUnapplied(_ context.Context, _ int) ([]store.PendingInboxEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Return only rows not yet stamped terminal (mirrors the WHERE applied_at IS
	// NULL filter so a re-scan does not re-drive an already-applied event).
	out := make([]store.PendingInboxEvent, 0, len(r.pending))
	for _, p := range r.pending {
		if !r.marked[p.EventID] {
			out = append(out, p)
		}
	}
	return out, nil
}

func (r *recovererStub) MarkApplied(_ context.Context, eventID, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.marked[eventID] = true
	return nil
}

func (r *recovererStub) isMarked(eventID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.marked[eventID]
}

func (r *recovererStub) setPending(p ...store.PendingInboxEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pending = p
}

// notifier records the (projectClientID, result) pairs the queue publishes after
// a successful apply, so the test can assert federation-origin notifications fire
// for an applied change (US-3.1 AC2).
type notifier struct {
	mu     sync.Mutex
	calls  []string
	called chan struct{}
}

func newNotifier() *notifier {
	return &notifier{called: make(chan struct{}, 16)}
}

func (n *notifier) Notify(_ context.Context, ev inbox.Applied) {
	n.mu.Lock()
	n.calls = append(n.calls, ev.Event.EventID)
	n.mu.Unlock()
	n.called <- struct{}{}
}

func (n *notifier) count() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.calls)
}

func makeEvent(id string) events.Event {
	return events.Event{EventID: id, EntityType: events.EntityTask, EntityID: "t-" + id, ProjectClientID: "proj-1"}
}

// TestQueue_AppliesOffPath asserts an enqueued event is applied by the single
// goroutine and the enqueue returns immediately (the POST handler does not block
// on apply, F3.2 "return fast").
func TestQueue_AppliesOffPath(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ap := newApplierStub()
	q := inbox.NewQueue(ap, nil, nil, nil)
	q.Start(ctx)
	// Cancel BEFORE Stop so the goroutine can exit (Stop waits on the goroutine).
	defer func() { cancel(); q.Stop() }()

	q.Enqueue(inbox.Job{Event: makeEvent("e1"), PeerURL: "https://a.example"})

	select {
	case <-ap.done:
	case <-time.After(2 * time.Second):
		t.Fatal("event was not applied off the HTTP path within budget")
	}
	if ap.count() != 1 {
		t.Errorf("applied count: got %d, want 1", ap.count())
	}
}

// TestQueue_NotifiesOnAppliedChange asserts the queue calls the federation-origin
// notifier after an apply that changed something (US-3.1 AC2 — a remote change is
// surfaced via SSE, not echo-suppressed).
func TestQueue_NotifiesOnAppliedChange(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ap := newApplierStub()
	ap.results["e1"] = &inbox.ApplyResult{AppliedFields: map[string]bool{"title": true}}
	n := newNotifier()
	q := inbox.NewQueue(ap, n, nil, nil)
	q.Start(ctx)
	defer func() { cancel(); q.Stop() }()

	q.Enqueue(inbox.Job{Event: makeEvent("e1"), PeerURL: "https://a.example"})

	select {
	case <-n.called:
	case <-time.After(2 * time.Second):
		t.Fatal("notifier was not called for an applied change")
	}
	if n.count() != 1 {
		t.Errorf("notify count: got %d, want 1", n.count())
	}
}

// TestQueue_NoNotifyOnNoChange asserts an apply that changed nothing (all fields
// stale) does NOT fire a refresh notification — a redundant refresh is the
// flicker the self-refresh work removed; a no-op merge must stay silent.
func TestQueue_NoNotifyOnNoChange(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ap := newApplierStub()
	ap.results["e1"] = &inbox.ApplyResult{AppliedFields: map[string]bool{}} // nothing applied
	n := newNotifier()
	q := inbox.NewQueue(ap, n, nil, nil)
	q.Start(ctx)
	defer func() { cancel(); q.Stop() }()

	q.Enqueue(inbox.Job{Event: makeEvent("e1"), PeerURL: "https://a.example"})

	select {
	case <-ap.done:
	case <-time.After(2 * time.Second):
		t.Fatal("event not applied")
	}
	// Give the notifier a chance to (wrongly) fire.
	select {
	case <-n.called:
		t.Fatal("notifier fired for a no-op merge")
	case <-time.After(150 * time.Millisecond):
	}
}

// TestQueue_StopDrainsAndExits asserts Stop cancels the goroutine cleanly (the
// cleanupCtx teardown in main.go).
func TestQueue_StopDrainsAndExits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ap := newApplierStub()
	q := inbox.NewQueue(ap, nil, nil, nil)
	q.Start(ctx)
	cancel()
	q.Stop() // blocks until the goroutine returns; a hang fails via test timeout.
}

// payload encodes an event to its inbox-stored JSON so a recoverer stub can hand
// it back through ListUnapplied.
func payload(t *testing.T, e events.Event) string {
	t.Helper()
	b, err := events.Marshal(e)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	return string(b)
}

// TestQueue_TransientFailureRecoveredByRescan asserts a TRANSIENT apply failure
// is NOT silently lost: the durably-recorded inbox row (applied_at NULL) is
// re-driven by the recovery re-scan. The startup re-scan re-enqueues it; the
// applier succeeds on the second attempt. (Closes the at-least-once gap — without
// recovery a DB-busy blip would drop a 202-acknowledged event until the F4.1 pull
// loop, which does not exist yet.)
func TestQueue_TransientFailureRecoveredByRescan(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ap := newApplierStub()
	// Fail the FIRST attempt transiently (a plain error, NOT poison), then succeed.
	ap.errFor = func(_ string, attempt int) error {
		if attempt == 1 {
			return errors.New("database is locked")
		}
		return nil
	}
	rec := newRecovererStub()
	rec.setPending(store.PendingInboxEvent{
		EventID: "e1", PeerInstanceURL: "https://a.example", Payload: payload(t, makeEvent("e1")),
	})
	q := inbox.NewQueue(ap, nil, rec, nil).WithRecoverInterval(20 * time.Millisecond)
	q.Start(ctx)
	defer func() { cancel(); q.Stop() }()

	// The startup re-scan re-enqueues e1; the first apply fails transiently, the
	// recovery re-scan re-drives it, and the second apply succeeds.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ap.attemptCount("e1") >= 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := ap.attemptCount("e1"); got < 2 {
		t.Fatalf("transient failure was not re-driven: attempts=%d, want >=2", got)
	}
	if rec.isMarked("e1") {
		t.Errorf("a transient failure must NOT stamp the row terminal (it must stay re-driveable)")
	}
}

// TestQueue_PoisonStampedTerminal asserts a POISON (permanent) apply failure is
// stamped applied_at (terminal) via the recoverer so it is dropped and never
// re-driven by the re-scan — a permanent reject must not be retried forever.
func TestQueue_PoisonStampedTerminal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ap := newApplierStub()
	ap.errFor = func(_ string, _ int) error {
		return &inbox.PoisonError{EventID: "e1", Reason: "out-of-domain status"}
	}
	rec := newRecovererStub()
	q := inbox.NewQueue(ap, nil, rec, nil)
	q.Start(ctx)
	defer func() { cancel(); q.Stop() }()

	q.Enqueue(inbox.Job{Event: makeEvent("e1"), PeerURL: "https://a.example"})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if rec.isMarked("e1") {
			return // poison stamped terminal — will not be re-driven.
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("poison event was not stamped terminal: marked=%v", rec.isMarked("e1"))
}

// TestQueue_ShutdownDrainsBufferedJobs asserts the buffered jobs channel is
// best-effort drained on shutdown so a queued-but-unapplied event gets a final
// apply attempt rather than being dropped on ctx.Done (mirrors the outbox worker).
func TestQueue_ShutdownDrainsBufferedJobs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ap := newApplierStub()
	// Block the applier on the first event so the second stays buffered when we
	// cancel — proving the final drain (not just the in-flight job) applies it. The
	// stub signals "started" before blocking so the test knows the apply is in
	// flight (the run loop is parked inside q.apply, not yet at the ctx.Done select).
	started := make(chan struct{})
	release := make(chan struct{})
	ap.errFor = func(eventID string, _ int) error {
		if eventID == "block" {
			close(started)
			<-release
		}
		return nil
	}
	q := inbox.NewQueue(ap, nil, nil, nil)
	q.Start(ctx)

	q.Enqueue(inbox.Job{Event: makeEvent("block"), PeerURL: "https://a.example"})
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("blocking apply never started")
	}
	// Buffer a second job, then cancel: it is still sitting in the channel while the
	// run loop is parked inside the blocking apply.
	q.Enqueue(inbox.Job{Event: makeEvent("buffered"), PeerURL: "https://a.example"})
	cancel()
	close(release) // unblock the in-flight apply so run reaches ctx.Done and drains.

	q.Stop()

	if ap.attemptCount("buffered") != 1 {
		t.Errorf("buffered job was not drained on shutdown: attempts=%d, want 1", ap.attemptCount("buffered"))
	}
}

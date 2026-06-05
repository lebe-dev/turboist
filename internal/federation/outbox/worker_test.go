package outbox_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/federation/outbox"
	"github.com/lebe-dev/turboist/internal/federation/store"
	"github.com/lebe-dev/turboist/internal/model"
)

// peerStub records pushes and lets a test inject a per-peer failure.
type peerStub struct {
	mu        sync.Mutex
	pushed    map[string][]string // peerURL -> event ids delivered
	failFor   map[string]error    // peerURL -> error to return
	attempts  map[string]int      // peerURL -> POST attempts (incl. failures)
	pushSeen  int
	clearFail bool // when true, a peer's failFor entry is cleared after first attempt
}

func newPeerStub() *peerStub {
	return &peerStub{pushed: map[string][]string{}, failFor: map[string]error{}, attempts: map[string]int{}}
}

func (p *peerStub) Push(_ context.Context, peerURL string, payloads []string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pushSeen++
	p.attempts[peerURL]++
	if err := p.failFor[peerURL]; err != nil {
		if p.clearFail {
			delete(p.failFor, peerURL) // next attempt (after backoff) succeeds.
		}
		return err
	}
	p.pushed[peerURL] = append(p.pushed[peerURL], payloads...)
	return nil
}

func (p *peerStub) deliveredCount(peerURL string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.pushed[peerURL])
}

func (p *peerStub) attemptCount(peerURL string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.attempts[peerURL]
}

// permErr is a PEER-SCOPED permanent (do-not-retry) push error: it satisfies both
// the worker's FederationPermanent() and FederationPeerScoped() seams, mirroring
// the service-layer *RemoteHandshakeError for a 403 link reject (a revoked /
// read-only peer). A peer-scoped permanent reject gates the WHOLE peer so it is
// not re-POSTed.
type permErr struct{ msg string }

func (e *permErr) Error() string              { return e.msg }
func (e *permErr) FederationPermanent() bool  { return true }
func (e *permErr) FederationPeerScoped() bool { return true }

// peerLister returns the configured peers for a project.
type peerLister struct {
	peers map[int64][]outbox.Peer
}

func (l peerLister) PeersForProject(_ context.Context, projectID int64) ([]outbox.Peer, error) {
	return l.peers[projectID], nil
}

func openWorkerDB(t *testing.T) (*sql.DB, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "worker.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.RunMigrations(context.Background(), d); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d, store.New(d)
}

func seedFedProject(t *testing.T, d *sql.DB) int64 {
	t.Helper()
	if _, err := d.Exec(
		`INSERT OR IGNORE INTO contexts (id, name, color, client_id, created_at, updated_at)
		 VALUES (1, 'c', 'blue', 'w-ctx', '2024-01-01T00:00:00.000Z', '2024-01-01T00:00:00.000Z')`,
	); err != nil {
		t.Fatalf("ctx: %v", err)
	}
	res, err := d.Exec(
		`INSERT INTO projects (context_id, title, color, status, is_federated, client_id, created_at, updated_at)
		 VALUES (1, 'Shared', 'blue', 'open', 1, ?, '2024-01-01T00:00:00.000Z', '2024-01-01T00:00:00.000Z')`,
		model.NewClientID())
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func enqueue(t *testing.T, ctx context.Context, d *sql.DB, s *store.Store, eventID string, pid int64) {
	t.Helper()
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := s.InsertOutboxTx(ctx, tx, eventID, pid, `{"event_id":"`+eventID+`"}`, 1, "2024-01-01T00:00:00.000Z"); err != nil {
		t.Fatalf("insert outbox: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

// TestWorker_DrainDeliversAndStamps asserts a single drain pass pushes every
// undelivered event to each non-revoked peer and stamps delivered_to so a second
// drain is a no-op (US-3.2 AC2 — delivered_to stamped; US-3.1 AC1 push path).
func TestWorker_DrainDeliversAndStamps(t *testing.T) {
	d, s := openWorkerDB(t)
	ctx := context.Background()
	pid := seedFedProject(t, d)
	enqueue(t, ctx, d, s, "e1", pid)
	enqueue(t, ctx, d, s, "e2", pid)

	stub := newPeerStub()
	w := outbox.NewWorker(s, peerLister{peers: map[int64][]outbox.Peer{
		pid: {{InstanceURL: "https://a.example"}},
	}}, stub, nil)

	if err := w.DrainOnce(ctx); err != nil {
		t.Fatalf("drain: %v", err)
	}
	if got := stub.deliveredCount("https://a.example"); got != 2 {
		t.Fatalf("delivered: got %d, want 2", got)
	}

	// Second drain: nothing pending, no further pushes.
	before := stub.pushSeen
	if err := w.DrainOnce(ctx); err != nil {
		t.Fatalf("drain2: %v", err)
	}
	pending, _ := s.PendingDeliveryCount(ctx, pid, "https://a.example")
	if pending != 0 {
		t.Errorf("pending after drain: got %d, want 0", pending)
	}
	if stub.pushSeen != before {
		t.Errorf("second drain should not push: pushSeen %d -> %d", before, stub.pushSeen)
	}
}

// TestWorker_PeerFailureIsolated asserts a failing peer does not block delivery
// to a healthy peer and does NOT stamp the failing peer's delivered_to, so the
// event is retried on the next drain (US-3.2 AC3 — per-peer backoff isolation).
func TestWorker_PeerFailureIsolated(t *testing.T) {
	d, s := openWorkerDB(t)
	ctx := context.Background()
	pid := seedFedProject(t, d)
	enqueue(t, ctx, d, s, "e1", pid)

	stub := newPeerStub()
	stub.failFor["https://down.example"] = errors.New("502 bad gateway")

	w := outbox.NewWorker(s, peerLister{peers: map[int64][]outbox.Peer{
		pid: {
			{InstanceURL: "https://up.example"},
			{InstanceURL: "https://down.example"},
		},
	}}, stub, nil)

	if err := w.DrainOnce(ctx); err != nil {
		t.Fatalf("drain: %v", err)
	}

	if got := stub.deliveredCount("https://up.example"); got != 1 {
		t.Errorf("healthy peer delivered: got %d, want 1", got)
	}
	upPending, _ := s.PendingDeliveryCount(ctx, pid, "https://up.example")
	if upPending != 0 {
		t.Errorf("healthy peer should be drained: pending %d", upPending)
	}
	downPending, _ := s.PendingDeliveryCount(ctx, pid, "https://down.example")
	if downPending != 1 {
		t.Errorf("failed peer event must remain pending for retry: got %d, want 1", downPending)
	}
}

// TestWorker_PingTriggersImmediateDrain asserts a commit-ping wakes the running
// worker so push is immediate (NFR-1.1 push <5s, not tick-delayed). The worker
// is started with a long tick; only the ping should drive the drain.
func TestWorker_PingTriggersImmediateDrain(t *testing.T) {
	d, s := openWorkerDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pid := seedFedProject(t, d)

	stub := newPeerStub()
	w := outbox.NewWorker(s, peerLister{peers: map[int64][]outbox.Peer{
		pid: {{InstanceURL: "https://a.example"}},
	}}, stub, nil)

	// A deliberately long tick so a passing test cannot be the ticker firing.
	w.Start(ctx, time.Hour)

	enqueue(t, ctx, d, s, "e1", pid)
	w.Ping()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if stub.deliveredCount("https://a.example") == 1 {
			return // delivered well under the 5s budget via the ping.
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("ping did not trigger drain within budget: delivered=%d", stub.deliveredCount("https://a.example"))
}

// TestWorker_StopCancels asserts the worker goroutine exits when its context is
// cancelled (clean shutdown — the cleanupCtx teardown in main.go).
func TestWorker_StopCancels(t *testing.T) {
	d, s := openWorkerDB(t)
	pid := seedFedProject(t, d)
	stub := newPeerStub()
	w := outbox.NewWorker(s, peerLister{peers: map[int64][]outbox.Peer{
		pid: {{InstanceURL: "https://a.example"}},
	}}, stub, nil)

	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx, time.Hour)
	cancel()
	// Stop blocks until the goroutine returns; a hang here fails via the test
	// timeout, proving the worker did not exit.
	w.Stop()
}

// TestWorker_PermanentErrorNotRetried asserts a PERMANENT push rejection (a 4xx
// ≠429, e.g. a revoked-peer 403) marks the peer failed so it is NOT re-POSTed on
// subsequent drains — a permanent reject must not be retried forever (F3.2 /
// finding fix). Under F4.4 the failed event is parked in the dead-letter table
// (US-4.4 AC3) and therefore excluded from the pending-delivery count.
func TestWorker_PermanentErrorNotRetried(t *testing.T) {
	d, s := openWorkerDB(t)
	ctx := context.Background()
	pid := seedFedProject(t, d)
	enqueue(t, ctx, d, s, "e1", pid)

	stub := newPeerStub()
	stub.failFor["https://revoked.example"] = &permErr{msg: "403 forbidden"}
	w := outbox.NewWorker(s, peerLister{peers: map[int64][]outbox.Peer{
		pid: {{InstanceURL: "https://revoked.example"}},
	}}, stub, nil)

	// First drain: one POST attempt, permanent reject → peer marked failed.
	if err := w.DrainOnce(ctx); err != nil {
		t.Fatalf("drain1: %v", err)
	}
	// Many subsequent drains (as the 60s safety-net tick / commit-ping would fire):
	// the permanently-failed peer must NOT be re-POSTed.
	for i := 0; i < 5; i++ {
		if err := w.DrainOnce(ctx); err != nil {
			t.Fatalf("drain%d: %v", i+2, err)
		}
	}
	if got := stub.attemptCount("https://revoked.example"); got != 1 {
		t.Errorf("permanent reject must not be retried: POST attempts got %d, want 1", got)
	}
	// The event is dead-lettered (F4.4 US-4.4 AC3), so it no longer counts as
	// pending — a permanent reject must not keep the sync status stuck forever.
	pending, _ := s.PendingDeliveryCount(ctx, pid, "https://revoked.example")
	if pending != 0 {
		t.Errorf("permanently-failed peer event should be dead-lettered (excluded from pending): got %d, want 0", pending)
	}
	dls, err := s.ListDeadLetter(ctx, 0)
	if err != nil {
		t.Fatalf("list dead-letter: %v", err)
	}
	if len(dls) != 1 || dls[0].EventID != "e1" {
		t.Errorf("permanent reject should be parked in dead-letter: got %+v", dls)
	}
}

// TestWorker_TransientBackoffGatesThenReprobes asserts a TRANSIENT failure gates
// the peer for the backoff window (not re-POSTed while gated), and that once the
// window elapses the peer is re-probed and a recovered peer drains (F3.2 per-peer
// backoff 1s..1h). A fake clock makes the timing deterministic.
func TestWorker_TransientBackoffGatesThenReprobes(t *testing.T) {
	d, s := openWorkerDB(t)
	ctx := context.Background()
	pid := seedFedProject(t, d)
	enqueue(t, ctx, d, s, "e1", pid)

	clock := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	stub := newPeerStub()
	stub.clearFail = true // the peer recovers: first attempt fails transiently, later succeeds.
	stub.failFor["https://flaky.example"] = errors.New("503 service unavailable")
	w := outbox.NewWorker(s, peerLister{peers: map[int64][]outbox.Peer{
		pid: {{InstanceURL: "https://flaky.example"}},
	}}, stub, nil).WithClock(clock.now)

	// Drain 1: transient failure → peer gated for backoffMin (1s).
	if err := w.DrainOnce(ctx); err != nil {
		t.Fatalf("drain1: %v", err)
	}
	if got := stub.attemptCount("https://flaky.example"); got != 1 {
		t.Fatalf("first drain should POST once: got %d", got)
	}

	// Drain 2, still inside the 1s window (clock not advanced): peer SKIPPED, no POST.
	if err := w.DrainOnce(ctx); err != nil {
		t.Fatalf("drain2: %v", err)
	}
	if got := stub.attemptCount("https://flaky.example"); got != 1 {
		t.Errorf("gated peer must not be re-POSTed inside backoff: attempts got %d, want 1", got)
	}

	// Advance past the 1s window and drain again: peer re-probed, recovers, delivers.
	clock.advance(2 * time.Second)
	if err := w.DrainOnce(ctx); err != nil {
		t.Fatalf("drain3: %v", err)
	}
	if got := stub.attemptCount("https://flaky.example"); got != 2 {
		t.Errorf("peer should be re-probed after backoff: attempts got %d, want 2", got)
	}
	if got := stub.deliveredCount("https://flaky.example"); got != 1 {
		t.Errorf("recovered peer should deliver: got %d, want 1", got)
	}
	pending, _ := s.PendingDeliveryCount(ctx, pid, "https://flaky.example")
	if pending != 0 {
		t.Errorf("recovered peer should be drained: pending got %d, want 0", pending)
	}
}

// fakeClock is a manually-advanced clock for deterministic backoff-gate tests.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

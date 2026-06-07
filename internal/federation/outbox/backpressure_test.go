package outbox_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/federation/outbox"
	"github.com/lebe-dev/turboist/internal/federation/store"
)

// classifiedErr is a push error carrying the full F4.4 classification seams the
// worker reads: permanent (4xx ≠ 429) → dead-letter, peerScoped (a 403 link
// reject) → gate the whole peer, retryAfter (429) → honor the peer's Retry-After
// window, plus the status/reason recorded on a dead-letter row. It mirrors the
// service-layer *RemoteHandshakeError.
type classifiedErr struct {
	msg        string
	permanent  bool
	peerScoped bool
	statusCode int
	reason     string
	retryAfter time.Duration
	hasRetry   bool
}

func (e *classifiedErr) Error() string              { return e.msg }
func (e *classifiedErr) FederationPermanent() bool  { return e.permanent }
func (e *classifiedErr) FederationPeerScoped() bool { return e.peerScoped }
func (e *classifiedErr) FederationStatusCode() int  { return e.statusCode }
func (e *classifiedErr) FederationReason() string   { return e.reason }
func (e *classifiedErr) FederationRetryAfter() (time.Duration, bool) {
	return e.retryAfter, e.hasRetry
}

// TestWorker_429HonorsRetryAfter asserts a 429 with a Retry-After gates the peer
// for EXACTLY the Retry-After window (not the exponential default), and that the
// peer is re-probed only after that window elapses (US-4.4 AC1).
func TestWorker_429HonorsRetryAfter(t *testing.T) {
	d, s := openWorkerDB(t)
	ctx := context.Background()
	pid := seedFedProject(t, d)
	enqueue(t, ctx, d, s, "e1", pid)

	clock := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	stub := newPeerStub()
	stub.clearFail = true // the peer recovers after the Retry-After window.
	stub.failFor["https://busy.example"] = &classifiedErr{
		msg: "429 too many requests", retryAfter: 30 * time.Second, hasRetry: true,
	}
	w := outbox.NewWorker(s, peerLister{peers: map[int64][]outbox.Peer{
		pid: {{InstanceURL: "https://busy.example"}},
	}}, stub, nil).WithClock(clock.now)

	// Drain 1: 429 → gated for the 30s Retry-After.
	if err := w.DrainOnce(ctx); err != nil {
		t.Fatalf("drain1: %v", err)
	}
	if got := stub.attemptCount("https://busy.example"); got != 1 {
		t.Fatalf("first drain should POST once: got %d", got)
	}

	// Drain 2 at +20s, still inside the 30s Retry-After: peer SKIPPED, no POST.
	clock.advance(20 * time.Second)
	if err := w.DrainOnce(ctx); err != nil {
		t.Fatalf("drain2: %v", err)
	}
	if got := stub.attemptCount("https://busy.example"); got != 1 {
		t.Errorf("gated peer must not be re-POSTed inside Retry-After: attempts got %d, want 1", got)
	}

	// Drain 3 at +40s, past the 30s Retry-After: peer re-probed, recovers.
	clock.advance(20 * time.Second)
	if err := w.DrainOnce(ctx); err != nil {
		t.Fatalf("drain3: %v", err)
	}
	if got := stub.attemptCount("https://busy.example"); got != 2 {
		t.Errorf("peer should be re-probed after Retry-After: attempts got %d, want 2", got)
	}
	if got := stub.deliveredCount("https://busy.example"); got != 1 {
		t.Errorf("recovered peer should deliver: got %d, want 1", got)
	}
}

// TestWorker_5xxBackoffSequenceIsolatedPerPeer asserts a 5xx backs off the peer
// with the exponential 1s, 2s, 4s sequence and that the backoff is isolated per
// peer (a 5xx peer never delays a healthy one) — US-4.4 AC2.
func TestWorker_5xxBackoffSequenceIsolatedPerPeer(t *testing.T) {
	d, s := openWorkerDB(t)
	ctx := context.Background()
	pid := seedFedProject(t, d)
	enqueue(t, ctx, d, s, "e1", pid)

	clock := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	stub := newPeerStub()
	stub.failFor["https://down.example"] = errors.New("503 service unavailable")
	w := outbox.NewWorker(s, peerLister{peers: map[int64][]outbox.Peer{
		pid: {
			{InstanceURL: "https://up.example"},
			{InstanceURL: "https://down.example"},
		},
	}}, stub, nil).WithClock(clock.now)

	// Drain at t0: down peer fails once → gated 1s; up peer delivers immediately.
	if err := w.DrainOnce(ctx); err != nil {
		t.Fatalf("drain1: %v", err)
	}
	if got := stub.deliveredCount("https://up.example"); got != 1 {
		t.Errorf("healthy peer delivered: got %d, want 1 (backoff must be isolated)", got)
	}
	if got := stub.attemptCount("https://down.example"); got != 1 {
		t.Fatalf("down peer attempts after t0: got %d, want 1", got)
	}

	// Inside the 1s window (no advance): down peer SKIPPED.
	if err := w.DrainOnce(ctx); err != nil {
		t.Fatalf("drain2: %v", err)
	}
	if got := stub.attemptCount("https://down.example"); got != 1 {
		t.Errorf("down peer re-POSTed inside 1s window: got %d, want 1", got)
	}

	// +1s → second attempt fails → gated 2s.
	clock.advance(time.Second)
	if err := w.DrainOnce(ctx); err != nil {
		t.Fatalf("drain3: %v", err)
	}
	if got := stub.attemptCount("https://down.example"); got != 2 {
		t.Errorf("down peer attempts at +1s: got %d, want 2", got)
	}
	// +1s more (total +2s from attempt 2, only 1s passed) → still gated.
	clock.advance(time.Second)
	if err := w.DrainOnce(ctx); err != nil {
		t.Fatalf("drain4: %v", err)
	}
	if got := stub.attemptCount("https://down.example"); got != 2 {
		t.Errorf("down peer re-POSTed inside 2s window: got %d, want 2", got)
	}
	// +1s more (now 2s since attempt 2) → third attempt.
	clock.advance(time.Second)
	if err := w.DrainOnce(ctx); err != nil {
		t.Fatalf("drain5: %v", err)
	}
	if got := stub.attemptCount("https://down.example"); got != 3 {
		t.Errorf("down peer attempts after 2s window: got %d, want 3", got)
	}
}

// TestWorker_PermanentFailureDeadLetters asserts a 4xx (≠429) permanent reject
// parks the event in the dead-letter table (NOT retried) and that the event no
// longer counts toward the per-peer pending count (US-4.4 AC3 / dead-letter
// excluded from pending).
func TestWorker_PermanentFailureDeadLetters(t *testing.T) {
	d, s := openWorkerDB(t)
	ctx := context.Background()
	pid := seedFedProject(t, d)
	enqueue(t, ctx, d, s, "e1", pid)

	stub := newPeerStub()
	stub.failFor["https://revoked.example"] = &classifiedErr{
		msg: "403 forbidden", permanent: true, peerScoped: true, statusCode: 403, reason: "federation_read_only",
	}
	w := outbox.NewWorker(s, peerLister{peers: map[int64][]outbox.Peer{
		pid: {{InstanceURL: "https://revoked.example"}},
	}}, stub, nil)

	if err := w.DrainOnce(ctx); err != nil {
		t.Fatalf("drain1: %v", err)
	}

	// The event is parked in the dead-letter table with the classified status/reason.
	dls, err := s.ListDeadLetter(ctx, 0)
	if err != nil {
		t.Fatalf("list dead-letter: %v", err)
	}
	if len(dls) != 1 {
		t.Fatalf("dead-letter rows: got %d, want 1", len(dls))
	}
	if dls[0].EventID != "e1" || dls[0].PeerURL != "https://revoked.example" || dls[0].StatusCode != 403 || dls[0].Reason != "federation_read_only" {
		t.Errorf("dead-letter row: got %+v", dls[0])
	}

	// The dead-lettered event no longer counts as pending for the peer.
	pending, _ := s.PendingDeliveryCount(ctx, pid, "https://revoked.example")
	if pending != 0 {
		t.Errorf("dead-lettered event must be excluded from pending: got %d, want 0", pending)
	}

	// Subsequent drains do NOT re-POST the dead-lettered event.
	for i := 0; i < 3; i++ {
		if err := w.DrainOnce(ctx); err != nil {
			t.Fatalf("drain%d: %v", i+2, err)
		}
	}
	if got := stub.attemptCount("https://revoked.example"); got != 1 {
		t.Errorf("dead-lettered event must not be re-POSTed: attempts got %d, want 1", got)
	}
}

// TestWorker_RestoresBackoffAcrossRestart asserts a fresh Worker constructed over
// the SAME store restores a peer's persisted retry gate so it is NOT re-POSTed
// immediately after a restart while still inside its backoff window (§7 F4.4
// risk: "persist retry-not-before across restart").
func TestWorker_RestoresBackoffAcrossRestart(t *testing.T) {
	d, s := openWorkerDB(t)
	ctx := context.Background()
	pid := seedFedProject(t, d)
	enqueue(t, ctx, d, s, "e1", pid)

	clock := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	stub := newPeerStub()
	stub.failFor["https://flaky.example"] = errors.New("503 service unavailable")
	w1 := outbox.NewWorker(s, peerLister{peers: map[int64][]outbox.Peer{
		pid: {{InstanceURL: "https://flaky.example"}},
	}}, stub, nil).WithClock(clock.now)
	if err := w1.RestoreBackoff(ctx); err != nil {
		t.Fatalf("restore w1: %v", err)
	}

	// Drain 1: transient failure → peer gated 1s, persisted to the store.
	if err := w1.DrainOnce(ctx); err != nil {
		t.Fatalf("drain w1: %v", err)
	}
	if got := stub.attemptCount("https://flaky.example"); got != 1 {
		t.Fatalf("w1 first drain should POST once: got %d", got)
	}

	// Simulate a restart: a brand-new worker over the same store, same clock (the
	// 1s backoff window has NOT elapsed). Restore the persisted gate.
	stub2 := newPeerStub()
	stub2.failFor["https://flaky.example"] = errors.New("503 service unavailable")
	w2 := outbox.NewWorker(s, peerLister{peers: map[int64][]outbox.Peer{
		pid: {{InstanceURL: "https://flaky.example"}},
	}}, stub2, nil).WithClock(clock.now)
	if err := w2.RestoreBackoff(ctx); err != nil {
		t.Fatalf("restore w2: %v", err)
	}

	// The restored worker must NOT re-POST inside the persisted backoff window.
	if err := w2.DrainOnce(ctx); err != nil {
		t.Fatalf("drain w2: %v", err)
	}
	if got := stub2.attemptCount("https://flaky.example"); got != 0 {
		t.Errorf("restarted worker must honor persisted backoff: attempts got %d, want 0", got)
	}
}

// TestWorker_ChunksBatchByCount asserts a batch larger than the max-events chunk
// size is split into multiple POSTs (US-4.4 AC4 — chunk by count).
func TestWorker_ChunksBatchByCount(t *testing.T) {
	d, s := openWorkerDB(t)
	ctx := context.Background()
	pid := seedFedProject(t, d)
	// 5 events with a 2-event chunk cap → 3 POSTs (2, 2, 1).
	for _, id := range []string{"e1", "e2", "e3", "e4", "e5"} {
		enqueue(t, ctx, d, s, id, pid)
	}

	stub := newChunkStub()
	w := outbox.NewWorker(s, peerLister{peers: map[int64][]outbox.Peer{
		pid: {{InstanceURL: "https://a.example"}},
	}}, stub, nil).WithChunkLimits(2, 0)

	if err := w.DrainOnce(ctx); err != nil {
		t.Fatalf("drain: %v", err)
	}
	sizes := stub.batchSizes("https://a.example")
	if len(sizes) != 3 {
		t.Fatalf("chunk count: got %d POSTs %v, want 3 (2,2,1)", len(sizes), sizes)
	}
	if sizes[0] != 2 || sizes[1] != 2 || sizes[2] != 1 {
		t.Errorf("chunk sizes: got %v, want [2 2 1]", sizes)
	}
	if stub.deliveredCount("https://a.example") != 5 {
		t.Errorf("all events delivered across chunks: got %d, want 5", stub.deliveredCount("https://a.example"))
	}
}

// TestWorker_ChunksBatchByBytes asserts a batch is split when the serialized
// payload bytes exceed the max-bytes chunk size, even when the count is under the
// per-chunk count cap (US-4.4 AC4 — chunk by bytes / 5 MB).
func TestWorker_ChunksBatchByBytes(t *testing.T) {
	d, s := openWorkerDB(t)
	ctx := context.Background()
	pid := seedFedProject(t, d)
	// Each event payload `{"event_id":"eN"}` is ~20 bytes. With a 30-byte cap, a
	// single payload fits but two do not → one event per chunk.
	for _, id := range []string{"e1", "e2", "e3"} {
		enqueue(t, ctx, d, s, id, pid)
	}

	stub := newChunkStub()
	w := outbox.NewWorker(s, peerLister{peers: map[int64][]outbox.Peer{
		pid: {{InstanceURL: "https://a.example"}},
	}}, stub, nil).WithChunkLimits(100, 30)

	if err := w.DrainOnce(ctx); err != nil {
		t.Fatalf("drain: %v", err)
	}
	sizes := stub.batchSizes("https://a.example")
	if len(sizes) != 3 {
		t.Fatalf("byte-chunk count: got %d POSTs %v, want 3 (one event each)", len(sizes), sizes)
	}
	for i, n := range sizes {
		if n != 1 {
			t.Errorf("byte-chunk %d size: got %d, want 1", i, n)
		}
	}
	if stub.deliveredCount("https://a.example") != 3 {
		t.Errorf("all events delivered across byte chunks: got %d, want 3", stub.deliveredCount("https://a.example"))
	}
}

// TestWorker_EventScopedRejectDoesNotStrandPeer is the regression test for the
// F4.4 BLOCKING finding: a SINGLE event-specific permanent 4xx (a 410 stale-
// tombstone — a re-edit of a tombstoned entity per the offline contract) must
// dead-letter ONLY the offending event WITHOUT gating the whole peer, so the
// peer's OTHER healthy events still flow. Before the fix, the event-scoped 410
// marked the entire link permanent and silently stranded every other event.
func TestWorker_EventScopedRejectDoesNotStrandPeer(t *testing.T) {
	d, s := openWorkerDB(t)
	ctx := context.Background()
	pid := seedFedProject(t, d)
	// e1 is the offending (tombstoned re-edit → 410) event; e2/e3 are healthy.
	for _, id := range []string{"e1", "e2", "e3"} {
		enqueue(t, ctx, d, s, id, pid)
	}

	const peer = "https://stale.example"
	stub := newPayloadFailStub()
	// e1 → event-scoped 410 (NOT peer-scoped); everything else delivers.
	stub.failEvent("e1", &classifiedErr{
		msg: "410 gone", permanent: true, peerScoped: false, statusCode: 410, reason: "federation_stale_pull",
	})
	// One event per POST so the offending event is isolated from the healthy ones.
	w := outbox.NewWorker(s, peerLister{peers: map[int64][]outbox.Peer{
		pid: {{InstanceURL: peer}},
	}}, stub, nil).WithChunkLimits(1, 0)

	if err := w.DrainOnce(ctx); err != nil {
		t.Fatalf("drain1: %v", err)
	}

	// e2 and e3 must have been delivered despite e1's 410 — the link is NOT gated.
	if !stub.delivered("e2") || !stub.delivered("e3") {
		t.Errorf("healthy events stranded by an event-scoped reject: delivered=%v", stub.deliveredIDs())
	}
	// e1 is parked in the dead-letter table (and only e1).
	dls, err := s.ListDeadLetter(ctx, 0)
	if err != nil {
		t.Fatalf("list dead-letter: %v", err)
	}
	if len(dls) != 1 || dls[0].EventID != "e1" || dls[0].StatusCode != 410 {
		t.Errorf("only the offending event should be dead-lettered: got %+v", dls)
	}
	// Pending drops to 0: e2/e3 delivered, e1 dead-lettered (excluded from pending).
	pending, _ := s.PendingDeliveryCount(ctx, pid, peer)
	if pending != 0 {
		t.Errorf("pending after event-scoped reject: got %d, want 0", pending)
	}
}

// TestWorker_NewHealthyEventNotStrandedAfterEventScopedReject asserts the exact
// scenario the finding calls out: a NEW healthy event enqueued AFTER a peer saw an
// (unrelated) event-scoped permanent 4xx is NOT silently stranded — it is
// delivered on the next drain because the link was never gated.
func TestWorker_NewHealthyEventNotStrandedAfterEventScopedReject(t *testing.T) {
	d, s := openWorkerDB(t)
	ctx := context.Background()
	pid := seedFedProject(t, d)
	enqueue(t, ctx, d, s, "bad", pid)

	const peer = "https://peer.example"
	stub := newPayloadFailStub()
	stub.failEvent("bad", &classifiedErr{
		msg: "400 author mismatch", permanent: true, peerScoped: false, statusCode: 400, reason: "federation_author_mismatch",
	})
	w := outbox.NewWorker(s, peerLister{peers: map[int64][]outbox.Peer{
		pid: {{InstanceURL: peer}},
	}}, stub, nil).WithChunkLimits(1, 0)

	// Drain 1: the bad event is rejected (event-scoped 400) and dead-lettered.
	if err := w.DrainOnce(ctx); err != nil {
		t.Fatalf("drain1: %v", err)
	}

	// A brand-new HEALTHY event is enqueued for the same peer AFTER the reject.
	enqueue(t, ctx, d, s, "fresh", pid)

	// Drain 2: the new healthy event must be delivered — NOT stranded behind the
	// earlier event-scoped reject.
	if err := w.DrainOnce(ctx); err != nil {
		t.Fatalf("drain2: %v", err)
	}
	if !stub.delivered("fresh") {
		t.Errorf("new healthy event stranded after an unrelated event-scoped reject: delivered=%v", stub.deliveredIDs())
	}
	pending, _ := s.PendingDeliveryCount(ctx, pid, peer)
	if pending != 0 {
		t.Errorf("new healthy event not delivered: pending got %d, want 0", pending)
	}
}

// TestWorker_PeerScopedRejectGatesLink_RetryPeerReEnables asserts a PEER-SCOPED
// reject (a revoked/read-only 403) DOES gate the whole link — a healthy event
// enqueued afterward is held — and that the explicit operator re-enable path
// (Worker.RetryPeer) clears the gate so delivery resumes (the F4.4 operator
// remediation path).
func TestWorker_PeerScopedRejectGatesLink_RetryPeerReEnables(t *testing.T) {
	d, s := openWorkerDB(t)
	ctx := context.Background()
	pid := seedFedProject(t, d)
	enqueue(t, ctx, d, s, "e1", pid)

	const peer = "https://revoked.example"
	stub := newPeerStub()
	stub.failFor[peer] = &classifiedErr{
		msg: "403 revoked", permanent: true, peerScoped: true, statusCode: 403, reason: "federation_revoked",
	}
	w := outbox.NewWorker(s, peerLister{peers: map[int64][]outbox.Peer{
		pid: {{InstanceURL: peer}},
	}}, stub, nil)

	// Drain 1: peer-scoped 403 → e1 dead-lettered AND the whole link gated permanent.
	if err := w.DrainOnce(ctx); err != nil {
		t.Fatalf("drain1: %v", err)
	}

	// A new healthy event is enqueued; while the link is gated it must NOT be POSTed.
	enqueue(t, ctx, d, s, "e2", pid)
	for i := 0; i < 3; i++ {
		if err := w.DrainOnce(ctx); err != nil {
			t.Fatalf("drain%d: %v", i+2, err)
		}
	}
	if got := stub.attemptCount(peer); got != 1 {
		t.Errorf("gated link must not be re-POSTed: attempts got %d, want 1", got)
	}

	// The persisted gate row exists (so a restart would re-load permanent=true).
	rows, err := s.LoadPeerRetry(ctx)
	if err != nil {
		t.Fatalf("load peer retry: %v", err)
	}
	if len(rows) != 1 || rows[0].PeerURL != peer || !rows[0].Permanent {
		t.Fatalf("expected a persisted permanent gate for the peer: got %+v", rows)
	}

	// The peer stops failing (operator resolved the cause); RetryPeer re-enables it.
	delete(stub.failFor, peer)
	if err := w.RetryPeer(ctx, peer); err != nil {
		t.Fatalf("retry peer: %v", err)
	}
	// The durable gate is cleared.
	rows, err = s.LoadPeerRetry(ctx)
	if err != nil {
		t.Fatalf("load peer retry after re-enable: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("RetryPeer must clear the durable gate: got %+v", rows)
	}

	// The next drain re-probes the peer and delivers the held event.
	if err := w.DrainOnce(ctx); err != nil {
		t.Fatalf("drain after re-enable: %v", err)
	}
	if got := stub.attemptCount(peer); got != 2 {
		t.Errorf("re-enabled peer should be re-probed: attempts got %d, want 2", got)
	}
	if got := stub.deliveredCount(peer); got != 1 {
		t.Errorf("re-enabled peer should deliver the held event: got %d, want 1", got)
	}
}

// TestWorker_PeerScopedRejectMidBatchDeadLettersRemaining is the regression test
// for the F4.4 multi-chunk-then-permanent-tail finding: when a MULTI-CHUNK batch
// hits a PEER-SCOPED permanent reject (a revoked/read-only 403 that gates the
// whole link), the chunks NOT yet attempted must NOT be left pending-but-
// undelivered (invisible, stranded behind the permanent gate forever). They are
// dead-lettered alongside the offending chunk so every event in the read batch is
// accounted for (visible in the dead-letter diagnostics, excluded from pending),
// and the peer is gated so nothing is re-POSTed.
func TestWorker_PeerScopedRejectMidBatchDeadLettersRemaining(t *testing.T) {
	d, s := openWorkerDB(t)
	ctx := context.Background()
	pid := seedFedProject(t, d)
	// 5 events, 2-event chunk cap → chunks (e1,e2),(e3,e4),(e5). The peer-scoped
	// 403 fails the FIRST POST; the remaining two chunks are never attempted.
	for _, id := range []string{"e1", "e2", "e3", "e4", "e5"} {
		enqueue(t, ctx, d, s, id, pid)
	}

	const peer = "https://revoked.example"
	stub := newPeerStub()
	stub.failFor[peer] = &classifiedErr{
		msg: "403 revoked", permanent: true, peerScoped: true, statusCode: 403, reason: "federation_revoked",
	}
	w := outbox.NewWorker(s, peerLister{peers: map[int64][]outbox.Peer{
		pid: {{InstanceURL: peer}},
	}}, stub, nil).WithChunkLimits(2, 0)

	if err := w.DrainOnce(ctx); err != nil {
		t.Fatalf("drain: %v", err)
	}

	// Only the first chunk was POSTed before the peer-scoped reject halted the link;
	// the remaining chunks are dead-lettered WITHOUT a re-POST.
	if got := stub.attemptCount(peer); got != 1 {
		t.Errorf("peer-scoped reject must halt after one POST: attempts got %d, want 1", got)
	}

	// All five events are parked in the dead-letter table — none stranded pending.
	dls, err := s.ListDeadLetter(ctx, 0)
	if err != nil {
		t.Fatalf("list dead-letter: %v", err)
	}
	if len(dls) != 5 {
		t.Fatalf("all batch events must be dead-lettered: got %d, want 5", len(dls))
	}
	for _, dl := range dls {
		if dl.StatusCode != 403 || dl.Reason != "federation_revoked" {
			t.Errorf("dead-letter row %s: got status=%d reason=%q, want 403/federation_revoked", dl.EventID, dl.StatusCode, dl.Reason)
		}
	}

	// Nothing is left pending-but-undelivered for the gated peer.
	pending, _ := s.PendingDeliveryCount(ctx, pid, peer)
	if pending != 0 {
		t.Errorf("no event may be stranded pending after a peer-scoped reject: got %d, want 0", pending)
	}
}

// payloadFailStub fails a Push only when a specific event id appears in the POSTed
// payloads (so the offending event can be isolated from healthy ones within a
// drain), and records the event ids successfully delivered.
type payloadFailStub struct {
	mu       sync.Mutex
	failByID map[string]error
	deliv    map[string]bool
	order    []string
}

func newPayloadFailStub() *payloadFailStub {
	return &payloadFailStub{failByID: map[string]error{}, deliv: map[string]bool{}}
}

func (p *payloadFailStub) failEvent(eventID string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failByID[eventID] = err
}

func (p *payloadFailStub) Push(_ context.Context, _ string, payloads []string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, err := range p.failByID {
		for _, pl := range payloads {
			if strings.Contains(pl, `"event_id":"`+id+`"`) {
				return err
			}
		}
	}
	for _, pl := range payloads {
		id := eventIDOf(pl)
		p.deliv[id] = true
		p.order = append(p.order, id)
	}
	return nil
}

func (p *payloadFailStub) delivered(eventID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.deliv[eventID]
}

func (p *payloadFailStub) deliveredIDs() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.order...)
}

// eventIDOf extracts the event_id from the test payload `{"event_id":"eN"}`.
func eventIDOf(payload string) string {
	const key = `"event_id":"`
	i := strings.Index(payload, key)
	if i < 0 {
		return ""
	}
	rest := payload[i+len(key):]
	j := strings.Index(rest, `"`)
	if j < 0 {
		return ""
	}
	return rest[:j]
}

// chunkStub records the size of each Push batch so chunking tests can assert the
// split shape.
type chunkStub struct {
	mu        sync.Mutex
	sizes     map[string][]int
	delivered map[string]int
}

func newChunkStub() *chunkStub {
	return &chunkStub{sizes: map[string][]int{}, delivered: map[string]int{}}
}

func (c *chunkStub) Push(_ context.Context, peerURL string, payloads []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sizes[peerURL] = append(c.sizes[peerURL], len(payloads))
	c.delivered[peerURL] += len(payloads)
	return nil
}

func (c *chunkStub) batchSizes(peerURL string) []int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]int(nil), c.sizes[peerURL]...)
}

func (c *chunkStub) deliveredCount(peerURL string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.delivered[peerURL]
}

// ensure the store reference is used (silence unused import in some build orders).
var _ = store.OutboxEvent{}

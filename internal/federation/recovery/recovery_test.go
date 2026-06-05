package recovery

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/federation/inbox"
	"github.com/lebe-dev/turboist/internal/federation/store"
	"github.com/lebe-dev/turboist/internal/model"
)

// stubTargets returns a fixed set of pull targets and records how many times it
// was scanned (so a ctx-cancel test can assert the loop stopped).
type stubTargets struct {
	mu      sync.Mutex
	targets []store.PullTarget
	scans   int
}

func (s *stubTargets) ListPullTargets(_ context.Context) ([]store.PullTarget, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scans++
	return s.targets, nil
}

func (s *stubTargets) scanCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scans
}

// stubPuller serves a pull response per (peerURL,sinceHLC) call, recording the
// since_hlc cursors it was asked for and any forced error.
type stubPuller struct {
	mu        sync.Mutex
	responses []*events.PullResponse
	err       error
	sinceSeen []string
	calls     int
}

func (p *stubPuller) Pull(_ context.Context, _ /*peerURL*/, _ /*remoteProjectID*/, sinceHLC string, _ int) (*events.PullResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sinceSeen = append(p.sinceSeen, sinceHLC)
	if p.err != nil {
		return nil, p.err
	}
	idx := p.calls
	p.calls++
	if idx >= len(p.responses) {
		return &events.PullResponse{NextHLC: sinceHLC}, nil
	}
	return p.responses[idx], nil
}

// recordingSink records every event handed to it (dedup-aware) and every cursor
// advance, so a test can assert events were durably recorded + enqueued and the
// cursor advanced only once per batch.
type recordingSink struct {
	mu         sync.Mutex
	recorded   []string // event ids passed to Record (deduped: a repeat returns inserted=false)
	enqueued   []string // event ids handed to Enqueue (only newly-inserted)
	advances   []string // cursors advanced to
	touched    []string // peer urls TouchContact was called for (F5.6a pull touchpoint)
	seen       map[string]bool
	recordErr  error
	advanceErr error
}

func newRecordingSink() *recordingSink {
	return &recordingSink{seen: map[string]bool{}}
}

func (s *recordingSink) Record(_ context.Context, e events.Event, _ /*peerURL*/ string, _ /*localProjectID*/ int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.recordErr != nil {
		return false, s.recordErr
	}
	s.recorded = append(s.recorded, e.EventID)
	if s.seen[e.EventID] {
		return false, nil // duplicate (NFR-2 dedup) — not re-enqueued
	}
	s.seen[e.EventID] = true
	return true, nil
}

func (s *recordingSink) Enqueue(job inbox.Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enqueued = append(s.enqueued, job.Event.EventID)
}

func (s *recordingSink) AdvanceCursor(_ context.Context, _ /*localProjectID*/ int64, _ /*peerURL*/, toHLC string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.advanceErr != nil {
		return s.advanceErr
	}
	s.advances = append(s.advances, toHLC)
	return nil
}

func (s *recordingSink) TouchContact(_ context.Context, peerURL string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.touched = append(s.touched, peerURL)
	return nil
}

func (s *recordingSink) snapshot() ([]string, []string, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.recorded...),
		append([]string(nil), s.enqueued...),
		append([]string(nil), s.advances...)
}

func (s *recordingSink) touchedPeers() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.touched...)
}

// stubValidator stands in for the F3.2a per-event validator on the pull path. By
// default every event passes and resolves to passProjectID, so the happy-path
// tests exercise the record/enqueue/advance logic; reject lets a test fail a
// specific event_id with a chosen sentinel (mirroring a forged / wrong-author /
// not-permitted event) so the loop's "rejected event → zero rows, no cursor
// advance" guarantee is asserted.
type stubValidator struct {
	mu            sync.Mutex
	passProjectID int64
	reject        map[string]error // event_id → sentinel; absent → pass
}

func newStubValidator(projectID int64) *stubValidator {
	return &stubValidator{passProjectID: projectID, reject: map[string]error{}}
}

func (v *stubValidator) Validate(_ context.Context, e events.Event, _ string) (*inbox.ValidationResult, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if err, bad := v.reject[e.EventID]; bad {
		return nil, err
	}
	return &inbox.ValidationResult{LocalProjectID: v.passProjectID, Permissions: model.FederationPermissionWrite}, nil
}

func evt(id, hlc string) events.Event {
	return events.Event{
		EventID:         id,
		Op:              events.OpUpdate,
		EntityType:      events.EntityTask,
		EntityID:        "task-1",
		ProjectClientID: "proj-client-1",
		Fields:          map[string]events.Field{"title": {Value: "x", HLC: hlc}},
	}
}

// stubKeyMismatch captures every MarkKeyMismatchByRemote call the loop makes so a
// test can assert symmetric key-rotation detection on the PULL leg (Federation v1
// F4.3/F5.6b, US-4.3 AC4 / US-6.4 AC2) — the pull mirror of the push handler's
// MarkKeyMismatchByRemote. err lets a test force a marker failure to prove the
// stamp is best-effort and never changes the rejection.
type stubKeyMismatch struct {
	mu    sync.Mutex
	calls []keyMismatchCall
	err   error
}

type keyMismatchCall struct {
	peerURL         string
	projectClientID string
}

func (m *stubKeyMismatch) MarkKeyMismatchByRemote(_ context.Context, peerURL, projectClientID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, keyMismatchCall{peerURL, projectClientID})
	return m.err
}

func (m *stubKeyMismatch) snapshot() []keyMismatchCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]keyMismatchCall(nil), m.calls...)
}

// TestRecovery_PullAppliesAndAdvancesCursor asserts a single pull pass records +
// enqueues each returned event and advances the (peer,project) cursor to the
// response's next_hlc (US-4.1 AC2 — pull applies + advances cursor).
func TestRecovery_PullAppliesAndAdvancesCursor(t *testing.T) {
	targets := &stubTargets{targets: []store.PullTarget{{
		LocalProjectID: 7, PeerInstanceURL: "https://owner.example", RemoteProjectID: "remote-abc", LastReceivedHLC: "00000000010000-0000-nodeO",
	}}}
	puller := &stubPuller{responses: []*events.PullResponse{{
		Events:  []events.Event{evt("e1", "00000000020000-0000-nodeO"), evt("e2", "00000000030000-0000-nodeO")},
		NextHLC: "00000000030000-0000-nodeO",
	}}}
	sink := newRecordingSink()

	loop := NewLoop(targets, puller, sink, nil).WithValidator(newStubValidator(7)).WithBatchLimit(500)
	if err := loop.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}

	rec, enq, adv := sink.snapshot()
	if len(rec) != 2 || rec[0] != "e1" || rec[1] != "e2" {
		t.Errorf("recorded: got %v, want [e1 e2]", rec)
	}
	if len(enq) != 2 || enq[0] != "e1" || enq[1] != "e2" {
		t.Errorf("enqueued: got %v, want [e1 e2]", enq)
	}
	if len(adv) != 1 || adv[0] != "00000000030000-0000-nodeO" {
		t.Errorf("cursor advances: got %v, want one advance to next_hlc", adv)
	}
	if len(puller.sinceSeen) != 1 || puller.sinceSeen[0] != "00000000010000-0000-nodeO" {
		t.Errorf("pulled since: got %v, want the target's last_received_hlc", puller.sinceSeen)
	}
}

// TestRecovery_DuplicateNotReEnqueued asserts an event already recorded (a push +
// pull overlap, NFR-2 dedup) is recorded but NOT re-enqueued for apply.
func TestRecovery_DuplicateNotReEnqueued(t *testing.T) {
	targets := &stubTargets{targets: []store.PullTarget{{LocalProjectID: 1, PeerInstanceURL: "https://o.example", RemoteProjectID: "r", LastReceivedHLC: ""}}}
	dup := evt("dup", "00000000020000-0000-nodeO")
	puller := &stubPuller{responses: []*events.PullResponse{
		{Events: []events.Event{dup}, NextHLC: "00000000020000-0000-nodeO"},
		{Events: []events.Event{dup}, NextHLC: "00000000020000-0000-nodeO"},
	}}
	sink := newRecordingSink()
	loop := NewLoop(targets, puller, sink, nil).WithValidator(newStubValidator(1))

	if err := loop.RunOnce(context.Background()); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if err := loop.RunOnce(context.Background()); err != nil {
		t.Fatalf("second run: %v", err)
	}
	_, enq, _ := sink.snapshot()
	if len(enq) != 1 || enq[0] != "dup" {
		t.Errorf("enqueued: got %v, want only one (duplicate deduped)", enq)
	}
}

// TestRecovery_PullTouchesContactOnSuccess asserts a successful pull refreshes the
// peer's last_contact_at (Federation v1 F5.6a, US-6.5 AC1/AC3 — the pull
// touchpoint that clears a joiner's owner-offline flag when the owner returns).
func TestRecovery_PullTouchesContactOnSuccess(t *testing.T) {
	targets := &stubTargets{targets: []store.PullTarget{{LocalProjectID: 7, PeerInstanceURL: "https://owner.example", RemoteProjectID: "r", LastReceivedHLC: "00000000010000-0000-nodeO"}}}
	puller := &stubPuller{responses: []*events.PullResponse{{
		Events:  []events.Event{evt("e1", "00000000020000-0000-nodeO")},
		NextHLC: "00000000020000-0000-nodeO",
	}}}
	sink := newRecordingSink()
	loop := NewLoop(targets, puller, sink, nil).WithValidator(newStubValidator(7))

	if err := loop.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}
	touched := sink.touchedPeers()
	if len(touched) != 1 || touched[0] != "https://owner.example" {
		t.Errorf("touched peers: got %v, want [https://owner.example]", touched)
	}
}

// TestRecovery_PullTouchesContactWhenCaughtUp asserts an EMPTY (caught-up) pull
// still refreshes last_contact_at — reachability, not new events, is what clears
// the owner-offline flag (Federation v1 F5.6a, US-6.5 AC3).
func TestRecovery_PullTouchesContactWhenCaughtUp(t *testing.T) {
	targets := &stubTargets{targets: []store.PullTarget{{LocalProjectID: 7, PeerInstanceURL: "https://owner.example", RemoteProjectID: "r", LastReceivedHLC: "00000000010000-0000-nodeO"}}}
	// No responses → stubPuller returns an empty caught-up PullResponse.
	puller := &stubPuller{}
	sink := newRecordingSink()
	loop := NewLoop(targets, puller, sink, nil).WithValidator(newStubValidator(7))

	if err := loop.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}
	if touched := sink.touchedPeers(); len(touched) != 1 || touched[0] != "https://owner.example" {
		t.Errorf("touched on caught-up pull: got %v, want [https://owner.example]", touched)
	}
	// A caught-up pull records/enqueues/advances nothing.
	if rec, enq, adv := sink.snapshot(); len(rec) != 0 || len(enq) != 0 || len(adv) != 0 {
		t.Errorf("caught-up pull should not record/enqueue/advance: rec=%v enq=%v adv=%v", rec, enq, adv)
	}
}

// TestRecovery_PullErrorDoesNotTouchContact asserts a failed pull (peer
// unreachable, NOT a 410) does NOT refresh last_contact_at — an unreachable owner
// must stay flagged offline (Federation v1 F5.6a, US-6.5 AC1).
func TestRecovery_PullErrorDoesNotTouchContact(t *testing.T) {
	targets := &stubTargets{targets: []store.PullTarget{{LocalProjectID: 7, PeerInstanceURL: "https://owner.example", RemoteProjectID: "r", LastReceivedHLC: "00000000010000-0000-nodeO"}}}
	puller := &stubPuller{err: errors.New("peer unreachable")}
	sink := newRecordingSink()
	loop := NewLoop(targets, puller, sink, nil).WithValidator(newStubValidator(7))

	if err := loop.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}
	if touched := sink.touchedPeers(); len(touched) != 0 {
		t.Errorf("touched on pull error: got %v, want none (unreachable owner stays offline)", touched)
	}
}

// TestRecovery_PullErrorDoesNotAdvanceCursor asserts a failed pull (peer
// unreachable) leaves the cursor where it was, so the next pass retries the same
// range — no loss (US-4.1 AC1 — resumes after gap, no loss).
func TestRecovery_PullErrorDoesNotAdvanceCursor(t *testing.T) {
	targets := &stubTargets{targets: []store.PullTarget{{LocalProjectID: 1, PeerInstanceURL: "https://o.example", RemoteProjectID: "r", LastReceivedHLC: "00000000010000-0000-nodeO"}}}
	puller := &stubPuller{err: errors.New("peer unreachable")}
	sink := newRecordingSink()
	loop := NewLoop(targets, puller, sink, nil).WithValidator(newStubValidator(1))

	if err := loop.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once (peer error is isolated, not a loop error): %v", err)
	}
	_, _, adv := sink.snapshot()
	if len(adv) != 0 {
		t.Errorf("cursor advances on pull error: got %v, want none", adv)
	}
}

// TestRecovery_RecordErrorDoesNotAdvanceCursor asserts a durable-record failure
// (DB busy) does NOT advance the cursor, so the unrecorded events are re-pulled
// next pass (partial-apply must not advance cursor — the F4.1 risk).
func TestRecovery_RecordErrorDoesNotAdvanceCursor(t *testing.T) {
	targets := &stubTargets{targets: []store.PullTarget{{LocalProjectID: 1, PeerInstanceURL: "https://o.example", RemoteProjectID: "r", LastReceivedHLC: "00000000010000-0000-nodeO"}}}
	puller := &stubPuller{responses: []*events.PullResponse{{
		Events:  []events.Event{evt("e1", "00000000020000-0000-nodeO")},
		NextHLC: "00000000020000-0000-nodeO",
	}}}
	sink := newRecordingSink()
	sink.recordErr = errors.New("db busy")
	loop := NewLoop(targets, puller, sink, nil).WithValidator(newStubValidator(1))

	if err := loop.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}
	_, _, adv := sink.snapshot()
	if len(adv) != 0 {
		t.Errorf("cursor advanced despite record failure: got %v, want none", adv)
	}
}

// TestRecovery_EmptyResponseDoesNotAdvance asserts a pull that returns no new
// events does not spuriously advance the cursor (NextHLC == since means caught
// up).
func TestRecovery_EmptyResponseDoesNotAdvance(t *testing.T) {
	targets := &stubTargets{targets: []store.PullTarget{{LocalProjectID: 1, PeerInstanceURL: "https://o.example", RemoteProjectID: "r", LastReceivedHLC: "00000000010000-0000-nodeO"}}}
	puller := &stubPuller{responses: []*events.PullResponse{{Events: nil, NextHLC: "00000000010000-0000-nodeO"}}}
	sink := newRecordingSink()
	loop := NewLoop(targets, puller, sink, nil).WithValidator(newStubValidator(1))

	if err := loop.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}
	_, enq, adv := sink.snapshot()
	if len(enq) != 0 {
		t.Errorf("enqueued on empty response: got %v, want none", enq)
	}
	if len(adv) != 0 {
		t.Errorf("advanced on empty response: got %v, want none (already caught up)", adv)
	}
}

// TestRecovery_StartCancelsOnContext asserts the recovery goroutine starts (one
// immediate pass) and stops when its context is cancelled (US-4.1 — recovery
// cancels on ctx).
func TestRecovery_StartCancelsOnContext(t *testing.T) {
	targets := &stubTargets{}
	puller := &stubPuller{}
	sink := newRecordingSink()
	loop := NewLoop(targets, puller, sink, nil).WithValidator(newStubValidator(1)).WithInterval(5 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	loop.Start(ctx)

	// Wait for at least the startup scan.
	deadline := time.Now().Add(time.Second)
	for targets.scanCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if targets.scanCount() == 0 {
		t.Fatal("recovery loop did not run a startup scan")
	}

	cancel()
	loop.Stop()
	after := targets.scanCount()
	time.Sleep(20 * time.Millisecond)
	if targets.scanCount() != after {
		t.Errorf("loop kept scanning after cancel: %d then %d", after, targets.scanCount())
	}
}

// TestRecovery_ForgedEventRejectedNoRows asserts a pulled event that fails the
// F3.2a per-event validator (a forged / tampered signature relayed by an
// otherwise-trusted owner) is NOT recorded, NOT enqueued, and the cursor is NOT
// advanced — the pull-path mirror of TestEvents_BadSignatureRejectedNoRows
// (US-7.2 AC1, R22/§404). The transport response signature authenticates only the
// relaying peer, so a third-party-authored event must still be re-authenticated.
func TestRecovery_ForgedEventRejectedNoRows(t *testing.T) {
	targets := &stubTargets{targets: []store.PullTarget{{
		LocalProjectID: 7, PeerInstanceURL: "https://owner.example", RemoteProjectID: "r", LastReceivedHLC: "00000000010000-0000-nodeO",
	}}}
	puller := &stubPuller{responses: []*events.PullResponse{{
		Events:  []events.Event{evt("forged", "00000000020000-0000-nodeO")},
		NextHLC: "00000000020000-0000-nodeO",
	}}}
	sink := newRecordingSink()
	validator := newStubValidator(7)
	validator.reject["forged"] = inbox.ErrEventSignatureInvalid
	loop := NewLoop(targets, puller, sink, nil).WithValidator(validator)

	if err := loop.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once (a rejected event is isolated, not a loop error): %v", err)
	}
	rec, enq, adv := sink.snapshot()
	if len(rec) != 0 {
		t.Errorf("forged event must write no inbox rows: got %v", rec)
	}
	if len(enq) != 0 {
		t.Errorf("forged event must not enqueue: got %v", enq)
	}
	if len(adv) != 0 {
		t.Errorf("forged event must not advance the cursor: got %v", adv)
	}
}

// TestRecovery_WrongAuthorRejectedNoRows asserts a pulled event whose author !=
// origin_instance is rejected with zero rows / no cursor advance on the pull path
// too (US-7.2 AC3, the pull mirror of TestEvents_AuthorOriginMismatch400).
func TestRecovery_WrongAuthorRejectedNoRows(t *testing.T) {
	targets := &stubTargets{targets: []store.PullTarget{{
		LocalProjectID: 7, PeerInstanceURL: "https://owner.example", RemoteProjectID: "r", LastReceivedHLC: "00000000010000-0000-nodeO",
	}}}
	puller := &stubPuller{responses: []*events.PullResponse{{
		Events:  []events.Event{evt("wrong-author", "00000000020000-0000-nodeO")},
		NextHLC: "00000000020000-0000-nodeO",
	}}}
	sink := newRecordingSink()
	validator := newStubValidator(7)
	validator.reject["wrong-author"] = inbox.ErrAuthorOriginMismatch
	loop := NewLoop(targets, puller, sink, nil).WithValidator(validator)

	if err := loop.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}
	rec, enq, adv := sink.snapshot()
	if len(rec) != 0 || len(enq) != 0 || len(adv) != 0 {
		t.Errorf("wrong-author event must write/enqueue/advance nothing: rec=%v enq=%v adv=%v", rec, enq, adv)
	}
}

// TestRecovery_NotPermittedPeerRejectedNoRows asserts a pulled event for a project
// the relaying peer has no write relationship with (read-only / revoked / not a
// member) is rejected with zero rows — the pull-path enforcement of the F5.2/
// US-5.1 "read-only peer write → not applied" invariant the push transport
// already asserts (the high finding). Membership/permission is enforced via the
// same validator on the pull path.
func TestRecovery_NotPermittedPeerRejectedNoRows(t *testing.T) {
	targets := &stubTargets{targets: []store.PullTarget{{
		LocalProjectID: 7, PeerInstanceURL: "https://owner.example", RemoteProjectID: "r", LastReceivedHLC: "00000000010000-0000-nodeO",
	}}}
	puller := &stubPuller{responses: []*events.PullResponse{{
		Events:  []events.Event{evt("not-permitted", "00000000020000-0000-nodeO")},
		NextHLC: "00000000020000-0000-nodeO",
	}}}
	sink := newRecordingSink()
	validator := newStubValidator(7)
	validator.reject["not-permitted"] = inbox.ErrPeerNotPermitted
	loop := NewLoop(targets, puller, sink, nil).WithValidator(validator)

	if err := loop.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}
	rec, enq, adv := sink.snapshot()
	if len(rec) != 0 || len(enq) != 0 || len(adv) != 0 {
		t.Errorf("not-permitted event must write/enqueue/advance nothing: rec=%v enq=%v adv=%v", rec, enq, adv)
	}
}

// TestRecovery_RejectedEventStopsBatchBeforeLaterEvents asserts that when an
// earlier event in a pulled batch fails validation, the batch STOPS: the rejected
// event and every event behind it are neither recorded nor enqueued, and the
// cursor is not advanced (it would skip past the rejected event). The next pass
// re-pulls the same range — no valid event behind a rejected one is silently lost
// or admitted out of order.
func TestRecovery_RejectedEventStopsBatchBeforeLaterEvents(t *testing.T) {
	targets := &stubTargets{targets: []store.PullTarget{{
		LocalProjectID: 7, PeerInstanceURL: "https://owner.example", RemoteProjectID: "r", LastReceivedHLC: "00000000010000-0000-nodeO",
	}}}
	puller := &stubPuller{responses: []*events.PullResponse{{
		Events: []events.Event{
			evt("forged", "00000000020000-0000-nodeO"),
			evt("legit", "00000000030000-0000-nodeO"),
		},
		NextHLC: "00000000030000-0000-nodeO",
	}}}
	sink := newRecordingSink()
	validator := newStubValidator(7)
	validator.reject["forged"] = inbox.ErrEventSignatureInvalid
	loop := NewLoop(targets, puller, sink, nil).WithValidator(validator)

	if err := loop.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}
	rec, enq, adv := sink.snapshot()
	if len(rec) != 0 || len(enq) != 0 || len(adv) != 0 {
		t.Errorf("a rejected leading event stops the batch: rec=%v enq=%v adv=%v", rec, enq, adv)
	}
}

// TestRecovery_RecordsAgainstValidatedProjectID asserts the event is recorded
// against the membership-checked LOCAL project id the validator resolves, not
// whatever the pull target or event payload claims (the high finding: the pull
// path must not apply an event into a different local federated project).
func TestRecovery_RecordsAgainstValidatedProjectID(t *testing.T) {
	// The pull target row says local project 7, but the validator's membership
	// check resolves the (peer, project) to local project 42 — Record must receive 42.
	targets := &stubTargets{targets: []store.PullTarget{{
		LocalProjectID: 7, PeerInstanceURL: "https://owner.example", RemoteProjectID: "r", LastReceivedHLC: "00000000010000-0000-nodeO",
	}}}
	puller := &stubPuller{responses: []*events.PullResponse{{
		Events:  []events.Event{evt("e1", "00000000020000-0000-nodeO")},
		NextHLC: "00000000020000-0000-nodeO",
	}}}
	sink := &projectCapturingSink{recordingSink: newRecordingSink()}
	loop := NewLoop(targets, puller, sink, nil).WithValidator(newStubValidator(42))

	if err := loop.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}
	if sink.recordedProjectID != 42 {
		t.Errorf("recorded project id: got %d, want 42 (the membership-checked id)", sink.recordedProjectID)
	}
}

// stalePuller fails a pull with a typed stale-pull error (the 410 the F3.3 emit
// half returns) on its first call, then serves an empty caught-up response — the
// shape of a peer that has been re-bootstrapped and is now caught up.
type stalePuller struct {
	mu      sync.Mutex
	calls   int
	staleAt int // index (0-based) at which to return the stale error
	err     error
}

func (p *stalePuller) Pull(_ context.Context, _ /*peerURL*/, _ /*remoteProjectID*/, sinceHLC string, _ int) (*events.PullResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	idx := p.calls
	p.calls++
	if idx == p.staleAt {
		return nil, p.err
	}
	return &events.PullResponse{NextHLC: sinceHLC}, nil
}

// recordingStaleConsumer records the (peer, project, snapshot_url, as_of_hlc) of
// every stale-pull consume the loop drives, so a test can assert the 410 was
// CONSUMED (re-bootstrap triggered) rather than silently swallowed.
type recordingStaleConsumer struct {
	mu    sync.Mutex
	calls []staleCall
	err   error
}

type staleCall struct {
	localProjectID int64
	peerURL        string
	snapshotURL    string
	asOfHLC        string
}

func (c *recordingStaleConsumer) ConsumeStalePull(_ context.Context, localProjectID int64, peerURL, snapshotURL, asOfHLC string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, staleCall{localProjectID, peerURL, snapshotURL, asOfHLC})
	return c.err
}

func (c *recordingStaleConsumer) snapshot() []staleCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]staleCall(nil), c.calls...)
}

// TestRecovery_StalePullTriggersReBootstrap asserts the F4.2 consume half
// (US-4.2 AC1→consume): a pull that returns 410 stale_pull with {snapshot_url,
// as_of_hlc} drives the StaleConsumer (re-bootstrap), and the cursor is NOT
// advanced by the failed pull (the re-bootstrap itself sets last_received_hlc).
func TestRecovery_StalePullTriggersReBootstrap(t *testing.T) {
	targets := &stubTargets{targets: []store.PullTarget{{
		LocalProjectID: 7, PeerInstanceURL: "https://owner.example", RemoteProjectID: "r", LastReceivedHLC: "00000000000100-0000-nodeO",
	}}}
	puller := &stalePuller{staleAt: 0, err: &events.StalePullError{
		SnapshotURL: "https://owner.example/federation/projects/9/snapshot",
		AsOfHLC:     "00000000050000-0000-nodeO",
	}}
	sink := newRecordingSink()
	consumer := &recordingStaleConsumer{}
	loop := NewLoop(targets, puller, sink, nil).
		WithValidator(newStubValidator(7)).
		WithStaleConsumer(consumer)

	if err := loop.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}

	calls := consumer.snapshot()
	if len(calls) != 1 {
		t.Fatalf("stale consumer calls: got %d, want 1 (410 must be CONSUMED, not swallowed)", len(calls))
	}
	got := calls[0]
	if got.localProjectID != 7 || got.peerURL != "https://owner.example" {
		t.Errorf("consume target: got (%d, %q), want (7, owner.example)", got.localProjectID, got.peerURL)
	}
	if got.snapshotURL != "https://owner.example/federation/projects/9/snapshot" {
		t.Errorf("consume snapshot_url: got %q", got.snapshotURL)
	}
	if got.asOfHLC != "00000000050000-0000-nodeO" {
		t.Errorf("consume as_of_hlc: got %q", got.asOfHLC)
	}
	// The failed pull never advances the cursor (re-bootstrap owns that).
	if _, _, adv := sink.snapshot(); len(adv) != 0 {
		t.Errorf("stale pull advanced the cursor: got %v, want none", adv)
	}
}

// TestRecovery_StalePullWithoutConsumerDoesNotPanic asserts a loop with no stale
// consumer wired (a F4.1-only build) treats a 410 like any other pull error:
// isolated, logged, no cursor advance — it must not panic or advance.
func TestRecovery_StalePullWithoutConsumerDoesNotPanic(t *testing.T) {
	targets := &stubTargets{targets: []store.PullTarget{{
		LocalProjectID: 1, PeerInstanceURL: "https://o.example", RemoteProjectID: "r", LastReceivedHLC: "00000000000100-0000-nodeO",
	}}}
	puller := &stalePuller{staleAt: 0, err: &events.StalePullError{SnapshotURL: "https://o.example/s", AsOfHLC: "00000000050000-0000-nodeO"}}
	sink := newRecordingSink()
	loop := NewLoop(targets, puller, sink, nil).WithValidator(newStubValidator(1)) // no WithStaleConsumer

	if err := loop.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}
	if _, _, adv := sink.snapshot(); len(adv) != 0 {
		t.Errorf("stale pull advanced the cursor without a consumer: got %v, want none", adv)
	}
}

// projectCapturingSink captures the localProjectID Record was called with.
type projectCapturingSink struct {
	*recordingSink
	recordedProjectID int64
}

func (s *projectCapturingSink) Record(ctx context.Context, e events.Event, peerURL string, localProjectID int64) (bool, error) {
	s.recordedProjectID = localProjectID
	return s.recordingSink.Record(ctx, e, peerURL, localProjectID)
}

// TestRecovery_SignatureInvalidStampsKeyMismatch asserts symmetric key-rotation
// detection on the PULL leg (Federation v1 F4.3/F5.6b, US-4.3 AC4 / US-6.4 AC2):
// a pulled event whose per-event signature was verified against the pinned key and
// REJECTED (ErrEventSignatureInvalid — genuine proof the relaying author rotated
// its key) stamps the SAME sticky key-mismatch marker the push handler does
// (MarkKeyMismatchByRemote with the peer + the event's project_client_id), so a
// rotation first observed via pull raises the incident banner + red badge instead
// of failing silently. The event is STILL rejected with zero rows / no cursor
// advance — the marker is a side signal, not a relaxation of the rejection.
func TestRecovery_SignatureInvalidStampsKeyMismatch(t *testing.T) {
	targets := &stubTargets{targets: []store.PullTarget{{
		LocalProjectID: 7, PeerInstanceURL: "https://owner.example", RemoteProjectID: "r", LastReceivedHLC: "00000000010000-0000-nodeO",
	}}}
	puller := &stubPuller{responses: []*events.PullResponse{{
		Events:  []events.Event{evt("rotated", "00000000020000-0000-nodeO")},
		NextHLC: "00000000020000-0000-nodeO",
	}}}
	sink := newRecordingSink()
	validator := newStubValidator(7)
	validator.reject["rotated"] = inbox.ErrEventSignatureInvalid
	marker := &stubKeyMismatch{}
	loop := NewLoop(targets, puller, sink, nil).WithValidator(validator).WithKeyMismatch(marker)

	if err := loop.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once (a rejected event is isolated, not a loop error): %v", err)
	}

	calls := marker.snapshot()
	if len(calls) != 1 {
		t.Fatalf("key-mismatch marks: got %d, want 1 (a verified-rejected sig is a rotation, detected via pull too)", len(calls))
	}
	if calls[0].peerURL != "https://owner.example" || calls[0].projectClientID != "proj-client-1" {
		t.Errorf("marked (%q, %q), want (https://owner.example, proj-client-1)", calls[0].peerURL, calls[0].projectClientID)
	}
	// The rejection is unchanged: zero rows, no enqueue, no cursor advance.
	if rec, enq, adv := sink.snapshot(); len(rec) != 0 || len(enq) != 0 || len(adv) != 0 {
		t.Errorf("a rotated-key event must still write/enqueue/advance nothing: rec=%v enq=%v adv=%v", rec, enq, adv)
	}
}

// TestRecovery_KeyUnresolvedDoesNotStampKeyMismatch asserts a TRANSIENT author-key
// resolution failure on the pull leg (ErrEventKeyUnresolved — a .well-known fetch
// error, NOT a rotation) does NOT stamp the sticky marker, mirroring the push
// handler / mapEventValidationError (Federation v1 F4.3 review fix). The batch just
// retries next pass; only a verified-and-rejected signature is a rotation.
func TestRecovery_KeyUnresolvedDoesNotStampKeyMismatch(t *testing.T) {
	targets := &stubTargets{targets: []store.PullTarget{{
		LocalProjectID: 7, PeerInstanceURL: "https://owner.example", RemoteProjectID: "r", LastReceivedHLC: "00000000010000-0000-nodeO",
	}}}
	puller := &stubPuller{responses: []*events.PullResponse{{
		Events:  []events.Event{evt("transient", "00000000020000-0000-nodeO")},
		NextHLC: "00000000020000-0000-nodeO",
	}}}
	sink := newRecordingSink()
	validator := newStubValidator(7)
	validator.reject["transient"] = inbox.ErrEventKeyUnresolved
	marker := &stubKeyMismatch{}
	loop := NewLoop(targets, puller, sink, nil).WithValidator(validator).WithKeyMismatch(marker)

	if err := loop.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}
	if calls := marker.snapshot(); len(calls) != 0 {
		t.Errorf("a transient key-resolution failure must NOT stamp the sticky marker: got %v", calls)
	}
}

// TestRecovery_NonSignatureRejectDoesNotStampKeyMismatch asserts a non-signature
// validation failure (author/origin mismatch — not a key rotation) does NOT stamp
// the key-mismatch marker on the pull leg: only ErrEventSignatureInvalid is a
// rotation signal, matching the push handler.
func TestRecovery_NonSignatureRejectDoesNotStampKeyMismatch(t *testing.T) {
	targets := &stubTargets{targets: []store.PullTarget{{
		LocalProjectID: 7, PeerInstanceURL: "https://owner.example", RemoteProjectID: "r", LastReceivedHLC: "00000000010000-0000-nodeO",
	}}}
	puller := &stubPuller{responses: []*events.PullResponse{{
		Events:  []events.Event{evt("wrong-author", "00000000020000-0000-nodeO")},
		NextHLC: "00000000020000-0000-nodeO",
	}}}
	sink := newRecordingSink()
	validator := newStubValidator(7)
	validator.reject["wrong-author"] = inbox.ErrAuthorOriginMismatch
	marker := &stubKeyMismatch{}
	loop := NewLoop(targets, puller, sink, nil).WithValidator(validator).WithKeyMismatch(marker)

	if err := loop.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}
	if calls := marker.snapshot(); len(calls) != 0 {
		t.Errorf("an author/origin mismatch must NOT stamp the key-mismatch marker: got %v", calls)
	}
}

// TestRecovery_KeyMismatchStampIsBestEffort asserts a failure to record the
// key-mismatch marker (DB hiccup, unknown project) does NOT change the rejection:
// the rotated-key event is still rejected with zero rows / no cursor advance, the
// marker error is swallowed (logged), and RunOnce returns nil (the rejected event
// is isolated). The marker is a best-effort side signal.
func TestRecovery_KeyMismatchStampIsBestEffort(t *testing.T) {
	targets := &stubTargets{targets: []store.PullTarget{{
		LocalProjectID: 7, PeerInstanceURL: "https://owner.example", RemoteProjectID: "r", LastReceivedHLC: "00000000010000-0000-nodeO",
	}}}
	puller := &stubPuller{responses: []*events.PullResponse{{
		Events:  []events.Event{evt("rotated", "00000000020000-0000-nodeO")},
		NextHLC: "00000000020000-0000-nodeO",
	}}}
	sink := newRecordingSink()
	validator := newStubValidator(7)
	validator.reject["rotated"] = inbox.ErrEventSignatureInvalid
	marker := &stubKeyMismatch{err: errors.New("db busy")}
	loop := NewLoop(targets, puller, sink, nil).WithValidator(validator).WithKeyMismatch(marker)

	if err := loop.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: a marker failure must not surface as a loop error: %v", err)
	}
	if calls := marker.snapshot(); len(calls) != 1 {
		t.Errorf("marker still attempted once: got %d calls", len(calls))
	}
	if rec, enq, adv := sink.snapshot(); len(rec) != 0 || len(enq) != 0 || len(adv) != 0 {
		t.Errorf("rejection unchanged despite marker failure: rec=%v enq=%v adv=%v", rec, enq, adv)
	}
}

// TestRecovery_SignatureInvalidWithoutMarkerStillRejects asserts a loop with no
// key-mismatch marker wired (a build without the status surface) still rejects a
// rotated-key event with zero rows / no cursor advance — the marker is optional;
// the rejection is not.
func TestRecovery_SignatureInvalidWithoutMarkerStillRejects(t *testing.T) {
	targets := &stubTargets{targets: []store.PullTarget{{
		LocalProjectID: 7, PeerInstanceURL: "https://owner.example", RemoteProjectID: "r", LastReceivedHLC: "00000000010000-0000-nodeO",
	}}}
	puller := &stubPuller{responses: []*events.PullResponse{{
		Events:  []events.Event{evt("rotated", "00000000020000-0000-nodeO")},
		NextHLC: "00000000020000-0000-nodeO",
	}}}
	sink := newRecordingSink()
	validator := newStubValidator(7)
	validator.reject["rotated"] = inbox.ErrEventSignatureInvalid
	loop := NewLoop(targets, puller, sink, nil).WithValidator(validator) // no WithKeyMismatch

	if err := loop.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}
	if rec, enq, adv := sink.snapshot(); len(rec) != 0 || len(enq) != 0 || len(adv) != 0 {
		t.Errorf("rotated-key event rejected with no marker wired: rec=%v enq=%v adv=%v", rec, enq, adv)
	}
}

// TestRecovery_NoValidatorRecordsNothing asserts the defensive fail-closed
// behaviour: a loop with no validator wired (a misconfiguration) records NOTHING
// rather than admitting unauthenticated events — the pull path must never write
// an event it could not authenticate.
func TestRecovery_NoValidatorRecordsNothing(t *testing.T) {
	targets := &stubTargets{targets: []store.PullTarget{{
		LocalProjectID: 7, PeerInstanceURL: "https://owner.example", RemoteProjectID: "r", LastReceivedHLC: "00000000010000-0000-nodeO",
	}}}
	puller := &stubPuller{responses: []*events.PullResponse{{
		Events:  []events.Event{evt("e1", "00000000020000-0000-nodeO")},
		NextHLC: "00000000020000-0000-nodeO",
	}}}
	sink := newRecordingSink()
	loop := NewLoop(targets, puller, sink, nil) // no WithValidator

	if err := loop.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}
	rec, enq, adv := sink.snapshot()
	if len(rec) != 0 || len(enq) != 0 || len(adv) != 0 {
		t.Errorf("a loop without a validator must record/enqueue/advance nothing: rec=%v enq=%v adv=%v", rec, enq, adv)
	}
}

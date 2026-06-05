// Federation v1 F7.5 — NFR-2 crash-safety. These tests stand a full federation up
// through the F7.1 harness (the REAL signed handshake + snapshot bootstrap, the
// REAL outbox publisher / signed /federation/events endpoint / per-event validator
// / single-goroutine inbox apply, the REAL ctx-cancellable publisher worker) and
// drive the five NFR-2 crash-safety scenarios the milestone pins (plan §6 line 406):
//
//   - One-tx atomicity (TestF75_EmitRollbackNeitherDomainNorOutboxNorFieldHLC): the
//     domain write + the entity_field_hlc CAS + the federation_outbox row are
//     committed in ONE db.WithTx; if the domain write closure fails, the tx rolls
//     back and NEITHER the domain row, NOR the field HLC, NOR the outbox event
//     persists — there is never a half-applied federated mutation a crash could
//     leave behind.
//
//   - At-least-once across a crash (TestF75_ReopenDBReSendsUndelivered): an event
//     committed to federation_outbox but not yet delivered (a "kill -9" between
//     commit and the network POST, approximated here by closing + reopening the DB
//     file — see ReopenDB) survives the reopen and is RE-SENT by a fresh worker over
//     the same DB file, converging on the peer (NFR-2.1 at-least-once).
//
//   - Dedup on event_id (TestF75_DuplicateEventIDIsNoOp): at-least-once delivery
//     means the receiver WILL see the same event twice; the ON CONFLICT(event_id) DO
//     NOTHING ledger makes the redelivery a no-op — exactly one inbox row, the
//     converged value unchanged, no double-apply (NFR-2.2). Dedup is keyed on the
//     event_id, NOT on the absent idempotency_keys table (the milestone risk).
//
//   - WAL + synchronous=NORMAL (TestF75_DBRunsWALAndSynchronousNormal): the durable
//     pragma posture the crash-safety argument rests on — WAL journaling with a
//     NORMAL fsync barrier — is actually in force on a harness instance's DB
//     (regression guard; a silent drop to journal_mode=DELETE / synchronous=OFF
//     would void the at-least-once guarantee).
//
//   - Drain on ctx cancel (TestF75_OutboxDrainsOnCtxCancel): the production
//     publisher worker (outbox.Worker.Start) does a best-effort FINAL drain when its
//     context is cancelled (the cmd/turboist graceful-shutdown path), so an event
//     committed just before teardown is still delivered — not stranded until the
//     next process start (NFR-2.1).
package fedtest

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// TestF75_EmitRollbackNeitherDomainNorOutboxNorFieldHLC asserts the F3.1
// transactional-emit invariant under NFR-2: the domain write, the per-field HLC
// bump, and the federation_outbox event are all written in ONE db.WithTx, so when
// the domain write CLOSURE fails the whole tx rolls back and NONE of the three
// persists. A crash mid-emit can therefore never leave a federated entity whose
// outbox event or field HLC is out of step with the (un)written domain row.
//
// The instance owns a federated project (so emit takes the full federated path,
// not the zero-overhead non-federated branch). It then drives the REAL Emitter
// with a write closure that performs a genuine partial domain write and THEN
// returns an error — the strongest form of the test, proving the domain write the
// closure already did inside the tx is rolled back alongside the federation
// sidecar, not merely that a no-op closure wrote nothing.
func TestF75_EmitRollbackNeitherDomainNorOutboxNorFieldHLC(t *testing.T) {
	ctx := context.Background()
	h := NewHarness(t)
	alice := h.AddInstance(t, "https://alice.example")

	aliceProj := alice.CreateFederatedProject(t, ctx, "Shared")
	clientID := model.NewClientID()
	cx := int64(1)

	// Drive the production Emitter on the federated project with a write closure
	// that inserts the task row (a genuine domain write inside the emit tx) and THEN
	// fails. The emit tx must roll back domain + field_hlc + outbox together.
	wantErr := errors.New("injected domain write failure")
	err := alice.EmitTaskCreate(ctx, aliceProj, clientID, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cx, ProjectID: &aliceProj}, Title: "Doomed",
	}, func(tx *sql.Tx) error {
		// Genuine PARTIAL domain write INSIDE the emit tx, THEN fail — the strongest
		// form of the test (F7.5 review C): it proves the already-inserted task row is
		// rolled back ALONGSIDE the field-HLC CAS + outbox event, not merely that a
		// no-op closure wrote nothing. The column set mirrors the proven minimal task
		// INSERT used elsewhere so it satisfies every NOT NULL / CHECK / FK.
		now := model.FormatUTC(time.Now())
		if _, werr := tx.ExecContext(ctx,
			`INSERT INTO tasks (title, description, context_id, project_id, priority, status, day_part, plan_state, is_pinned, client_id, created_at, updated_at)
			 VALUES ('Doomed', '', ?, ?, 'no-priority', 'open', 'none', 'none', 0, ?, ?, ?)`,
			cx, aliceProj, clientID, now, now); werr != nil {
			return werr
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("emit error: got %v, want %v", err, wantErr)
	}

	// (1) NO domain row: the task the closure inserted was rolled back.
	if got := taskRowCount(alice, clientID); got != 0 {
		t.Errorf("domain task rows after rollback: got %d, want 0 (half-applied write)", got)
	}
	// (2) NO per-field HLC: the title field's CAS was rolled back.
	if got := fieldHLCRowCount(alice, string(events.EntityTask), clientID); got != 0 {
		t.Errorf("entity_field_hlc rows after rollback: got %d, want 0 (orphan field clock)", got)
	}
	// (3) NO outbox event: nothing is queued for a mutation that never committed.
	if got := outboxRowCountForProject(alice, aliceProj); got != 0 {
		t.Errorf("federation_outbox rows after rollback: got %d, want 0 (phantom event)", got)
	}
}

// TestF75_ReopenDBReSendsUndelivered approximates a kill -9 between the outbox
// commit and the network POST (the milestone's documented approximation — closing
// and reopening the DB file): Alice commits a federated edit to federation_outbox
// but it is NOT delivered before the "crash". After reopening the SAME DB file
// (the WAL-checkpointed durable state), a FRESH publisher worker over the reopened
// DB re-sends the still-undelivered event, and it converges on Bob (NFR-2.1
// at-least-once). The undelivered row is the source of truth; delivery survives the
// process restart.
func TestF75_ReopenDBReSendsUndelivered(t *testing.T) {
	ctx := context.Background()
	h := NewHarness(t)
	// Both instances opt into the no-op TRANSPORT nonce cache: the in-process
	// app.Test() transport can re-serve the SAME signed request and trip a spurious
	// federation_replay (401) that would fail the Join/push, a pure harness artifact a
	// real one-shot HTTP client never produces. This F7.5 test asserts DOMAIN
	// crash-safety (at-least-once re-send keyed on event_id, NFR-2.1/2.2), not
	// transport anti-replay — which is owned by F0.3's dedicated, single-request
	// HTTPSignatureMiddleware tests. See WithPermissiveNonces.
	alice := h.AddInstance(t, "https://alice.example", WithPermissiveNonces())
	bob := h.AddInstance(t, "https://bob.example", WithPermissiveNonces())

	aliceProj := alice.CreateFederatedProject(t, ctx, "Shared")
	taskClientID := model.NewClientID()
	cx := int64(1)
	if _, err := alice.Mutator().Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cx, ProjectID: &aliceProj}, Title: "Original",
	}, taskClientID); err != nil {
		t.Fatalf("alice create task: %v", err)
	}

	// Bob JOINS through the REAL signed handshake + snapshot (the create rode the
	// bootstrap), so the federation relationship + Bob's local project exist.
	invite := alice.CreateInvite(t, ctx, aliceProj, model.FederationPermissionWrite)
	bob.Join(t, ctx, alice.URL(), invite)
	AssertConverged(t, func() bool { return bob.TaskTitle(taskClientID) == "Original" },
		"initial snapshot did not converge onto Bob")

	// Alice commits an edit to federation_outbox — but it is NEVER pumped, modelling a
	// crash AFTER the durable commit and BEFORE the network POST.
	aliceTask := alice.TaskByClientID(t, ctx, taskClientID)
	newTitle := "Edited before crash"
	if err := alice.Mutator().Update(ctx, aliceTask, repo.TaskUpdate{Title: &newTitle}); err != nil {
		t.Fatalf("alice update task: %v", err)
	}
	// Precondition: the edit is durably queued but undelivered (nothing pumped yet).
	if got := outboxRowCountForProject(alice, aliceProj); got == 0 {
		t.Fatalf("edit was not queued to federation_outbox before the crash")
	}
	if got := bob.TaskTitle(taskClientID); got != "Original" {
		t.Fatalf("Bob already has the edit %q before any delivery — test precondition broken", got)
	}

	// "kill -9": close + reopen the SAME DB file. The reopened instance rebuilds its
	// store/publisher/worker over the durable state — the undelivered outbox row is
	// still there.
	alice.ReopenDB(t)
	if got := outboxRowCountForProject(alice, aliceProj); got == 0 {
		t.Fatalf("undelivered outbox event did not survive the DB reopen (NFR-2.1 durability lost)")
	}

	// A FRESH worker over the reopened DB re-sends the undelivered event; it converges
	// on Bob — at-least-once delivery survives the crash.
	bob.StartApply(t, ctx)
	alice.PumpOutbox(t, ctx)
	AssertConverged(t, func() bool { return bob.TaskTitle(taskClientID) == newTitle },
		"undelivered event was not re-sent after the DB reopen (NFR-2.1 at-least-once)")
}

// TestF75_DuplicateEventIDIsNoOp asserts the receiver-side dedup that makes
// at-least-once delivery safe (NFR-2.2): the SAME signed event delivered twice
// creates exactly ONE federation_inbox row (ON CONFLICT(event_id) DO NOTHING) and
// the converged value is unchanged — no double-apply. Dedup is keyed on the
// event_id carried in the signed payload, NOT on any idempotency_keys table (which
// does not exist — the milestone risk). This is the crash-safety leg of the same
// dedup F7.3 exercises through the 3-way hub; here it is the direct two-instance
// "redeliver the exact bytes" assertion.
func TestF75_DuplicateEventIDIsNoOp(t *testing.T) {
	ctx := context.Background()
	h := NewHarness(t)
	// Both instances opt into the no-op TRANSPORT nonce cache. This test asserts
	// receiver-side dedup keyed on the EVENT_ID carried in the signed payload
	// (NFR-2.2) — explicitly NOT the transport anti-replay nonce. The legitimate
	// redelivery below is re-signed with a FRESH transport nonce each time (Publisher
	// mints a new crypto/rand nonce per Push), so the real nonce cache would never
	// reject it; what it WOULD do is spuriously 401 the in-process app.Test() re-serve
	// of the Join/first-push (a harness transport artifact). Disabling the transport
	// nonce here removes that flake without weakening the event_id dedup assertion or
	// the transport anti-replay coverage owned by F0.3. See WithPermissiveNonces.
	alice := h.AddInstance(t, "https://alice.example", WithPermissiveNonces())
	bob := h.AddInstance(t, "https://bob.example", WithPermissiveNonces())

	aliceProj := alice.CreateFederatedProject(t, ctx, "Shared")
	taskClientID := model.NewClientID()
	cx := int64(1)
	if _, err := alice.Mutator().Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cx, ProjectID: &aliceProj}, Title: "Original",
	}, taskClientID); err != nil {
		t.Fatalf("alice create task: %v", err)
	}

	invite := alice.CreateInvite(t, ctx, aliceProj, model.FederationPermissionWrite)
	bob.Join(t, ctx, alice.URL(), invite)
	AssertConverged(t, func() bool { return bob.TaskTitle(taskClientID) == "Original" },
		"initial snapshot did not converge onto Bob")

	// Alice edits the task and delivers it ONCE through the real push path.
	aliceTask := alice.TaskByClientID(t, ctx, taskClientID)
	newTitle := "Edited once"
	if err := alice.Mutator().Update(ctx, aliceTask, repo.TaskUpdate{Title: &newTitle}); err != nil {
		t.Fatalf("alice update task: %v", err)
	}

	// Capture the exact signed bytes Alice enqueued for this edit (what gets
	// redelivered verbatim — the same event_id).
	payload := alice.FirstOutboxPayload(t, aliceProj)
	var ev events.Event
	if err := events.Unmarshal([]byte(payload), &ev); err != nil {
		t.Fatalf("unmarshal alice edit payload: %v", err)
	}

	bob.StartApply(t, ctx)
	alice.PumpOutbox(t, ctx)
	AssertConverged(t, func() bool { return bob.TaskTitle(taskClientID) == newTitle },
		"edit did not converge onto Bob before the redelivery probe")
	if got := inboxRowCount(bob, ev.EventID); got != 1 {
		t.Fatalf("Bob recorded event %q in %d inbox rows after the first delivery, want 1", ev.EventID, got)
	}

	// REDELIVER the exact same signed event to Bob through the REAL signed
	// /federation/events a SECOND time (at-least-once). It must be ACCEPTED (2xx —
	// RedeliverToOwner fails the test on a non-2xx) and deduped on event_id.
	alice.RedeliverToOwner(t, ctx, bob.URL(), payload)

	// Still exactly one inbox row, and the converged title is unchanged — the
	// redelivery neither double-applied nor created a second ledger row (NFR-2.2).
	unchanged := func() bool { return bob.TaskTitle(taskClientID) == newTitle }
	AssertConverged(t, unchanged, "redelivery changed Bob's converged title (idempotency violated)")
	if got := inboxRowCount(bob, ev.EventID); got != 1 {
		t.Errorf("Bob recorded event %q in %d inbox rows after redelivery, want 1 (dedup failed)", ev.EventID, got)
	}
}

// TestF75_DBRunsWALAndSynchronousNormal is the regression guard the crash-safety
// argument rests on: a harness instance's DB must actually run journal_mode=WAL
// with synchronous=NORMAL (the production db.Open posture). A silent regression to
// journal_mode=DELETE / synchronous=OFF would void the at-least-once durability the
// other F7.5 tests assume. PRAGMA values are case-insensitive on return, so the
// comparison is lower-cased.
func TestF75_DBRunsWALAndSynchronousNormal(t *testing.T) {
	h := NewHarness(t)
	alice := h.AddInstance(t, "https://alice.example")

	var journalMode string
	if err := alice.DB().QueryRow(`PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		t.Fatalf("read journal_mode: %v", err)
	}
	if lower(journalMode) != "wal" {
		t.Errorf("journal_mode: got %q, want wal (durability posture regressed)", journalMode)
	}

	// synchronous returns the integer level: 1 = NORMAL (the db.Open default).
	var synchronous int
	if err := alice.DB().QueryRow(`PRAGMA synchronous`).Scan(&synchronous); err != nil {
		t.Fatalf("read synchronous: %v", err)
	}
	if synchronous != 1 {
		t.Errorf("synchronous: got %d, want 1 (NORMAL — durability posture regressed)", synchronous)
	}
}

// TestF75_OutboxDrainsOnCtxCancel asserts the NFR-2.1 graceful-shutdown drain: the
// PRODUCTION publisher worker (outbox.Worker, wired exactly as cmd/turboist wires
// it under cleanupCtx) does a best-effort FINAL drain when its context is cancelled
// + Stop() returns, so an event committed JUST before teardown is still delivered
// rather than stranded until the next process start. Alice commits an edit, the
// worker's ctx is cancelled, and after Stop() the edit has converged onto Bob — the
// shutdown drain delivered it.
func TestF75_OutboxDrainsOnCtxCancel(t *testing.T) {
	ctx := context.Background()
	h := NewHarness(t)
	// Both instances opt into the no-op TRANSPORT nonce cache: the in-process
	// app.Test() re-serve can trip a spurious federation_replay on the Join/push that
	// would fail this test. F7.5 asserts the DOMAIN graceful-shutdown drain (NFR-2.1),
	// not transport anti-replay (owned by F0.3). See WithPermissiveNonces.
	alice := h.AddInstance(t, "https://alice.example", WithPermissiveNonces())
	bob := h.AddInstance(t, "https://bob.example", WithPermissiveNonces())

	aliceProj := alice.CreateFederatedProject(t, ctx, "Shared")
	taskClientID := model.NewClientID()
	cx := int64(1)
	if _, err := alice.Mutator().Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cx, ProjectID: &aliceProj}, Title: "Original",
	}, taskClientID); err != nil {
		t.Fatalf("alice create task: %v", err)
	}

	invite := alice.CreateInvite(t, ctx, aliceProj, model.FederationPermissionWrite)
	bob.Join(t, ctx, alice.URL(), invite)
	AssertConverged(t, func() bool { return bob.TaskTitle(taskClientID) == "Original" },
		"initial snapshot did not converge onto Bob")
	bob.StartApply(t, ctx)

	// Quiesce Alice's outbox BEFORE the worker starts. The seed create rode the Join
	// SNAPSHOT to Bob, but its federation_outbox row is still UNDELIVERED (the snapshot
	// path does not stamp outbox delivery), so the worker's startup drain would push it
	// to Bob. If the test's cancel() lands while that startup push is mid-flight, the
	// push aborts with the cancelled ctx and arms a per-peer backoff gate, which then
	// makes the FINAL drain skip Bob — stranding the edit and spuriously failing the
	// NFR-2.1 assertion. Draining now leaves the worker genuinely idle so the ONLY
	// thing the ctx-cancel final drain has to deliver is the edit committed below.
	alice.PumpOutbox(t, ctx)

	// Run Alice's PRODUCTION worker under a cancellable context with a LONG safety-net
	// tick, so the only thing that can deliver the edit committed below is the
	// ctx-cancel FINAL drain (not a tick that happened to fire). The commit-ping wires
	// nothing here — the run loop's shutdown branch is what we are asserting.
	workerCtx, cancel := context.WithCancel(ctx)
	alice.StartWorker(t, workerCtx, time.Hour)

	// Commit the edit AFTER the worker started; it sits undelivered in the outbox
	// (the next safety-net tick is an hour away).
	aliceTask := alice.TaskByClientID(t, ctx, taskClientID)
	newTitle := "Committed just before shutdown"
	if err := alice.Mutator().Update(ctx, aliceTask, repo.TaskUpdate{Title: &newTitle}); err != nil {
		t.Fatalf("alice update task: %v", err)
	}

	// Cancel the worker's ctx and block on Stop(): the run loop performs its
	// best-effort FINAL drain on the way out (cmd/turboist graceful shutdown).
	cancel()
	alice.StopWorker(t)

	// The edit committed just before shutdown converged onto Bob — the final drain
	// delivered it (NFR-2.1). Polled because Bob's apply runs off the HTTP path.
	AssertConverged(t, func() bool { return bob.TaskTitle(taskClientID) == newTitle },
		"edit committed before shutdown was not delivered by the ctx-cancel final drain (NFR-2.1)")
}

// taskRowCount counts task rows (live OR tombstoned) on an instance carrying a
// cross-instance client_id — used to assert a rolled-back emit left NO domain row
// at all (not merely no live row).
func taskRowCount(in *Instance, clientID string) int {
	var n int
	if err := in.DB().QueryRow(`SELECT COUNT(*) FROM tasks WHERE client_id = ?`, clientID).Scan(&n); err != nil {
		return -1
	}
	return n
}

// fieldHLCRowCount counts entity_field_hlc rows for an entity — used to assert a
// rolled-back emit left NO orphan per-field clock behind the (un)written row.
func fieldHLCRowCount(in *Instance, entityType, entityID string) int {
	var n int
	if err := in.DB().QueryRow(
		`SELECT COUNT(*) FROM entity_field_hlc WHERE entity_type = ? AND entity_id = ?`,
		entityType, entityID).Scan(&n); err != nil {
		return -1
	}
	return n
}

// outboxRowCountForProject counts federation_outbox rows queued under a project —
// used both to assert a rolled-back emit queued NO phantom event and to confirm an
// edit is durably queued (and survives a DB reopen) before delivery.
func outboxRowCountForProject(in *Instance, localProjectID int64) int {
	var n int
	if err := in.DB().QueryRow(
		`SELECT COUNT(*) FROM federation_outbox WHERE local_project_id = ?`, localProjectID).Scan(&n); err != nil {
		return -1
	}
	return n
}

// lower lower-cases a PRAGMA string value so the WAL/NORMAL posture assertion is
// case-insensitive (SQLite returns "wal"/"WAL" inconsistently across drivers).
func lower(s string) string { return strings.ToLower(s) }

// Federation v1 F7.3 — 3-way concurrent-edit convergence (§15.5). These tests
// stand a full owner-hub federation up through the F7.1 harness — Alice (owner)
// federated with Bob AND Carol (write peers) — with every relationship
// established by the REAL signed handshake + REAL snapshot bootstrap (no seeded
// peer rows), every edit driven through the REAL TaskMutator/Emitter (HLC bump +
// signed outbox event), and every cross-instance delivery routed through the REAL
// outbox publisher → signed POST /federation/events → per-event validator →
// single-goroutine inbox apply. Alice runs in owner-hub re-broadcast mode (W-7),
// so a peer's edit reaches the OTHER peer only by being relayed through Alice.
//
// The four §15.5 / F7.3 scenarios, each asserted after quiescence (the milestone's
// "assert after quiescence" risk) via the harness's single AssertConverged gate:
//
//   - TestConvergence_DisjointFieldsMergeOwnerHub: Bob edits title, Carol edits
//     description, BOTH before either reaches Alice (genuine concurrency). Per-field
//     LWW merges them so all three end with title=Bob AND description=Carol — no edit
//     clobbers the other (US-3.3 AC1).
//   - TestConvergence_SameFieldSameMsDeterministicAllThree: Bob and Carol both edit
//     the SAME field at the SAME physical_ms (same fixed clock). The HLC node_id
//     tie-break decides a single deterministic winner, and ALL THREE instances
//     converge to that one value — order of delivery is irrelevant (US-3.3 AC3 +
//     total-order node_id tie-break, F7.2's "heart of correctness" exercised
//     end-to-end across three nodes).
//   - TestConvergence_UpdateVsDeleteNoResurrection: Bob deletes the shared task while
//     Carol concurrently edits it; the tombstone wins per-field LWW so the stale edit
//     cannot resurrect the entity — all three converge to deleted (US-3.7 AC2 / §10.4).
//   - TestConvergence_IdempotentRedelivery: the SAME signed event delivered TWICE
//     (at-least-once redelivery) is a dedup no-op on the receiver — convergence is
//     unchanged and no duplicate inbox row applies (NFR-2.2).
package fedtest

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/federation/hlc"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// fedTriad is the established Alice(owner-hub) + Bob + Carol federation: all three
// share the project (Bob/Carol joined via the real handshake+snapshot) and the one
// seed task by its cross-instance client_id, both peers' apply queues are running,
// and Bob and Carol may originate writes (both joined with WRITE permission).
type fedTriad struct {
	alice, bob, carol *Instance
	aliceProj         int64
	taskClientID      string
}

// newOwnerHubTriad builds the Alice-owner-hub triad and converges the seed task
// onto Bob and Carol via the real snapshot bootstrap, so the three-way edit
// scenarios start from a single shared, converged task. clockAt (when non-zero)
// pins ALL THREE instances to the same fixed wall clock so an emit's HLC
// physical_ms is deterministic (the same-ms scenario needs this); a zero clockAt
// leaves every instance on the real wall clock.
func newOwnerHubTriad(t *testing.T, ctx context.Context, clockAt time.Time, seedTitle string) fedTriad {
	t.Helper()
	var aliceClock, bobClock, carolClock time.Time
	if !clockAt.IsZero() {
		aliceClock, bobClock, carolClock = clockAt, clockAt, clockAt
	}
	return newOwnerHubTriadClocks(t, ctx, seedTitle, aliceClock, bobClock, carolClock)
}

// newOwnerHubTriadClocks is the general triad builder: it pins each instance to its
// own (possibly zero = real-wall-clock) fixed clock. Distinct clocks let a test
// order concurrent edits deterministically by physical_ms (e.g. the
// update-vs-delete race makes the delete strictly newer than the stale edit), while
// keeping every clock inside the transport ±5min window of the others so the real
// signature middleware still accepts the cross-instance requests.
func newOwnerHubTriadClocks(t *testing.T, ctx context.Context, seedTitle string, aliceClock, bobClock, carolClock time.Time) fedTriad {
	t.Helper()
	h := NewHarness(t)

	// All three opt into the no-op transport nonce cache: the in-process app.Test()
	// transport can re-serve the SAME signed request and trip a spurious replay that
	// would dead-letter an event and break this multi-hop owner-hub convergence. F7.3
	// asserts DOMAIN convergence + dedup-by-event_id (NFR-2.2), not transport
	// anti-replay (owned by F0.3's dedicated tests). See WithPermissiveNonces.
	aliceOpts := []instanceOption{WithReBroadcast(), WithPermissiveNonces()}
	bobOpts := []instanceOption{WithPermissiveNonces()}
	carolOpts := []instanceOption{WithPermissiveNonces()}
	if !aliceClock.IsZero() {
		aliceOpts = append(aliceOpts, WithFixedClock(aliceClock))
	}
	if !bobClock.IsZero() {
		bobOpts = append(bobOpts, WithFixedClock(bobClock))
	}
	if !carolClock.IsZero() {
		carolOpts = append(carolOpts, WithFixedClock(carolClock))
	}
	alice := h.AddInstance(t, "https://alice.example", aliceOpts...)
	bob := h.AddInstance(t, "https://bob.example", bobOpts...)
	carol := h.AddInstance(t, "https://carol.example", carolOpts...)

	// Alice owns a federated project with one task carrying a fixed cross-instance
	// client_id, created through the production mutator (op=create + HLC stamp).
	aliceProj := alice.CreateFederatedProject(t, ctx, "Shared")
	taskClientID := model.NewClientID()
	cx := int64(1)
	if _, err := alice.Mutator().Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cx, ProjectID: &aliceProj},
		Title:     seedTitle,
	}, taskClientID); err != nil {
		t.Fatalf("alice create seed task: %v", err)
	}

	// Bob and Carol each JOIN Alice through the REAL signed handshake + snapshot
	// bootstrap (write grade — both may originate edits). The snapshot converges the
	// seed task onto each joiner under the same client_id.
	bobInvite := alice.CreateInvite(t, ctx, aliceProj, model.FederationPermissionWrite)
	bob.Join(t, ctx, alice.URL(), bobInvite)
	carolInvite := alice.CreateInvite(t, ctx, aliceProj, model.FederationPermissionWrite)
	carol.Join(t, ctx, alice.URL(), carolInvite)

	// Every node must hold the seed task by the shared client_id before any edit, or
	// a later "converged" assertion could pass against a half-bootstrapped peer.
	AssertConverged(t, func() bool {
		return bob.TaskTitle(taskClientID) == seedTitle && carol.TaskTitle(taskClientID) == seedTitle
	}, "snapshot did not converge the seed task onto Bob and Carol")

	// All three apply queues run so a pushed/relayed event is applied off the HTTP
	// path the way production runs it.
	alice.StartApply(t, ctx)
	bob.StartApply(t, ctx)
	carol.StartApply(t, ctx)

	return fedTriad{alice: alice, bob: bob, carol: carol, aliceProj: aliceProj, taskClientID: taskClientID}
}

// editTitle renames the shared task on an instance via the production TaskMutator
// (op=update {title}) so the edit emits a signed HLC-stamped outbox event.
func editTitle(t *testing.T, ctx context.Context, in *Instance, taskClientID, title string) {
	t.Helper()
	task := in.TaskByClientID(t, ctx, taskClientID)
	if err := in.Mutator().Update(ctx, task, repo.TaskUpdate{Title: &title}); err != nil {
		t.Fatalf("edit title on %s: %v", in.URL(), err)
	}
}

// editDescription edits the shared task's description on an instance via the
// production TaskMutator (op=update {description}).
func editDescription(t *testing.T, ctx context.Context, in *Instance, taskClientID, desc string) {
	t.Helper()
	task := in.TaskByClientID(t, ctx, taskClientID)
	if err := in.Mutator().Update(ctx, task, repo.TaskUpdate{Description: &desc}); err != nil {
		t.Fatalf("edit description on %s: %v", in.URL(), err)
	}
}

// deleteTask soft-deletes (federated tombstone) the shared task on an instance via
// the production TaskMutator (op=delete + synthetic _deleted HLC).
func deleteTask(t *testing.T, ctx context.Context, in *Instance, taskClientID string) {
	t.Helper()
	task := in.TaskByClientID(t, ctx, taskClientID)
	if err := in.Mutator().Delete(ctx, task); err != nil {
		t.Fatalf("delete task on %s: %v", in.URL(), err)
	}
}

// settleBudget is the wall-clock window settleHub pumps the owner-hub topology
// within. It is intentionally LARGER than the per-hop NFR-1.1 5s push budget
// (convergeBudget): convergence here requires a MULTI-HOP relay through the owner
// (peer → owner apply → owner re-broadcast → owner pump → other peer apply), and
// the apply queues run on their own goroutines, so under a contended test binary
// the full quiescence legitimately needs several pump rounds (steady state is
// reached in well under a second in practice). The per-hop 5s budget is still
// asserted independently by the F3.2/F7.1 two-instance push tests
// (TestTwoInstance_*); this larger window only governs the multi-hop settle, not a
// single push.
const settleBudget = 15 * time.Second

// settleHub drives the whole owner-hub topology to quiescence and returns as soon
// as the caller's done predicate holds (or after settleBudget elapses, leaving the
// caller's AssertConverged to report the failure). Each round it pumps every peer's
// outbox to Alice, lets the async apply queues drain (so Alice's owner-hub
// re-broadcast rows exist), then pumps Alice's outbox so the relay fans out to the
// OTHER peer. Because the apply queues run on their own goroutines, the flow is
// asynchronous; rather than rely on a fixed number of rounds (which flakes under
// load), settleHub re-pumps until done converges — PumpOutbox is idempotent
// (already-delivered rows are skipped), so extra rounds are harmless.
func settleHub(t *testing.T, ctx context.Context, tr fedTriad, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(settleBudget)
	for time.Now().Before(deadline) {
		tr.bob.PumpOutbox(t, ctx)
		tr.carol.PumpOutbox(t, ctx)
		// Let the async apply queues drain the just-pushed events so Alice's owner-hub
		// re-broadcast rows are written before her outbox is pumped to the other peer.
		time.Sleep(10 * time.Millisecond)
		tr.alice.PumpOutbox(t, ctx)
		if done() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// taskDescriptionOf reads a live (non-tombstoned) task's description by
// cross-instance client_id, or "" when the task is absent/tombstoned.
func taskDescriptionOf(in *Instance, clientID string) string {
	var desc sql.NullString
	if err := in.DB().QueryRow(
		`SELECT description FROM tasks WHERE client_id = ? AND deleted_at IS NULL`, clientID).Scan(&desc); err != nil {
		return ""
	}
	return desc.String
}

// taskTombstoned reports whether the task with clientID exists locally AND carries
// a deleted_at tombstone (a soft-delete, never a hard DELETE — §8). A row that is
// absent entirely returns false (it was never created), distinguishing "deleted"
// from "never seen".
func taskTombstoned(in *Instance, clientID string) bool {
	var deletedAt sql.NullString
	err := in.DB().QueryRow(
		`SELECT deleted_at FROM tasks WHERE client_id = ?`, clientID).Scan(&deletedAt)
	if err != nil {
		return false
	}
	return deletedAt.Valid && deletedAt.String != ""
}

// inboxRowCount returns how many federation_inbox rows on an instance carry an
// event_id — the dedup ledger's row count. The receiver's ON CONFLICT(event_id)
// DO NOTHING means an at-least-once redelivery must NOT create a second row, so
// this stays 1 across the redelivery (NFR-2.2).
func inboxRowCount(in *Instance, eventID string) int {
	var n int
	if err := in.DB().QueryRow(
		`SELECT COUNT(*) FROM federation_inbox WHERE event_id = ?`, eventID).Scan(&n); err != nil {
		return -1
	}
	return n
}

// TestConvergence_DisjointFieldsMergeOwnerHub drives genuine concurrency through
// the owner hub: Bob edits the task's TITLE and Carol edits its DESCRIPTION, BOTH
// before either edit reaches Alice. Per-field LWW merges the two disjoint fields,
// so after the hub settles ALL THREE instances carry title=Bob AND
// description=Carol — neither edit clobbers the other (US-3.3 AC1).
func TestConvergence_DisjointFieldsMergeOwnerHub(t *testing.T) {
	ctx := context.Background()
	tr := newOwnerHubTriad(t, ctx, time.Time{}, "Original")

	const bobTitle, carolDesc = "Bob's title", "Carol's description"
	// Genuine concurrency: both peers edit BEFORE either pushes to the owner hub.
	editTitle(t, ctx, tr.bob, tr.taskClientID, bobTitle)
	editDescription(t, ctx, tr.carol, tr.taskClientID, carolDesc)

	merged := func(in *Instance) bool {
		return in.TaskTitle(tr.taskClientID) == bobTitle && taskDescriptionOf(in, tr.taskClientID) == carolDesc
	}
	allMerged := func() bool { return merged(tr.alice) && merged(tr.bob) && merged(tr.carol) }

	settleHub(t, ctx, tr, allMerged)

	AssertConverged(t, allMerged,
		"disjoint-field edits did not merge on all three (title=Bob AND description=Carol)")
}

// TestConvergence_SameFieldSameMsDeterministicAllThree pins all three instances to
// the SAME fixed wall clock, then has Bob and Carol BOTH edit the SAME field
// (title) — each their first emit, so each lands at the same physical_ms and
// logical=0. Their HLCs therefore differ ONLY in node_id, and the HLC total order's
// node_id tie-break picks a single deterministic winner. The test reads each peer's
// actually-stamped HLC, computes the lexically-greater (winning) one, and asserts
// ALL THREE instances converge to THAT peer's title — proving the same-field
// same-ms outcome is deterministic and identical on every node (US-3.3 AC3, the
// F7.2 node_id tie-break exercised end-to-end through three real nodes).
func TestConvergence_SameFieldSameMsDeterministicAllThree(t *testing.T) {
	ctx := context.Background()
	// Alice seeds at base; Bob AND Carol both edit at base+1min (the SAME instant as
	// each other, but strictly AFTER the seed so their edits beat the seed's title
	// HLC regardless of node_id ordering). Their two edits then differ ONLY by
	// node_id — the genuine same-field same-ms tie-break. base+1min is inside the
	// transport ±5min window, so the real signature middleware still accepts the
	// cross-instance requests.
	base := time.Date(2030, 6, 1, 12, 0, 0, 0, time.UTC)
	peers := base.Add(1 * time.Minute)
	tr := newOwnerHubTriadClocks(t, ctx, "Original", base, peers, peers)

	const bobTitle, carolTitle = "Bob same-ms title", "Carol same-ms title"
	// Both edit the SAME field BEFORE either reaches the hub — genuine same-field
	// concurrency at the same physical millisecond.
	editTitle(t, ctx, tr.bob, tr.taskClientID, bobTitle)
	editTitle(t, ctx, tr.carol, tr.taskClientID, carolTitle)

	// Read the HLC each peer stamped on the title field. With the same fixed clock
	// and each peer's title-edit being its first emit, the two HLCs share physical_ms
	// + logical and differ only by node_id — exactly the same-ms tie-break case.
	bobHLC := titleFieldHLC(t, tr.bob, tr.taskClientID)
	carolHLC := titleFieldHLC(t, tr.carol, tr.taskClientID)
	if bobHLC == carolHLC {
		t.Fatalf("Bob and Carol stamped identical HLCs %q — the node_id tie-break is not exercised", bobHLC)
	}
	assertSamePhysicalLogical(t, bobHLC, carolHLC)

	// The deterministic winner is the lexically-greater HLC string (CompareString IS
	// the per-field LWW comparator; node_id is its final, deciding component here).
	wantTitle := bobTitle
	if hlc.CompareString(carolHLC, bobHLC) > 0 {
		wantTitle = carolTitle
	}

	allWinner := func() bool {
		return tr.alice.TaskTitle(tr.taskClientID) == wantTitle &&
			tr.bob.TaskTitle(tr.taskClientID) == wantTitle &&
			tr.carol.TaskTitle(tr.taskClientID) == wantTitle
	}
	settleHub(t, ctx, tr, allWinner)

	AssertConverged(t, allWinner,
		"same-field same-ms edit did not converge to the single node_id-tie-break winner on all three")

	// And the stored per-field HLC on every node must be the winner's HLC — the live
	// value and the field clock agree across the federation (no split-brain).
	wantHLC := bobHLC
	if wantTitle == carolTitle {
		wantHLC = carolHLC
	}
	for _, in := range []*Instance{tr.alice, tr.bob, tr.carol} {
		if got := titleFieldHLC(t, in, tr.taskClientID); got != wantHLC {
			t.Errorf("%s title field HLC: got %q, want %q (converged value/clock disagree)", in.URL(), got, wantHLC)
		}
	}
}

// TestConvergence_UpdateVsDeleteNoResurrection drives update-vs-delete concurrency
// through the owner hub: Bob DELETES the shared task while Carol concurrently EDITS
// its title, both before either reaches Alice. The delete's synthetic _deleted HLC
// wins per-field LWW, so the stale title edit cannot resurrect the tombstoned
// entity. After the hub settles, ALL THREE instances converge to DELETED — the task
// is tombstoned everywhere, never live (US-3.7 AC2 / §10.4 no resurrection).
func TestConvergence_UpdateVsDeleteNoResurrection(t *testing.T) {
	ctx := context.Background()
	// Pin each instance to its own clock so the concurrent edits order
	// deterministically by physical_ms (node_id-independent), all inside the
	// transport ±5min window: the seed lands at base (Alice), Carol's title edit at
	// base+1min, and Bob's delete at base+2min. Bob's delete is therefore
	// unambiguously the NEWEST HLC — Carol's title edit is a STALE pre-deletion edit
	// that must NOT resurrect the tombstoned task.
	base := time.Date(2030, 6, 1, 12, 0, 0, 0, time.UTC)
	tr := newOwnerHubTriadClocks(t, ctx, "Original",
		base, base.Add(2*time.Minute), base.Add(1*time.Minute))

	// Bob deletes (tombstone HLC at base+2min); Carol edits the title (HLC at
	// base+1min) — a concurrent edit STALE relative to the tombstone.
	editTitle(t, ctx, tr.carol, tr.taskClientID, "Carol's doomed edit")
	deleteTask(t, ctx, tr.bob, tr.taskClientID)

	allDeleted := func() bool {
		return taskTombstoned(tr.alice, tr.taskClientID) &&
			taskTombstoned(tr.bob, tr.taskClientID) &&
			taskTombstoned(tr.carol, tr.taskClientID)
	}
	settleHub(t, ctx, tr, allDeleted)

	AssertConverged(t, allDeleted,
		"update-vs-delete did not converge to deleted on all three (resurrection?)")

	// The task must NOT be live on any node — the stale edit did not resurrect it.
	for _, in := range []*Instance{tr.alice, tr.bob, tr.carol} {
		if title := in.TaskTitle(tr.taskClientID); title != "" {
			t.Errorf("%s shows a LIVE task %q after a delete — the stale edit resurrected it", in.URL(), title)
		}
	}
}

// TestConvergence_IdempotentRedelivery proves at-least-once redelivery is a dedup
// no-op (NFR-2.2): Bob edits the title, the hub settles and converges, and then the
// SAME signed event is delivered to Alice a SECOND time. The receiver dedups on
// event_id, so exactly one inbox row stays applied and the converged title is
// unchanged — a redelivered event neither double-applies nor corrupts state.
func TestConvergence_IdempotentRedelivery(t *testing.T) {
	ctx := context.Background()
	tr := newOwnerHubTriad(t, ctx, time.Time{}, "Original")

	const bobTitle = "Bob's idempotent title"
	editTitle(t, ctx, tr.bob, tr.taskClientID, bobTitle)

	// The signed payload Bob enqueued for this edit (the exact bytes that will be
	// redelivered verbatim).
	payload := tr.bob.FirstOutboxPayload(t, bobLocalProjectID(t, tr.bob, tr.taskClientID))
	var ev events.Event
	if err := events.Unmarshal([]byte(payload), &ev); err != nil {
		t.Fatalf("unmarshal bob edit payload: %v", err)
	}

	convergedBeforeProbe := func() bool {
		return tr.alice.TaskTitle(tr.taskClientID) == bobTitle &&
			tr.carol.TaskTitle(tr.taskClientID) == bobTitle
	}
	settleHub(t, ctx, tr, convergedBeforeProbe)
	AssertConverged(t, convergedBeforeProbe, "Bob's edit did not converge before the redelivery probe")

	if got := inboxRowCount(tr.alice, ev.EventID); got != 1 {
		t.Fatalf("Alice recorded event %q in %d inbox rows before redelivery, want 1", ev.EventID, got)
	}

	// REDELIVER the exact same signed event to Alice through the REAL signed
	// /federation/events again (at-least-once). The receiver must ACCEPT it (2xx —
	// RedeliverToOwner fails the test on a non-2xx) and dedup it on event_id.
	tr.bob.RedeliverToOwner(t, ctx, tr.alice.URL(), payload)
	// Let any (no-op) apply settle; the converged title must be unchanged.
	unchanged := func() bool {
		return tr.alice.TaskTitle(tr.taskClientID) == bobTitle &&
			tr.bob.TaskTitle(tr.taskClientID) == bobTitle &&
			tr.carol.TaskTitle(tr.taskClientID) == bobTitle
	}
	settleHub(t, ctx, tr, unchanged)

	if got := inboxRowCount(tr.alice, ev.EventID); got != 1 {
		t.Errorf("Alice recorded event %q in %d inbox rows after redelivery, want 1 (dedup failed)", ev.EventID, got)
	}
	AssertConverged(t, unchanged, "redelivery changed the converged title (idempotency violated)")
}

// titleFieldHLC reads the stored per-field HLC for an entity's title on an
// instance — the per-field LWW clock the same-ms determinism assertion compares.
func titleFieldHLC(t *testing.T, in *Instance, taskClientID string) string {
	t.Helper()
	var h string
	err := in.DB().QueryRow(
		`SELECT hlc FROM entity_field_hlc WHERE entity_type = ? AND entity_id = ? AND field_name = 'title'`,
		string(events.EntityTask), taskClientID).Scan(&h)
	if err != nil {
		t.Fatalf("read title field HLC on %s: %v", in.URL(), err)
	}
	return h
}

// assertSamePhysicalLogical fails the test unless two HLC strings share the same
// physical_ms AND logical component (differing only in node_id) — the precondition
// that makes the same-field same-ms scenario a genuine node_id tie-break rather
// than an ordinary physical/logical ordering.
func assertSamePhysicalLogical(t *testing.T, a, b string) {
	t.Helper()
	ha, err := hlc.Parse(a)
	if err != nil {
		t.Fatalf("parse HLC %q: %v", a, err)
	}
	hb, err := hlc.Parse(b)
	if err != nil {
		t.Fatalf("parse HLC %q: %v", b, err)
	}
	if ha.PhysicalMS != hb.PhysicalMS || ha.Logical != hb.Logical {
		t.Fatalf("HLCs differ beyond node_id: %q vs %q (physical/logical not equal) — not a same-ms tie-break", a, b)
	}
	if ha.NodeID == hb.NodeID {
		t.Fatalf("HLCs share node_id %q — cannot exercise the tie-break", ha.NodeID)
	}
}

// bobLocalProjectID resolves the local project id of the shared task on Bob so the
// redelivery probe can read the exact outbox payload Bob enqueued for his edit.
func bobLocalProjectID(t *testing.T, in *Instance, taskClientID string) int64 {
	t.Helper()
	var pid int64
	if err := in.DB().QueryRow(
		`SELECT project_id FROM tasks WHERE client_id = ? AND deleted_at IS NULL`, taskClientID).Scan(&pid); err != nil {
		t.Fatalf("resolve local project id on %s: %v", in.URL(), err)
	}
	return pid
}

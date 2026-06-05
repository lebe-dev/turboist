// Federation v1 F7.4 — 60-day-offline pull→snapshot fallback + clock-skew
// (§15.5; the end-to-end close of US-3.7 AC4). These tests stand a full
// federation up through the F7.1 harness (the REAL signed handshake + snapshot
// bootstrap, the REAL outbox publisher / signed pull endpoint / per-event
// validator / single-goroutine inbox apply) and drive the two §15.5 scenarios the
// earlier slices proved only in pieces, now asserted together through the
// production recovery loop:
//
//   - Offline past retention → 410 → re-bootstrap (TestF74_OfflinePastRetention*):
//     a joiner that fell behind the owner's retention window pulls and is answered
//     410 stale_pull (the F3.3 emit half) because the owner's outbox was GC'd above
//     its cursor with a durable pruned floor; the joiner's REAL recovery loop
//     CONSUMES the 410 (the F4.2 consume half) — re-fetches the owner snapshot and
//     overwrites local state in one tx — and the test asserts, end-to-end and in one
//     place (the milestone's "canonical place AC4's emit+consume halves are asserted
//     together"): the owner-deleted task is NOT resurrected; a tombstone younger than
//     the 90-day retention SURVIVES in the snapshot; the joiner's UNSENT outbox is
//     preserved (R3, the highest-impact bug); and the re-sync cutoff X (cutoff HLC +
//     wall-clock) is stamped on the joiner's mapping so the F4.2 ResyncBanner can
//     surface it (US-4.2 AC4).
//
//   - Clock-skew (TestF74_Skew*): the three skew bands the spec pins (plan §6 / line
//     403), driven through the REAL transport + per-event validator on the harness's
//     ISOLATED federation-vs-auth clocks (the milestone risk "isolate federation
//     clock from auth clocks"):
//
//   - a +6min FEDERATION (HLC) skew with the transport/auth clocks aligned is a
//     SOFT pass, not a hard fail: the handshake completes and an event whose HLC
//     is +6min in the future (< the 10min hard ceiling) is accepted and the HLC
//     Recv-merge still converges both instances;
//
//   - a +6min TRANSPORT skew (the sender's auth clock ahead of the receiver's
//     ±5min window) is a HARD 401 federation_timestamp_stale (F0.3);
//
//   - an event HLC +11min in the future (> the 10min ceiling) is a HARD 400
//     federation_clock_skew with ZERO rows applied (F3.2a, US-7.2 AC4).
package fedtest

import (
	"context"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// staleCursorHLC is the OLD cursor the offline joiner is rewound to;
// retentionTombstoneHLC is the owner's pruned-floor high-water mark, set WELL above
// the stale cursor (so the joiner's pull is answered 410) but well within the
// 90-day retention window (so the owner's tombstone for the deleted task still
// rides the re-bootstrap snapshot — the deletion is applied, not silently dropped).
// Both are zero-padded to 14 physical digits so they sort lexically. retentionPruneNow
// is the wall-clock recorded with the pruned-floor advance.
const (
	staleCursorHLC        = "00000000000100-0000-nodeJoiner"
	retentionTombstoneHLC = "00000000500000-0000-nodeOwner"
	retentionPruneNow     = "2026-06-02T00:00:00.000Z"
)

// TestF74_OfflinePastRetentionReBootstrapsNoResurrection is the canonical
// end-to-end close of US-3.7 AC4 through the F7.1 harness: an owner (Alice) shares
// a project carrying two tasks ("Keep" survives, "Gone" is deleted while the
// joiner is offline); a joiner (Bob) JOINS through the real handshake + snapshot,
// then falls behind retention. Alice deletes "Gone" (a tombstone younger than
// 90 days), her outbox is GC'd above Bob's cursor with a durable pruned floor, and
// Bob's REAL recovery loop pulls → is answered 410 → consumes it → re-bootstraps
// from Alice's fresh snapshot. The test asserts in ONE place: "Gone" is tombstoned
// (not resurrected) on Bob, "Keep" survives, the <90d tombstone rode the snapshot,
// Bob's unsent outbox survived, and the cutoff X marker is stamped (US-4.2 AC4).
func TestF74_OfflinePastRetentionReBootstrapsNoResurrection(t *testing.T) {
	ctx := context.Background()
	h := NewHarness(t)
	// Both instances opt into the no-op TRANSPORT nonce cache: the in-process
	// app.Test() transport can re-serve the SAME signed request and trip a spurious
	// federation_replay (401) on the Join, the first push, or the recovery re-fetch of
	// Alice's snapshot — a pure harness artifact a real one-shot HTTP client never
	// produces. This test asserts the DOMAIN 60-day-offline → 410 → re-bootstrap
	// no-resurrection invariant (US-3.7 AC4 / US-4.2 AC4), not transport anti-replay,
	// which is owned by F0.3's dedicated single-request tests. See WithPermissiveNonces.
	alice := h.AddInstance(t, "https://alice.example", WithPermissiveNonces())
	bob := h.AddInstance(t, "https://bob.example", WithPermissiveNonces())

	// Alice owns the federated project with two tasks. Both are created through the
	// production mutator so each carries an HLC + outbox event.
	aliceProj := alice.CreateFederatedProject(t, ctx, "Shared")
	keepClientID := model.NewClientID()
	goneClientID := model.NewClientID()
	cx := int64(1)
	if _, err := alice.Mutator().Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cx, ProjectID: &aliceProj}, Title: "Keep",
	}, keepClientID); err != nil {
		t.Fatalf("alice create Keep: %v", err)
	}
	if _, err := alice.Mutator().Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cx, ProjectID: &aliceProj}, Title: "Gone",
	}, goneClientID); err != nil {
		t.Fatalf("alice create Gone: %v", err)
	}

	// Bob JOINS through the REAL signed handshake + snapshot bootstrap — no seeded
	// peer rows. The snapshot converges BOTH tasks onto Bob (initial bootstrap).
	invite := alice.CreateInvite(t, ctx, aliceProj, model.FederationPermissionWrite)
	joined := bob.Join(t, ctx, alice.URL(), invite)
	AssertConverged(t, func() bool {
		return bob.TaskTitle(keepClientID) == "Keep" && bob.TaskTitle(goneClientID) == "Gone"
	}, "initial snapshot did not converge both tasks onto Bob")
	bobProj := joined.ProjectID

	// Alice deletes "Gone" through the production mutator: a soft-delete tombstone +
	// synthetic _deleted HLC well within the 90-day retention window. Bob still holds
	// the live row from his earlier bootstrap, so the re-bootstrap MUST tombstone it.
	aliceGone := alice.TaskByClientID(t, ctx, goneClientID)
	if err := alice.Mutator().Delete(ctx, aliceGone); err != nil {
		t.Fatalf("alice delete Gone: %v", err)
	}

	// Bob fell behind retention: set his cursor to an OLD HLC, then prune Alice's
	// outbox to a durable floor ABOVE that cursor. With no retained outbox + a pruned
	// floor > Bob's since_hlc, Alice's signed pull endpoint answers 410 stale_pull.
	bob.SetStaleCursor(t, bobProj, staleCursorHLC)
	alice.AdvancePrunedFloor(t, ctx, aliceProj, retentionTombstoneHLC, retentionPruneNow)

	// Bob has an UNSENT local outbox event awaiting delivery (R3 — the headline bug
	// the re-bootstrap must NOT clear).
	bob.SeedUnsentOutbox(t, bobProj, "bob-unsent")

	// Bob's REAL recovery loop: pull from Alice (→ 410) and CONSUME it (→ re-bootstrap
	// from Alice's fresh snapshot through her real signed snapshot endpoint).
	bob.StartApply(t, ctx)
	loop := bob.RecoveryLoop()
	if err := loop.RunOnce(ctx); err != nil {
		t.Fatalf("recovery pass (consume 410 → re-bootstrap): %v", err)
	}

	// (1) "Gone" is tombstoned on Bob (NOT resurrected); "Keep" survives. The <90d
	// tombstone rode the snapshot — the deletion converged, not silently dropped.
	AssertConverged(t, func() bool {
		return bob.TaskTombstoned(goneClientID) && bob.TaskTitle(keepClientID) == "Keep"
	}, "re-bootstrap did not converge: Gone must be tombstoned (no resurrection) and Keep must survive")
	if title := bob.TaskTitle(goneClientID); title != "" {
		t.Errorf("owner-deleted task is LIVE on Bob after re-bootstrap (%q) — resurrected", title)
	}

	// (2) Bob's unsent outbox event survived the re-bootstrap (R3).
	if got := bob.OutboxRowCount("bob-unsent"); got != 1 {
		t.Errorf("Bob's unsent outbox event cleared by re-bootstrap: got %d, want 1 (R3)", got)
	}

	// (3) The re-bootstrap cutoff X (cutoff HLC + wall-clock) is stamped on Bob's
	// mapping row so the F4.2 ResyncBanner can render X (US-4.2 AC4) — a real
	// persisted value, never a placeholder.
	cutoffHLC, rebootAt := bob.ReBootstrapMarker(t, bobProj)
	if cutoffHLC == "" {
		t.Errorf("re-bootstrap cutoff HLC not stamped (US-4.2 AC4)")
	}
	if rebootAt == "" {
		t.Errorf("re-bootstrap wall-clock cutoff X not stamped (US-4.2 AC4 — must be a real persisted value)")
	}
}

// TestF74_SkewPlus6MinHandshakeSoftWarnHLCConverges drives the milestone's headline
// clock-skew band: a +6min FEDERATION (HLC) skew with the transport/auth clocks
// ALIGNED is a SOFT pass, not a hard fail (the milestone risk "isolate federation
// clock from auth clocks"). Bob's HLC clock runs 6 minutes ahead of Alice's while
// BOTH instances' transport/auth clocks stay aligned, so: (a) Bob's handshake +
// push stay inside the transport ±5min window (the handshake COMPLETES — soft, not
// a hard 401); (b) the event Bob emits carries an HLC +6min in the future, which is
// below the 10min hard ceiling so Alice's per-event validator ACCEPTS it; and (c)
// the HLC Recv-merge still converges Alice onto Bob's edit despite the +6min HLC
// skew. HLC convergence is clock-skew-tolerant by construction (Recv takes the max
// physical_ms); this asserts it end-to-end through the real transport.
func TestF74_SkewPlus6MinHandshakeSoftWarnHLCConverges(t *testing.T) {
	ctx := context.Background()
	h := NewHarness(t)

	// Transport/auth clocks are aligned (default real wall clock on both), so every
	// signed request is inside the ±5min window — the handshake is never hard-failed
	// on transport. Bob's FEDERATION (HLC) clock alone is skewed +6min ahead: the
	// event Bob emits is stamped 6 minutes in the future of Alice's wall clock.
	// Both instances opt into the no-op TRANSPORT nonce cache: the in-process
	// app.Test() re-serve can trip a spurious federation_replay (401) on the Join or
	// push and break this convergence test — a harness transport artifact. This test
	// asserts the DOMAIN +6min FEDERATION-clock soft-accept + HLC Recv-merge
	// convergence, isolated from the TRANSPORT ±5min window (the aligned transport
	// clock keeps every request inside it — asserted as the soft band) and from
	// transport anti-replay (owned by F0.3). See WithPermissiveNonces.
	base := time.Now().UTC()
	alice := h.AddInstance(t, "https://alice.example", WithFixedClock(base), WithPermissiveNonces())
	bob := h.AddInstance(t, "https://bob.example",
		WithFixedClock(base), WithHLCClock(func() time.Time { return base.Add(6 * time.Minute) }), WithPermissiveNonces())

	aliceProj := alice.CreateFederatedProject(t, ctx, "Shared")
	taskClientID := model.NewClientID()
	cx := int64(1)
	if _, err := alice.Mutator().Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cx, ProjectID: &aliceProj}, Title: "Original",
	}, taskClientID); err != nil {
		t.Fatalf("alice create seed task: %v", err)
	}

	// Bob JOINS through the REAL signed handshake (transport clock aligned → the +6min
	// federation skew does NOT hard-fail the handshake — the soft-warn band).
	invite := alice.CreateInvite(t, ctx, aliceProj, model.FederationPermissionWrite)
	bob.Join(t, ctx, alice.URL(), invite)
	AssertConverged(t, func() bool { return bob.TaskTitle(taskClientID) == "Original" },
		"snapshot did not converge under a +6min federation-clock skew (handshake hard-failed?)")

	// Bob edits the task: the emit stamps an HLC +6min in the future of Alice's wall
	// clock. Alice's per-event validator (on her aligned wall clock) sees +6min < 10min
	// → ACCEPTS it (soft), and the HLC Recv-merge converges.
	bobTask := bob.TaskByClientID(t, ctx, taskClientID)
	skewedTitle := "Bob +6min skew edit"
	if err := bob.Mutator().Update(ctx, bobTask, repo.TaskUpdate{Title: &skewedTitle}); err != nil {
		t.Fatalf("bob update under skew: %v", err)
	}
	alice.StartApply(t, ctx)
	bob.PumpOutbox(t, ctx)

	AssertConverged(t, func() bool { return alice.TaskTitle(taskClientID) == skewedTitle },
		"+6min federation-clock-skewed edit did not converge on Alice (soft warn should accept + HLC merge)")
}

// TestF74_SkewPlus6MinTransportHardRejects is the complementary HARD band (plan §6
// / line 403, transport leg, F0.3): when the SENDER's transport/auth clock is +6min
// ahead of the RECEIVER's ±5min window, the signed request is a HARD 401
// federation_timestamp_stale — the join fails and the owner records NO peer row.
// This is the auth-clock check that a moderate FEDERATION skew is deliberately
// isolated from (the prior test); here the transport clock itself is skewed, so the
// hard transport window bites.
func TestF74_SkewPlus6MinTransportHardRejects(t *testing.T) {
	ctx := context.Background()
	h := NewHarness(t)

	// Alice's receiver window is at base; Bob's transport/auth clock is +6min ahead,
	// so Bob's signed handshake timestamp is 6 minutes outside Alice's ±5min window.
	base := time.Now().UTC()
	alice := h.AddInstance(t, "https://alice.example", WithFixedClock(base))
	bob := h.AddInstance(t, "https://bob.example", WithFixedClock(base.Add(6*time.Minute)))

	aliceProj := alice.CreateFederatedProject(t, ctx, "Shared")
	invite := alice.CreateInvite(t, ctx, aliceProj, model.FederationPermissionWrite)

	// The +6min transport-skewed handshake must be HARD-rejected by Alice's window.
	if _, err := bob.TryJoin(ctx, alice.URL(), invite); err == nil {
		t.Fatalf("a +6min transport-skewed handshake was accepted (transport ±5min window not enforced)")
	}

	// No peer mapping created on the owner — the hard transport reject ran before any
	// invite consume / row write.
	if got := alice.PeerRowCount(aliceProj); got != 0 {
		t.Errorf("transport-skewed handshake created %d owner peer row(s), want 0", got)
	}
}

// TestF74_SkewPlus11MinEventHardRejectsZeroRows is the event-payload HARD band
// (plan §6 / line 403, F3.2a, US-7.2 AC4): an event whose HLC physical_ms is +11min
// in the future (ABOVE the 10min ceiling) is rejected 400 federation_clock_skew by
// the per-event validator with ZERO rows applied — even though its transport
// signature is valid. It is the hard ceiling the +6min soft band sits below: the
// SAME push that converged at +6min must NOT apply at +11min.
func TestF74_SkewPlus11MinEventHardRejectsZeroRows(t *testing.T) {
	ctx := context.Background()
	h := NewHarness(t)

	// Transport clocks aligned (push passes the ±5min window); Bob's HLC clock is
	// +11min ahead → the event HLC is 11min in the future, above the 10min ceiling.
	// Both instances opt into the no-op TRANSPORT nonce cache so the in-process
	// app.Test() re-serve cannot trip a spurious federation_replay (401) on the Join —
	// which would mask the assertion under test. The HARD reject this test asserts is
	// the PER-EVENT HLC clock-skew check (400 federation_clock_skew), a distinct
	// receiver-side stage from the transport nonce: disabling the transport nonce
	// leaves the +11min event reject fully intact (it still surfaces 400, not 401).
	// Transport anti-replay itself is owned by F0.3. See WithPermissiveNonces.
	base := time.Now().UTC()
	alice := h.AddInstance(t, "https://alice.example", WithFixedClock(base), WithPermissiveNonces())
	bob := h.AddInstance(t, "https://bob.example",
		WithFixedClock(base), WithHLCClock(func() time.Time { return base.Add(11 * time.Minute) }), WithPermissiveNonces())

	aliceProj := alice.CreateFederatedProject(t, ctx, "Shared")
	taskClientID := model.NewClientID()
	cx := int64(1)
	if _, err := alice.Mutator().Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cx, ProjectID: &aliceProj}, Title: "Original",
	}, taskClientID); err != nil {
		t.Fatalf("alice create seed task: %v", err)
	}

	invite := alice.CreateInvite(t, ctx, aliceProj, model.FederationPermissionWrite)
	bob.Join(t, ctx, alice.URL(), invite)
	AssertConverged(t, func() bool { return bob.TaskTitle(taskClientID) == "Original" },
		"snapshot did not converge before the +11min skew push")

	// Bob edits the task: the emit stamps an HLC +11min in the future. Pushing it to
	// Alice through her real signed endpoint must be HARD-rejected (400) by the
	// per-event validator BEFORE any inbox/domain write.
	bobTask := bob.TaskByClientID(t, ctx, taskClientID)
	rejectedTitle := "Bob +11min skew edit"
	if err := bob.Mutator().Update(ctx, bobTask, repo.TaskUpdate{Title: &rejectedTitle}); err != nil {
		t.Fatalf("bob update under +11min skew: %v", err)
	}
	alice.StartApply(t, ctx)

	// PushFirstOutbox surfaces the receiver's HTTP status: a +11min event is a HARD
	// 400 (federation_clock_skew), distinct from the +6min soft accept (2xx).
	status := bob.PushFirstOutboxToOwner(t, ctx, alice.URL(), bobLocalProjectID(t, bob, taskClientID))
	if status != 400 {
		t.Errorf("+11min event push status: got %d, want 400 (federation_clock_skew hard reject)", status)
	}

	// ZERO rows applied: Alice's title is unchanged (the skewed edit never landed) and
	// no inbox row was recorded for the rejected event.
	if got := alice.TaskTitle(taskClientID); got != "Original" {
		t.Errorf("Alice's title changed to %q — a +11min event was applied (want ZERO rows)", got)
	}
	bobOutbox := bob.FirstOutboxPayload(t, bobLocalProjectID(t, bob, taskClientID))
	var ev events.Event
	if err := events.Unmarshal([]byte(bobOutbox), &ev); err != nil {
		t.Fatalf("unmarshal bob skewed payload: %v", err)
	}
	if got := inboxRowCount(alice, ev.EventID); got != 0 {
		t.Errorf("Alice recorded %d inbox rows for the rejected +11min event, want 0", got)
	}
}

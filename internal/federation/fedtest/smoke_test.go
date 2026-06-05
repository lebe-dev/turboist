// Federation v1 F7.1 — the two/three-instance in-process integration harness
// smoke test. It drives the canonical end-to-end lifecycle named in the F7.1
// milestone — create → invite → handshake → snapshot → edit → converge — across
// two FULL in-process instances built from httpapi.NewApp + app.Test(), with
// every cross-instance call routed A↔B through the REAL signature middleware,
// the REAL handshake/snapshot/events handlers, the REAL per-event validator, and
// the REAL HLC/emit/apply path. Nothing here is faked but the network transport
// (it is in-process via app.Test()) and the .well-known fetch (resolved from the
// harness's shared key map). The harness this test exercises (Harness, Instance,
// PeerClient, PumpOutbox, AssertConverged, per-instance clock injection) is the
// foundation §15.5 / F7.3 / F7.4 / F7.5 build on.
package fedtest

import (
	"context"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// TestHarness_SmokeCreateInviteHandshakeSnapshotEditConverge is the F7.1 smoke
// scenario. Alice (owner) federates a project holding one task, mints an invite,
// and Bob JOINS through the real signed handshake + snapshot bootstrap (no seeded
// peer rows — the federation relationship is established by the handshake itself).
// The snapshot converges Alice's task onto Bob. Alice then EDITS the task; the
// edit is pumped through the real outbox → POST /federation/events → Bob's
// validator + apply, and converges on Bob within the NFR-1.1 5s budget.
func TestHarness_SmokeCreateInviteHandshakeSnapshotEditConverge(t *testing.T) {
	ctx := context.Background()
	h := NewHarness(t)
	// Both instances opt into the no-op TRANSPORT nonce cache: the in-process
	// app.Test() transport can re-serve the SAME signed request and trip a spurious
	// federation_replay (401) on the Join or push — a pure harness artifact a real
	// one-shot HTTP client never produces. This smoke test asserts the DOMAIN
	// create→invite→handshake→snapshot→edit→converge lifecycle, not transport
	// anti-replay, which is owned by F0.3's dedicated single-request
	// HTTPSignatureMiddleware tests. See WithPermissiveNonces.
	alice := h.AddInstance(t, "https://alice.example", WithPermissiveNonces())
	bob := h.AddInstance(t, "https://bob.example", WithPermissiveNonces())

	// 1) CREATE: Alice owns a federated project with one task (the production
	// enable path flips is_federated + writes the is_owner self-row + ensures keys).
	aliceProj := alice.CreateFederatedProject(t, ctx, "Shared")
	taskClientID := model.NewClientID()
	cx := int64(1)
	if _, err := alice.Mutator().Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cx, ProjectID: &aliceProj},
		Title:     "Original",
	}, taskClientID); err != nil {
		t.Fatalf("alice create task: %v", err)
	}

	// 2) INVITE: Alice mints a write invite (the real CreateInvite — 256-bit secret,
	// hashed at rest, returned once with the join link).
	invite := alice.CreateInvite(t, ctx, aliceProj, model.FederationPermissionWrite)

	// 3) HANDSHAKE + 4) SNAPSHOT: Bob joins through the REAL signed handshake and
	// the REAL snapshot bootstrap, routed to Alice's app via the harness PeerClient.
	// Join returns Bob's LOCAL project id (created by the snapshot apply).
	joined := bob.Join(t, ctx, alice.URL(), invite)
	if joined.ProjectID == 0 {
		t.Fatalf("join returned no local project id")
	}
	if joined.Permissions != model.FederationPermissionWrite {
		t.Errorf("granted permission: got %q, want %q", joined.Permissions, model.FederationPermissionWrite)
	}

	// The snapshot must have converged Alice's task onto Bob (same cross-instance
	// client_id), proving handshake → snapshot end-to-end.
	AssertConverged(t, func() bool { return bob.TaskTitle(taskClientID) == "Original" },
		"snapshot did not converge Alice's task onto Bob")

	// 5) EDIT: Alice renames the task via the production TaskMutator (op=update),
	// and 6) the edit is pumped through the real outbox → signed POST → Bob applies.
	aliceTask := alice.TaskByClientID(t, ctx, taskClientID)
	newTitle := "Renamed by Alice"
	if err := alice.Mutator().Update(ctx, aliceTask, repo.TaskUpdate{Title: &newTitle}); err != nil {
		t.Fatalf("alice update task: %v", err)
	}

	// Start Bob's apply queue so a pushed event is applied off the HTTP path, then
	// pump Alice's outbox synchronously (the F7.1 PumpOutbox drain).
	bob.StartApply(t, ctx)
	alice.PumpOutbox(t, ctx)

	AssertConverged(t, func() bool { return bob.TaskTitle(taskClientID) == newTitle },
		"edit did not converge on Bob within the 5s budget")
}

// TestHarness_BodyTamperRejectedNoBypass proves the harness routes through the
// REAL signature middleware (not a bypass): a handshake whose signed body is
// tampered after signing is rejected by Alice's transport signature check, so the
// join fails and NO federated_projects peer row is created on the owner. This is
// the §15.5 / F7.1 "assert body-tamper → no bypass" guard — without the real
// middleware in the loop, the tampered handshake would be silently accepted.
func TestHarness_BodyTamperRejectedNoBypass(t *testing.T) {
	ctx := context.Background()
	h := NewHarness(t)
	alice := h.AddInstance(t, "https://alice.example")
	bob := h.AddInstance(t, "https://bob.example")

	aliceProj := alice.CreateFederatedProject(t, ctx, "Shared")
	invite := alice.CreateInvite(t, ctx, aliceProj, model.FederationPermissionWrite)

	// Route Bob's handshake through a PeerClient that flips one body byte AFTER the
	// joiner signs — the digest no longer matches, so the real middleware must 401.
	h.TamperNextBody()
	if _, err := bob.TryJoin(ctx, alice.URL(), invite); err == nil {
		t.Fatalf("tampered handshake was accepted (signature middleware bypassed)")
	}

	// The owner must have created NO peer mapping for Bob (nothing consumed/recorded).
	var peers int
	if err := alice.DB().QueryRow(
		`SELECT COUNT(*) FROM federated_projects WHERE local_project_id = ? AND is_owner = 0`,
		aliceProj).Scan(&peers); err != nil {
		t.Fatalf("count owner peer rows: %v", err)
	}
	if peers != 0 {
		t.Errorf("tampered handshake created %d owner peer row(s), want 0", peers)
	}
}

// TestHarness_InjectedClockReachesHLC asserts the F7.1 per-instance clock
// injection reaches the HLC: an instance pinned to a fixed wall clock stamps that
// clock's millisecond into the physical_ms of the HLC its emit advances (the
// "clock injection must reach HLC" risk). This is the seam F7.2/F7.4 rely on to
// drive deterministic HLC ordering and clock-skew scenarios.
func TestHarness_InjectedClockReachesHLC(t *testing.T) {
	ctx := context.Background()
	h := NewHarness(t)
	fixed := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	alice := h.AddInstance(t, "https://alice.example", WithFixedClock(fixed))

	aliceProj := alice.CreateFederatedProject(t, ctx, "Shared")
	taskClientID := model.NewClientID()
	cx := int64(1)
	if _, err := alice.Mutator().Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cx, ProjectID: &aliceProj},
		Title:     "Clocked",
	}, taskClientID); err != nil {
		t.Fatalf("create on A: %v", err)
	}

	// The emit advanced hlc_state through the injected clock: its physical_ms must
	// equal the fixed wall clock's UnixMilli (R11 — physical_ms tracks updated_at).
	var phys int64
	if err := alice.DB().QueryRow(`SELECT last_physical_ms FROM hlc_state WHERE id = 1`).Scan(&phys); err != nil {
		t.Fatalf("read hlc_state: %v", err)
	}
	if phys != fixed.UnixMilli() {
		t.Errorf("hlc physical_ms: got %d, want %d (injected clock did not reach HLC)", phys, fixed.UnixMilli())
	}

	// And the outbox event the emit wrote carries an HLC stamped from that clock.
	payload := alice.FirstOutboxPayload(t, aliceProj)
	var e events.Event
	if err := events.Unmarshal([]byte(payload), &e); err != nil {
		t.Fatalf("unmarshal outbox payload: %v", err)
	}
	if e.Op != events.OpCreate {
		t.Errorf("outbox op: got %q, want %q", e.Op, events.OpCreate)
	}
}

// TestHarness_FixedClockReachesReceiverSkewCheck enforces the package-doc
// contract (harness_test.go:24-25 / smoke_test.go:10-11) that per-instance clock
// injection reaches the RECEIVER's transport ±5min window check — not just the
// HLC store and the outbound timestamp. BOTH instances are pinned to the SAME
// fixed 2030 instant (far from the real wall clock), then Bob JOINS through the
// real signed handshake and Alice's edit is pumped through the real signed
// /federation/events. If the receiver window were left at the real time.Now, the
// 2030-stamped requests would be rejected 401 federation_timestamp_stale and
// neither the handshake nor the push would land; the convergence below would
// time out. This is the seam F7.4 ("+6min skew → handshake SOFT warn, HLC still
// converges") builds on, asserted by a test rather than only by a comment.
func TestHarness_FixedClockReachesReceiverSkewCheck(t *testing.T) {
	ctx := context.Background()
	h := NewHarness(t)
	// A fixed instant well outside the ±5min transport window relative to the real
	// wall clock — proving the receiver compares against the injected clock, not now.
	fixed := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	// Both instances opt into the no-op TRANSPORT nonce cache so the in-process
	// app.Test() re-serve cannot trip a spurious federation_replay (401) on the Join or
	// push and mask the assertion under test. This test asserts the RECEIVER's
	// transport ±5min TIMESTAMP window runs on the injected clock (a 2030-stamped
	// request must be accepted, not rejected federation_timestamp_stale) — a distinct
	// receiver-side stage from the transport nonce; disabling the nonce leaves the
	// timestamp-window assertion fully intact. Transport anti-replay itself is owned by
	// F0.3. See WithPermissiveNonces.
	alice := h.AddInstance(t, "https://alice.example", WithFixedClock(fixed), WithPermissiveNonces())
	bob := h.AddInstance(t, "https://bob.example", WithFixedClock(fixed), WithPermissiveNonces())

	aliceProj := alice.CreateFederatedProject(t, ctx, "Shared")
	taskClientID := model.NewClientID()
	cx := int64(1)
	if _, err := alice.Mutator().Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cx, ProjectID: &aliceProj},
		Title:     "Original",
	}, taskClientID); err != nil {
		t.Fatalf("alice create task: %v", err)
	}

	invite := alice.CreateInvite(t, ctx, aliceProj, model.FederationPermissionWrite)

	// HANDSHAKE + SNAPSHOT routed through Alice's REAL signed endpoints: the
	// 2030-stamped signed request must pass Alice's receiver window (injected clock),
	// not be rejected 401 federation_timestamp_stale.
	joined := bob.Join(t, ctx, alice.URL(), invite)
	if joined.ProjectID == 0 {
		t.Fatalf("join returned no local project id (receiver rejected the fixed-clock handshake)")
	}
	AssertConverged(t, func() bool { return bob.TaskTitle(taskClientID) == "Original" },
		"snapshot did not converge under a fixed receiver clock")

	// EDIT pushed through the real signed /federation/events: same fixed-clock
	// request must pass Bob's receiver window and the HLC must still converge.
	aliceTask := alice.TaskByClientID(t, ctx, taskClientID)
	newTitle := "Renamed by Alice"
	if err := alice.Mutator().Update(ctx, aliceTask, repo.TaskUpdate{Title: &newTitle}); err != nil {
		t.Fatalf("alice update task: %v", err)
	}
	bob.StartApply(t, ctx)
	alice.PumpOutbox(t, ctx)
	AssertConverged(t, func() bool { return bob.TaskTitle(taskClientID) == newTitle },
		"edit did not converge under a fixed receiver clock (receiver window not on injected clock?)")
}

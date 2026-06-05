package inbox_test

import (
	"context"
	"testing"

	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/model"
)

// leaveEvent builds a peer→owner federation_leave control event targeting the
// env's project. It is authored/originated by the leaving peer (bob), carries no
// per-field LWW, and uses the env project's client_id as the entity/project id so
// the owner resolves it locally (Federation v1 F5.5, US-6.3 AC2).
func leaveEvent(env *applyEnv, peerURL string) events.Event {
	return events.Event{
		EventID:         eventID("leave-" + peerURL + "-" + env.projectClient),
		Op:              events.OpLeave,
		EntityType:      events.EntityProject,
		EntityID:        env.projectClient,
		ProjectClientID: env.projectClient,
		Author:          peerURL,
		OriginInstance:  peerURL,
		CreatedAt:       "2026-06-01T10:00:00.000Z",
		Fields:          map[string]events.Field{},
	}
}

// TestApply_LeaveMarksPeerLeft asserts applying an op=leave control event on the
// OWNER marks the leaving peer's mapping lost (reason=left) so the owner stops
// fanning out to it and the peers list renders it "left" (Federation v1 F5.5,
// US-6.3 AC2). The transition is reported in the result so the owner's open tabs
// refresh.
func TestApply_LeaveMarksPeerLeft(t *testing.T) {
	env := newApplyEnv(t)
	ctx := context.Background()
	// Owner topology: this instance owns the project (self-row) and bob is a joined
	// peer. The default applyEnv peer row maps alice as a write peer; we add bob.
	const ownerURL = "https://me.example"
	const peerURL = "https://bob.example"
	rebroadcastApplier(env, ownerURL)
	markOwner(t, env, ownerURL)
	addPeer(t, env, peerURL, model.FederationPermissionWrite)

	res, err := env.applier.Apply(ctx, leaveEvent(env, peerURL), peerURL)
	if err != nil {
		t.Fatalf("apply leave: %v", err)
	}
	if !res.ProjectLost {
		t.Errorf("ProjectLost (peer-left transition): got false, want true (US-6.3 AC2)")
	}

	var lost int
	var reason string
	if err := env.db.QueryRow(
		`SELECT lost, lost_reason FROM federated_projects WHERE local_project_id = ? AND peer_instance_url = ? AND is_owner = 0`,
		env.projectID, peerURL,
	).Scan(&lost, &reason); err != nil {
		t.Fatalf("read peer lost: %v", err)
	}
	if lost != 1 || reason != string(model.FederationLostLeft) {
		t.Errorf("peer lost state: got (lost=%d, reason=%q), want (1, left)", lost, reason)
	}
}

// TestApply_LeaveIdempotent asserts a redelivered leave (at-least-once) that lands
// on an already-left peer is a no-op transition (ProjectLost false) and does not
// re-mark the row (Federation v1 F5.5, US-6.3 — idempotency).
func TestApply_LeaveIdempotent(t *testing.T) {
	env := newApplyEnv(t)
	ctx := context.Background()
	const ownerURL = "https://me.example"
	const peerURL = "https://bob.example"
	rebroadcastApplier(env, ownerURL)
	markOwner(t, env, ownerURL)
	addPeer(t, env, peerURL, model.FederationPermissionWrite)

	if _, err := env.applier.Apply(ctx, leaveEvent(env, peerURL), peerURL); err != nil {
		t.Fatalf("first apply leave: %v", err)
	}
	res, err := env.applier.Apply(ctx, leaveEvent(env, peerURL), peerURL)
	if err != nil {
		t.Fatalf("second apply leave: %v", err)
	}
	if res.ProjectLost {
		t.Errorf("second apply ProjectLost: got true, want false (idempotent)")
	}
	var n int
	if err := env.db.QueryRow(
		`SELECT COUNT(*) FROM federated_projects WHERE local_project_id = ? AND lost = 1`, env.projectID,
	).Scan(&n); err != nil {
		t.Fatalf("count lost: %v", err)
	}
	if n != 1 {
		t.Errorf("lost rows after double leave: got %d, want 1", n)
	}
}

// TestApply_LeaveNotReBroadcast asserts a federation_leave is NEVER re-broadcast
// to the other peers (it is point-to-point from the leaver to the owner): the
// owner's outbox stays empty after applying it (Federation v1 F5.5, mirrors the
// op=revoke point-to-point control-event handling).
func TestApply_LeaveNotReBroadcast(t *testing.T) {
	env := newApplyEnv(t)
	ctx := context.Background()
	const ownerURL = "https://me.example"
	const peerURL = "https://bob.example"
	rebroadcastApplier(env, ownerURL)
	markOwner(t, env, ownerURL)
	addPeer(t, env, peerURL, model.FederationPermissionWrite)
	addPeer(t, env, "https://carol.example", model.FederationPermissionWrite)

	if _, err := env.applier.Apply(ctx, leaveEvent(env, peerURL), peerURL); err != nil {
		t.Fatalf("apply leave: %v", err)
	}
	rows := outboxRows(t, env)
	if len(rows) != 0 {
		t.Errorf("leave must not be re-broadcast: outbox rows got %d, want 0", len(rows))
	}
}

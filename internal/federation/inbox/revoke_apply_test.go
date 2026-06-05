package inbox_test

import (
	"context"
	"testing"

	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/model"
)

// revokeEvent builds an owner→joiner federation_revoke control event targeting
// the env's project. It is authored/originated by the owner the env's peer row
// maps to (https://alice.example), carries no per-field LWW.
func revokeEvent(env *applyEnv) events.Event {
	return events.Event{
		EventID:         eventID("revoke-" + env.projectClient),
		Op:              events.OpRevoke,
		EntityType:      events.EntityProject,
		EntityID:        env.projectClient,
		ProjectClientID: env.projectClient,
		Author:          "https://alice.example",
		OriginInstance:  "https://alice.example",
		CreatedAt:       "2026-06-01T10:00:00.000Z",
		Fields:          map[string]events.Field{},
	}
}

// TestApply_RevokeMarksLost asserts applying an op=revoke control event marks the
// joiner's local copy federation_lost (reason=revoked), rendering it read-only
// (Federation v1 F5.4, US-6.2 AC3). The transition is reported in the result so
// the joiner's open tabs refresh.
func TestApply_RevokeMarksLost(t *testing.T) {
	env := newApplyEnv(t)
	ctx := context.Background()

	res, err := env.applier.Apply(ctx, revokeEvent(env), "https://alice.example")
	if err != nil {
		t.Fatalf("apply revoke: %v", err)
	}
	if !res.ProjectLost {
		t.Errorf("ProjectLost: got false, want true (US-6.2 AC3)")
	}

	var lost int
	var reason string
	if err := env.db.QueryRow(
		`SELECT lost, lost_reason FROM federated_projects WHERE local_project_id = ? AND origin_instance_url = 'https://alice.example' AND is_owner = 0`,
		env.projectID,
	).Scan(&lost, &reason); err != nil {
		t.Fatalf("read lost: %v", err)
	}
	if lost != 1 || reason != string(model.FederationLostRevoked) {
		t.Errorf("lost state: got (lost=%d, reason=%q), want (1, revoked)", lost, reason)
	}
}

// TestApply_RevokeIdempotent asserts a redelivered revoke (at-least-once) that
// lands on an already-lost copy is a no-op transition (ProjectLost false) and does
// not change the recorded reason (Federation v1 F5.4, idempotency).
func TestApply_RevokeIdempotent(t *testing.T) {
	env := newApplyEnv(t)
	ctx := context.Background()

	if _, err := env.applier.Apply(ctx, revokeEvent(env), "https://alice.example"); err != nil {
		t.Fatalf("first apply revoke: %v", err)
	}
	// A second apply of the same revoke (the event_id is the same; the inbox dedup
	// normally guards this, but the apply itself must also be idempotent).
	res, err := env.applier.Apply(ctx, revokeEvent(env), "https://alice.example")
	if err != nil {
		t.Fatalf("second apply revoke: %v", err)
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
		t.Errorf("lost rows after double revoke: got %d, want 1", n)
	}
}

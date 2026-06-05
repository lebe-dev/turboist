package inbox_test

import (
	"context"
	"strings"
	"testing"

	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/federation/inbox"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// newFedProjects returns a FederatedProjectRepo bound to the env's DB.
func newFedProjects(env *applyEnv) *repo.FederatedProjectRepo {
	return repo.NewFederatedProjectRepo(env.db)
}

// rebroadcastApplier reconfigures env.applier with the F5.1 owner-hub re-broadcast
// hook enabled, identifying this instance by ownURL (so the applier knows which
// federated_projects self-row marks ownership) and writing relayed events to the
// store's outbox. A no-op ping is used in tests.
func rebroadcastApplier(env *applyEnv, ownURL string) {
	env.applier = inbox.NewApplier(env.db, env.tasks, repo.NewProjectRepo(env.db, repo.NewProjectLabelsRepo(env.db)),
		repo.NewProjectSectionRepo(env.db), newFedProjects(env), env.store).
		WithReBroadcast(env.store, ownURL, func() {})
}

// containsURL reports whether the JSON-array delivered_to string contains url.
func containsURL(deliveredTo, url string) bool {
	return strings.Contains(deliveredTo, `"`+url+`"`)
}

// markOwner promotes the env's project to an owner-enabled federated project by
// adding the is_owner=1 self-row, so the applier treats this instance as the
// hub that re-broadcasts a peer's applied event to the OTHER peers (Federation v1
// F5.1, US-5.2 AC2 hub-and-spoke). ownerURL is this instance's own URL.
func markOwner(t *testing.T, env *applyEnv, ownerURL string) {
	t.Helper()
	fedProjects := newFedProjects(env)
	if err := fedProjects.UpsertPeerRow(context.Background(), model.FederatedProject{
		LocalProjectID:    env.projectID,
		PeerInstanceURL:   ownerURL,
		OriginInstanceURL: ownerURL,
		IsOwner:           true,
		Permissions:       model.FederationPermissionAdmin,
		ProtocolVersion:   1,
	}); err != nil {
		t.Fatalf("self row: %v", err)
	}
}

// addPeer registers a remote peer mapping on the env's project with the given
// permission, so the re-broadcast fan-out has a delivery target. A read peer
// still appears as a peer (US-5.1 AC3 — read peers receive fan-out).
func addPeer(t *testing.T, env *applyEnv, peerURL string, perm model.FederationPermission) {
	t.Helper()
	fedProjects := newFedProjects(env)
	if err := fedProjects.UpsertPeerRow(context.Background(), model.FederatedProject{
		LocalProjectID:    env.projectID,
		PeerInstanceURL:   peerURL,
		OriginInstanceURL: "https://owner.example",
		Permissions:       perm,
		ProtocolVersion:   1,
	}); err != nil {
		t.Fatalf("peer row %s: %v", peerURL, err)
	}
}

// outboxRows returns every (event_id, delivered_to) row in federation_outbox for
// the env's project, so a re-broadcast assertion can inspect what was enqueued.
func outboxRows(t *testing.T, env *applyEnv) map[string]string {
	t.Helper()
	rows, err := env.db.Query(`SELECT event_id, delivered_to FROM federation_outbox WHERE local_project_id = ?`, env.projectID)
	if err != nil {
		t.Fatalf("query outbox: %v", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var eventID, deliveredTo string
		if err := rows.Scan(&eventID, &deliveredTo); err != nil {
			t.Fatalf("scan outbox: %v", err)
		}
		out[eventID] = deliveredTo
	}
	return out
}

// signedRelayEvent builds an update event authored/originated by a PEER (not this
// instance), as the owner would receive it for hub-and-spoke relay. The event_id
// is fixed so dedup can be asserted.
func signedRelayEvent(env *applyEnv, eventID, origin string, fields map[string]events.Field) events.Event {
	return events.Event{
		EventID:         eventID,
		Op:              events.OpUpdate,
		EntityType:      events.EntityTask,
		EntityID:        env.taskClientID,
		ProjectClientID: env.projectClient,
		Author:          origin,
		OriginInstance:  origin,
		CreatedAt:       "2026-06-01T10:00:00.000Z",
		Fields:          fields,
	}
}

// TestApply_OwnerReBroadcastsToOtherPeers asserts that when THIS instance owns the
// federated project, applying a peer's event re-enqueues that event to
// federation_outbox with delivered_to pre-stamped to the origin, so the publisher
// fans it out to the OTHER peers but never back to the origin (Federation v1 F5.1,
// US-5.2 AC2 — owner-hub re-broadcast, no echo loop).
func TestApply_OwnerReBroadcastsToOtherPeers(t *testing.T) {
	env := newApplyEnv(t)
	ctx := context.Background()

	const ownerURL = "https://owner.example"
	const bob = "https://bob.example"
	const carol = "https://carol.example"
	markOwner(t, env, ownerURL)
	addPeer(t, env, bob, model.FederationPermissionWrite)
	addPeer(t, env, carol, model.FederationPermissionRead)

	rebroadcastApplier(env, ownerURL)

	e := signedRelayEvent(env, "evt-from-bob", bob, map[string]events.Field{
		"title": {Value: "Bob's edit", HLC: "00000000000200-0000-nodeB"},
	})
	res, err := env.applier.Apply(ctx, e, bob)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !res.AppliedFields["title"] {
		t.Fatalf("title should have applied")
	}

	rows := outboxRows(t, env)
	deliveredTo, ok := rows["evt-from-bob"]
	if !ok {
		t.Fatalf("owner must re-broadcast the peer event to the outbox; outbox=%v", rows)
	}
	if deliveredTo == "" || deliveredTo == "[]" {
		t.Fatalf("re-broadcast must pre-stamp delivered_to with the origin %q; got %q", bob, deliveredTo)
	}
	if !containsURL(deliveredTo, bob) {
		t.Errorf("delivered_to must contain origin %q (echo guard); got %q", bob, deliveredTo)
	}
}

// TestApply_JoinerDoesNotReBroadcast asserts a NON-owner (a joined peer copy) does
// NOT re-broadcast events it applies — only the owner is the hub (W-7 owner-hub).
// A joiner that re-broadcast would create a second fan-out path and echo loops.
func TestApply_JoinerDoesNotReBroadcast(t *testing.T) {
	env := newApplyEnv(t)
	ctx := context.Background()

	// The env's default peer row is a JOINED mapping (is_owner=0): this instance is
	// not the owner of the project.
	rebroadcastApplier(env, "https://me.example")

	e := signedRelayEvent(env, "evt-from-owner", "https://alice.example", map[string]events.Field{
		"title": {Value: "Owner edit", HLC: "00000000000200-0000-nodeA"},
	})
	if _, err := env.applier.Apply(ctx, e, "https://alice.example"); err != nil {
		t.Fatalf("apply: %v", err)
	}

	if rows := outboxRows(t, env); len(rows) != 0 {
		t.Fatalf("a non-owner must NOT re-broadcast; outbox=%v", rows)
	}
}

// TestApply_ReBroadcastSkippedForNoOpMerge asserts that a stale (no-op) apply —
// every field loses per-field LWW — does NOT re-broadcast, so a redundant relay is
// never enqueued (US-5.2 AC2 convergence: only a change is relayed).
func TestApply_ReBroadcastSkippedForNoOpMerge(t *testing.T) {
	env := newApplyEnv(t)
	ctx := context.Background()

	const ownerURL = "https://owner.example"
	markOwner(t, env, ownerURL)
	addPeer(t, env, "https://bob.example", model.FederationPermissionWrite)
	rebroadcastApplier(env, ownerURL)

	// Seed a high HLC for the field so the incoming (lower) one is stale.
	e1 := signedRelayEvent(env, "evt-high", "https://bob.example", map[string]events.Field{
		"title": {Value: "High", HLC: "00000000000900-0000-nodeB"},
	})
	if _, err := env.applier.Apply(ctx, e1, "https://bob.example"); err != nil {
		t.Fatalf("seed apply: %v", err)
	}

	// A second event with a LOWER HLC for the same field is stale → no-op merge.
	e2 := signedRelayEvent(env, "evt-stale", "https://bob.example", map[string]events.Field{
		"title": {Value: "Stale", HLC: "00000000000100-0000-nodeB"},
	})
	res, err := env.applier.Apply(ctx, e2, "https://bob.example")
	if err != nil {
		t.Fatalf("stale apply: %v", err)
	}
	if res.AppliedFields["title"] {
		t.Fatalf("stale field must not apply")
	}

	rows := outboxRows(t, env)
	if _, ok := rows["evt-stale"]; ok {
		t.Errorf("a no-op (stale) merge must NOT re-broadcast: outbox=%v", rows)
	}
}

// TestApply_ReBroadcastDedupOnRedelivery asserts that re-delivering the SAME event
// to the owner re-broadcasts it at most once (ON CONFLICT(event_id) DO NOTHING),
// so an at-least-once redelivery cannot duplicate the relayed row (NFR-2 dedup).
func TestApply_ReBroadcastDedupOnRedelivery(t *testing.T) {
	env := newApplyEnv(t)
	ctx := context.Background()

	const ownerURL = "https://owner.example"
	markOwner(t, env, ownerURL)
	addPeer(t, env, "https://bob.example", model.FederationPermissionWrite)
	rebroadcastApplier(env, ownerURL)

	e := signedRelayEvent(env, "evt-dup", "https://bob.example", map[string]events.Field{
		"title": {Value: "Once", HLC: "00000000000200-0000-nodeB"},
	})
	if _, err := env.applier.Apply(ctx, e, "https://bob.example"); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	// Redeliver the exact same event (idempotent apply, per-field CAS is a no-op the
	// second time; the re-broadcast must still not duplicate the outbox row).
	if _, err := env.applier.Apply(ctx, e, "https://bob.example"); err != nil {
		t.Fatalf("second apply: %v", err)
	}

	var count int
	if err := env.db.QueryRow(`SELECT COUNT(*) FROM federation_outbox WHERE event_id = ?`, "evt-dup").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("re-broadcast must dedup on event_id: got %d rows, want 1", count)
	}
}

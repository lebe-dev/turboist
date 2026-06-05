package fedtest

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/gofiber/fiber/v3"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/federation/hlc"
	"github.com/lebe-dev/turboist/internal/federation/outbox"
	"github.com/lebe-dev/turboist/internal/federation/peerkeys"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

// TestThreeInstance_OwnerHubReBroadcast drives the hub-and-spoke fan-out
// end-to-end across THREE real in-process instances (Federation v1 F5.1, US-5.2):
//
//	Alice (owner) ── federates with ── Bob (write peer) AND Carol (read peer)
//
// Bob edits the shared task and pushes the op=update event to Alice. Alice
// validates + applies it, and because she OWNS the project she re-broadcasts the
// relayed event to her OTHER peers — Carol receives it (US-5.1 AC3: a READ peer
// still receives fan-out) but the event is NEVER pushed back to Bob (US-5.2 AC2:
// no echo loop). All three converge on Bob's edit (US-5.2 AC3). The test also
// asserts Alice's outbox row for the relayed event is pre-stamped delivered-to-Bob
// (the echo guard) and never re-delivered to Bob.
func TestThreeInstance_OwnerHubReBroadcast(t *testing.T) {
	const aliceURL, bobURL, carolURL = "https://alice.example", "https://bob.example", "https://carol.example"

	pubKeys := map[string]string{}
	resolver := func(_ context.Context, instanceURL string) (*peerkeys.Instance, error) {
		return &peerkeys.Instance{InstanceURL: instanceURL, PublicKey: pubKeys[instanceURL], DisplayName: instanceURL}, nil
	}
	keyCache := peerkeys.NewCache(resolver)
	// Alice is the owner-hub: enable re-broadcast so she relays a peer's applied
	// edit to her other peers.
	alice := newInstance(t, aliceURL, keyCache, instanceOpt{reBroadcast: true})
	bob := newInstance(t, bobURL, keyCache)
	carol := newInstance(t, carolURL, keyCache)
	pubKeys[aliceURL] = alice.pubKeyB64
	pubKeys[bobURL] = bob.pubKeyB64
	pubKeys[carolURL] = carol.pubKeyB64

	ctx := context.Background()

	// Alice owns the project and federates with Bob (write) + Carol (read). Bob and
	// Carol each hold a joined mapping whose origin is Alice.
	projClientID := model.NewClientID()
	aliceProj := seedHubProject(t, alice, projClientID, aliceURL, true, aliceURL, model.FederationPermissionAdmin,
		hubPeer{url: bobURL, perm: model.FederationPermissionWrite},
		hubPeer{url: carolURL, perm: model.FederationPermissionRead})
	bobProj := seedFederatedProject(t, bob, projClientID, aliceURL, false, aliceURL, model.FederationPermissionWrite)
	carolProj := seedFederatedProject(t, carol, projClientID, aliceURL, false, aliceURL, model.FederationPermissionRead)

	// The shared task exists on all three with the SAME cross-instance client_id.
	taskClientID := model.NewClientID()
	cx := int64(1)
	seedTask(t, alice, aliceProj, cx, taskClientID, "Original")
	seedTask(t, bob, bobProj, cx, taskClientID, "Original")
	seedTask(t, carol, carolProj, cx, taskClientID, "Original")

	// One router knows all three apps; one publisher per sender instance.
	apps := map[string]*fiber.App{aliceURL: alice.app, bobURL: bob.app, carolURL: carol.app}
	bobPublisher := fedsvc.NewPublisher(bob.fedProjects, bob.keys, crypto.NewTokenCipher(cipherKey), newRoutingSender(apps), bobURL, nil)
	bobWorker := outbox.NewWorker(bob.store, bobPublisher, bobPublisher, nil)
	alicePublisher := fedsvc.NewPublisher(alice.fedProjects, alice.keys, crypto.NewTokenCipher(cipherKey), newRoutingSender(apps), aliceURL, nil)
	aliceWorker := outbox.NewWorker(alice.store, alicePublisher, alicePublisher, nil)

	actx, acancel := context.WithCancel(ctx)
	defer acancel()
	alice.queue.Start(actx)
	cctx, ccancel := context.WithCancel(ctx)
	defer ccancel()
	carol.queue.Start(cctx)

	// 1) Bob edits the shared task via the production mutator → op=update emitted to
	// Bob's outbox (Bob is a write peer, so he may originate writes).
	bobEmitter := fedsvc.NewEmitter(bob.db, bob.keys, crypto.NewTokenCipher(cipherKey),
		hlc.NewStore(bob.db, nodeID(t, bob.keys)), bobURL)
	var bobTaskID int64
	if err := bob.db.QueryRow(`SELECT id FROM tasks WHERE client_id = ?`, taskClientID).Scan(&bobTaskID); err != nil {
		t.Fatalf("resolve bob task id: %v", err)
	}
	bobTask, err := bob.tasks.Get(ctx, bobTaskID)
	if err != nil {
		t.Fatalf("get bob task: %v", err)
	}
	newTitle := "Bob's edit"
	if err := fedsvc.NewTaskMutator(bobEmitter, bob.tasks).Update(ctx, bobTask, repo.TaskUpdate{Title: &newTitle}); err != nil {
		t.Fatalf("bob update: %v", err)
	}

	// 2) Bob pushes to Alice (the owner-hub). Alice validates + applies + re-broadcasts.
	pushAndDrain(t, ctx, bob, bobProj, bobPublisher, aliceURL, bobWorker)
	if !converged(alice, func() bool { return taskExistsOnB(alice, taskClientID, newTitle) }) {
		t.Fatalf("Bob's edit did not converge on Alice (owner) within 5s")
	}

	// Alice must have RE-BROADCAST the relayed event to her outbox, pre-stamped
	// delivered-to-Bob (the echo guard, US-5.2 AC2).
	relayEventID := relayedEventID(t, alice, aliceProj)
	if relayEventID == "" {
		t.Fatalf("Alice did not re-broadcast Bob's event to her outbox")
	}
	if got := deliveredToOf(t, alice, relayEventID); !sliceHas(got, bobURL) {
		t.Errorf("re-broadcast delivered_to must contain origin %q (echo guard); got %v", bobURL, got)
	}

	// 3) Alice fans the re-broadcast out. It must reach Carol (US-5.1 AC3: a READ
	// peer receives fan-out) but NEVER be pushed back to Bob (US-5.2 AC2).
	carolBatch := pushHubToPeer(t, ctx, alice, aliceProj, alicePublisher, carolURL)
	if len(carolBatch) == 0 {
		t.Fatalf("Alice must fan the re-broadcast out to Carol (read peer, US-5.1 AC3)")
	}
	if err := aliceWorker.DrainOnce(ctx); err != nil {
		t.Fatalf("alice drain: %v", err)
	}
	if !converged(carol, func() bool { return taskExistsOnB(carol, taskClientID, newTitle) }) {
		t.Fatalf("Bob's edit did not converge on Carol via the owner hub within 5s")
	}

	// The relayed event must be pending for NOBODY toward Bob (the origin), proving
	// no echo back to the author.
	bobBatch := pendingForPeer(t, alice, aliceProj, bobURL)
	if sliceHas(bobBatch, relayEventID) {
		t.Errorf("re-broadcast must NOT be pushed back to its origin Bob (echo loop); pending=%v", bobBatch)
	}
}

// hubPeer is one (url, permission) peer mapping for the owner's project.
type hubPeer struct {
	url  string
	perm model.FederationPermission
}

// seedHubProject seeds the owner's project with its is_owner self-row AND one
// federated_projects mapping per peer, so the owner can fan a re-broadcast out to
// every peer (write + read alike — a read peer still receives, US-5.1 AC3).
func seedHubProject(t *testing.T, in *instance, projClientID, originURL string, isOwner bool, selfURL string, selfPerm model.FederationPermission, peers ...hubPeer) int64 {
	t.Helper()
	ctx := context.Background()
	p, err := in.projects.Create(ctx, repo.CreateProject{ContextID: 1, Title: "Shared", Color: "blue"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := in.db.Exec(`UPDATE projects SET client_id = ?, is_federated = 1 WHERE id = ?`, projClientID, p.ID); err != nil {
		t.Fatalf("federate project: %v", err)
	}
	// Owner self-row.
	if err := in.fedProjects.UpsertPeerRow(ctx, model.FederatedProject{
		LocalProjectID: p.ID, PeerInstanceURL: selfURL, IsOwner: isOwner,
		OriginInstanceURL: originURL, Permissions: selfPerm,
	}); err != nil {
		t.Fatalf("self row: %v", err)
	}
	for _, peer := range peers {
		if err := in.fedProjects.UpsertPeerRow(ctx, model.FederatedProject{
			LocalProjectID: p.ID, PeerInstanceURL: peer.url, IsOwner: false,
			OriginInstanceURL: originURL, Permissions: peer.perm,
		}); err != nil {
			t.Fatalf("peer row %s: %v", peer.url, err)
		}
	}
	return p.ID
}

// seedTask creates a local task with a fixed cross-instance client_id on an
// instance so all three nodes share the same entity identity.
func seedTask(t *testing.T, in *instance, projectID, ctxID int64, taskClientID, title string) int64 {
	t.Helper()
	ctx := context.Background()
	tk, err := in.tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &ctxID, ProjectID: &projectID},
		Title:     title,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := in.db.Exec(`UPDATE tasks SET client_id = ? WHERE id = ?`, taskClientID, tk.ID); err != nil {
		t.Fatalf("set task client_id: %v", err)
	}
	return tk.ID
}

// relayedEventID returns the event_id of the re-broadcast row Alice enqueued (a
// row with a non-empty delivered_to stamp, the echo guard), or "" if none.
func relayedEventID(t *testing.T, in *instance, projectID int64) string {
	t.Helper()
	var eventID string
	err := in.db.QueryRow(
		`SELECT event_id FROM federation_outbox WHERE local_project_id = ? AND delivered_to != '' AND delivered_to != '[]' ORDER BY id ASC LIMIT 1`,
		projectID).Scan(&eventID)
	if err == sql.ErrNoRows {
		return ""
	}
	if err != nil {
		t.Fatalf("relayed event id: %v", err)
	}
	return eventID
}

// deliveredToOf returns the delivered_to set recorded on an outbox row.
func deliveredToOf(t *testing.T, in *instance, eventID string) []string {
	t.Helper()
	var deliveredTo string
	if err := in.db.QueryRow(`SELECT delivered_to FROM federation_outbox WHERE event_id = ?`, eventID).Scan(&deliveredTo); err != nil {
		t.Fatalf("delivered_to: %v", err)
	}
	return decodeURLs(deliveredTo)
}

// pushHubToPeer reads the owner's pending events for peerURL and pushes them to
// that peer's app, returning the batch that was pending (so a test can assert a
// read peer actually received something).
func pushHubToPeer(t *testing.T, ctx context.Context, in *instance, projectID int64, publisher *fedsvc.Publisher, peerURL string) []string {
	t.Helper()
	batch := pendingForPeer(t, in, projectID, peerURL)
	payloads := pendingPayloadsForPeer(t, in, projectID, peerURL)
	if len(payloads) > 0 {
		if err := publisher.Push(ctx, peerURL, payloads); err != nil {
			t.Fatalf("push hub→%s: %v", peerURL, err)
		}
	}
	return batch
}

// pendingForPeer returns the event_ids in the owner's outbox NOT yet delivered to
// peerURL (the publisher's view of what it would push to that peer).
func pendingForPeer(t *testing.T, in *instance, projectID int64, peerURL string) []string {
	t.Helper()
	evs, err := in.store.ListUndeliveredForPeer(context.Background(), projectID, peerURL, 500)
	if err != nil {
		t.Fatalf("pending for %s: %v", peerURL, err)
	}
	out := make([]string, len(evs))
	for i, e := range evs {
		out[i] = e.EventID
	}
	return out
}

// pendingPayloadsForPeer returns the payloads for pendingForPeer (the actual push
// body bytes).
func pendingPayloadsForPeer(t *testing.T, in *instance, projectID int64, peerURL string) []string {
	t.Helper()
	evs, err := in.store.ListUndeliveredForPeer(context.Background(), projectID, peerURL, 500)
	if err != nil {
		t.Fatalf("pending payloads for %s: %v", peerURL, err)
	}
	out := make([]string, len(evs))
	for i, e := range evs {
		out[i] = e.Payload
	}
	return out
}

func decodeURLs(deliveredTo string) []string {
	if deliveredTo == "" || deliveredTo == "[]" {
		return nil
	}
	var urls []string
	if err := json.Unmarshal([]byte(deliveredTo), &urls); err != nil {
		return nil
	}
	return urls
}

func sliceHas(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

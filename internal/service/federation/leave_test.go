package federation_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/federation/hlc"
	"github.com/lebe-dev/turboist/internal/federation/store"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

// newLeaveSvc builds a federation service wired with the sync store (so the
// federation_leave event is enqueued into the outbox) and returns it alongside
// the repos + store needed to seed a joined project and inspect the outbox.
func newLeaveSvc(t *testing.T, instanceURL string) (*fedsvc.Service, *sql.DB, *repo.ProjectRepo, *repo.FederatedProjectRepo, *repo.FederatedInstanceRepo, *store.Store) {
	t.Helper()
	d, projects, fedProjects, keys := setup(t)
	seedContext(t, d)
	instances := repo.NewFederatedInstanceRepo(d)
	st := store.New(d)
	svc := fedsvc.NewService(d, projects, fedProjects, keys, repo.NewFederationInviteRepo(d), instances, crypto.NewTokenCipher(fedSvcKey), instanceURL).
		WithSyncStore(st)
	return svc, d, projects, fedProjects, instances, st
}

// TestLeaveProject_EnqueuesAndMarksLost asserts LeaveProject builds + delivers a
// signed federation_leave to the owner and marks the local copy lost (reason=left,
// editable local copy) (Federation v1 F5.5, US-6.3 AC1).
func TestLeaveProject_EnqueuesAndMarksLost(t *testing.T) {
	svc, d, projects, fp, instances, st := newLeaveSvc(t, "https://me.example")
	ctx := context.Background()
	pid := seedProject(t, projects)
	const ownerURL = "https://alice.example"
	const ownerCID = "owner-project-cid"
	markProjectFederated(t, d, pid)
	recent := time.Now().Add(-1 * time.Hour)
	seedOwnerMapping(t, fp, instances, pid, ownerURL, ownerCID, &recent)

	var sentPeer string
	var sent []string
	svc = svc.WithLeaveSender(func(_ context.Context, peerURL string, payloads []string) error {
		sentPeer = peerURL
		sent = payloads
		return nil
	})

	if err := svc.LeaveProject(ctx, pid); err != nil {
		t.Fatalf("LeaveProject: %v", err)
	}

	// AC1: the local copy is marked lost with reason=left (editable local copy).
	row, err := fp.Get(ctx, pid, ownerURL)
	if err != nil {
		t.Fatalf("get joiner row: %v", err)
	}
	if !row.Lost || row.LostReason != model.FederationLostLeft {
		t.Errorf("joiner lost state: got (lost=%v, reason=%q), want (true, left) (US-6.3 AC1)", row.Lost, row.LostReason)
	}

	// AC1: a signed federation_leave was sent to the owner, targeting the owner's
	// project client_id so the owner resolves it locally.
	if sentPeer != ownerURL {
		t.Errorf("leave delivered to: got %q, want %q", sentPeer, ownerURL)
	}
	if len(sent) != 1 {
		t.Fatalf("leave payloads: got %d, want 1", len(sent))
	}
	var evt events.Event
	if err := json.Unmarshal([]byte(sent[0]), &evt); err != nil {
		t.Fatalf("decode leave event: %v", err)
	}
	if evt.Op != events.OpLeave {
		t.Errorf("leave event op: got %q, want %q", evt.Op, events.OpLeave)
	}
	if evt.ProjectClientID != ownerCID || evt.EntityID != ownerCID {
		t.Errorf("leave event target: got project=%q entity=%q, want %q", evt.ProjectClientID, evt.EntityID, ownerCID)
	}
	if evt.Author != "https://me.example" || evt.OriginInstance != "https://me.example" {
		t.Errorf("leave event author/origin: got %q/%q, want https://me.example", evt.Author, evt.OriginInstance)
	}
	if evt.Signature == "" {
		t.Errorf("leave event must be signed")
	}

	// The event is durably recorded in the outbox and marked delivered to the owner
	// (pending count 0 after the direct push).
	n, err := st.PendingDeliveryCount(ctx, pid, ownerURL)
	if err != nil {
		t.Fatalf("pending count: %v", err)
	}
	if n != 0 {
		t.Errorf("leave pending after delivery: got %d, want 0", n)
	}
}

// TestLeaveProject_OfflineLeavesEventPending asserts that when the direct leave
// delivery fails (owner offline) the leave STILL takes effect locally (lost=left)
// and the event stays pending in the outbox to flush later (Federation v1 F5.5).
func TestLeaveProject_OfflineLeavesEventPending(t *testing.T) {
	svc, d, projects, fp, instances, st := newLeaveSvc(t, "https://me.example")
	ctx := context.Background()
	pid := seedProject(t, projects)
	const ownerURL = "https://alice.example"
	markProjectFederated(t, d, pid)
	recent := time.Now().Add(-1 * time.Hour)
	seedOwnerMapping(t, fp, instances, pid, ownerURL, "owner-cid", &recent)

	svc = svc.WithLeaveSender(func(_ context.Context, _ string, _ []string) error {
		return errors.New("owner offline")
	})

	if err := svc.LeaveProject(ctx, pid); err != nil {
		t.Fatalf("LeaveProject must succeed even when delivery fails: %v", err)
	}
	row, err := fp.Get(ctx, pid, ownerURL)
	if err != nil {
		t.Fatalf("get joiner row: %v", err)
	}
	if !row.Lost || row.LostReason != model.FederationLostLeft {
		t.Errorf("joiner lost after offline leave: got (lost=%v, reason=%q), want (true, left)", row.Lost, row.LostReason)
	}
	n, err := st.PendingDeliveryCount(ctx, pid, ownerURL)
	if err != nil {
		t.Fatalf("pending count: %v", err)
	}
	if n != 1 {
		t.Errorf("leave pending after failed delivery: got %d, want 1 (flushes later)", n)
	}
}

// TestLeaveProject_AlreadyLostIsNoOp asserts leaving a project that is already
// lost (e.g. already left, or revoked) is a no-op success — the lost reason is not
// overwritten and no second leave is enqueued (Federation v1 F5.5 — idempotent;
// leave-after-revoke is a no-op).
func TestLeaveProject_AlreadyLostIsNoOp(t *testing.T) {
	svc, d, projects, fp, instances, st := newLeaveSvc(t, "https://me.example")
	ctx := context.Background()
	pid := seedProject(t, projects)
	const ownerURL = "https://alice.example"
	markProjectFederated(t, d, pid)
	recent := time.Now().Add(-1 * time.Hour)
	seedOwnerMapping(t, fp, instances, pid, ownerURL, "owner-cid", &recent)
	// Pre-mark the copy lost with reason=revoked (the owner already revoked us).
	if _, err := fp.MarkLost(ctx, pid, ownerURL, model.FederationLostRevoked); err != nil {
		t.Fatalf("pre-mark lost: %v", err)
	}

	called := false
	svc = svc.WithLeaveSender(func(_ context.Context, _ string, _ []string) error {
		called = true
		return nil
	})

	if err := svc.LeaveProject(ctx, pid); err != nil {
		t.Fatalf("LeaveProject on already-lost: %v", err)
	}
	if called {
		t.Errorf("leave-after-revoke must be a no-op: no leave event should be sent (US-6.3)")
	}
	row, err := fp.Get(ctx, pid, ownerURL)
	if err != nil {
		t.Fatalf("get joiner row: %v", err)
	}
	// The reason stays revoked — leave does not overwrite a prior terminal reason.
	if row.LostReason != model.FederationLostRevoked {
		t.Errorf("lost reason after leave-on-revoked: got %q, want revoked (not overwritten)", row.LostReason)
	}
	n, err := st.PendingDeliveryCount(ctx, pid, ownerURL)
	if err != nil {
		t.Fatalf("pending count: %v", err)
	}
	if n != 0 {
		t.Errorf("no leave should be enqueued on an already-lost copy: pending got %d, want 0", n)
	}
}

// TestLeaveProject_NotJoined asserts leaving the owner's OWN project (or a
// non-federated project) reports ErrNotJoined (→ a 4xx in the handler): only a
// joined copy can be left (Federation v1 F5.5).
func TestLeaveProject_NotJoined(t *testing.T) {
	svc, _, projects, _, _, _ := newLeaveSvc(t, "https://me.example")
	ctx := context.Background()
	pid := seedProject(t, projects)
	// Enable federation as the OWNER (is_owner=1 self-row) — there is no joiner row.
	if _, err := svc.EnableForProject(ctx, pid); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if err := svc.LeaveProject(ctx, pid); !errors.Is(err, fedsvc.ErrNotJoined) {
		t.Fatalf("leaving an owned project must be ErrNotJoined, got %v", err)
	}
}

// TestLeaveProject_ProjectNotFound asserts leaving an unknown project reports
// ErrProjectNotFound.
func TestLeaveProject_ProjectNotFound(t *testing.T) {
	svc, _, _, _, _, _ := newLeaveSvc(t, "https://me.example")
	if err := svc.LeaveProject(context.Background(), 99999); !errors.Is(err, fedsvc.ErrProjectNotFound) {
		t.Fatalf("leaving an unknown project must be ErrProjectNotFound, got %v", err)
	}
}

// TestLeaveProject_StopsEmittingAfterLeave asserts that once a joined project is
// left (lost=left), a subsequent local mutation on a task in it emits NOTHING to
// the outbox — the copy is a plain editable LOCAL project now (US-6.3 AC3 "stop
// sending"). The emit gate keys on the lost flag, not merely is_federated.
func TestLeaveProject_StopsEmittingAfterLeave(t *testing.T) {
	svc, d, projects, fp, instances, st := newLeaveSvc(t, "https://me.example")
	ctx := context.Background()
	keys := repo.NewFederationKeysRepo(d)
	if _, err := keys.Ensure(ctx, crypto.NewTokenCipher(fedSvcKey), "me"); err != nil {
		t.Fatalf("ensure keys: %v", err)
	}
	pid := seedProject(t, projects)
	const ownerURL = "https://alice.example"
	markProjectFederated(t, d, pid)
	recent := time.Now().Add(-1 * time.Hour)
	seedOwnerMapping(t, fp, instances, pid, ownerURL, "owner-cid", &recent)

	// A task in the joined project (the entity a local edit would emit for).
	tasks := repo.NewTaskRepo(d, repo.NewTaskLabelsRepo(d))
	cx := int64(1)
	tk, err := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cx, ProjectID: &pid}, Title: "Joined task"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	svc = svc.WithLeaveSender(func(_ context.Context, _ string, _ []string) error { return nil })
	if err := svc.LeaveProject(ctx, pid); err != nil {
		t.Fatalf("LeaveProject: %v", err)
	}
	// After the leave there is exactly the leave control event in the outbox.
	leaveRows := outboxCount(t, d, pid)

	// Now edit a task locally AFTER leaving: it must NOT emit a new federated event.
	emitter := fedsvc.NewEmitter(d, keys, crypto.NewTokenCipher(fedSvcKey), hlc.NewStore(d, mustNodeID(t, keys)), "https://me.example")
	clientID := taskClientID(t, d, tk.ID)
	if err := emitter.EmitMutation(ctx, fedsvc.MutationSpec{
		LocalProjectID: pid,
		EntityType:     events.EntityTask,
		EntityID:       clientID,
		Op:             events.OpUpdate,
		Fields:         map[string]any{"title": "Edited after leaving"},
	}, func(tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, `UPDATE tasks SET title = ? WHERE id = ?`, "Edited after leaving", tk.ID)
		return e
	}); err != nil {
		t.Fatalf("emit after leave: %v", err)
	}

	if got := outboxCount(t, d, pid); got != leaveRows {
		t.Errorf("outbox after editing a LEFT project: got %d rows, want %d (no new emit — stop sending, US-6.3 AC3)", got, leaveRows)
	}
	_ = st
}

// markProjectFederated flips is_federated on a project without adding a self-row
// (the joiner-copy case — the only mapping is the is_owner=0 owner row).
func markProjectFederated(t *testing.T, d *sql.DB, pid int64) {
	t.Helper()
	if _, err := d.ExecContext(context.Background(), `UPDATE projects SET is_federated = 1 WHERE id = ?`, pid); err != nil {
		t.Fatalf("mark federated: %v", err)
	}
}

// seedOwnerMapping inserts the joiner-side is_owner=0 mapping to the origin owner
// plus its directory row (so the local copy is a joined federated project).
func seedOwnerMapping(t *testing.T, fp *repo.FederatedProjectRepo, instances *repo.FederatedInstanceRepo, pid int64, ownerURL, ownerCID string, lastContact *time.Time) {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := instances.Upsert(ctx, model.FederatedInstance{
		InstanceURL:   ownerURL,
		PublicKey:     "owner-pk",
		DisplayName:   "Owner Box",
		LastContactAt: lastContact,
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("seed owner instance: %v", err)
	}
	if err := fp.UpsertPeerRow(ctx, model.FederatedProject{
		LocalProjectID:    pid,
		PeerInstanceURL:   ownerURL,
		RemoteProjectID:   ownerCID,
		IsOwner:           false,
		OriginInstanceURL: ownerURL,
		Permissions:       model.FederationPermissionWrite,
		ProtocolVersion:   1,
		LastReceivedHLC:   "00000000000000-0000-node",
		JoinedAt:          now,
	}); err != nil {
		t.Fatalf("seed owner mapping: %v", err)
	}
}

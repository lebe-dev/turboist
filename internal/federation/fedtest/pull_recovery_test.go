// Federation v1 F4.1 — the pull/recovery leg of the two-instance harness. Where
// twoinstance_test asserts the PUSH path (A → B via POST /federation/events),
// this file asserts the PULL path (B catches up by GET-ing A's signed
// /federation/projects/:id/events from its last_received_hlc cursor, applying the
// returned events through the same inbox path push uses). It proves push/pull
// convergence (US-4.1 AC3) and at-least-once dedup across both transports (an
// event delivered by BOTH push and pull is applied once).
package fedtest

import (
	"context"
	"database/sql"
	"strconv"
	"testing"

	"github.com/gofiber/fiber/v3"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/federation/hlc"
	"github.com/lebe-dev/turboist/internal/federation/peerkeys"
	"github.com/lebe-dev/turboist/internal/federation/recovery"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

// TestTwoInstance_PullCatchesUpAfterGap drives the F4.1 recovery loop end-to-end:
// A emits TWO federated creates while B is "offline" (no push drain). B then runs
// one recovery pass — a signed GET pull from its last_received_hlc cursor through
// A's REAL signed pull endpoint — applies both events through its single inbox
// apply goroutine, and converges; its cursor advances so a second pass pulls
// nothing (US-4.1 AC1 resume-after-gap, AC2 pull-applies-+-advances-cursor, AC3
// push/pull convergence).
func TestTwoInstance_PullCatchesUpAfterGap(t *testing.T) {
	const aURL, bURL = "https://a.example", "https://b.example"

	pubKeys := map[string]string{}
	resolver := func(_ context.Context, instanceURL string) (*peerkeys.Instance, error) {
		return &peerkeys.Instance{InstanceURL: instanceURL, PublicKey: pubKeys[instanceURL], DisplayName: instanceURL}, nil
	}
	a := newInstance(t, aURL, peerkeys.NewCache(resolver))
	b := newInstance(t, bURL, peerkeys.NewCache(resolver))
	pubKeys[aURL] = a.pubKeyB64
	pubKeys[bURL] = b.pubKeyB64

	ctx := context.Background()

	projClientID := model.NewClientID()
	aProj := seedFederatedProject(t, a, projClientID, aURL, true, bURL, model.FederationPermissionWrite)
	bProj := seedFederatedProject(t, b, projClientID, aURL, false, aURL, model.FederationPermissionWrite)
	setRemoteProjectID(t, b, bProj, aProj)

	// A emits two creates (B is offline — never pushed).
	aMutator := newMutator(t, a, aURL)
	cx := int64(1)
	t1ClientID := model.NewClientID()
	t2ClientID := model.NewClientID()
	if _, err := aMutator.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cx, ProjectID: &aProj}, Title: "First"}, t1ClientID); err != nil {
		t.Fatalf("create t1 on A: %v", err)
	}
	if _, err := aMutator.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cx, ProjectID: &aProj}, Title: "Second"}, t2ClientID); err != nil {
		t.Fatalf("create t2 on A: %v", err)
	}

	// B's recovery loop pulls from A through the real signed pull endpoint.
	sender := newRoutingSender(map[string]*fiber.App{aURL: a.app, bURL: b.app})
	puller := fedsvc.NewPublisher(b.fedProjects, b.keys, crypto.NewTokenCipher(cipherKey), sender, bURL, nil)
	sink := recovery.NewStoreSink(b.store, b.queue)
	// Pull runs the SAME per-event validator the push handler uses (F3.2a): each
	// pulled event is authenticated end-to-end before any inbox/domain write.
	loop := recovery.NewLoop(b.store, puller, sink, nil).WithValidator(b.validator)

	bctx, bcancel := context.WithCancel(ctx)
	defer bcancel()
	b.queue.Start(bctx)

	if err := loop.RunOnce(ctx); err != nil {
		t.Fatalf("recovery pass 1: %v", err)
	}
	if !converged(b, func() bool {
		return taskExistsOnB(b, t1ClientID, "First") && taskExistsOnB(b, t2ClientID, "Second")
	}) {
		t.Fatalf("B did not catch up both tasks via pull within 5s")
	}

	// Cursor advanced: a second pass pulls nothing new (idempotent, no dup apply).
	bInboxBefore := countInbox(t, b.db)
	if err := loop.RunOnce(ctx); err != nil {
		t.Fatalf("recovery pass 2: %v", err)
	}
	if got := countInbox(t, b.db); got != bInboxBefore {
		t.Errorf("second pull recorded new inbox rows: got %d, want %d (cursor should have advanced)", got, bInboxBefore)
	}
}

// TestTwoInstance_PushAndPullSameEventAppliedOnce asserts an event delivered to B
// by BOTH push and pull is recorded once and applied once (NFR-2 dedup across
// transports; US-4.1 "duplicate via push+pull no-op").
func TestTwoInstance_PushAndPullSameEventAppliedOnce(t *testing.T) {
	const aURL, bURL = "https://a.example", "https://b.example"

	pubKeys := map[string]string{}
	resolver := func(_ context.Context, instanceURL string) (*peerkeys.Instance, error) {
		return &peerkeys.Instance{InstanceURL: instanceURL, PublicKey: pubKeys[instanceURL], DisplayName: instanceURL}, nil
	}
	a := newInstance(t, aURL, peerkeys.NewCache(resolver))
	b := newInstance(t, bURL, peerkeys.NewCache(resolver))
	pubKeys[aURL] = a.pubKeyB64
	pubKeys[bURL] = b.pubKeyB64

	ctx := context.Background()

	projClientID := model.NewClientID()
	aProj := seedFederatedProject(t, a, projClientID, aURL, true, bURL, model.FederationPermissionWrite)
	bProj := seedFederatedProject(t, b, projClientID, aURL, false, aURL, model.FederationPermissionWrite)
	setRemoteProjectID(t, b, bProj, aProj)

	aMutator := newMutator(t, a, aURL)
	cx := int64(1)
	tClientID := model.NewClientID()
	if _, err := aMutator.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cx, ProjectID: &aProj}, Title: "Once"}, tClientID); err != nil {
		t.Fatalf("create on A: %v", err)
	}

	sender := newRoutingSender(map[string]*fiber.App{aURL: a.app, bURL: b.app})
	publisher := fedsvc.NewPublisher(a.fedProjects, a.keys, crypto.NewTokenCipher(cipherKey), sender, aURL, nil)
	puller := fedsvc.NewPublisher(b.fedProjects, b.keys, crypto.NewTokenCipher(cipherKey), sender, bURL, nil)

	bctx, bcancel := context.WithCancel(ctx)
	defer bcancel()
	b.queue.Start(bctx)

	// PUSH path: A pushes the event to B.
	if err := publisher.Push(ctx, bURL, outboxPayloads(t, a, aProj)); err != nil {
		t.Fatalf("push to B: %v", err)
	}
	// PULL path: B also pulls the same event from A, through the SAME per-event
	// validator the push handler uses (F3.2a).
	loop := recovery.NewLoop(b.store, puller, recovery.NewStoreSink(b.store, b.queue), nil).WithValidator(b.validator)
	if err := loop.RunOnce(ctx); err != nil {
		t.Fatalf("recovery pass: %v", err)
	}

	if !converged(b, func() bool { return taskExistsOnB(b, tClientID, "Once") }) {
		t.Fatalf("B did not converge the task within 5s")
	}
	if got := countInbox(t, b.db); got != 1 {
		t.Errorf("inbox rows for the same event across push+pull: got %d, want 1 (deduped)", got)
	}
	if got := countTasksByClientID(t, b.db, tClientID); got != 1 {
		t.Errorf("applied tasks for the same event: got %d, want 1 (applied once)", got)
	}
}

// TestTwoInstance_ForgedPulledEventRejectedNoRows drives the F3.2a per-event
// authentication on the PULL transport end-to-end (the critical F4.1 finding):
// A's outbox payload is TAMPERED after signing (its title is rewritten without
// re-signing), so the relayed event's per-event Ed25519 signature no longer
// verifies. B pulls it through A's REAL signed pull endpoint and its REAL
// validator — and must reject it with ZERO inbox rows and apply nothing, the pull
// mirror of TestEvents_BadSignatureRejectedNoRows (US-7.2 AC1, R22/§404). The
// transport response signature authenticates only the relaying peer A, never the
// tampered event payload, so without the pull-path validator this forged event
// would be silently recorded and merged.
func TestTwoInstance_ForgedPulledEventRejectedNoRows(t *testing.T) {
	const aURL, bURL = "https://a.example", "https://b.example"

	pubKeys := map[string]string{}
	resolver := func(_ context.Context, instanceURL string) (*peerkeys.Instance, error) {
		return &peerkeys.Instance{InstanceURL: instanceURL, PublicKey: pubKeys[instanceURL], DisplayName: instanceURL}, nil
	}
	a := newInstance(t, aURL, peerkeys.NewCache(resolver))
	b := newInstance(t, bURL, peerkeys.NewCache(resolver))
	pubKeys[aURL] = a.pubKeyB64
	pubKeys[bURL] = b.pubKeyB64

	ctx := context.Background()

	projClientID := model.NewClientID()
	aProj := seedFederatedProject(t, a, projClientID, aURL, true, bURL, model.FederationPermissionWrite)
	bProj := seedFederatedProject(t, b, projClientID, aURL, false, aURL, model.FederationPermissionWrite)
	setRemoteProjectID(t, b, bProj, aProj)

	// A emits a legitimate create, then its stored outbox payload is TAMPERED so the
	// per-event signature no longer verifies (a malicious relay / corrupted log).
	aMutator := newMutator(t, a, aURL)
	cx := int64(1)
	tClientID := model.NewClientID()
	if _, err := aMutator.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cx, ProjectID: &aProj}, Title: "Original"}, tClientID); err != nil {
		t.Fatalf("create on A: %v", err)
	}
	tamperOutboxTitle(t, a, aProj, "Tampered")

	sender := newRoutingSender(map[string]*fiber.App{aURL: a.app, bURL: b.app})
	puller := fedsvc.NewPublisher(b.fedProjects, b.keys, crypto.NewTokenCipher(cipherKey), sender, bURL, nil)
	loop := recovery.NewLoop(b.store, puller, recovery.NewStoreSink(b.store, b.queue), nil).WithValidator(b.validator)

	bctx, bcancel := context.WithCancel(ctx)
	defer bcancel()
	b.queue.Start(bctx)

	if err := loop.RunOnce(ctx); err != nil {
		t.Fatalf("recovery pass: %v", err)
	}

	// The forged event must leave ZERO inbox rows and apply nothing on B.
	if got := countInbox(t, b.db); got != 0 {
		t.Errorf("forged pulled event recorded inbox rows: got %d, want 0", got)
	}
	if got := countTasksByClientID(t, b.db, tClientID); got != 0 {
		t.Errorf("forged pulled event applied a task: got %d, want 0", got)
	}
}

// tamperOutboxTitle rewrites the title field VALUE of the single event in a
// project's outbox without re-signing, breaking its per-event signature while
// leaving its HLCs (and thus its pull eligibility) intact.
func tamperOutboxTitle(t *testing.T, in *instance, projectID int64, newTitle string) {
	t.Helper()
	var eventID, payload string
	if err := in.db.QueryRow(
		`SELECT event_id, payload FROM federation_outbox WHERE local_project_id = ? ORDER BY id ASC LIMIT 1`,
		projectID).Scan(&eventID, &payload); err != nil {
		t.Fatalf("read outbox: %v", err)
	}
	var e events.Event
	if err := events.Unmarshal([]byte(payload), &e); err != nil {
		t.Fatalf("unmarshal outbox payload: %v", err)
	}
	f := e.Fields["title"]
	f.Value = newTitle // mutate the value but keep the original signature → verify fails
	e.Fields["title"] = f
	out, err := events.Marshal(e)
	if err != nil {
		t.Fatalf("marshal tampered payload: %v", err)
	}
	if _, err := in.db.Exec(`UPDATE federation_outbox SET payload = ? WHERE event_id = ?`, string(out), eventID); err != nil {
		t.Fatalf("update outbox payload: %v", err)
	}
}

// newMutator builds the production TaskMutator over an instance, used to emit
// federated creates that the recovery loop then pulls.
func newMutator(t *testing.T, in *instance, url string) *fedsvc.TaskMutator {
	t.Helper()
	emitter := fedsvc.NewEmitter(in.db, in.keys, crypto.NewTokenCipher(cipherKey),
		hlc.NewStore(in.db, nodeID(t, in.keys)), url)
	return fedsvc.NewTaskMutator(emitter, in.tasks)
}

// setRemoteProjectID points B's joined mapping row at A's project id so the pull
// URL targets A's project segment.
func setRemoteProjectID(t *testing.T, b *instance, bProj, aProj int64) {
	t.Helper()
	if _, err := b.db.Exec(
		`UPDATE federated_projects SET remote_project_id = ? WHERE local_project_id = ? AND is_owner = 0`,
		strconv.FormatInt(aProj, 10), bProj); err != nil {
		t.Fatalf("set remote_project_id on B: %v", err)
	}
}

func countInbox(t *testing.T, d *sql.DB) int {
	t.Helper()
	var n int
	if err := d.QueryRow(`SELECT COUNT(*) FROM federation_inbox`).Scan(&n); err != nil {
		t.Fatalf("count inbox: %v", err)
	}
	return n
}

func countTasksByClientID(t *testing.T, d *sql.DB, clientID string) int {
	t.Helper()
	var n int
	if err := d.QueryRow(`SELECT COUNT(*) FROM tasks WHERE client_id = ? AND deleted_at IS NULL`, clientID).Scan(&n); err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	return n
}

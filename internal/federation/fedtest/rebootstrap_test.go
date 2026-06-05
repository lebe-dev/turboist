// Federation v1 F4.2 — the 410-stale-pull CONSUME leg of the two-instance
// harness, end-to-end (the canonical place US-3.7 AC4's emit (F3.3) + consume
// (F4.2) halves are asserted together). B falls behind A's retention: A's outbox
// is pruned with a durable pruned-floor above B's cursor, so B's signed pull is
// answered 410 stale_pull. B's recovery loop CONSUMES the 410 — re-fetches A's
// snapshot through the real signed snapshot endpoint and overwrites its local
// project — without touching its unsent outbox, and without resurrecting a task A
// deleted while B was offline (US-4.2 AC2/AC3 + US-3.7 AC4 closed end-to-end).
package fedtest

import (
	"context"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/federation/nonce"
	"github.com/lebe-dev/turboist/internal/federation/peerkeys"
	"github.com/lebe-dev/turboist/internal/federation/recovery"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/handlers"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

// TestTwoInstance_StalePull410ReBootstrapsNoResurrection drives the whole F4.2
// path: A owns a federated project with two tasks; B joins (initial bootstrap).
// A deletes one task and the GC prunes A's outbox above B's cursor (B has been
// offline > retention). B's recovery pull is answered 410, B re-bootstraps from
// A's fresh snapshot, and: (1) B's project converges (the deleted task is gone
// and NOT resurrected); (2) B's UNSENT outbox event survives (R3); (3) the
// re-bootstrap marker (cutoff X) is stamped on B's mapping row (US-4.2 AC4).
func TestTwoInstance_StalePull410ReBootstrapsNoResurrection(t *testing.T) {
	const aURL, bURL = "https://a.example", "https://b.example"

	pubKeys := map[string]string{}
	resolver := func(_ context.Context, instanceURL string) (*peerkeys.Instance, error) {
		return &peerkeys.Instance{InstanceURL: instanceURL, PublicKey: pubKeys[instanceURL], DisplayName: instanceURL}, nil
	}
	a := newSnapshotInstance(t, aURL, peerkeys.NewCache(resolver))
	b := newSnapshotInstance(t, bURL, peerkeys.NewCache(resolver))
	pubKeys[aURL] = a.pubKeyB64
	pubKeys[bURL] = b.pubKeyB64

	ctx := context.Background()

	// A owns the project with two tasks: "Keep" (survives) and "Gone" (A deletes it
	// while B is offline). B holds the mapped joined project (initial bootstrap state).
	projClientID := model.NewClientID()
	aProj := seedFederatedProject(t, a, projClientID, aURL, true, bURL, model.FederationPermissionWrite)
	bProj := seedFederatedProject(t, b, projClientID, aURL, false, aURL, model.FederationPermissionWrite)
	setRemoteProjectID(t, b, bProj, aProj)

	keepClientID := model.NewClientID()
	goneClientID := model.NewClientID()
	seedTaskWithClientID(t, a, aProj, keepClientID, "Keep")
	goneLocalID := seedTaskWithClientID(t, a, aProj, goneClientID, "Gone")
	// B already has BOTH tasks locally (it bootstrapped earlier), including the one
	// A is about to delete — so the re-bootstrap must tombstone it, not keep it.
	seedTaskWithClientID(t, b, bProj, keepClientID, "Keep")
	seedTaskWithClientID(t, b, bProj, goneClientID, "Gone")
	// Set B's cursor to an OLD HLC so its pull is stale once A's outbox is pruned.
	if _, err := b.db.Exec(`UPDATE federated_projects SET last_received_hlc = '00000000000100-0000-nodeA' WHERE local_project_id = ? AND is_owner = 0`, bProj); err != nil {
		t.Fatalf("set B cursor: %v", err)
	}

	// A deletes "Gone" and records a tombstone field HLC, then A's outbox is pruned
	// to a durable floor ABOVE B's cursor (B fell behind retention). With no
	// retained outbox + a pruned floor > B's since_hlc, A's pull endpoint returns 410.
	if _, err := a.db.Exec(`UPDATE tasks SET deleted_at = '2026-06-02T00:00:00.000Z' WHERE id = ?`, goneLocalID); err != nil {
		t.Fatalf("A delete Gone: %v", err)
	}
	if _, err := a.db.Exec(
		`INSERT INTO entity_field_hlc (entity_type, entity_id, field_name, hlc) VALUES ('task', ?, '_deleted', '00000000050000-0000-nodeA')`,
		goneClientID); err != nil {
		t.Fatalf("A tombstone hlc: %v", err)
	}
	if _, err := a.store.AdvancePrunedFloor(ctx, aProj, "00000000050000-0000-nodeA", "2026-06-02T00:00:00.000Z"); err != nil {
		t.Fatalf("A prune floor: %v", err)
	}

	// B has an UNSENT local outbox event (a local edit awaiting delivery).
	if _, err := b.db.Exec(
		`INSERT INTO federation_outbox (event_id, local_project_id, payload, delivered_to, created_at)
		 VALUES ('b-unsent', ?, '{}', '', '2026-06-02T12:00:00.000Z')`, bProj); err != nil {
		t.Fatalf("seed B unsent outbox: %v", err)
	}

	// B's recovery loop: pull from A (→ 410) and consume it (→ re-bootstrap). The
	// consumer re-fetches A's snapshot through A's real signed snapshot endpoint.
	sender := newRoutingSender(map[string]*fiber.App{aURL: a.app, bURL: b.app})
	puller := fedsvc.NewPublisher(b.fedProjects, b.keys, crypto.NewTokenCipher(cipherKey), sender, bURL, nil)
	bSvc := newSnapshotService(t, b, bURL, sender)
	consumer := fedsvc.NewReBootstrapConsumer(bSvc, nil, nil)
	loop := recovery.NewLoop(b.store, puller, recovery.NewStoreSink(b.store, b.queue), nil).
		WithValidator(b.validator).
		WithStaleConsumer(consumer)

	bctx, bcancel := context.WithCancel(ctx)
	defer bcancel()
	b.queue.Start(bctx)

	if err := loop.RunOnce(ctx); err != nil {
		t.Fatalf("recovery pass (consume 410): %v", err)
	}

	// (1) "Gone" is tombstoned on B and NOT resurrected; "Keep" survives.
	if deletedAtOnB(t, b, goneClientID) == "" {
		t.Errorf("owner-deleted task resurrected on B after re-bootstrap (still live)")
	}
	if !taskExistsOnB(b, keepClientID, "Keep") {
		t.Errorf("surviving task missing on B after re-bootstrap")
	}

	// (2) B's unsent outbox event survived the re-bootstrap (R3 — the headline bug).
	var unsent int
	if err := b.db.QueryRow(`SELECT COUNT(*) FROM federation_outbox WHERE event_id = 'b-unsent'`).Scan(&unsent); err != nil {
		t.Fatalf("count unsent: %v", err)
	}
	if unsent != 1 {
		t.Errorf("B's unsent outbox event cleared by re-bootstrap: got %d, want 1 (R3)", unsent)
	}

	// (3) The re-bootstrap marker (cutoff X) is stamped on B's mapping row so the
	// joiner UI can render the re-sync banner naming X (US-4.2 AC4).
	var cutoffHLC, rebootAt *string
	if err := b.db.QueryRow(
		`SELECT rebootstrap_cutoff_hlc, rebootstrapped_at FROM federated_projects WHERE local_project_id = ? AND is_owner = 0`,
		bProj).Scan(&cutoffHLC, &rebootAt); err != nil {
		t.Fatalf("read marker: %v", err)
	}
	if cutoffHLC == nil || *cutoffHLC == "" {
		t.Errorf("re-bootstrap cutoff HLC not stamped (US-4.2 AC4)")
	}
	if rebootAt == nil || *rebootAt == "" {
		t.Errorf("re-bootstrap wall-clock cutoff X not stamped (US-4.2 AC4 — must be a real persisted value)")
	}
}

// newSnapshotInstance builds an instance whose owner-side signed group ALSO serves
// the snapshot endpoint backed by the federation service (so a peer can re-fetch
// a snapshot). It reuses the standard instance wiring and overlays the service.
func newSnapshotInstance(t *testing.T, url string, peerKeys *peerkeys.Cache) *instance {
	t.Helper()
	in := newInstance(t, url, peerKeys)
	cipher := crypto.NewTokenCipher(cipherKey)
	svc := fedsvc.NewService(
		in.db, in.projects, in.fedProjects, in.keys,
		repo.NewFederationInviteRepo(in.db), repo.NewFederatedInstanceRepo(in.db), cipher, url,
	)
	svc.WithSnapshotDeps(in.tasks, repo.NewProjectSectionRepo(in.db), repo.NewContextRepo(in.db), repo.NewFederationSnapshotRepo(in.db))

	// Rebuild the app so the signed group's snapshot route is backed by the service
	// AND the events deps (membership for the token-less re-bootstrap).
	fedHandler := handlers.NewFederationHandler(in.keys, cipher, url).
		WithService(svc).
		WithEventsDeps(handlers.FederationEventsDeps{
			Store: in.store, Validator: in.validator, Queue: in.queue, Projects: in.fedProjects, BaseURL: url,
		})
	app := httpapi.NewApp(httpapi.Deps{})
	signed := app.Group("/federation", httpapi.HTTPSignatureMiddleware(httpapi.FederationSignatureDeps{
		// No-op TRANSPORT nonce cache: the in-process app.Test() transport can re-serve
		// the SAME signed request and trip a spurious federation_replay (401) on the
		// re-bootstrap handshake/snapshot, a pure harness artifact a real one-shot HTTP
		// client never produces. This re-bootstrap test asserts DOMAIN snapshot
		// overwrite behavior, not transport anti-replay — which is owned by F0.3's
		// dedicated single-request HTTPSignatureMiddleware tests.
		Nonces:   nonce.NewDisabledCache(),
		PeerKeys: peerKeys,
	}))
	fedHandler.RegisterSigned(signed)
	in.app = app
	return in
}

// newSnapshotService builds the joiner-side federation service used to drive the
// re-bootstrap consume (fetch + overwrite). It shares the instance's repos + DB.
func newSnapshotService(t *testing.T, in *instance, url string, sender fedsvc.HandshakeSender) *fedsvc.Service {
	t.Helper()
	cipher := crypto.NewTokenCipher(cipherKey)
	svc := fedsvc.NewService(
		in.db, in.projects, in.fedProjects, in.keys,
		repo.NewFederationInviteRepo(in.db), repo.NewFederatedInstanceRepo(in.db), cipher, url,
	)
	svc.WithSnapshotDeps(in.tasks, repo.NewProjectSectionRepo(in.db), repo.NewContextRepo(in.db), repo.NewFederationSnapshotRepo(in.db))
	fetch := func(_ context.Context, instanceURL string) (*peerkeys.Instance, error) {
		return &peerkeys.Instance{InstanceURL: instanceURL, DisplayName: instanceURL}, nil
	}
	svc.WithJoinDeps(sender, fetch, peerkeys.NewCache(fetch), time.Now)
	return svc
}

// seedTaskWithClientID creates a task in a project carrying a fixed cross-instance
// client_id, returning its local int64 id.
func seedTaskWithClientID(t *testing.T, in *instance, projectID int64, clientID, title string) int64 {
	t.Helper()
	ctx := context.Background()
	cx := int64(1)
	tk, err := in.tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cx, ProjectID: &projectID}, Title: title})
	if err != nil {
		t.Fatalf("create task %q: %v", title, err)
	}
	if _, err := in.db.Exec(`UPDATE tasks SET client_id = ? WHERE id = ?`, clientID, tk.ID); err != nil {
		t.Fatalf("set task client_id: %v", err)
	}
	return tk.ID
}

// deletedAtOnB reads a task's deleted_at by cross-instance client_id on B
// (empty string when live or absent).
func deletedAtOnB(t *testing.T, b *instance, clientID string) string {
	t.Helper()
	var deletedAt *string
	if err := b.db.QueryRow(`SELECT deleted_at FROM tasks WHERE client_id = ?`, clientID).Scan(&deletedAt); err != nil {
		return ""
	}
	if deletedAt == nil {
		return ""
	}
	return *deletedAt
}

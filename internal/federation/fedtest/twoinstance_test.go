// Package fedtest holds in-process multi-instance federation integration tests
// (Federation v1). This F3.2 slice asserts the end-to-end push path — emit →
// outbox → signed POST /federation/events → per-event validation → inbox dedup
// → single-goroutine apply → converged peer state — runs through the REAL
// signature middleware and handlers, with a deterministic synchronous outbox
// drain. F7.1 expands this into the full two/three-instance harness.
package fedtest

import (
	"bytes"
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/federation/hlc"
	"github.com/lebe-dev/turboist/internal/federation/inbox"
	"github.com/lebe-dev/turboist/internal/federation/nonce"
	"github.com/lebe-dev/turboist/internal/federation/outbox"
	"github.com/lebe-dev/turboist/internal/federation/peerkeys"
	"github.com/lebe-dev/turboist/internal/federation/store"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/handlers"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

const cipherKey = "fedtest-cipher-key-needs-32-bytes-min!!"

// instance is one full in-process federation node.
type instance struct {
	url         string
	db          *sql.DB
	store       *store.Store
	keys        *repo.FederationKeysRepo
	fedProjects *repo.FederatedProjectRepo
	projects    *repo.ProjectRepo
	tasks       *repo.TaskRepo
	app         *fiber.App
	queue       *inbox.Queue
	validator   *inbox.Validator
	pubKeyB64   string
}

// instanceOpt tweaks an instance at construction (e.g. enabling the F5.1 owner-hub
// re-broadcast on the owner node before its inbox queue + handler are wired).
type instanceOpt struct {
	// reBroadcast enables owner-hub re-broadcast on this instance's applier
	// (Federation v1 F5.1, US-5.2 AC2): an apply that changes an entity of a project
	// this instance owns is re-enqueued to the outbox for fan-out to the OTHER peers.
	reBroadcast bool
}

func newInstance(t *testing.T, url string, peerKeys *peerkeys.Cache, opts ...instanceOpt) *instance {
	t.Helper()
	var opt instanceOpt
	if len(opts) > 0 {
		opt = opts[0]
	}
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "node.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.RunMigrations(context.Background(), d); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx := context.Background()
	cipher := crypto.NewTokenCipher(cipherKey)
	keys := repo.NewFederationKeysRepo(d)
	fk, err := keys.Ensure(ctx, cipher, url)
	if err != nil {
		t.Fatalf("ensure keys: %v", err)
	}

	if _, err := d.Exec(
		`INSERT INTO contexts (id, name, color, client_id, created_at, updated_at)
		 VALUES (1, 'c', 'blue', ?, '2024-01-01T00:00:00.000Z', '2024-01-01T00:00:00.000Z')`,
		model.NewClientID()); err != nil {
		t.Fatalf("ctx: %v", err)
	}

	projects := repo.NewProjectRepo(d, repo.NewProjectLabelsRepo(d))
	tasks := repo.NewTaskRepo(d, repo.NewTaskLabelsRepo(d))
	fedProjects := repo.NewFederatedProjectRepo(d)
	st := store.New(d)

	in := &instance{
		url: url, db: d, store: st, keys: keys, fedProjects: fedProjects,
		projects: projects, tasks: tasks, pubKeyB64: fk.PublicKey,
	}

	// Inbound signed group with the REAL signature middleware + REAL handler. The
	// SAME per-event validator backs both the push handler and the pull/recovery
	// loop (the pull tests build their Loop with in.validator), so both transports
	// authenticate each event end-to-end through one seam.
	applier := inbox.NewApplier(d, tasks, projects, repo.NewProjectSectionRepo(d), fedProjects, st)
	if opt.reBroadcast {
		// Owner-hub re-broadcast (Federation v1 F5.1, US-5.2 AC2): identify this
		// instance by its own URL so the applier knows which is_owner=1 self-row marks
		// ownership, and write relayed events to the same store. The commit-ping is a
		// no-op here — the test drives the owner's outbox drain synchronously.
		applier = applier.WithReBroadcast(st, url, func() {})
	}
	in.queue = inbox.NewQueue(applier, nil, inbox.NewStoreRecoverer(st), nil)
	in.validator = inbox.NewDBValidator(d, fedProjects, peerKeys, nil)
	fedHandler := handlers.NewFederationHandler(keys, cipher, url).
		WithEventsDeps(handlers.FederationEventsDeps{
			Store: st, Validator: in.validator, Queue: in.queue, Projects: fedProjects,
		})

	app := httpapi.NewApp(httpapi.Deps{})
	signed := app.Group("/federation", httpapi.HTTPSignatureMiddleware(httpapi.FederationSignatureDeps{
		// No-op TRANSPORT nonce cache: the in-process app.Test() transport can re-serve
		// the SAME signed request and trip a spurious federation_replay (401) on the
		// Join/push/pull, a pure harness artifact a real one-shot HTTP client never
		// produces. These two-instance tests assert DOMAIN behavior (convergence,
		// stale-pull re-bootstrap, dedup-by-event_id), not transport anti-replay — which
		// is owned by F0.3's dedicated single-request HTTPSignatureMiddleware tests.
		Nonces:   nonce.NewDisabledCache(),
		PeerKeys: peerKeys,
	}))
	fedHandler.RegisterSigned(signed)
	in.app = app
	return in
}

// TestTwoInstance_PushConvergesUnder5s drives the full push path between two real
// in-process instances: A emits a federated create, the outbox publisher pushes
// it through B's real signature middleware + events handler, B validates +
// dedups + applies, and the task converges on B — under the NFR-1.1 5s budget
// (US-3.1 AC1, US-3.2 AC2).
func TestTwoInstance_PushConvergesUnder5s(t *testing.T) {
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
	seedFederatedProject(t, b, projClientID, aURL, false, aURL, model.FederationPermissionWrite)

	// A creates a task with a fixed cross-instance client_id and emits a create.
	taskClientID := model.NewClientID()
	cx := int64(1)
	aTask, err := a.tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cx, ProjectID: &aProj}, Title: "Shared task"})
	if err != nil {
		t.Fatalf("create task on A: %v", err)
	}
	if _, err := a.db.Exec(`UPDATE tasks SET client_id = ? WHERE id = ?`, taskClientID, aTask.ID); err != nil {
		t.Fatalf("set task client_id: %v", err)
	}
	emitCreate(t, a, aProj, projClientID, taskClientID, "Shared task")

	// Route the publisher's POST to B's real app.
	sender := newRoutingSender(map[string]*fiber.App{aURL: a.app, bURL: b.app})
	publisher := fedsvc.NewPublisher(a.fedProjects, a.keys, crypto.NewTokenCipher(cipherKey), sender, aURL, nil)
	worker := outbox.NewWorker(a.store, publisher, publisher, nil)

	bctx, bcancel := context.WithCancel(ctx)
	defer bcancel()
	b.queue.Start(bctx)

	// Direct push first so a transport/validation failure surfaces clearly.
	batch := outboxPayloads(t, a, aProj)
	if err := publisher.Push(ctx, bURL, batch); err != nil {
		t.Fatalf("direct push to B failed: %v", err)
	}
	var bInbox int
	if err := b.db.QueryRow(`SELECT COUNT(*) FROM federation_inbox`).Scan(&bInbox); err != nil {
		t.Fatalf("b inbox count: %v", err)
	}
	if bInbox == 0 {
		t.Fatalf("push accepted but B recorded no inbox row")
	}

	start := time.Now()
	if err := worker.DrainOnce(ctx); err != nil {
		t.Fatalf("drain: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if taskExistsOnB(b, taskClientID, "Shared task") {
			if elapsed := time.Since(start); elapsed > 5*time.Second {
				t.Fatalf("converged but over budget: %s", elapsed)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("task did not converge on B within 5s")
}

// TestTwoInstance_CreateThenUpdateConverges drives the full push path for BOTH a
// create AND a subsequent update between two real in-process instances, proving
// create+edit propagation end-to-end (not just delete): A creates a task via the
// production TaskMutator (op=create), it converges on B; A then renames the task
// via the TaskMutator (op=update), and the new title converges on B — each within
// the NFR-1.1 5s budget (US-3.1 AC1 create-visible, US-3.2 AC1 federated edits
// propagate).
func TestTwoInstance_CreateThenUpdateConverges(t *testing.T) {
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
	seedFederatedProject(t, b, projClientID, aURL, false, aURL, model.FederationPermissionWrite)

	// The production TaskMutator on A drives both the create and the update.
	emitter := fedsvc.NewEmitter(a.db, a.keys, crypto.NewTokenCipher(cipherKey),
		hlc.NewStore(a.db, nodeID(t, a.keys)), aURL)
	mutator := fedsvc.NewTaskMutator(emitter, a.tasks)

	sender := newRoutingSender(map[string]*fiber.App{aURL: a.app, bURL: b.app})
	publisher := fedsvc.NewPublisher(a.fedProjects, a.keys, crypto.NewTokenCipher(cipherKey), sender, aURL, nil)
	worker := outbox.NewWorker(a.store, publisher, publisher, nil)

	bctx, bcancel := context.WithCancel(ctx)
	defer bcancel()
	b.queue.Start(bctx)

	// 1) CREATE on A via the mutator → op=create event emitted to the outbox.
	cx := int64(1)
	taskClientID := model.NewClientID()
	taskID, err := mutator.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cx, ProjectID: &aProj},
		Title:     "Shared task",
	}, taskClientID)
	if err != nil {
		t.Fatalf("mutator create on A: %v", err)
	}

	pushAndDrain(t, ctx, a, aProj, publisher, bURL, worker)
	if !converged(b, func() bool { return taskExistsOnB(b, taskClientID, "Shared task") }) {
		t.Fatalf("create did not converge on B within 5s")
	}

	// 2) UPDATE on A via the mutator → op=update event with the changed title.
	task, err := a.tasks.Get(ctx, taskID)
	if err != nil {
		t.Fatalf("get task on A: %v", err)
	}
	newTitle := "Renamed shared task"
	if err := mutator.Update(ctx, task, repo.TaskUpdate{Title: &newTitle}); err != nil {
		t.Fatalf("mutator update on A: %v", err)
	}

	pushAndDrain(t, ctx, a, aProj, publisher, bURL, worker)
	if !converged(b, func() bool { return taskExistsOnB(b, taskClientID, newTitle) }) {
		t.Fatalf("update did not converge on B within 5s (title still stale)")
	}
}

// TestTwoInstance_CompleteConverges drives the task-complete emit path end-to-end
// (TASK A): A creates a task via the production TaskMutator, it converges on B; A
// then COMPLETES the task via the production CompleteService wired to a
// CompleteMutator (op=update{status:completed}), and B converges to status=completed
// within the NFR-1.1 5s budget (US-3.2 AC1 — completion, the app's core action,
// propagates).
func TestTwoInstance_CompleteConverges(t *testing.T) {
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
	seedFederatedProject(t, b, projClientID, aURL, false, aURL, model.FederationPermissionWrite)

	emitter := fedsvc.NewEmitter(a.db, a.keys, crypto.NewTokenCipher(cipherKey),
		hlc.NewStore(a.db, nodeID(t, a.keys)), aURL)
	taskMutator := fedsvc.NewTaskMutator(emitter, a.tasks)
	completeSvc := service.NewCompleteService(a.tasks, a.projects, repo.NewUserRepo(a.db)).
		WithFederation(fedsvc.NewCompleteMutator(emitter, a.tasks))
	if _, err := repo.NewUserRepo(a.db).Create(ctx, "admin", "h"); err != nil {
		t.Fatalf("seed user A: %v", err)
	}

	sender := newRoutingSender(map[string]*fiber.App{aURL: a.app, bURL: b.app})
	publisher := fedsvc.NewPublisher(a.fedProjects, a.keys, crypto.NewTokenCipher(cipherKey), sender, aURL, nil)
	worker := outbox.NewWorker(a.store, publisher, publisher, nil)

	bctx, bcancel := context.WithCancel(ctx)
	defer bcancel()
	b.queue.Start(bctx)

	cx := int64(1)
	taskClientID := model.NewClientID()
	taskID, err := taskMutator.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cx, ProjectID: &aProj},
		Title:     "Complete me",
	}, taskClientID)
	if err != nil {
		t.Fatalf("create on A: %v", err)
	}
	pushAndDrain(t, ctx, a, aProj, publisher, bURL, worker)
	if !converged(b, func() bool { return taskExistsOnB(b, taskClientID, "Complete me") }) {
		t.Fatalf("create did not converge on B within 5s")
	}

	if _, err := completeSvc.Complete(ctx, taskID); err != nil {
		t.Fatalf("complete on A: %v", err)
	}
	pushAndDrain(t, ctx, a, aProj, publisher, bURL, worker)
	if !converged(b, func() bool { return taskStatusOnB(b, taskClientID) == "completed" }) {
		t.Fatalf("complete did not converge on B within 5s (status still %q)", taskStatusOnB(b, taskClientID))
	}
}

// taskStatusOnB reads a task's status by cross-instance client_id on B.
func taskStatusOnB(b *instance, clientID string) string {
	var status string
	if err := b.db.QueryRow(`SELECT status FROM tasks WHERE client_id = ? AND deleted_at IS NULL`, clientID).Scan(&status); err != nil {
		return ""
	}
	return status
}

// TestTwoInstance_ProjectArchiveConverges drives the project-status emit path
// end-to-end: A archives a federated project via the production ProjectMutator
// (op=update {status}), the outbox publisher pushes it to B, B validates +
// applies, and B's project converges to status=archived within the NFR-1.1 5s
// budget (US-3.2 AC1, TASK B).
func TestTwoInstance_ProjectArchiveConverges(t *testing.T) {
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

	emitter := fedsvc.NewEmitter(a.db, a.keys, crypto.NewTokenCipher(cipherKey),
		hlc.NewStore(a.db, nodeID(t, a.keys)), aURL)
	mutator := fedsvc.NewProjectMutator(emitter, a.projects)

	sender := newRoutingSender(map[string]*fiber.App{aURL: a.app, bURL: b.app})
	publisher := fedsvc.NewPublisher(a.fedProjects, a.keys, crypto.NewTokenCipher(cipherKey), sender, aURL, nil)
	worker := outbox.NewWorker(a.store, publisher, publisher, nil)

	bctx, bcancel := context.WithCancel(ctx)
	defer bcancel()
	b.queue.Start(bctx)

	if err := mutator.UpdateStatus(ctx, aProj, model.ProjectStatusArchived); err != nil {
		t.Fatalf("archive on A: %v", err)
	}

	pushAndDrain(t, ctx, a, aProj, publisher, bURL, worker)
	if !converged(b, func() bool { return projectStatusOnB(b, bProj) == "archived" }) {
		t.Fatalf("archive did not converge on B within 5s (status still %q)", projectStatusOnB(b, bProj))
	}
}

// projectStatusOnB reads a project's status by local id on B.
func projectStatusOnB(b *instance, projectID int64) string {
	var status string
	if err := b.db.QueryRow(`SELECT status FROM projects WHERE id = ?`, projectID).Scan(&status); err != nil {
		return ""
	}
	return status
}

// pushAndDrain pushes A's project outbox batch to B's app and drains B's apply
// queue once, surfacing a transport/validation failure clearly.
func pushAndDrain(t *testing.T, ctx context.Context, a *instance, projectID int64, publisher *fedsvc.Publisher, bURL string, worker *outbox.Worker) {
	t.Helper()
	batch := outboxPayloads(t, a, projectID)
	if err := publisher.Push(ctx, bURL, batch); err != nil {
		t.Fatalf("push to B failed: %v", err)
	}
	if err := worker.DrainOnce(ctx); err != nil {
		t.Fatalf("drain: %v", err)
	}
}

// converged polls cond until true or the 5s NFR-1.1 budget elapses.
func converged(_ *instance, cond func() bool) bool {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}

// seedFederatedProject creates a local project carrying projClientID, marks it
// federated, and inserts the (project, peer) mapping row.
func seedFederatedProject(t *testing.T, in *instance, projClientID, originURL string, isOwner bool, peerURL string, perm model.FederationPermission) int64 {
	t.Helper()
	ctx := context.Background()
	p, err := in.projects.Create(ctx, repo.CreateProject{ContextID: 1, Title: "Shared", Color: "blue"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := in.db.Exec(`UPDATE projects SET client_id = ?, is_federated = 1 WHERE id = ?`, projClientID, p.ID); err != nil {
		t.Fatalf("federate project: %v", err)
	}
	if err := in.fedProjects.UpsertPeerRow(ctx, model.FederatedProject{
		LocalProjectID:    p.ID,
		PeerInstanceURL:   peerURL,
		IsOwner:           isOwner,
		OriginInstanceURL: originURL,
		Permissions:       perm,
	}); err != nil {
		t.Fatalf("peer row: %v", err)
	}
	return p.ID
}

func emitCreate(t *testing.T, in *instance, projectID int64, projClientID, entityID, title string) {
	t.Helper()
	emitter := fedsvc.NewEmitter(in.db, in.keys, crypto.NewTokenCipher(cipherKey),
		hlc.NewStore(in.db, nodeID(t, in.keys)), in.url)
	err := emitter.EmitMutation(context.Background(), fedsvc.MutationSpec{
		LocalProjectID: projectID,
		EntityType:     events.EntityTask,
		EntityID:       entityID,
		Op:             events.OpCreate,
		Fields:         map[string]any{"title": title},
	}, func(tx *sql.Tx) error { return nil })
	if err != nil {
		t.Fatalf("emit create: %v", err)
	}
}

// outboxPayloads reads a project's outbox event payloads for a direct push.
func outboxPayloads(t *testing.T, in *instance, projectID int64) []string {
	t.Helper()
	rows, err := in.db.Query(`SELECT payload FROM federation_outbox WHERE local_project_id = ? ORDER BY id ASC`, projectID)
	if err != nil {
		t.Fatalf("read outbox: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			t.Fatalf("scan outbox: %v", err)
		}
		out = append(out, p)
	}
	return out
}

func nodeID(t *testing.T, keys *repo.FederationKeysRepo) string {
	t.Helper()
	k, err := keys.Get(context.Background())
	if err != nil {
		t.Fatalf("get keys: %v", err)
	}
	return k.NodeID
}

func taskExistsOnB(b *instance, clientID, title string) bool {
	var got string
	err := b.db.QueryRow(`SELECT title FROM tasks WHERE client_id = ? AND deleted_at IS NULL`, clientID).Scan(&got)
	if err != nil {
		return false
	}
	return got == title
}

// routingSender routes an outbound SignedRequest to the in-process Fiber app for
// the request's host, exercising the real signature middleware + handler.
type routingSender struct {
	apps map[string]*fiber.App
}

func newRoutingSender(apps map[string]*fiber.App) *routingSender {
	return &routingSender{apps: apps}
}

func (s *routingSender) Send(_ context.Context, sr fedsvc.SignedRequest) (*fedsvc.SignedResponse, error) {
	app := s.apps[hostPrefix(sr.URL)]
	if app == nil {
		return &fedsvc.SignedResponse{StatusCode: 502}, nil
	}
	var body *bytes.Reader
	if sr.Body != nil {
		body = bytes.NewReader(sr.Body)
	} else {
		body = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(sr.Method, sr.URL, body)
	if sr.Body != nil {
		req.ContentLength = int64(len(sr.Body))
	}
	for k, v := range sr.Headers {
		req.Header.Set(k, v)
	}
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	return &fedsvc.SignedResponse{StatusCode: resp.StatusCode, Body: readAll(resp)}, nil
}

// hostPrefix returns the scheme://host of a URL (the routing key).
func hostPrefix(raw string) string {
	idx := strings.Index(raw, "://")
	if idx < 0 {
		return raw
	}
	rest := raw[idx+3:]
	if slash := strings.IndexByte(rest, '/'); slash >= 0 {
		return raw[:idx+3+slash]
	}
	return raw
}

func readAll(resp *http.Response) []byte {
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	return buf.Bytes()
}

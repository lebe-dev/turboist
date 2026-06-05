package handlers_test

import (
	"bytes"
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/federation/hlc"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/handlers"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

const fedEmitCipherKey = "federation-emit-cipher-key-32-bytes!!!!"

// fedEmitEnv wires a Fiber app with the project + task handlers federation-
// enabled over a migrated DB carrying one federated project and one local-only
// project, so a CREATE (POST /projects/:id/tasks) and an UPDATE (PATCH /tasks/:id)
// driven through HTTP exercise the EmitMutation origin hook end-to-end (US-3.1
// AC1 create, US-3.2 AC1 edit). pings counts the commit-ping firings (item 7).
type fedEmitEnv struct {
	app       *fiber.App
	db        *sql.DB
	tasks     *repo.TaskRepo
	sections  *repo.ProjectSectionRepo
	fedProjID int64
	plainProj int64
	pings     *int64
}

func newFedEmitEnv(t *testing.T) *fedEmitEnv {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "fedemit.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.RunMigrations(context.Background(), d); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx := context.Background()

	if _, err := d.Exec(
		`INSERT INTO contexts (id, name, color, client_id, created_at, updated_at)
		 VALUES (1, 'c', 'blue', 'emit-ctx', '2024-01-01T00:00:00.000Z', '2024-01-01T00:00:00.000Z')`,
	); err != nil {
		t.Fatalf("ctx: %v", err)
	}

	projects := repo.NewProjectRepo(d, repo.NewProjectLabelsRepo(d))
	tasks := repo.NewTaskRepo(d, repo.NewTaskLabelsRepo(d))
	keys := repo.NewFederationKeysRepo(d)
	fedProjects := repo.NewFederatedProjectRepo(d)
	cipher := crypto.NewTokenCipher(fedEmitCipherKey)

	fedProj, err := projects.Create(ctx, repo.CreateProject{ContextID: 1, Title: "Shared", Color: "blue"})
	if err != nil {
		t.Fatalf("fed project: %v", err)
	}
	plainProj, err := projects.Create(ctx, repo.CreateProject{ContextID: 1, Title: "Private", Color: "red"})
	if err != nil {
		t.Fatalf("plain project: %v", err)
	}

	svc := fedsvc.NewService(d, projects, fedProjects, keys,
		repo.NewFederationInviteRepo(d), repo.NewFederatedInstanceRepo(d), cipher, "https://me.example")
	if _, err := svc.EnableForProject(ctx, fedProj.ID); err != nil {
		t.Fatalf("enable federation: %v", err)
	}
	nodeID, err := svc.EnsureKeys(ctx)
	if err != nil {
		t.Fatalf("ensure keys: %v", err)
	}

	var pings int64
	emitter := fedsvc.NewEmitter(d, keys, cipher, hlc.NewStore(d, nodeID), "https://me.example").
		WithCommitPing(func() { atomic.AddInt64(&pings, 1) })
	sections := repo.NewProjectSectionRepo(d)
	taskMutator := fedsvc.NewTaskMutator(emitter, tasks)
	projectMutator := fedsvc.NewProjectMutator(emitter, projects)
	sectionMutator := fedsvc.NewSectionMutator(emitter, sections)

	taskSvc := service.NewTaskService(tasks, projects, repo.NewTaskLabelsRepo(d),
		service.NewAutoLabelsService(repo.NewLabelRepo(d), repo.NewAppSettingsRepo(d)))
	taskSvc.WithFederation(taskMutator)

	app := httpapi.NewApp(httpapi.Deps{})
	api := app.Group("/api/v1", func(c fiber.Ctx) error {
		c.Locals("auth_method", httpapi.AuthMethodJWT)
		return c.Next()
	})
	handlers.NewProjectHandler(projects, sections, tasks, taskSvc,
		repo.NewLabelRepo(d), repo.NewContextRepo(d), service.NewPinService(tasks, projects, 10), fedProjects, "https://me.example").
		WithFederation(projectMutator, sectionMutator).Register(api)
	handlers.NewSectionHandler(sections, projects, tasks, taskSvc, "https://me.example").
		WithFederation(sectionMutator).Register(api.Group("/sections"))
	handlers.NewTaskHandler(tasks, projects, taskSvc, "https://me.example").
		WithFederation(taskMutator).Register(api)

	return &fedEmitEnv{app: app, db: d, tasks: tasks, sections: sections, fedProjID: fedProj.ID, plainProj: plainProj.ID, pings: &pings}
}

func emitOutboxEvents(t *testing.T, d *sql.DB, projectID int64) []events.Event {
	t.Helper()
	rows, err := d.Query(`SELECT payload FROM federation_outbox WHERE local_project_id = ? ORDER BY id ASC`, projectID)
	if err != nil {
		t.Fatalf("read outbox: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var out []events.Event
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			t.Fatalf("scan: %v", err)
		}
		var e events.Event
		if err := events.Unmarshal([]byte(p), &e); err != nil {
			t.Fatalf("decode: %v", err)
		}
		out = append(out, e)
	}
	return out
}

// TestTaskCreate_FederatedEmitsCreateOutbox drives POST /projects/:id/tasks for a
// FEDERATED project through the HTTP handler and asserts a signed op=create event
// lands in federation_outbox AND the commit-ping fired (US-3.1 AC1, item 7).
func TestTaskCreate_FederatedEmitsCreateOutbox(t *testing.T) {
	env := newFedEmitEnv(t)

	body := []byte(`{"title":"HTTP created","priority":"high"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+itoa(env.fedProjID)+"/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := env.app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status: got %d, want 201", resp.StatusCode)
	}

	evts := emitOutboxEvents(t, env.db, env.fedProjID)
	if len(evts) != 1 {
		t.Fatalf("outbox count: got %d, want 1", len(evts))
	}
	e := evts[0]
	if e.Op != events.OpCreate {
		t.Errorf("op: got %q, want create", e.Op)
	}
	if e.EntityType != events.EntityTask {
		t.Errorf("entity_type: got %q, want task", e.EntityType)
	}
	if e.Signature == "" {
		t.Errorf("create event must be signed")
	}
	if f, ok := e.Fields["title"]; !ok || f.Value != "HTTP created" {
		t.Errorf("title field: got %+v", e.Fields["title"])
	}
	if f, ok := e.Fields["priority"]; !ok || f.Value != "high" {
		t.Errorf("priority field: got %+v", e.Fields["priority"])
	}
	if got := atomic.LoadInt64(env.pings); got < 1 {
		t.Errorf("commit ping must fire through the handler: got %d", got)
	}
}

// TestProjectPatch_FederatedEmitsUpdateOutbox drives PATCH /projects/:id for a
// FEDERATED project through HTTP and asserts a signed op=update event lands in the
// outbox carrying the changed federated field (US-3.2 AC1).
func TestProjectPatch_FederatedEmitsUpdateOutbox(t *testing.T) {
	env := newFedEmitEnv(t)

	body := []byte(`{"title":"Project renamed"}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/projects/"+itoa(env.fedProjID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := env.app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("patch request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch status: got %d, want 200", resp.StatusCode)
	}
	evts := emitOutboxEvents(t, env.db, env.fedProjID)
	if len(evts) != 1 {
		t.Fatalf("outbox count: got %d, want 1", len(evts))
	}
	e := evts[0]
	if e.Op != events.OpUpdate || e.EntityType != events.EntityProject {
		t.Errorf("event: op=%q type=%q, want update/project", e.Op, e.EntityType)
	}
	if f, ok := e.Fields["title"]; !ok || f.Value != "Project renamed" {
		t.Errorf("title field: got %+v", e.Fields["title"])
	}
}

// TestProjectDelete_FederatedEmitsDeleteOutbox drives DELETE /projects/:id for a
// FEDERATED project through HTTP and asserts a signed op=delete tombstone lands in
// the outbox (US-3.2 AC1).
func TestProjectDelete_FederatedEmitsDeleteOutbox(t *testing.T) {
	env := newFedEmitEnv(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/projects/"+itoa(env.fedProjID), nil)
	resp, err := env.app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("delete request: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status: got %d, want 204", resp.StatusCode)
	}
	evts := emitOutboxEvents(t, env.db, env.fedProjID)
	if len(evts) != 1 {
		t.Fatalf("outbox count: got %d, want 1", len(evts))
	}
	e := evts[0]
	if e.Op != events.OpDelete || e.EntityType != events.EntityProject {
		t.Errorf("event: op=%q type=%q, want delete/project", e.Op, e.EntityType)
	}
}

// TestSectionCreate_FederatedEmitsCreateOutbox drives POST /projects/:id/sections
// for a FEDERATED project and asserts a signed op=create event lands in the outbox.
func TestSectionCreate_FederatedEmitsCreateOutbox(t *testing.T) {
	env := newFedEmitEnv(t)

	body := []byte(`{"title":"Section A"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+itoa(env.fedProjID)+"/sections", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := env.app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("create section request: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create section status: got %d, want 201", resp.StatusCode)
	}
	evts := emitOutboxEvents(t, env.db, env.fedProjID)
	if len(evts) != 1 {
		t.Fatalf("outbox count: got %d, want 1", len(evts))
	}
	e := evts[0]
	if e.Op != events.OpCreate || e.EntityType != events.EntitySection {
		t.Errorf("event: op=%q type=%q, want create/section", e.Op, e.EntityType)
	}
	if f, ok := e.Fields["title"]; !ok || f.Value != "Section A" {
		t.Errorf("title field: got %+v", e.Fields["title"])
	}
	if _, ok := e.Fields["position"]; !ok {
		t.Errorf("section create must carry position: got %v", e.Fields)
	}
}

// TestTaskCreate_NonFederatedWritesNoOutbox drives the create on a LOCAL-ONLY
// project through HTTP and asserts ZERO outbox events (US-3.2 AC1 scoped overlay).
func TestTaskCreate_NonFederatedWritesNoOutbox(t *testing.T) {
	env := newFedEmitEnv(t)

	body := []byte(`{"title":"plain create"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+itoa(env.plainProj)+"/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := env.app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status: got %d, want 201", resp.StatusCode)
	}
	var n int
	if err := env.db.QueryRow(`SELECT COUNT(*) FROM federation_outbox`).Scan(&n); err != nil {
		t.Fatalf("count outbox: %v", err)
	}
	if n != 0 {
		t.Errorf("non-federated create outbox: got %d, want 0", n)
	}
}

// TestTaskPatch_FederatedEmitsUpdateOutbox drives PATCH /tasks/:id for a task in a
// FEDERATED project through HTTP and asserts a signed op=update event carrying the
// changed field lands in federation_outbox (US-3.2 AC1 edit).
func TestTaskPatch_FederatedEmitsUpdateOutbox(t *testing.T) {
	env := newFedEmitEnv(t)
	ctx := context.Background()

	cx := int64(1)
	task, err := env.tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cx, ProjectID: &env.fedProjID}, Title: "to patch"})
	if err != nil {
		t.Fatalf("seed task: %v", err)
	}

	body := []byte(`{"title":"patched title"}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/tasks/"+itoa(task.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := env.app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("patch request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch status: got %d, want 200", resp.StatusCode)
	}

	evts := emitOutboxEvents(t, env.db, env.fedProjID)
	if len(evts) != 1 {
		t.Fatalf("outbox count: got %d, want 1", len(evts))
	}
	e := evts[0]
	if e.Op != events.OpUpdate {
		t.Errorf("op: got %q, want update", e.Op)
	}
	if e.EntityID != task.ClientID {
		t.Errorf("entity_id: got %q, want %q", e.EntityID, task.ClientID)
	}
	if e.Signature == "" {
		t.Errorf("update event must be signed")
	}
	if f, ok := e.Fields["title"]; !ok || f.Value != "patched title" {
		t.Errorf("title field: got %+v", e.Fields["title"])
	}
	if got := atomic.LoadInt64(env.pings); got < 1 {
		t.Errorf("commit ping must fire through the handler: got %d", got)
	}
}

package handlers_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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

const fedDeleteCipherKey = "federation-delete-cipher-key-32-bytes!!"

// fedDeleteEnv wires a minimal Fiber app with the task handler federation-enabled
// (the production WithFederation path) over a migrated DB carrying one federated
// project and one local-only project, so a DELETE driven through HTTP exercises
// the EmitDeleteCascade origin hook end-to-end (US-3.7 AC3).
type fedDeleteEnv struct {
	app       *fiber.App
	db        *sql.DB
	tasks     *repo.TaskRepo
	fedTaskID int64
	plainID   int64
}

// newFedDeleteEnv builds the env: a federated project + task and a local-only
// project + task, each with a seeded comment + checklist item child.
func newFedDeleteEnv(t *testing.T) *fedDeleteEnv {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "feddel.db"))
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
		 VALUES (1, 'c', 'blue', 'del-ctx', '2024-01-01T00:00:00.000Z', '2024-01-01T00:00:00.000Z')`,
	); err != nil {
		t.Fatalf("ctx: %v", err)
	}

	projects := repo.NewProjectRepo(d, repo.NewProjectLabelsRepo(d))
	tasks := repo.NewTaskRepo(d, repo.NewTaskLabelsRepo(d))
	keys := repo.NewFederationKeysRepo(d)
	fedProjects := repo.NewFederatedProjectRepo(d)
	cipher := crypto.NewTokenCipher(fedDeleteCipherKey)

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

	cx := int64(1)
	fedTask, err := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cx, ProjectID: &fedProj.ID}, Title: "Fed task"})
	if err != nil {
		t.Fatalf("fed task: %v", err)
	}
	plainTask, err := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cx, ProjectID: &plainProj.ID}, Title: "Plain task"})
	if err != nil {
		t.Fatalf("plain task: %v", err)
	}
	seedTaskComment(t, d, fedTask.ID, "comment-del")
	seedTaskChecklist(t, d, fedTask.ID, "checklist-del")
	seedTaskComment(t, d, plainTask.ID, "comment-plain")

	emitter := fedsvc.NewEmitter(d, keys, cipher, hlc.NewStore(d, nodeID), "https://me.example")
	mutator := fedsvc.NewTaskMutator(emitter, tasks)

	taskSvc := service.NewTaskService(tasks, projects, repo.NewTaskLabelsRepo(d),
		service.NewAutoLabelsService(repo.NewLabelRepo(d), repo.NewAppSettingsRepo(d)))

	app := httpapi.NewApp(httpapi.Deps{})
	// Inject the JWT auth method so RequireScope passes; the federation wiring is
	// the unit under test, not auth.
	api := app.Group("/api/v1", func(c fiber.Ctx) error {
		c.Locals("auth_method", httpapi.AuthMethodJWT)
		return c.Next()
	})
	handlers.NewTaskHandler(tasks, projects, taskSvc, "https://me.example").
		WithFederation(mutator).Register(api)

	return &fedDeleteEnv{app: app, db: d, tasks: tasks, fedTaskID: fedTask.ID, plainID: plainTask.ID}
}

func seedTaskComment(t *testing.T, d *sql.DB, taskID int64, clientID string) {
	t.Helper()
	if _, err := d.Exec(
		`INSERT INTO comments (task_id, body, client_id, created_at, updated_at) VALUES (?, 'note', ?, '2026-06-01T00:00:00.000Z', '2026-06-01T00:00:00.000Z')`,
		taskID, clientID); err != nil {
		t.Fatalf("seed comment: %v", err)
	}
}

func seedTaskChecklist(t *testing.T, d *sql.DB, taskID int64, clientID string) {
	t.Helper()
	if _, err := d.Exec(
		`INSERT INTO checklist_items (task_id, title, is_completed, position, client_id, created_at, updated_at) VALUES (?, 'step', 0, 0, ?, '2026-06-01T00:00:00.000Z', '2026-06-01T00:00:00.000Z')`,
		taskID, clientID); err != nil {
		t.Fatalf("seed checklist: %v", err)
	}
}

// TestTaskDelete_FederatedEmitsCascadeOutbox drives a DELETE of a FEDERATED task
// through the HTTP handler and asserts the federation origin-emit hook fired: an
// op=delete event for the task PLUS one per child comment / checklist item lands
// in federation_outbox, all signed (US-3.7 AC3 emit, the F3.3 review fix that
// wires EmitDeleteCascade into production).
func TestTaskDelete_FederatedEmitsCascadeOutbox(t *testing.T) {
	env := newFedDeleteEnv(t)
	ctx := context.Background()

	// The task's cross-instance client_id is the EntityID the origin event carries.
	var taskClient string
	if err := env.db.QueryRow(`SELECT client_id FROM tasks WHERE id = ?`, env.fedTaskID).Scan(&taskClient); err != nil {
		t.Fatalf("task client_id: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tasks/"+itoa(env.fedTaskID), nil)
	resp, err := env.app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("delete request: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status: got %d, want 204", resp.StatusCode)
	}

	// Three op=delete outbox rows: the task + its comment + its checklist item.
	rows, err := env.db.QueryContext(ctx, `SELECT payload FROM federation_outbox ORDER BY id ASC`)
	if err != nil {
		t.Fatalf("read outbox: %v", err)
	}
	defer func() { _ = rows.Close() }()
	seen := map[string]string{}
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			t.Fatalf("scan: %v", err)
		}
		var e events.Event
		if err := events.Unmarshal([]byte(payload), &e); err != nil {
			t.Fatalf("decode event: %v", err)
		}
		if e.Signature == "" {
			t.Errorf("cascade event %s must be signed", e.EntityID)
		}
		if e.Op != events.OpDelete {
			t.Errorf("event %s op: got %q, want delete", e.EntityID, e.Op)
		}
		if f, ok := e.Fields[events.FieldDeleted]; !ok || f.HLC == "" {
			t.Errorf("event %s must carry a _deleted field HLC", e.EntityID)
		}
		seen[e.EntityID] = string(e.Op)
	}
	for _, id := range []string{taskClient, "comment-del", "checklist-del"} {
		if seen[id] != string(events.OpDelete) {
			t.Errorf("missing op=delete event for %s: got %q (seen=%v)", id, seen[id], seen)
		}
	}
	if len(seen) != 3 {
		t.Errorf("outbox event count: got %d, want 3 (task + comment + checklist)", len(seen))
	}

	// The domain delete still happened: the task is tombstoned (invisible to Get).
	if _, err := env.tasks.Get(ctx, env.fedTaskID); err == nil {
		t.Errorf("federated task must be soft-deleted")
	}
}

// TestTaskDelete_NonFederatedWritesNoOutbox drives a DELETE of a task in a
// LOCAL-ONLY project through the HTTP handler and asserts the domain delete runs
// but ZERO outbox events are written — federation stays a scoped overlay (US-3.2
// AC1) even with the mutator wired.
func TestTaskDelete_NonFederatedWritesNoOutbox(t *testing.T) {
	env := newFedDeleteEnv(t)
	ctx := context.Background()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tasks/"+itoa(env.plainID), nil)
	resp, err := env.app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("delete request: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status: got %d, want 204", resp.StatusCode)
	}

	var n int
	if err := env.db.QueryRow(`SELECT COUNT(*) FROM federation_outbox`).Scan(&n); err != nil {
		t.Fatalf("count outbox: %v", err)
	}
	if n != 0 {
		t.Errorf("non-federated delete outbox count: got %d, want 0", n)
	}
	if _, err := env.tasks.Get(ctx, env.plainID); err == nil {
		t.Errorf("plain task must still be soft-deleted")
	}
}

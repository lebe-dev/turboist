package inbox_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/federation/inbox"
	"github.com/lebe-dev/turboist/internal/federation/store"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// applyEnv is the joiner-side environment: a migrated DB with one federated
// project (a task placed inside it) the incoming events target.
type applyEnv struct {
	db            *sql.DB
	applier       *inbox.Applier
	store         *store.Store
	tasks         *repo.TaskRepo
	projectID     int64
	projectClient string
	taskClientID  string
	localTaskID   int64
}

func newApplyEnv(t *testing.T) *applyEnv {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "inbox.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.RunMigrations(context.Background(), d); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx := context.Background()

	contexts := repo.NewContextRepo(d)
	cx, err := contexts.Create(ctx, "Work", "blue", false)
	if err != nil {
		t.Fatalf("create context: %v", err)
	}
	projects := repo.NewProjectRepo(d, repo.NewProjectLabelsRepo(d))
	p, err := projects.Create(ctx, repo.CreateProject{ContextID: cx.ID, Title: "Shared", Color: "blue"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	// Mark the project federated and add a peer mapping so apply is permitted.
	if _, err := d.Exec(`UPDATE projects SET is_federated = 1 WHERE id = ?`, p.ID); err != nil {
		t.Fatalf("set federated: %v", err)
	}
	fedProjects := repo.NewFederatedProjectRepo(d)
	if err := fedProjects.UpsertPeerRow(ctx, model.FederatedProject{
		LocalProjectID:    p.ID,
		PeerInstanceURL:   "https://alice.example",
		OriginInstanceURL: "https://alice.example",
		Permissions:       model.FederationPermissionWrite,
		ProtocolVersion:   1,
	}); err != nil {
		t.Fatalf("peer row: %v", err)
	}

	tasks := repo.NewTaskRepo(d, repo.NewTaskLabelsRepo(d))
	tk, err := tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cx.ID, ProjectID: &p.ID},
		Title:     "Original",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	st := store.New(d)
	applier := inbox.NewApplier(d, tasks, projects, repo.NewProjectSectionRepo(d), fedProjects, st)
	return &applyEnv{
		db:            d,
		applier:       applier,
		store:         st,
		tasks:         tasks,
		projectID:     p.ID,
		projectClient: p.ClientID,
		taskClientID:  tk.ClientID,
		localTaskID:   tk.ID,
	}
}

func eventID(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:16])
}

func updateEvent(env *applyEnv, fields map[string]events.Field) events.Event {
	return events.Event{
		EventID:         eventID(fmt.Sprintf("%v", fields)),
		Op:              events.OpUpdate,
		EntityType:      events.EntityTask,
		EntityID:        env.taskClientID,
		ProjectClientID: env.projectClient,
		Author:          "https://alice.example",
		OriginInstance:  "https://alice.example",
		CreatedAt:       "2026-06-01T10:00:00.000Z",
		Fields:          fields,
	}
}

// TestApply_TwoFieldMerge applies two updates that each touch a DIFFERENT field
// at a high HLC; both must land (US-3.3 AC1 — disjoint-field merge converges).
func TestApply_TwoFieldMerge(t *testing.T) {
	env := newApplyEnv(t)
	ctx := context.Background()

	e1 := updateEvent(env, map[string]events.Field{
		"title": {Value: "New Title", HLC: "00000000000200-0000-nodeA"},
	})
	e1.EventID = eventID("title")
	if _, err := env.applier.Apply(ctx, e1, "https://alice.example"); err != nil {
		t.Fatalf("apply title: %v", err)
	}

	e2 := updateEvent(env, map[string]events.Field{
		"priority": {Value: "medium", HLC: "00000000000300-0000-nodeA"},
	})
	e2.EventID = eventID("priority")
	if _, err := env.applier.Apply(ctx, e2, "https://alice.example"); err != nil {
		t.Fatalf("apply priority: %v", err)
	}

	tk, err := env.tasks.Get(ctx, env.localTaskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if tk.Title != "New Title" {
		t.Errorf("title: got %q, want New Title", tk.Title)
	}
	if string(tk.Priority) != "medium" {
		t.Errorf("priority: got %q, want medium", tk.Priority)
	}
}

// TestApply_StaleFieldIgnored applies a newer write to title, then a STALE
// (lower-HLC) write that touches both title and priority: title keeps the newer
// value, but priority (whose HLC is the highest seen for that field) still
// applies — per-field LWW resolves each field independently (US-3.3 AC2).
func TestApply_StaleFieldIgnored(t *testing.T) {
	env := newApplyEnv(t)
	ctx := context.Background()

	winner := updateEvent(env, map[string]events.Field{
		"title": {Value: "Winning Title", HLC: "00000000000500-0000-nodeA"},
	})
	winner.EventID = eventID("winner")
	if _, err := env.applier.Apply(ctx, winner, "https://alice.example"); err != nil {
		t.Fatalf("apply winner: %v", err)
	}

	mixed := updateEvent(env, map[string]events.Field{
		"title":    {Value: "Stale Title", HLC: "00000000000400-0000-nodeA"}, // stale -> ignored
		"priority": {Value: "high", HLC: "00000000000600-0000-nodeA"},        // newest -> applied
	})
	mixed.EventID = eventID("mixed")
	res, err := env.applier.Apply(ctx, mixed, "https://alice.example")
	if err != nil {
		t.Fatalf("apply mixed: %v", err)
	}
	if res.AppliedFields["title"] {
		t.Errorf("stale title must NOT be applied")
	}
	if !res.AppliedFields["priority"] {
		t.Errorf("newer priority MUST be applied")
	}

	tk, err := env.tasks.Get(ctx, env.localTaskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if tk.Title != "Winning Title" {
		t.Errorf("title must keep the newer value: got %q", tk.Title)
	}
	if string(tk.Priority) != "high" {
		t.Errorf("priority must apply: got %q", tk.Priority)
	}
}

// TestApply_OrderIndependentConvergence asserts that applying the same set of
// per-field events in two different orders converges to the same final state
// (US-3.3 AC3 — order-independent convergence). Two instances are simulated by
// two joiner envs receiving the same events in opposite orders.
func TestApply_OrderIndependentConvergence(t *testing.T) {
	ctx := context.Background()

	envA := newApplyEnv(t)
	envB := newApplyEnv(t)

	mk := func(env *applyEnv) []events.Event {
		a := updateEvent(env, map[string]events.Field{"title": {Value: "T-low", HLC: "00000000000100-0000-nodeA"}})
		a.EventID = "ev-a"
		b := updateEvent(env, map[string]events.Field{"title": {Value: "T-high", HLC: "00000000000300-0000-nodeB"}})
		b.EventID = "ev-b"
		c := updateEvent(env, map[string]events.Field{"title": {Value: "T-mid", HLC: "00000000000200-0000-nodeA"}})
		c.EventID = "ev-c"
		return []events.Event{a, b, c}
	}

	evA := mk(envA)
	for _, e := range evA {
		if _, err := envA.applier.Apply(ctx, e, "https://alice.example"); err != nil {
			t.Fatalf("apply A: %v", err)
		}
	}

	evB := mk(envB)
	for _, i := range []int{2, 0, 1} { // reversed-ish order
		if _, err := envB.applier.Apply(ctx, evB[i], "https://alice.example"); err != nil {
			t.Fatalf("apply B: %v", err)
		}
	}

	tkA, err := envA.tasks.Get(ctx, envA.localTaskID)
	if err != nil {
		t.Fatalf("get A: %v", err)
	}
	tkB, err := envB.tasks.Get(ctx, envB.localTaskID)
	if err != nil {
		t.Fatalf("get B: %v", err)
	}
	if tkA.Title != tkB.Title {
		t.Errorf("convergence failed: A=%q B=%q", tkA.Title, tkB.Title)
	}
	if tkA.Title != "T-high" {
		t.Errorf("highest-HLC value must win: got %q, want T-high", tkA.Title)
	}
}

// TestApply_CreateGhostRowOnMissing applies an op=create for an entity the
// joiner has never seen: a new (ghost) row is created carrying the event's
// client_id so a later update resolves to it (§10.4a ghost row).
func TestApply_CreateGhostRowOnMissing(t *testing.T) {
	env := newApplyEnv(t)
	ctx := context.Background()

	ghostClient := "ghost-task-9"
	create := events.Event{
		EventID:         "ev-create",
		Op:              events.OpCreate,
		EntityType:      events.EntityTask,
		EntityID:        ghostClient,
		ProjectClientID: env.projectClient,
		Author:          "https://alice.example",
		OriginInstance:  "https://alice.example",
		CreatedAt:       "2026-06-01T10:00:00.000Z",
		Fields: map[string]events.Field{
			"title": {Value: "Ghost task", HLC: "00000000000700-0000-nodeA"},
		},
	}
	if _, err := env.applier.Apply(ctx, create, "https://alice.example"); err != nil {
		t.Fatalf("apply create: %v", err)
	}

	var title string
	err := env.db.QueryRow(`SELECT title FROM tasks WHERE client_id = ? AND deleted_at IS NULL`, ghostClient).Scan(&title)
	if err != nil {
		t.Fatalf("ghost row not created: %v", err)
	}
	if title != "Ghost task" {
		t.Errorf("ghost title: got %q, want Ghost task", title)
	}
}

// TestApply_PoisonStatusRejected asserts an out-of-domain task status is rejected
// as a PER-EVENT permanent poison error (do-not-retry) BEFORE any domain write —
// not silently passed to a raw UPDATE where the tasks.status CHECK constraint
// would roll the whole apply tx back as an opaque, retried-forever error that
// head-of-line blocks the per-project queue (§3/W-8).
func TestApply_PoisonStatusRejected(t *testing.T) {
	env := newApplyEnv(t)
	ctx := context.Background()

	bad := updateEvent(env, map[string]events.Field{
		"status": {Value: "garbage", HLC: "00000000000900-0000-nodeA"},
	})
	bad.EventID = eventID("poison-status")
	_, err := env.applier.Apply(ctx, bad, "https://alice.example")
	if err == nil {
		t.Fatal("out-of-domain status must be rejected")
	}
	pe, ok := inbox.IsPoison(err)
	if !ok {
		t.Fatalf("error must classify as poison (do-not-retry), got %v", err)
	}
	if pe.ErrorID == "" {
		t.Error("poison error must carry an errorId")
	}
	if pe.EventID != bad.EventID || pe.PeerURL != "https://alice.example" || pe.Field != "status" {
		t.Errorf("poison error must carry event/peer/field context: %+v", pe)
	}

	// The live row must be untouched — no partial write, no field HLC advanced.
	tk, err := env.tasks.Get(ctx, env.localTaskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if string(tk.Status) != "open" {
		t.Errorf("status must be untouched: got %q", tk.Status)
	}
	hlc, err := env.store.GetFieldHLC(ctx, "task", env.taskClientID, "status")
	if err != nil {
		t.Fatalf("get status hlc: %v", err)
	}
	if hlc != "" {
		t.Errorf("status field HLC must not advance on a poison reject: got %q", hlc)
	}
}

// TestApply_PoisonPriorityRejected asserts an out-of-domain task priority is a
// permanent poison reject, not a CHECK-constraint rollback.
func TestApply_PoisonPriorityRejected(t *testing.T) {
	env := newApplyEnv(t)
	ctx := context.Background()

	bad := updateEvent(env, map[string]events.Field{
		"priority": {Value: "p1", HLC: "00000000000900-0000-nodeA"},
	})
	bad.EventID = eventID("poison-priority")
	_, err := env.applier.Apply(ctx, bad, "https://alice.example")
	if _, ok := inbox.IsPoison(err); !ok {
		t.Fatalf("out-of-domain priority must be a poison reject, got %v", err)
	}
}

// TestApply_PoisonColorRejected asserts an out-of-domain project color is a
// permanent poison reject (projects.color is a fixed palette / #rrggbb hex).
func TestApply_PoisonColorRejected(t *testing.T) {
	env := newApplyEnv(t)
	ctx := context.Background()

	bad := events.Event{
		EventID:         eventID("poison-color"),
		Op:              events.OpUpdate,
		EntityType:      events.EntityProject,
		EntityID:        env.projectClient,
		ProjectClientID: env.projectClient,
		Author:          "https://alice.example",
		OriginInstance:  "https://alice.example",
		CreatedAt:       "2026-06-01T10:00:00.000Z",
		Fields: map[string]events.Field{
			"color": {Value: "chartreuse", HLC: "00000000000900-0000-nodeA"},
		},
	}
	_, err := env.applier.Apply(ctx, bad, "https://alice.example")
	pe, ok := inbox.IsPoison(err)
	if !ok {
		t.Fatalf("out-of-domain color must be a poison reject, got %v", err)
	}
	if pe.Field != "color" {
		t.Errorf("poison error field: got %q, want color", pe.Field)
	}

	// A legitimate value is NOT rejected (no over-rejection).
	good := bad
	good.EventID = eventID("good-color")
	good.Fields = map[string]events.Field{
		"color": {Value: "purple", HLC: "00000000001000-0000-nodeA"},
	}
	if _, err := env.applier.Apply(ctx, good, "https://alice.example"); err != nil {
		t.Fatalf("valid palette color must apply: %v", err)
	}
	var color string
	if err := env.db.QueryRow(`SELECT color FROM projects WHERE id = ?`, env.projectID).Scan(&color); err != nil {
		t.Fatalf("get color: %v", err)
	}
	if color != "purple" {
		t.Errorf("color: got %q, want purple", color)
	}
}

// TestApply_ProjectArchivedStatusAccepted asserts the project-only 'archived'
// status (valid per the projects.status CHECK, not a task status) is NOT
// over-rejected as poison.
func TestApply_ProjectArchivedStatusAccepted(t *testing.T) {
	env := newApplyEnv(t)
	ctx := context.Background()

	e := events.Event{
		EventID:         eventID("archived"),
		Op:              events.OpUpdate,
		EntityType:      events.EntityProject,
		EntityID:        env.projectClient,
		ProjectClientID: env.projectClient,
		Author:          "https://alice.example",
		OriginInstance:  "https://alice.example",
		CreatedAt:       "2026-06-01T10:00:00.000Z",
		Fields: map[string]events.Field{
			"status": {Value: "archived", HLC: "00000000001100-0000-nodeA"},
		},
	}
	if _, err := env.applier.Apply(ctx, e, "https://alice.example"); err != nil {
		t.Fatalf("project archived status must apply, not be poison-rejected: %v", err)
	}
	var status string
	if err := env.db.QueryRow(`SELECT status FROM projects WHERE id = ?`, env.projectID).Scan(&status); err != nil {
		t.Fatalf("get status: %v", err)
	}
	if status != "archived" {
		t.Errorf("status: got %q, want archived", status)
	}
}

// TestApply_PoisonProjectStatusRejected asserts an out-of-domain project status
// is a PER-EVENT permanent poison reject (do-not-retry) BEFORE any domain write,
// not a projects.status CHECK-constraint rollback that would head-of-line block
// the per-project apply queue forever (§3/W-8, TASK B receiver guard).
func TestApply_PoisonProjectStatusRejected(t *testing.T) {
	env := newApplyEnv(t)
	ctx := context.Background()

	bad := events.Event{
		EventID:         eventID("poison-project-status"),
		Op:              events.OpUpdate,
		EntityType:      events.EntityProject,
		EntityID:        env.projectClient,
		ProjectClientID: env.projectClient,
		Author:          "https://alice.example",
		OriginInstance:  "https://alice.example",
		CreatedAt:       "2026-06-01T10:00:00.000Z",
		Fields: map[string]events.Field{
			"status": {Value: "paused", HLC: "00000000001200-0000-nodeA"},
		},
	}
	_, err := env.applier.Apply(ctx, bad, "https://alice.example")
	pe, ok := inbox.IsPoison(err)
	if !ok {
		t.Fatalf("out-of-domain project status must be a poison reject, got %v", err)
	}
	if pe.Field != "status" {
		t.Errorf("poison error field: got %q, want status", pe.Field)
	}
	// The live project status must be untouched (open) — no partial write.
	var status string
	if err := env.db.QueryRow(`SELECT status FROM projects WHERE id = ?`, env.projectID).Scan(&status); err != nil {
		t.Fatalf("get status: %v", err)
	}
	if status != "open" {
		t.Errorf("status must be untouched: got %q", status)
	}
}

// TestIsPoison_TransientNotPoison asserts a plain (transient) error is NOT
// classified as poison, so the F3.2 worker retries it rather than dropping it.
func TestIsPoison_TransientNotPoison(t *testing.T) {
	if _, ok := inbox.IsPoison(context.DeadlineExceeded); ok {
		t.Error("a transient error must not classify as poison")
	}
	if _, ok := inbox.IsPoison(nil); ok {
		t.Error("nil must not classify as poison")
	}
}

// TestApply_DeleteTombstone applies an op=delete: the local row is soft-deleted
// and a synthetic _deleted field HLC is recorded so a later stale update cannot
// resurrect it (US-3.7 foundation; the tombstone participates in per-field LWW).
func TestApply_DeleteTombstone(t *testing.T) {
	env := newApplyEnv(t)
	ctx := context.Background()

	del := events.Event{
		EventID:         "ev-delete",
		Op:              events.OpDelete,
		EntityType:      events.EntityTask,
		EntityID:        env.taskClientID,
		ProjectClientID: env.projectClient,
		Author:          "https://alice.example",
		OriginInstance:  "https://alice.example",
		CreatedAt:       "2026-06-01T10:00:00.000Z",
		Fields: map[string]events.Field{
			"_deleted": {Value: true, HLC: "00000000000800-0000-nodeA"},
		},
	}
	if _, err := env.applier.Apply(ctx, del, "https://alice.example"); err != nil {
		t.Fatalf("apply delete: %v", err)
	}

	var deletedAt sql.NullString
	if err := env.db.QueryRow(`SELECT deleted_at FROM tasks WHERE id = ?`, env.localTaskID).Scan(&deletedAt); err != nil {
		t.Fatalf("scan deleted_at: %v", err)
	}
	if !deletedAt.Valid {
		t.Errorf("delete event must soft-delete the task")
	}
	tombHLC, err := env.store.GetFieldHLC(ctx, "task", env.taskClientID, "_deleted")
	if err != nil {
		t.Fatalf("get _deleted hlc: %v", err)
	}
	if tombHLC != "00000000000800-0000-nodeA" {
		t.Errorf("_deleted field HLC: got %q, want the delete HLC", tombHLC)
	}
}

// TestApply_StampsInboxAppliedAt asserts a successful apply stamps applied_at on
// the federation_inbox row in the SAME tx as the merge, so the at-least-once
// recovery scan no longer re-drives it (NFR-2; finding fix). A previously-recorded
// inbox row (the POST handler's dedup insert) is the precondition.
func TestApply_StampsInboxAppliedAt(t *testing.T) {
	env := newApplyEnv(t)
	ctx := context.Background()

	e := updateEvent(env, map[string]events.Field{
		"title": {Value: "Stamped", HLC: "00000000000900-0000-nodeA"},
	})
	e.EventID = eventID("stamped")
	// Mirror the POST handler: durably record the event BEFORE apply.
	if _, err := env.store.InsertInbox(ctx, e.EventID, "https://alice.example", env.projectID, `{"event_id":"`+e.EventID+`"}`, "2026-06-01T10:00:00.000Z"); err != nil {
		t.Fatalf("insert inbox: %v", err)
	}

	if _, err := env.applier.Apply(ctx, e, "https://alice.example"); err != nil {
		t.Fatalf("apply: %v", err)
	}

	var appliedAt sql.NullString
	if err := env.db.QueryRow(`SELECT applied_at FROM federation_inbox WHERE event_id = ?`, e.EventID).Scan(&appliedAt); err != nil {
		t.Fatalf("scan applied_at: %v", err)
	}
	if !appliedAt.Valid || appliedAt.String == "" {
		t.Errorf("successful apply must stamp applied_at (terminal): got valid=%v %q", appliedAt.Valid, appliedAt.String)
	}

	// The recovery scan must NOT return a stamped (applied) row.
	pending, err := env.store.ListUnappliedInbox(ctx, 100)
	if err != nil {
		t.Fatalf("list unapplied: %v", err)
	}
	for _, p := range pending {
		if p.EventID == e.EventID {
			t.Errorf("applied event must not be re-driven by the recovery scan")
		}
	}
}

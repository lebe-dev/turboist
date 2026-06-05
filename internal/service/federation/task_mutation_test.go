package federation_test

import (
	"context"
	"testing"

	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

// outboxEvents decodes every outbox event for a project, newest-last.
func outboxEvents(t *testing.T, env *emitEnv, projectID int64) []events.Event {
	t.Helper()
	rows, err := env.db.Query(`SELECT payload FROM federation_outbox WHERE local_project_id = ? ORDER BY id ASC`, projectID)
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

// TestTaskMutator_CreateFederatedEmitsOutbox asserts a Create in a FEDERATED
// project writes the task row AND a signed op=create event carrying the federated
// field set (US-3.2 AC1 emit). The non-federated counterpart writes no outbox row.
func TestTaskMutator_CreateFederatedEmitsOutbox(t *testing.T) {
	env := newEmitEnv(t)
	ctx := context.Background()
	mut := fedsvc.NewTaskMutator(env.emitter, env.tasks)

	cx := int64(1)
	clientID := model.NewClientID()
	id, err := mut.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cx, ProjectID: &env.fedProject},
		Title:     "Created via mutator",
		Priority:  model.PriorityHigh,
	}, clientID)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	tk, err := env.tasks.Get(ctx, id)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if tk.Title != "Created via mutator" {
		t.Errorf("title: got %q", tk.Title)
	}
	if tk.ClientID != clientID {
		t.Errorf("client_id: got %q, want %q", tk.ClientID, clientID)
	}

	evts := outboxEvents(t, env, env.fedProject)
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
	if e.EntityID != clientID {
		t.Errorf("entity_id: got %q, want %q", e.EntityID, clientID)
	}
	if e.Signature == "" {
		t.Errorf("create event must be signed")
	}
	if f, ok := e.Fields["title"]; !ok || f.Value != "Created via mutator" {
		t.Errorf("title field: got %+v", e.Fields["title"])
	}
	if f, ok := e.Fields["priority"]; !ok || f.Value != "high" {
		t.Errorf("priority field: got %+v", e.Fields["priority"])
	}
	if f, ok := e.Fields["status"]; !ok || f.Value != "open" {
		t.Errorf("status field: got %+v", e.Fields["status"])
	}
	// Local-only fields must NOT be emitted (§3 DEVIATE).
	for _, banned := range []string{"day_part", "plan_state", "postpone_count", "troiki_category"} {
		if _, ok := e.Fields[banned]; ok {
			t.Errorf("local-only field %q must not be emitted", banned)
		}
	}
}

func TestTaskMutator_CreateNonFederatedNoOutbox(t *testing.T) {
	env := newEmitEnv(t)
	ctx := context.Background()
	mut := fedsvc.NewTaskMutator(env.emitter, env.tasks)

	cx := int64(1)
	if _, err := mut.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cx, ProjectID: &env.plainProj},
		Title:     "Plain create",
	}, model.NewClientID()); err != nil {
		t.Fatalf("create: %v", err)
	}
	if got := outboxCount(t, env.db, env.plainProj); got != 0 {
		t.Errorf("non-federated create outbox: got %d, want 0", got)
	}
}

// TestTaskMutator_UpdateFederatedEmitsChangedFields asserts an Update in a
// FEDERATED project writes the row AND a signed op=update event carrying ONLY the
// changed federated fields.
func TestTaskMutator_UpdateFederatedEmitsChangedFields(t *testing.T) {
	env := newEmitEnv(t)
	ctx := context.Background()
	mut := fedsvc.NewTaskMutator(env.emitter, env.tasks)

	task, err := env.tasks.Get(ctx, env.fedTaskID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	newTitle := "Patched via mutator"
	if err := mut.Update(ctx, task, repo.TaskUpdate{Title: &newTitle}); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := env.tasks.Get(ctx, env.fedTaskID)
	if err != nil {
		t.Fatalf("get after: %v", err)
	}
	if got.Title != newTitle {
		t.Errorf("title: got %q, want %q", got.Title, newTitle)
	}

	evts := outboxEvents(t, env, env.fedProject)
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
	if f, ok := e.Fields["title"]; !ok || f.Value != newTitle {
		t.Errorf("title field: got %+v", e.Fields["title"])
	}
	// Only the changed field travels (per-field LWW).
	if _, ok := e.Fields["description"]; ok {
		t.Errorf("unchanged description must not be emitted")
	}
	if len(e.Fields) != 1 {
		t.Errorf("update fields: got %d (%v), want 1", len(e.Fields), e.Fields)
	}
}

func TestTaskMutator_UpdateNonFederatedNoOutbox(t *testing.T) {
	env := newEmitEnv(t)
	ctx := context.Background()
	mut := fedsvc.NewTaskMutator(env.emitter, env.tasks)

	task, err := env.tasks.Get(ctx, env.plainTaskID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	newTitle := "Plain patch"
	if err := mut.Update(ctx, task, repo.TaskUpdate{Title: &newTitle}); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := env.tasks.Get(ctx, env.plainTaskID)
	if err != nil {
		t.Fatalf("get after: %v", err)
	}
	if got.Title != newTitle {
		t.Errorf("domain write must still happen: got %q", got.Title)
	}
	if n := outboxCount(t, env.db, env.plainProj); n != 0 {
		t.Errorf("non-federated update outbox: got %d, want 0", n)
	}
}

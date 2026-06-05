package federation_test

import (
	"context"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

// completeServiceFor builds a CompleteService wired to the env's repos, with the
// federation CompleteMutator routing status changes through the Emitter.
func completeServiceFor(t *testing.T, env *emitEnv) *service.CompleteService {
	t.Helper()
	projects := newProjectRepoFor(env)
	users := repo.NewUserRepo(env.db)
	if _, err := users.Create(context.Background(), "admin", "h"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	svc := service.NewCompleteService(env.tasks, projects, users)
	svc.WithFederation(fedsvc.NewCompleteMutator(env.emitter, env.tasks))
	return svc
}

// TestCompleteMutator_CompleteFederatedEmitsCompleted asserts completing a
// federated NON-recurring task emits op=update{status:completed,completed_at}
// (TASK A) and the domain write ran.
func TestCompleteMutator_CompleteFederatedEmitsCompleted(t *testing.T) {
	env := newEmitEnv(t)
	ctx := context.Background()
	svc := completeServiceFor(t, env)

	clientID := taskClientID(t, env.db, env.fedTaskID)
	got, err := svc.Complete(ctx, env.fedTaskID)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if got.Status != model.TaskStatusCompleted {
		t.Errorf("status: got %q, want completed", got.Status)
	}

	evts := outboxEvents(t, env, env.fedProject)
	if len(evts) != 1 {
		t.Fatalf("outbox count: got %d, want 1", len(evts))
	}
	e := evts[0]
	if e.Op != events.OpUpdate {
		t.Errorf("op: got %q, want update", e.Op)
	}
	if e.EntityType != events.EntityTask || e.EntityID != clientID {
		t.Errorf("entity: got %q/%q, want task/%q", e.EntityType, e.EntityID, clientID)
	}
	if f, ok := e.Fields["status"]; !ok || f.Value != "completed" {
		t.Errorf("status field: got %+v", e.Fields["status"])
	}
	if f, ok := e.Fields["completed_at"]; !ok || f.Value == nil {
		t.Errorf("completed_at field must be set: got %+v", e.Fields["completed_at"])
	}
}

func TestCompleteMutator_CompleteNonFederatedNoOutbox(t *testing.T) {
	env := newEmitEnv(t)
	ctx := context.Background()
	svc := completeServiceFor(t, env)

	if _, err := svc.Complete(ctx, env.plainTaskID); err != nil {
		t.Fatalf("complete: %v", err)
	}
	got, err := env.tasks.Get(ctx, env.plainTaskID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != model.TaskStatusCompleted {
		t.Errorf("domain write must still happen: got status=%q", got.Status)
	}
	if n := outboxCount(t, env.db, env.plainProj); n != 0 {
		t.Errorf("non-federated complete outbox: got %d, want 0", n)
	}
}

// TestCompleteMutator_UncompleteFederatedEmitsOpen asserts uncompleting a
// federated task emits op=update{status:open,completed_at:null} (TASK A).
func TestCompleteMutator_UncompleteFederatedEmitsOpen(t *testing.T) {
	env := newEmitEnv(t)
	ctx := context.Background()
	svc := completeServiceFor(t, env)

	// Complete first (one event), then uncomplete (second event).
	if _, err := svc.Complete(ctx, env.fedTaskID); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if _, err := svc.Uncomplete(ctx, env.fedTaskID); err != nil {
		t.Fatalf("uncomplete: %v", err)
	}

	evts := outboxEvents(t, env, env.fedProject)
	if len(evts) != 2 {
		t.Fatalf("outbox count: got %d, want 2 (complete + uncomplete)", len(evts))
	}
	e := evts[1]
	if e.Op != events.OpUpdate {
		t.Errorf("op: got %q, want update", e.Op)
	}
	if f, ok := e.Fields["status"]; !ok || f.Value != "open" {
		t.Errorf("status field: got %+v", e.Fields["status"])
	}
	if f, ok := e.Fields["completed_at"]; !ok || f.Value != nil {
		t.Errorf("completed_at field must be cleared (nil): got %+v", e.Fields["completed_at"])
	}
}

// TestCompleteMutator_CancelFederatedEmitsCancelled asserts cancelling a
// federated task emits op=update{status:cancelled} (TASK A).
func TestCompleteMutator_CancelFederatedEmitsCancelled(t *testing.T) {
	env := newEmitEnv(t)
	ctx := context.Background()
	svc := completeServiceFor(t, env)

	if _, err := svc.Cancel(ctx, env.fedTaskID); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	evts := outboxEvents(t, env, env.fedProject)
	if len(evts) != 1 {
		t.Fatalf("outbox count: got %d, want 1", len(evts))
	}
	e := evts[0]
	if e.Op != events.OpUpdate {
		t.Errorf("op: got %q, want update", e.Op)
	}
	if f, ok := e.Fields["status"]; !ok || f.Value != "cancelled" {
		t.Errorf("status field: got %+v", e.Fields["status"])
	}
}

func TestCompleteMutator_CancelNonFederatedNoOutbox(t *testing.T) {
	env := newEmitEnv(t)
	ctx := context.Background()
	svc := completeServiceFor(t, env)

	if _, err := svc.Cancel(ctx, env.plainTaskID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if n := outboxCount(t, env.db, env.plainProj); n != 0 {
		t.Errorf("non-federated cancel outbox: got %d, want 0", n)
	}
}

// TestCompleteMutator_RecurringAdvanceEmitsUpdateAndCreate asserts completing a
// federated RECURRING task that advances IN PLACE emits TWO events: op=update on
// the parent (advanced due_at, status stays open) AND op=create for the new
// completed snapshot row (its own client_id, status=completed) — all in the same
// emit flow (TASK A recurrence).
func TestCompleteMutator_RecurringAdvanceEmitsUpdateAndCreate(t *testing.T) {
	env := newEmitEnv(t)
	ctx := context.Background()
	svc := completeServiceFor(t, env)

	// Make the federated task recurring with a future due date so it advances in
	// place (non-terminal) rather than completing.
	due := time.Now().Add(24 * time.Hour)
	rruleStr := "FREQ=DAILY;INTERVAL=1"
	if _, err := env.tasks.Update(ctx, env.fedTaskID, repo.TaskUpdate{DueAt: &due, RecurrenceRule: &rruleStr}); err != nil {
		t.Fatalf("set recurrence: %v", err)
	}
	parentClient := taskClientID(t, env.db, env.fedTaskID)

	got, err := svc.Complete(ctx, env.fedTaskID)
	if err != nil {
		t.Fatalf("complete recurring: %v", err)
	}
	if got.Status != model.TaskStatusOpen {
		t.Errorf("recurring parent must stay open: got %q", got.Status)
	}

	evts := outboxEvents(t, env, env.fedProject)
	if len(evts) != 2 {
		t.Fatalf("outbox count: got %d, want 2 (advance update + snapshot create)", len(evts))
	}

	var update, create *events.Event
	for i := range evts {
		switch evts[i].Op {
		case events.OpUpdate:
			update = &evts[i]
		case events.OpCreate:
			create = &evts[i]
		}
	}
	if update == nil {
		t.Fatal("missing op=update for the advanced parent")
	}
	if update.EntityID != parentClient {
		t.Errorf("update entity_id: got %q, want parent %q", update.EntityID, parentClient)
	}
	if _, ok := update.Fields["due_at"]; !ok {
		t.Errorf("parent advance must emit due_at: got %+v", update.Fields)
	}
	if create == nil {
		t.Fatal("missing op=create for the new occurrence snapshot")
	}
	if create.EntityID == parentClient || create.EntityID == "" {
		t.Errorf("snapshot create must carry its OWN client_id, got %q", create.EntityID)
	}
	if f, ok := create.Fields["status"]; !ok || f.Value != "completed" {
		t.Errorf("snapshot create status: got %+v", create.Fields["status"])
	}
}

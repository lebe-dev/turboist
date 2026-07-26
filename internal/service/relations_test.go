package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
)

type relationFixtures struct {
	svc       *service.RelationService
	tasks     *repo.TaskRepo
	relations *repo.TaskRelationsRepo
	contextID int64
}

func setupRelationService(t *testing.T) *relationFixtures {
	t.Helper()
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	trelations := repo.NewTaskRelationsRepo(d)
	plabels := repo.NewProjectLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels, trelations)
	_ = repo.NewProjectRepo(d, plabels)
	ctxs := repo.NewContextRepo(d)
	users := repo.NewUserRepo(d)
	if _, err := users.Create(context.Background(), "admin", "h"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	c, err := ctxs.Create(context.Background(), "Work", "blue", false)
	if err != nil {
		t.Fatalf("create context: %v", err)
	}
	return &relationFixtures{
		svc:       service.NewRelationService(tasks, trelations),
		tasks:     tasks,
		relations: trelations,
		contextID: c.ID,
	}
}

func (f *relationFixtures) newTask(t *testing.T, title string) *model.Task {
	t.Helper()
	task, err := f.tasks.Create(context.Background(), repo.CreateTask{
		Placement: repo.Placement{ContextID: &f.contextID},
		Title:     title,
	})
	if err != nil {
		t.Fatalf("create task %q: %v", title, err)
	}
	return task
}

func TestRelationService_Add_Self(t *testing.T) {
	f := setupRelationService(t)
	a := f.newTask(t, "a")
	_, err := f.svc.Add(context.Background(), a.ID, a.ID,
		model.RelationTypeRelated, model.RelationDirectionOutgoing)
	if !errors.Is(err, service.ErrRelationSelf) {
		t.Errorf("got %v, want ErrRelationSelf", err)
	}
}

func TestRelationService_Add_MissingTask(t *testing.T) {
	f := setupRelationService(t)
	a := f.newTask(t, "a")
	_, err := f.svc.Add(context.Background(), a.ID, 99999,
		model.RelationTypeBlocks, model.RelationDirectionIncoming)
	if !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

// Direction is relative to the task in the request: incoming means the peer blocks
// it, so the peer must end up as the stored row's source.
func TestRelationService_Add_BlocksDirection(t *testing.T) {
	f := setupRelationService(t)
	ctx := context.Background()
	blocked := f.newTask(t, "blocked")
	blocker := f.newTask(t, "blocker")

	got, err := f.svc.Add(ctx, blocked.ID, blocker.ID,
		model.RelationTypeBlocks, model.RelationDirectionIncoming)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if got.RelationSummary.BlockedByOpen != 1 {
		t.Errorf("blocked-by count: got %d, want 1", got.RelationSummary.BlockedByOpen)
	}
	if len(got.Relations) != 1 {
		t.Fatalf("relations: got %d, want 1", len(got.Relations))
	}
	if got.Relations[0].Direction != model.RelationDirectionIncoming {
		t.Errorf("direction: got %q, want incoming", got.Relations[0].Direction)
	}
	if got.Relations[0].SourceTaskID != blocker.ID {
		t.Errorf("source: got %d, want %d (the blocker)", got.Relations[0].SourceTaskID, blocker.ID)
	}

	// The reverse side must be the mirror, with nothing blocking the blocker.
	other, err := f.tasks.GetWithRelations(ctx, blocker.ID)
	if err != nil {
		t.Fatalf("get blocker: %v", err)
	}
	if other.RelationSummary.BlockedByOpen != 0 {
		t.Errorf("blocker blocked-by: got %d, want 0", other.RelationSummary.BlockedByOpen)
	}
	if len(other.Relations) != 1 || other.Relations[0].Direction != model.RelationDirectionOutgoing {
		t.Errorf("blocker relations: got %+v, want one outgoing", other.Relations)
	}
}

// `related` is symmetric, so adding it from either side must produce the same
// single row — the service normalises the pair so the UNIQUE constraint can see it.
func TestRelationService_Add_RelatedIsNormalised(t *testing.T) {
	f := setupRelationService(t)
	ctx := context.Background()
	a := f.newTask(t, "a")
	b := f.newTask(t, "b")

	if _, err := f.svc.Add(ctx, b.ID, a.ID, model.RelationTypeRelated, model.RelationDirectionOutgoing); err != nil {
		t.Fatalf("add b→a: %v", err)
	}
	// Same pair from the other side, and with the opposite direction hint.
	_, err := f.svc.Add(ctx, a.ID, b.ID, model.RelationTypeRelated, model.RelationDirectionIncoming)
	if !errors.Is(err, repo.ErrConflict) {
		t.Errorf("add a→b: got %v, want ErrConflict", err)
	}
	rels, err := f.relations.ListForTask(ctx, a.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rels) != 1 {
		t.Errorf("relations: got %d, want 1", len(rels))
	}
	if rels[0].SourceTaskID != a.ID || rels[0].TargetTaskID != b.ID {
		t.Errorf("normalisation: got (%d,%d), want (%d,%d) — lower id first",
			rels[0].SourceTaskID, rels[0].TargetTaskID, a.ID, b.ID)
	}
}

// A `related` link must never count towards the blocked-by tally.
func TestRelationService_Add_RelatedDoesNotBlock(t *testing.T) {
	f := setupRelationService(t)
	a := f.newTask(t, "a")
	b := f.newTask(t, "b")
	got, err := f.svc.Add(context.Background(), a.ID, b.ID,
		model.RelationTypeRelated, model.RelationDirectionIncoming)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if got.RelationSummary.BlockedByOpen != 0 {
		t.Errorf("blocked-by: got %d, want 0", got.RelationSummary.BlockedByOpen)
	}
	if got.RelationSummary.Total != 1 {
		t.Errorf("total: got %d, want 1", got.RelationSummary.Total)
	}
}

func TestRelationService_Add_Cycle(t *testing.T) {
	f := setupRelationService(t)
	ctx := context.Background()
	a := f.newTask(t, "a")
	b := f.newTask(t, "b")
	c := f.newTask(t, "c")

	if _, err := f.svc.Add(ctx, a.ID, b.ID, model.RelationTypeBlocks, model.RelationDirectionOutgoing); err != nil {
		t.Fatalf("add a→b: %v", err)
	}
	if _, err := f.svc.Add(ctx, b.ID, c.ID, model.RelationTypeBlocks, model.RelationDirectionOutgoing); err != nil {
		t.Fatalf("add b→c: %v", err)
	}
	_, err := f.svc.Add(ctx, c.ID, a.ID, model.RelationTypeBlocks, model.RelationDirectionOutgoing)
	if !errors.Is(err, service.ErrRelationCycle) {
		t.Errorf("c→a: got %v, want ErrRelationCycle", err)
	}
	// The same pair as `related` carries no deadlock risk and must be accepted.
	if _, err := f.svc.Add(ctx, c.ID, a.ID, model.RelationTypeRelated, model.RelationDirectionOutgoing); err != nil {
		t.Errorf("c related a: got %v, want nil", err)
	}
}

func TestRelationService_Remove(t *testing.T) {
	f := setupRelationService(t)
	ctx := context.Background()
	a := f.newTask(t, "a")
	b := f.newTask(t, "b")
	added, err := f.svc.Add(ctx, a.ID, b.ID, model.RelationTypeBlocks, model.RelationDirectionIncoming)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	relationID := added.Relations[0].ID

	got, err := f.svc.Remove(ctx, a.ID, relationID)
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if len(got.Relations) != 0 {
		t.Errorf("relations: got %d, want 0", len(got.Relations))
	}
	if got.RelationSummary.BlockedByOpen != 0 {
		t.Errorf("blocked-by: got %d, want 0", got.RelationSummary.BlockedByOpen)
	}
	if _, err := f.svc.Remove(ctx, a.ID, relationID); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("second remove: got %v, want ErrNotFound", err)
	}
}

package repo

import (
	"context"
	"errors"
	"testing"

	"github.com/lebe-dev/turboist/internal/model"
)

// newRelTask creates a bare open task in the fixture's context.
func newRelTask(t *testing.T, f *taskFixture, title string) *model.Task {
	t.Helper()
	task, err := f.tasks.Create(context.Background(), CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     title,
	})
	if err != nil {
		t.Fatalf("create task %q: %v", title, err)
	}
	return task
}

func TestTaskRelationsRepo_CreateAndList(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	a := newRelTask(t, f, "a")
	b := newRelTask(t, f, "b")

	rel, err := f.trelations.Create(ctx, a.ID, b.ID, model.RelationTypeBlocks)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if rel.ID == 0 {
		t.Errorf("id: got 0, want a generated id")
	}

	// The same row must read as outgoing from a and incoming from b — that is what
	// lets one stored edge serve both task pages.
	fromA, err := f.trelations.ListForTask(ctx, a.ID)
	if err != nil {
		t.Fatalf("list for a: %v", err)
	}
	if len(fromA) != 1 {
		t.Fatalf("relations for a: got %d, want 1", len(fromA))
	}
	if fromA[0].Direction != model.RelationDirectionOutgoing {
		t.Errorf("direction from a: got %q, want outgoing", fromA[0].Direction)
	}
	if fromA[0].Other == nil || fromA[0].Other.ID != b.ID {
		t.Errorf("peer from a: got %+v, want task %d", fromA[0].Other, b.ID)
	}

	fromB, err := f.trelations.ListForTask(ctx, b.ID)
	if err != nil {
		t.Fatalf("list for b: %v", err)
	}
	if len(fromB) != 1 {
		t.Fatalf("relations for b: got %d, want 1", len(fromB))
	}
	if fromB[0].Direction != model.RelationDirectionIncoming {
		t.Errorf("direction from b: got %q, want incoming", fromB[0].Direction)
	}
	if fromB[0].Other == nil || fromB[0].Other.Title != "a" {
		t.Errorf("peer from b: got %+v, want task a", fromB[0].Other)
	}
}

func TestTaskRelationsRepo_CreateDuplicate(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	a := newRelTask(t, f, "a")
	b := newRelTask(t, f, "b")

	if _, err := f.trelations.Create(ctx, a.ID, b.ID, model.RelationTypeBlocks); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := f.trelations.Create(ctx, a.ID, b.ID, model.RelationTypeBlocks)
	if !errors.Is(err, ErrConflict) {
		t.Errorf("duplicate: got %v, want ErrConflict", err)
	}
	// Same pair, different type is a distinct relation and must be allowed.
	if _, err := f.trelations.Create(ctx, a.ID, b.ID, model.RelationTypeRelated); err != nil {
		t.Errorf("other type: got %v, want nil", err)
	}
}

// Adding a relation must move updated_at on BOTH tasks: the SPA's hydrate()
// short-circuits on an unchanged updatedAt, so without this the peer's page keeps
// rendering a stale relation set.
func TestTaskRelationsRepo_CreateTouchesBothTasks(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	a := newRelTask(t, f, "a")
	b := newRelTask(t, f, "b")

	rel, err := f.trelations.Create(ctx, a.ID, b.ID, model.RelationTypeBlocks)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// Asserting equality with the relation's own timestamp rather than "after the
	// old value": timestamps have millisecond resolution, so a task created in the
	// same millisecond would make an `After` check pass vacuously.
	for _, tc := range []struct {
		name string
		id   int64
	}{{"a", a.ID}, {"b", b.ID}} {
		got, err := f.tasks.Get(ctx, tc.id)
		if err != nil {
			t.Fatalf("get %s: %v", tc.name, err)
		}
		if !got.UpdatedAt.Equal(rel.CreatedAt) {
			t.Errorf("%s updated_at: got %v, want %v", tc.name, got.UpdatedAt, rel.CreatedAt)
		}
	}
}

func TestTaskRelationsRepo_Delete(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	a := newRelTask(t, f, "a")
	b := newRelTask(t, f, "b")
	rel, err := f.trelations.Create(ctx, a.ID, b.ID, model.RelationTypeBlocks)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Deletable from either end.
	if err := f.trelations.Delete(ctx, rel.ID, b.ID); err != nil {
		t.Fatalf("delete from b: %v", err)
	}
	rels, err := f.trelations.ListForTask(ctx, a.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rels) != 0 {
		t.Errorf("relations after delete: got %d, want 0", len(rels))
	}
}

// The delete is scoped by task id so a relation cannot be removed through the
// endpoint of a task it does not touch — that scoping is what turns a wrong-task
// request into a 404 instead of a silent success.
func TestTaskRelationsRepo_DeleteScopedToTask(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	a := newRelTask(t, f, "a")
	b := newRelTask(t, f, "b")
	c := newRelTask(t, f, "c")
	rel, err := f.trelations.Create(ctx, a.ID, b.ID, model.RelationTypeBlocks)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := f.trelations.Delete(ctx, rel.ID, c.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete via unrelated task: got %v, want ErrNotFound", err)
	}
	if err := f.trelations.Delete(ctx, 99999, a.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete missing relation: got %v, want ErrNotFound", err)
	}
	rels, _ := f.trelations.ListForTask(ctx, a.ID)
	if len(rels) != 1 {
		t.Errorf("relation should survive: got %d, want 1", len(rels))
	}
}

func TestTaskRelationsRepo_SummaryByTaskIDs(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	a := newRelTask(t, f, "a")
	b := newRelTask(t, f, "b")
	c := newRelTask(t, f, "c")

	// a blocks b, b related c → b has 2 relations and 1 open blocker.
	if _, err := f.trelations.Create(ctx, a.ID, b.ID, model.RelationTypeBlocks); err != nil {
		t.Fatalf("create blocks: %v", err)
	}
	if _, err := f.trelations.Create(ctx, b.ID, c.ID, model.RelationTypeRelated); err != nil {
		t.Fatalf("create related: %v", err)
	}

	got, err := f.trelations.SummaryByTaskIDs(ctx, []int64{a.ID, b.ID, c.ID})
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if got[a.ID].Total != 1 || got[a.ID].BlockedByOpen != 0 {
		t.Errorf("a: got %+v, want {0 1}", got[a.ID])
	}
	if got[b.ID].Total != 2 || got[b.ID].BlockedByOpen != 1 {
		t.Errorf("b: got %+v, want {1 2}", got[b.ID])
	}
	if got[c.ID].Total != 1 || got[c.ID].BlockedByOpen != 0 {
		t.Errorf("c: got %+v, want {0 1}", got[c.ID])
	}
}

func TestTaskRelationsRepo_SummaryByTaskIDs_Empty(t *testing.T) {
	f := newTaskFixture(t)
	out, err := f.trelations.SummaryByTaskIDs(context.Background(), nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty, got %+v", out)
	}
}

// A `related` link must never count as a blocker, and a completed or cancelled
// blocker must stop blocking — otherwise a cancelled task deadlocks its dependents
// forever.
func TestTaskRelationsRepo_OpenBlockerIDs_IgnoresNonOpenAndRelated(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	target := newRelTask(t, f, "target")
	open := newRelTask(t, f, "open blocker")
	done := newRelTask(t, f, "completed blocker")
	cancelled := newRelTask(t, f, "cancelled blocker")
	related := newRelTask(t, f, "merely related")

	for _, src := range []*model.Task{open, done, cancelled} {
		if _, err := f.trelations.Create(ctx, src.ID, target.ID, model.RelationTypeBlocks); err != nil {
			t.Fatalf("create blocks from %d: %v", src.ID, err)
		}
	}
	if _, err := f.trelations.Create(ctx, related.ID, target.ID, model.RelationTypeRelated); err != nil {
		t.Fatalf("create related: %v", err)
	}
	completed := model.TaskStatusCompleted
	if _, err := f.tasks.Update(ctx, done.ID, TaskUpdate{Status: &completed}); err != nil {
		t.Fatalf("complete blocker: %v", err)
	}
	cancelledStatus := model.TaskStatusCancelled
	if _, err := f.tasks.Update(ctx, cancelled.ID, TaskUpdate{Status: &cancelledStatus}); err != nil {
		t.Fatalf("cancel blocker: %v", err)
	}

	got, err := f.trelations.OpenBlockerIDs(ctx, target.ID)
	if err != nil {
		t.Fatalf("open blockers: %v", err)
	}
	if len(got) != 1 || got[0] != open.ID {
		t.Errorf("open blockers: got %v, want [%d]", got, open.ID)
	}
}

func TestTaskRelationsRepo_WouldCycle(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	a := newRelTask(t, f, "a")
	b := newRelTask(t, f, "b")
	c := newRelTask(t, f, "c")

	// a blocks b blocks c. Adding "c blocks a" would close the loop.
	if _, err := f.trelations.Create(ctx, a.ID, b.ID, model.RelationTypeBlocks); err != nil {
		t.Fatalf("create a→b: %v", err)
	}
	if _, err := f.trelations.Create(ctx, b.ID, c.ID, model.RelationTypeBlocks); err != nil {
		t.Fatalf("create b→c: %v", err)
	}

	cycle, err := f.trelations.WouldCycle(ctx, c.ID, a.ID)
	if err != nil {
		t.Fatalf("would cycle c→a: %v", err)
	}
	if !cycle {
		t.Errorf("c→a: got false, want true (closes a→b→c→a)")
	}

	// The forward direction is not a cycle: a→c only shortcuts the existing chain.
	cycle, err = f.trelations.WouldCycle(ctx, a.ID, c.ID)
	if err != nil {
		t.Fatalf("would cycle a→c: %v", err)
	}
	if cycle {
		t.Errorf("a→c: got true, want false")
	}

	self, err := f.trelations.WouldCycle(ctx, a.ID, a.ID)
	if err != nil {
		t.Fatalf("would cycle a→a: %v", err)
	}
	if !self {
		t.Errorf("a→a: got false, want true")
	}
}

// A `related` edge must not participate in cycle detection: only `blocks` can
// deadlock a task.
func TestTaskRelationsRepo_WouldCycle_IgnoresRelated(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	a := newRelTask(t, f, "a")
	b := newRelTask(t, f, "b")

	if _, err := f.trelations.Create(ctx, a.ID, b.ID, model.RelationTypeRelated); err != nil {
		t.Fatalf("create related: %v", err)
	}
	cycle, err := f.trelations.WouldCycle(ctx, b.ID, a.ID)
	if err != nil {
		t.Fatalf("would cycle: %v", err)
	}
	if cycle {
		t.Errorf("got true, want false — a `related` edge cannot deadlock")
	}
}

// Tasks are hard-deleted, so the relation rows must go with them (ON DELETE
// CASCADE on both FKs) — a leftover row would resurrect as a phantom blocker.
func TestTaskRelationsRepo_CascadeOnTaskDelete(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	a := newRelTask(t, f, "a")
	b := newRelTask(t, f, "b")
	if _, err := f.trelations.Create(ctx, a.ID, b.ID, model.RelationTypeBlocks); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := f.tasks.Delete(ctx, a.ID); err != nil {
		t.Fatalf("delete a: %v", err)
	}
	rels, err := f.trelations.ListForTask(ctx, b.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rels) != 0 {
		t.Errorf("relations after cascade: got %d, want 0", len(rels))
	}
	blockers, err := f.trelations.OpenBlockerIDs(ctx, b.ID)
	if err != nil {
		t.Fatalf("open blockers: %v", err)
	}
	if len(blockers) != 0 {
		t.Errorf("blockers after cascade: got %v, want none", blockers)
	}
}

// The summary must reach every read path, so a blocked task renders as blocked in
// lists too — not only on the detail page.
func TestTaskRepo_HydratesRelationSummary(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	blocker := newRelTask(t, f, "blocker")
	blocked := newRelTask(t, f, "blocked")
	if _, err := f.trelations.Create(ctx, blocker.ID, blocked.ID, model.RelationTypeBlocks); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := f.tasks.Get(ctx, blocked.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.RelationSummary.BlockedByOpen != 1 || got.RelationSummary.Total != 1 {
		t.Errorf("Get summary: got %+v, want {1 1}", got.RelationSummary)
	}
	if len(got.Relations) != 0 {
		t.Errorf("Get must not hydrate the relation list: got %d", len(got.Relations))
	}

	withRels, err := f.tasks.GetWithRelations(ctx, blocked.ID)
	if err != nil {
		t.Fatalf("get with relations: %v", err)
	}
	if len(withRels.Relations) != 1 {
		t.Fatalf("GetWithRelations: got %d relations, want 1", len(withRels.Relations))
	}
	if withRels.Relations[0].Direction != model.RelationDirectionIncoming {
		t.Errorf("direction: got %q, want incoming", withRels.Relations[0].Direction)
	}

	items, _, err := f.tasks.ListByContext(ctx, f.contextID, false, TaskFilter{}, Page{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var seen bool
	for _, it := range items {
		if it.ID != blocked.ID {
			continue
		}
		seen = true
		if it.RelationSummary.BlockedByOpen != 1 {
			t.Errorf("list summary: got %+v, want BlockedByOpen 1", it.RelationSummary)
		}
	}
	if !seen {
		t.Fatalf("blocked task missing from the context listing")
	}
}

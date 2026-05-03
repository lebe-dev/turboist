package repo

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

type taskFixture struct {
	contexts  *ContextRepo
	projects  *ProjectRepo
	sections  *ProjectSectionRepo
	labels    *LabelRepo
	tasks     *TaskRepo
	tlabels   *TaskLabelsRepo
	contextID int64
	projectID int64
	sectionID int64
}

func newTaskFixture(t *testing.T) *taskFixture {
	t.Helper()
	d := setupTestDB(t)
	cr := NewContextRepo(d)
	plr := NewProjectLabelsRepo(d)
	pr := NewProjectRepo(d, plr)
	sr := NewProjectSectionRepo(d)
	lr := NewLabelRepo(d)
	tlr := NewTaskLabelsRepo(d)
	tr := NewTaskRepo(d, tlr)

	ctx := context.Background()
	c, err := cr.Create(ctx, "work", "blue", false)
	if err != nil {
		t.Fatalf("create context: %v", err)
	}
	p, err := pr.Create(ctx, CreateProject{ContextID: c.ID, Title: "alpha", Color: "blue"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	s, err := sr.Create(ctx, p.ID, "section")
	if err != nil {
		t.Fatalf("create section: %v", err)
	}
	return &taskFixture{
		contexts:  cr,
		projects:  pr,
		sections:  sr,
		labels:    lr,
		tasks:     tr,
		tlabels:   tlr,
		contextID: c.ID,
		projectID: p.ID,
		sectionID: s.ID,
	}
}

func TestPlacement_Validate(t *testing.T) {
	inboxID := int64(1)
	ctxID := int64(2)
	projID := int64(3)
	secID := int64(4)
	parID := int64(5)

	tests := []struct {
		name string
		p    Placement
		ok   bool
	}{
		{"inbox only", Placement{InboxID: &inboxID}, true},
		{"context only", Placement{ContextID: &ctxID}, true},
		{"context + project", Placement{ContextID: &ctxID, ProjectID: &projID}, true},
		{"context + project + section", Placement{ContextID: &ctxID, ProjectID: &projID, SectionID: &secID}, true},
		{"context + parent", Placement{ContextID: &ctxID, ParentID: &parID}, true},
		{"both inbox and context", Placement{InboxID: &inboxID, ContextID: &ctxID}, false},
		{"neither", Placement{}, false},
		{"inbox with project", Placement{InboxID: &inboxID, ProjectID: &projID}, false},
		{"inbox with parent", Placement{InboxID: &inboxID, ParentID: &parID}, false},
		{"section without project", Placement{ContextID: &ctxID, SectionID: &secID}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.p.Validate()
			if tc.ok && err != nil {
				t.Errorf("expected ok, got %v", err)
			}
			if !tc.ok && !errors.Is(err, ErrInvalidPlacement) {
				t.Errorf("expected ErrInvalidPlacement, got %v", err)
			}
		})
	}
}

func TestTaskRepo_Create_Inbox(t *testing.T) {
	f := newTaskFixture(t)
	inboxID := int64(1)
	task, err := f.tasks.Create(context.Background(), CreateTask{
		Placement: Placement{InboxID: &inboxID},
		Title:     "buy milk",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if task.InboxID == nil || *task.InboxID != 1 {
		t.Errorf("inbox_id mismatch: %+v", task.InboxID)
	}
	if task.Status != model.TaskStatusOpen {
		t.Errorf("status: got %s, want open", task.Status)
	}
	if task.Priority != model.PriorityNone {
		t.Errorf("priority: got %s, want no-priority", task.Priority)
	}
}

func TestTaskRepo_Create_AllPlacements(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()

	t1, err := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "in context",
	})
	if err != nil {
		t.Fatalf("ctx-only: %v", err)
	}
	t2, err := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID, ProjectID: &f.projectID},
		Title:     "in project",
	})
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	t3, err := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID, ProjectID: &f.projectID, SectionID: &f.sectionID},
		Title:     "in section",
	})
	if err != nil {
		t.Fatalf("section: %v", err)
	}
	if t1.ProjectID != nil || t2.SectionID != nil || t3.SectionID == nil {
		t.Errorf("placement bleed: %+v / %+v / %+v", t1, t2, t3)
	}
}

func TestTaskRepo_Create_InvalidPlacement(t *testing.T) {
	f := newTaskFixture(t)
	_, err := f.tasks.Create(context.Background(), CreateTask{Title: "no placement"})
	if !errors.Is(err, ErrInvalidPlacement) {
		t.Fatalf("expected ErrInvalidPlacement, got %v", err)
	}
}

func TestTaskRepo_Create_SubtaskInheritsViaParent(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	parent, err := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID, ProjectID: &f.projectID},
		Title:     "parent",
	})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	child, err := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID, ProjectID: &f.projectID, ParentID: &parent.ID},
		Title:     "child",
	})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	if child.ParentID == nil || *child.ParentID != parent.ID {
		t.Errorf("parent_id: %+v", child.ParentID)
	}
}

func TestTaskRepo_Update_BasicFields(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	task, _ := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "old",
	})
	newTitle := "new"
	highPri := model.PriorityHigh
	completed := model.TaskStatusCompleted
	got, err := f.tasks.Update(ctx, task.ID, TaskUpdate{
		Title:    &newTitle,
		Priority: &highPri,
		Status:   &completed,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.Title != "new" || got.Priority != model.PriorityHigh || got.Status != model.TaskStatusCompleted {
		t.Errorf("update: %+v", got)
	}
}

func TestTaskRepo_SetPinnedAndDelete(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	task, _ := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "pin me",
	})
	if err := f.tasks.SetPinned(ctx, task.ID, true); err != nil {
		t.Fatalf("pin: %v", err)
	}
	got, _ := f.tasks.Get(ctx, task.ID)
	if !got.IsPinned || got.PinnedAt == nil {
		t.Errorf("expected pinned, got %+v", got)
	}
	if err := f.tasks.Delete(ctx, task.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := f.tasks.Get(ctx, task.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestTaskRepo_Delete_CascadesSubtasks(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	parent, _ := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "p",
	})
	child, _ := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID, ParentID: &parent.ID},
		Title:     "c",
	})
	grandchild, _ := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID, ParentID: &child.ID},
		Title:     "gc",
	})
	if err := f.tasks.Delete(ctx, parent.ID); err != nil {
		t.Fatalf("delete parent: %v", err)
	}
	for _, id := range []int64{child.ID, grandchild.ID} {
		if _, err := f.tasks.Get(ctx, id); !errors.Is(err, ErrNotFound) {
			t.Errorf("expected cascade for %d, got %v", id, err)
		}
	}
}

func TestTaskRepo_Move_AcrossProjects(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	other, _ := f.projects.Create(ctx, CreateProject{ContextID: f.contextID, Title: "other", Color: "red"})

	parent, _ := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID, ProjectID: &f.projectID},
		Title:     "parent",
	})
	child, _ := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID, ProjectID: &f.projectID, ParentID: &parent.ID},
		Title:     "child",
	})
	if err := f.tasks.Move(ctx, parent.ID, Placement{ContextID: &f.contextID, ProjectID: &other.ID}); err != nil {
		t.Fatalf("move: %v", err)
	}
	gotParent, _ := f.tasks.Get(ctx, parent.ID)
	gotChild, _ := f.tasks.Get(ctx, child.ID)
	if gotParent.ProjectID == nil || *gotParent.ProjectID != other.ID {
		t.Errorf("parent project: %+v", gotParent.ProjectID)
	}
	if gotChild.ProjectID == nil || *gotChild.ProjectID != other.ID {
		t.Errorf("child project: %+v", gotChild.ProjectID)
	}
}

func TestTaskRepo_Move_RejectsCycle(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	a, _ := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "a",
	})
	b, _ := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID, ParentID: &a.ID},
		Title:     "b",
	})
	// Move a under b — would create a cycle.
	err := f.tasks.Move(ctx, a.ID, Placement{ContextID: &f.contextID, ParentID: &b.ID})
	if !errors.Is(err, ErrCycle) {
		t.Fatalf("expected ErrCycle, got %v", err)
	}
}

func TestTaskRepo_Move_RejectsSubtaskInInbox(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	parent, _ := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "p",
	})
	child, _ := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID, ParentID: &parent.ID},
		Title:     "c",
	})
	inboxID := int64(1)
	err := f.tasks.Move(ctx, child.ID, Placement{InboxID: &inboxID, ParentID: &parent.ID})
	if !errors.Is(err, ErrInvalidPlacement) {
		t.Fatalf("expected ErrInvalidPlacement, got %v", err)
	}
}

func TestTaskRepo_Counters(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	inboxID := int64(1)
	if _, err := f.tasks.Create(ctx, CreateTask{Placement: Placement{InboxID: &inboxID}, Title: "i1"}); err != nil {
		t.Fatalf("inbox: %v", err)
	}
	week := model.PlanStateWeek
	t1, _ := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "w",
		PlanState: week,
	})
	backlog := model.PlanStateBacklog
	if _, err := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "b",
		PlanState: backlog,
	}); err != nil {
		t.Fatalf("backlog: %v", err)
	}
	if err := f.tasks.SetPinned(ctx, t1.ID, true); err != nil {
		t.Fatalf("pin: %v", err)
	}

	if got, _ := f.tasks.CountInbox(ctx); got != 1 {
		t.Errorf("inbox: got %d, want 1", got)
	}
	if got, _ := f.tasks.CountWeek(ctx); got != 1 {
		t.Errorf("week: got %d, want 1", got)
	}
	if got, _ := f.tasks.CountBacklog(ctx); got != 1 {
		t.Errorf("backlog: got %d, want 1", got)
	}
	if got, _ := f.tasks.CountPinnedTasks(ctx); got != 1 {
		t.Errorf("pinned tasks: got %d, want 1", got)
	}
}

func TestTaskRepo_TroikiCategory_RoundTrip(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	task, _ := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "t",
	})
	if task.TroikiCategory != nil {
		t.Errorf("default troiki: got %v, want nil", task.TroikiCategory)
	}

	imp := model.TroikiCategoryImportant
	updated, err := f.tasks.Update(ctx, task.ID, TaskUpdate{TroikiCategory: &imp})
	if err != nil {
		t.Fatalf("set important: %v", err)
	}
	if updated.TroikiCategory == nil || *updated.TroikiCategory != model.TroikiCategoryImportant {
		t.Errorf("after set: got %v, want important", updated.TroikiCategory)
	}

	got, err := f.tasks.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.TroikiCategory == nil || *got.TroikiCategory != model.TroikiCategoryImportant {
		t.Errorf("after refetch: got %v, want important", got.TroikiCategory)
	}

	cleared, err := f.tasks.Update(ctx, task.ID, TaskUpdate{TroikiCategoryClear: true})
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if cleared.TroikiCategory != nil {
		t.Errorf("after clear: got %v, want nil", cleared.TroikiCategory)
	}
}

func TestTaskRepo_ListByTroikiCategory(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	imp := model.TroikiCategoryImportant
	med := model.TroikiCategoryMedium

	a, _ := f.tasks.Create(ctx, CreateTask{Placement: Placement{ContextID: &f.contextID}, Title: "a"})
	b, _ := f.tasks.Create(ctx, CreateTask{Placement: Placement{ContextID: &f.contextID}, Title: "b"})
	c, _ := f.tasks.Create(ctx, CreateTask{Placement: Placement{ContextID: &f.contextID}, Title: "c"})

	if _, err := f.tasks.Update(ctx, a.ID, TaskUpdate{TroikiCategory: &imp}); err != nil {
		t.Fatalf("a: %v", err)
	}
	if _, err := f.tasks.Update(ctx, b.ID, TaskUpdate{TroikiCategory: &imp}); err != nil {
		t.Fatalf("b: %v", err)
	}
	if _, err := f.tasks.Update(ctx, c.ID, TaskUpdate{TroikiCategory: &med}); err != nil {
		t.Fatalf("c: %v", err)
	}

	items, total, err := f.tasks.ListByTroikiCategory(ctx, model.TroikiCategoryImportant)
	if err != nil {
		t.Fatalf("list important: %v", err)
	}
	if total != 2 {
		t.Errorf("total: got %d, want 2", total)
	}
	if len(items) != 2 {
		t.Errorf("len: got %d, want 2", len(items))
	}
	for _, it := range items {
		if it.TroikiCategory == nil || *it.TroikiCategory != model.TroikiCategoryImportant {
			t.Errorf("filter: got %v", it.TroikiCategory)
		}
	}

	count, err := f.tasks.CountOpenByTroikiCategory(ctx, model.TroikiCategoryImportant)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Errorf("count: got %d, want 2", count)
	}

	completed := model.TaskStatusCompleted
	if _, err := f.tasks.Update(ctx, a.ID, TaskUpdate{Status: &completed}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	count, _ = f.tasks.CountOpenByTroikiCategory(ctx, model.TroikiCategoryImportant)
	if count != 1 {
		t.Errorf("count after complete: got %d, want 1", count)
	}
}

func TestTaskRepo_Move_ClearsTroikiCategoryWhenBecomingSubtask(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()

	parent, _ := f.tasks.Create(ctx, CreateTask{Placement: Placement{ContextID: &f.contextID}, Title: "parent"})
	tk, _ := f.tasks.Create(ctx, CreateTask{Placement: Placement{ContextID: &f.contextID}, Title: "categorised"})

	imp := model.TroikiCategoryImportant
	if _, err := f.tasks.Update(ctx, tk.ID, TaskUpdate{TroikiCategory: &imp}); err != nil {
		t.Fatalf("set cat: %v", err)
	}

	if err := f.tasks.Move(ctx, tk.ID, Placement{ContextID: &f.contextID, ParentID: &parent.ID}); err != nil {
		t.Fatalf("move: %v", err)
	}

	got, err := f.tasks.Get(ctx, tk.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.TroikiCategory != nil {
		t.Errorf("category after reparenting: got %v, want nil", *got.TroikiCategory)
	}
	if got.ParentID == nil || *got.ParentID != parent.ID {
		t.Errorf("parent_id: got %v, want %d", got.ParentID, parent.ID)
	}

	// Counter must reflect that the slot is now free.
	count, _ := f.tasks.CountOpenByTroikiCategory(ctx, model.TroikiCategoryImportant)
	if count != 0 {
		t.Errorf("important count after reparent: got %d, want 0", count)
	}
}

func TestTaskRepo_Move_KeepsTroikiCategoryWhenStayingRoot(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()

	other, err := f.contexts.Create(ctx, "other", "red", false)
	if err != nil {
		t.Fatalf("create other context: %v", err)
	}
	tk, _ := f.tasks.Create(ctx, CreateTask{Placement: Placement{ContextID: &f.contextID}, Title: "categorised"})

	imp := model.TroikiCategoryImportant
	if _, err := f.tasks.Update(ctx, tk.ID, TaskUpdate{TroikiCategory: &imp}); err != nil {
		t.Fatalf("set cat: %v", err)
	}

	if err := f.tasks.Move(ctx, tk.ID, Placement{ContextID: &other.ID}); err != nil {
		t.Fatalf("move: %v", err)
	}

	got, err := f.tasks.Get(ctx, tk.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.TroikiCategory == nil || *got.TroikiCategory != model.TroikiCategoryImportant {
		t.Errorf("category after lateral move: got %v, want important", got.TroikiCategory)
	}
}

func TestTaskRepo_Sort_PinnedAndPriority(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	low := model.PriorityLow
	high := model.PriorityHigh
	a, _ := f.tasks.Create(ctx, CreateTask{Placement: Placement{ContextID: &f.contextID}, Title: "a-low", Priority: low})
	time.Sleep(2 * time.Millisecond)
	b, _ := f.tasks.Create(ctx, CreateTask{Placement: Placement{ContextID: &f.contextID}, Title: "b-high", Priority: high})
	time.Sleep(2 * time.Millisecond)
	c, _ := f.tasks.Create(ctx, CreateTask{Placement: Placement{ContextID: &f.contextID}, Title: "c-low", Priority: low})
	if err := f.tasks.SetPinned(ctx, a.ID, true); err != nil {
		t.Fatalf("pin: %v", err)
	}

	items, total, err := f.tasks.ListByContext(ctx, f.contextID, true, TaskFilter{}, Page{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 3 {
		t.Errorf("total: got %d, want 3", total)
	}
	// pinned first → a; then by priority (b high) > then c (low, newer than... a but a was pinned).
	if items[0].ID != a.ID {
		t.Errorf("first should be pinned a, got %d", items[0].ID)
	}
	if items[1].ID != b.ID {
		t.Errorf("second by priority should be b, got %d", items[1].ID)
	}
	if items[2].ID != c.ID {
		t.Errorf("third should be c, got %d", items[2].ID)
	}
}

package repo

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

func newProjectFixtures(t *testing.T) (*ContextRepo, *ProjectRepo, *LabelRepo, *ProjectLabelsRepo, int64) {
	t.Helper()
	d := setupTestDB(t)
	cr := NewContextRepo(d)
	lr := NewLabelRepo(d)
	plr := NewProjectLabelsRepo(d)
	pr := NewProjectRepo(d, plr)
	c, err := cr.Create(context.Background(), "work", "blue", false)
	if err != nil {
		t.Fatalf("create context: %v", err)
	}
	return cr, pr, lr, plr, c.ID
}

func TestProjectRepo_CreateAndGet(t *testing.T) {
	_, pr, _, _, ctxID := newProjectFixtures(t)
	ctx := context.Background()

	p, err := pr.Create(ctx, CreateProject{ContextID: ctxID, Title: "alpha", Color: "blue"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if p.Title != "alpha" || p.ContextID != ctxID || p.Status != model.ProjectStatusOpen {
		t.Errorf("got %+v", p)
	}
	if p.IsPinned || p.PinnedAt != nil {
		t.Errorf("expected unpinned, got %+v", p)
	}
}

func TestProjectRepo_List_FilterByContextAndStatus_AndSort(t *testing.T) {
	_, pr, _, _, ctxID := newProjectFixtures(t)
	ctx := context.Background()

	p1, _ := pr.Create(ctx, CreateProject{ContextID: ctxID, Title: "a", Color: "blue"})
	time.Sleep(2 * time.Millisecond)
	p2, _ := pr.Create(ctx, CreateProject{ContextID: ctxID, Title: "b", Color: "blue"})
	time.Sleep(2 * time.Millisecond)
	p3, _ := pr.Create(ctx, CreateProject{ContextID: ctxID, Title: "c", Color: "blue"})

	if err := pr.SetPinned(ctx, p1.ID, true); err != nil {
		t.Fatalf("pin p1: %v", err)
	}

	items, total, err := pr.List(ctx, ProjectListFilter{ContextID: &ctxID}, Page{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 3 {
		t.Errorf("total: got %d, want 3", total)
	}
	if items[0].ID != p1.ID {
		t.Errorf("first should be pinned project p1, got id=%d", items[0].ID)
	}
	// Then unpinned by created_at DESC: p3, p2
	if items[1].ID != p3.ID || items[2].ID != p2.ID {
		t.Errorf("sort: got %d,%d, want %d,%d", items[1].ID, items[2].ID, p3.ID, p2.ID)
	}

	completedStatus := model.ProjectStatusCompleted
	if err := pr.UpdateStatus(ctx, p2.ID, completedStatus); err != nil {
		t.Fatalf("update status: %v", err)
	}
	items, _, err = pr.List(ctx, ProjectListFilter{Status: &completedStatus}, Page{})
	if err != nil {
		t.Fatalf("list status: %v", err)
	}
	if len(items) != 1 || items[0].ID != p2.ID {
		t.Errorf("status filter: got %+v", items)
	}
}

func TestProjectRepo_List_HydratesLabels(t *testing.T) {
	_, pr, lr, plr, ctxID := newProjectFixtures(t)
	ctx := context.Background()

	p, err := pr.Create(ctx, CreateProject{ContextID: ctxID, Title: "a", Color: "blue"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	l1, _ := lr.Create(ctx, "l1", "blue", false)
	l2, _ := lr.Create(ctx, "l2", "red", false)
	if err := plr.SetForProject(ctx, p.ID, []int64{l1.ID, l2.ID}); err != nil {
		t.Fatalf("set labels: %v", err)
	}

	items, _, err := pr.List(ctx, ProjectListFilter{}, Page{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 1 || len(items[0].Labels) != 2 {
		t.Fatalf("labels: %+v", items)
	}

	got, err := pr.Get(ctx, p.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got.Labels) != 2 {
		t.Errorf("get labels len: got %d, want 2", len(got.Labels))
	}

	// replacement clears previous
	if err := plr.SetForProject(ctx, p.ID, []int64{l1.ID}); err != nil {
		t.Fatalf("re-set labels: %v", err)
	}
	got, _ = pr.Get(ctx, p.ID)
	if len(got.Labels) != 1 {
		t.Errorf("after replace: got %d, want 1", len(got.Labels))
	}
}

func TestProjectRepo_SetPinned_ToggleAndGet(t *testing.T) {
	_, pr, _, _, ctxID := newProjectFixtures(t)
	ctx := context.Background()

	p, _ := pr.Create(ctx, CreateProject{ContextID: ctxID, Title: "a", Color: "blue"})
	if err := pr.SetPinned(ctx, p.ID, true); err != nil {
		t.Fatalf("pin: %v", err)
	}
	got, _ := pr.Get(ctx, p.ID)
	if !got.IsPinned || got.PinnedAt == nil {
		t.Errorf("expected pinned, got %+v", got)
	}
	if err := pr.SetPinned(ctx, p.ID, false); err != nil {
		t.Fatalf("unpin: %v", err)
	}
	got, _ = pr.Get(ctx, p.ID)
	if got.IsPinned || got.PinnedAt != nil {
		t.Errorf("expected unpinned, got %+v", got)
	}
}

func TestProjectRepo_Delete_CascadesSections(t *testing.T) {
	d := setupTestDB(t)
	cr := NewContextRepo(d)
	pr := NewProjectRepo(d, NewProjectLabelsRepo(d))
	sr := NewProjectSectionRepo(d)
	ctx := context.Background()

	c, err := cr.Create(ctx, "work", "blue", false)
	if err != nil {
		t.Fatalf("create context: %v", err)
	}
	p, err := pr.Create(ctx, CreateProject{ContextID: c.ID, Title: "a", Color: "blue"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := sr.Create(ctx, p.ID, "section"); err != nil {
		t.Fatalf("create section: %v", err)
	}
	if err := pr.Delete(ctx, p.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, total, err := sr.ListByProject(ctx, p.ID, Page{})
	if err != nil {
		t.Fatalf("list sections: %v", err)
	}
	if total != 0 {
		t.Errorf("expected cascade, got %d", total)
	}
}

func TestProjectRepo_Get_NotFound(t *testing.T) {
	_, pr, _, _, _ := newProjectFixtures(t)
	if _, err := pr.Get(context.Background(), 9999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestProjectRepo_Update_TroikiCategoryAndClear(t *testing.T) {
	_, pr, _, _, ctxID := newProjectFixtures(t)
	ctx := context.Background()

	p, err := pr.Create(ctx, CreateProject{ContextID: ctxID, Title: "a", Color: "blue"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	cat := model.TroikiCategoryImportant
	got, err := pr.Update(ctx, p.ID, ProjectUpdate{TroikiCategory: &cat})
	if err != nil {
		t.Fatalf("update set: %v", err)
	}
	if got.TroikiCategory == nil || *got.TroikiCategory != cat {
		t.Errorf("set: got %v, want %v", got.TroikiCategory, cat)
	}

	cleared, err := pr.Update(ctx, p.ID, ProjectUpdate{TroikiCategoryClear: true})
	if err != nil {
		t.Fatalf("update clear: %v", err)
	}
	if cleared.TroikiCategory != nil {
		t.Errorf("clear: got %v, want nil", cleared.TroikiCategory)
	}
}

func TestProjectRepo_SetTroikiCategoryIfRoom_AtomicCap(t *testing.T) {
	_, pr, _, _, ctxID := newProjectFixtures(t)
	ctx := context.Background()

	p1, _ := pr.Create(ctx, CreateProject{ContextID: ctxID, Title: "a", Color: "blue"})
	p2, _ := pr.Create(ctx, CreateProject{ContextID: ctxID, Title: "b", Color: "blue"})
	p3, _ := pr.Create(ctx, CreateProject{ContextID: ctxID, Title: "c", Color: "blue"})

	cat := model.TroikiCategoryImportant
	ok, err := pr.SetTroikiCategoryIfRoom(ctx, p1.ID, cat, 2)
	if err != nil || !ok {
		t.Fatalf("p1: ok=%v err=%v", ok, err)
	}
	ok, err = pr.SetTroikiCategoryIfRoom(ctx, p2.ID, cat, 2)
	if err != nil || !ok {
		t.Fatalf("p2: ok=%v err=%v", ok, err)
	}
	ok, err = pr.SetTroikiCategoryIfRoom(ctx, p3.ID, cat, 2)
	if err != nil {
		t.Fatalf("p3: err=%v", err)
	}
	if ok {
		t.Fatal("p3 should be rejected by capacity guard")
	}
}

func TestProjectRepo_SetTroikiCategoryIfRoom_NotFound(t *testing.T) {
	_, pr, _, _, _ := newProjectFixtures(t)
	_, err := pr.SetTroikiCategoryIfRoom(context.Background(), 9999, model.TroikiCategoryImportant, 3)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestProjectRepo_SetTroikiCategoryIfRoom_RejectsCompleted(t *testing.T) {
	_, pr, _, _, ctxID := newProjectFixtures(t)
	ctx := context.Background()
	p, _ := pr.Create(ctx, CreateProject{ContextID: ctxID, Title: "a", Color: "blue"})
	if err := pr.UpdateStatus(ctx, p.ID, model.ProjectStatusCompleted); err != nil {
		t.Fatalf("complete: %v", err)
	}
	ok, err := pr.SetTroikiCategoryIfRoom(ctx, p.ID, model.TroikiCategoryImportant, 3)
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if ok {
		t.Fatal("completed project should not accept troiki category")
	}
}

func TestProjectRepo_ListByTroikiCategory(t *testing.T) {
	_, pr, _, _, ctxID := newProjectFixtures(t)
	ctx := context.Background()

	a, _ := pr.Create(ctx, CreateProject{ContextID: ctxID, Title: "a", Color: "blue"})
	b, _ := pr.Create(ctx, CreateProject{ContextID: ctxID, Title: "b", Color: "blue"})
	c, _ := pr.Create(ctx, CreateProject{ContextID: ctxID, Title: "c", Color: "blue"})

	imp := model.TroikiCategoryImportant
	med := model.TroikiCategoryMedium
	if _, err := pr.Update(ctx, a.ID, ProjectUpdate{TroikiCategory: &imp}); err != nil {
		t.Fatalf("a: %v", err)
	}
	if _, err := pr.Update(ctx, b.ID, ProjectUpdate{TroikiCategory: &imp}); err != nil {
		t.Fatalf("b: %v", err)
	}
	if _, err := pr.Update(ctx, c.ID, ProjectUpdate{TroikiCategory: &med}); err != nil {
		t.Fatalf("c: %v", err)
	}

	items, total, err := pr.ListByTroikiCategory(ctx, imp)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Errorf("len/total: got %d/%d, want 2/2", len(items), total)
	}
	for _, it := range items {
		if it.TroikiCategory == nil || *it.TroikiCategory != imp {
			t.Errorf("filter: got %v", it.TroikiCategory)
		}
	}

	n, err := pr.CountOpenByTroikiCategory(ctx, imp)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 2 {
		t.Errorf("count: got %d, want 2", n)
	}

	if err := pr.UpdateStatus(ctx, a.ID, model.ProjectStatusCompleted); err != nil {
		t.Fatalf("complete a: %v", err)
	}
	n, _ = pr.CountOpenByTroikiCategory(ctx, imp)
	if n != 1 {
		t.Errorf("count after complete: got %d, want 1", n)
	}
}

func TestProjectRepo_UpdateStatus_ClearsTroikiCategoryWhenLeavingOpen(t *testing.T) {
	_, projects, _, _, ctxID := newProjectFixtures(t)
	ctx := context.Background()

	p, _ := projects.Create(ctx, CreateProject{ContextID: ctxID, Title: "p", Color: "blue"})
	cat := model.TroikiCategoryImportant
	if _, err := projects.Update(ctx, p.ID, ProjectUpdate{TroikiCategory: &cat}); err != nil {
		t.Fatalf("set category: %v", err)
	}

	if err := projects.UpdateStatus(ctx, p.ID, model.ProjectStatusArchived); err != nil {
		t.Fatalf("archive: %v", err)
	}
	got, err := projects.Get(ctx, p.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.TroikiCategory != nil {
		t.Errorf("category after archive: got %v, want nil", got.TroikiCategory)
	}

	// Reopening should leave the category cleared (re-assignment must go through
	// the capacity check).
	if err := projects.UpdateStatus(ctx, p.ID, model.ProjectStatusOpen); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, _ = projects.Get(ctx, p.ID)
	if got.TroikiCategory != nil {
		t.Errorf("category after reopen: got %v, want nil", got.TroikiCategory)
	}
}

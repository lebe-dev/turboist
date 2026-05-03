package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
)

func setupTroikiService(t *testing.T) (*service.TroikiService, *repo.TaskRepo, *repo.ContextRepo, *repo.UserRepo) {
	t.Helper()
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels)
	ctxs := repo.NewContextRepo(d)
	users := repo.NewUserRepo(d)
	if _, err := users.Create(context.Background(), "admin", "h"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return service.NewTroikiService(tasks, users), tasks, ctxs, users
}

func ptrCat(c model.TroikiCategory) *model.TroikiCategory { return &c }

func TestTroikiService_SetCategory_Important_HasCapacity(t *testing.T) {
	svc, tasks, ctxs, _ := setupTroikiService(t)
	ctx := context.Background()
	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID

	tk, err := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "x"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := svc.SetCategory(ctx, tk.ID, ptrCat(model.TroikiCategoryImportant))
	if err != nil {
		t.Fatalf("set important: %v", err)
	}
	if got.TroikiCategory == nil || *got.TroikiCategory != model.TroikiCategoryImportant {
		t.Errorf("category: got %v, want important", got.TroikiCategory)
	}
}

func TestTroikiService_SetCategory_Important_FullSlot(t *testing.T) {
	svc, tasks, ctxs, _ := setupTroikiService(t)
	ctx := context.Background()
	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID

	for i := 0; i < service.TroikiImportantCap; i++ {
		tk, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "imp"})
		if _, err := svc.SetCategory(ctx, tk.ID, ptrCat(model.TroikiCategoryImportant)); err != nil {
			t.Fatalf("seed important %d: %v", i, err)
		}
	}
	tk, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "extra"})
	_, err := svc.SetCategory(ctx, tk.ID, ptrCat(model.TroikiCategoryImportant))
	if !errors.Is(err, service.ErrTroikiSlotFull) {
		t.Fatalf("err: got %v, want ErrTroikiSlotFull", err)
	}
}

func TestTroikiService_SetCategory_Medium_NoCapacity(t *testing.T) {
	svc, tasks, ctxs, _ := setupTroikiService(t)
	ctx := context.Background()
	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID

	tk, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "m"})
	_, err := svc.SetCategory(ctx, tk.ID, ptrCat(model.TroikiCategoryMedium))
	if !errors.Is(err, service.ErrTroikiSlotFull) {
		t.Fatalf("err: got %v, want ErrTroikiSlotFull", err)
	}
}

func TestTroikiService_SetCategory_Medium_AfterCapacityGranted(t *testing.T) {
	svc, tasks, ctxs, users := setupTroikiService(t)
	ctx := context.Background()
	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID

	if err := users.IncTroikiCapacity(ctx, service.SingleUserID, model.TroikiCategoryMedium); err != nil {
		t.Fatalf("inc medium: %v", err)
	}
	tk, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "m"})
	got, err := svc.SetCategory(ctx, tk.ID, ptrCat(model.TroikiCategoryMedium))
	if err != nil {
		t.Fatalf("set medium: %v", err)
	}
	if got.TroikiCategory == nil || *got.TroikiCategory != model.TroikiCategoryMedium {
		t.Errorf("category: got %v, want medium", got.TroikiCategory)
	}
}

func TestTroikiService_SetCategory_Clear(t *testing.T) {
	svc, tasks, ctxs, _ := setupTroikiService(t)
	ctx := context.Background()
	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID

	tk, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "x"})
	if _, err := svc.SetCategory(ctx, tk.ID, ptrCat(model.TroikiCategoryImportant)); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := svc.SetCategory(ctx, tk.ID, nil)
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if got.TroikiCategory != nil {
		t.Errorf("category after clear: got %v, want nil", got.TroikiCategory)
	}
}

func TestTroikiService_SetCategory_RejectsSubtask(t *testing.T) {
	svc, tasks, ctxs, _ := setupTroikiService(t)
	ctx := context.Background()
	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID

	parent, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "parent"})
	pid := parent.ID
	child, _ := tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cid, ParentID: &pid},
		Title:     "child",
	})
	_, err := svc.SetCategory(ctx, child.ID, ptrCat(model.TroikiCategoryImportant))
	if !errors.Is(err, service.ErrTroikiNotRootTask) {
		t.Fatalf("err: got %v, want ErrTroikiNotRootTask", err)
	}
}

func TestTroikiService_SetCategory_RejectsCompleted(t *testing.T) {
	svc, tasks, ctxs, _ := setupTroikiService(t)
	ctx := context.Background()
	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID

	tk, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "x"})
	completed := model.TaskStatusCompleted
	if _, err := tasks.Update(ctx, tk.ID, repo.TaskUpdate{Status: &completed}); err != nil {
		t.Fatalf("update: %v", err)
	}
	_, err := svc.SetCategory(ctx, tk.ID, ptrCat(model.TroikiCategoryImportant))
	if !errors.Is(err, service.ErrTroikiNotRootTask) {
		t.Fatalf("err: got %v, want ErrTroikiNotRootTask", err)
	}
}

func TestTroikiService_SetCategory_SameCategoryNoop(t *testing.T) {
	svc, tasks, ctxs, _ := setupTroikiService(t)
	ctx := context.Background()
	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID

	for i := 0; i < service.TroikiImportantCap; i++ {
		tk, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "imp"})
		if _, err := svc.SetCategory(ctx, tk.ID, ptrCat(model.TroikiCategoryImportant)); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	// Re-set the first task to the same category. Slot is full but it's a no-op.
	first, _, _ := tasks.ListByTroikiCategory(ctx, model.TroikiCategoryImportant)
	if len(first) == 0 {
		t.Fatal("no important tasks")
	}
	if _, err := svc.SetCategory(ctx, first[0].ID, ptrCat(model.TroikiCategoryImportant)); err != nil {
		t.Errorf("re-set same: %v", err)
	}
}

func TestTroikiService_View(t *testing.T) {
	svc, tasks, ctxs, users := setupTroikiService(t)
	ctx := context.Background()
	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID

	if err := users.IncTroikiCapacity(ctx, service.SingleUserID, model.TroikiCategoryMedium); err != nil {
		t.Fatalf("inc medium: %v", err)
	}
	imp, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "i"})
	if _, err := svc.SetCategory(ctx, imp.ID, ptrCat(model.TroikiCategoryImportant)); err != nil {
		t.Fatalf("set imp: %v", err)
	}
	med, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "m"})
	if _, err := svc.SetCategory(ctx, med.ID, ptrCat(model.TroikiCategoryMedium)); err != nil {
		t.Fatalf("set med: %v", err)
	}

	view, err := svc.View(ctx)
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if view.Important.Capacity != service.TroikiImportantCap {
		t.Errorf("important capacity: got %d, want %d", view.Important.Capacity, service.TroikiImportantCap)
	}
	if len(view.Important.Tasks) != 1 {
		t.Errorf("important tasks: got %d, want 1", len(view.Important.Tasks))
	}
	if view.Medium.Capacity != 1 {
		t.Errorf("medium capacity: got %d, want 1", view.Medium.Capacity)
	}
	if len(view.Medium.Tasks) != 1 {
		t.Errorf("medium tasks: got %d, want 1", len(view.Medium.Tasks))
	}
	if view.Rest.Capacity != 0 {
		t.Errorf("rest capacity: got %d, want 0", view.Rest.Capacity)
	}
	if len(view.Rest.Tasks) != 0 {
		t.Errorf("rest tasks: got %d, want 0", len(view.Rest.Tasks))
	}
}

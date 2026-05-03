package service_test

import (
	"context"
	"errors"
	"sync"
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

func TestTroikiService_SetCategory_Medium_NoCapacity_AfterStart(t *testing.T) {
	svc, tasks, ctxs, _ := setupTroikiService(t)
	ctx := context.Background()
	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID

	if err := svc.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	tk, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "m"})
	_, err := svc.SetCategory(ctx, tk.ID, ptrCat(model.TroikiCategoryMedium))
	if !errors.Is(err, service.ErrTroikiSlotFull) {
		t.Fatalf("err: got %v, want ErrTroikiSlotFull", err)
	}
}

func TestTroikiService_SetCategory_Medium_BeforeStart_NoCap(t *testing.T) {
	svc, tasks, ctxs, _ := setupTroikiService(t)
	ctx := context.Background()
	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID

	tk, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "m"})
	got, err := svc.SetCategory(ctx, tk.ID, ptrCat(model.TroikiCategoryMedium))
	if err != nil {
		t.Fatalf("set medium before start: %v", err)
	}
	if got.TroikiCategory == nil || *got.TroikiCategory != model.TroikiCategoryMedium {
		t.Errorf("category: got %v, want medium", got.TroikiCategory)
	}
}

func TestTroikiService_SetCategory_Important_BeforeStart_StillCapped(t *testing.T) {
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
	if _, err := svc.SetCategory(ctx, tk.ID, ptrCat(model.TroikiCategoryImportant)); !errors.Is(err, service.ErrTroikiSlotFull) {
		t.Fatalf("important cap honored before start: got %v, want ErrTroikiSlotFull", err)
	}
}

func TestTroikiService_Start_SnapshotsCapacity(t *testing.T) {
	svc, tasks, ctxs, users := setupTroikiService(t)
	ctx := context.Background()
	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID

	for i := range 4 {
		tk, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "m"})
		if _, err := svc.SetCategory(ctx, tk.ID, ptrCat(model.TroikiCategoryMedium)); err != nil {
			t.Fatalf("seed medium %d: %v", i, err)
		}
	}
	for i := range 2 {
		tk, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "r"})
		if _, err := svc.SetCategory(ctx, tk.ID, ptrCat(model.TroikiCategoryRest)); err != nil {
			t.Fatalf("seed rest %d: %v", i, err)
		}
	}

	if err := svc.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	cap, err := users.GetTroikiCapacity(ctx, service.SingleUserID)
	if err != nil {
		t.Fatalf("get cap: %v", err)
	}
	if !cap.Started {
		t.Errorf("started: got false, want true")
	}
	if cap.Medium != 4 {
		t.Errorf("medium cap: got %d, want 4", cap.Medium)
	}
	if cap.Rest != 2 {
		t.Errorf("rest cap: got %d, want 2", cap.Rest)
	}

	// Idempotent: second Start does not re-snapshot if completions in between
	// changed counts (here, deletion of a medium would otherwise pull cap down).
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("start (idempotent): %v", err)
	}
	cap2, _ := users.GetTroikiCapacity(ctx, service.SingleUserID)
	if cap2.Medium != 4 || cap2.Rest != 2 {
		t.Errorf("cap after second start: got medium=%d rest=%d, want 4/2", cap2.Medium, cap2.Rest)
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

// Regression: with two near-simultaneous SetCategory calls for the same task
// and same target category, both should return success — the final state
// matches both requests. Before the fix, the loser's UPDATE saw the slot count
// inflated by its own just-assigned row and rejected the redundant write,
// surfacing as a false ErrTroikiSlotFull because the disambiguation reread
// only checked parent_id/status.
func TestTroikiService_SetCategory_ConcurrentSameCategory_NoFalseSlotFull(t *testing.T) {
	svc, tasks, ctxs, _ := setupTroikiService(t)
	ctx := context.Background()
	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID

	// Pre-fill capacity-1 slots so the racing pair targets the last slot.
	for i := 0; i < service.TroikiImportantCap-1; i++ {
		tk, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "imp"})
		if _, err := svc.SetCategory(ctx, tk.ID, ptrCat(model.TroikiCategoryImportant)); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	target, err := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "target"})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}

	const goroutines = 16
	var wg sync.WaitGroup
	errs := make([]error, goroutines)
	start := make(chan struct{})
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			_, errs[idx] = svc.SetCategory(ctx, target.ID, ptrCat(model.TroikiCategoryImportant))
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: got %v, want nil (final state matches request)", i, err)
		}
	}
	got, err := tasks.Get(ctx, target.ID)
	if err != nil {
		t.Fatalf("re-read target: %v", err)
	}
	if got.TroikiCategory == nil || *got.TroikiCategory != model.TroikiCategoryImportant {
		t.Errorf("final category: got %v, want important", got.TroikiCategory)
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

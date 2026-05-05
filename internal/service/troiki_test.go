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

func setupTroikiService(t *testing.T) (*service.TroikiService, *repo.TaskRepo, *repo.ProjectRepo, *repo.ContextRepo, *repo.UserRepo) {
	t.Helper()
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	plabels := repo.NewProjectLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels)
	projects := repo.NewProjectRepo(d, plabels)
	ctxs := repo.NewContextRepo(d)
	users := repo.NewUserRepo(d)
	if _, err := users.Create(context.Background(), "admin", "h"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return service.NewTroikiService(tasks, projects, users), tasks, projects, ctxs, users
}

func ptrCat(c model.TroikiCategory) *model.TroikiCategory { return &c }

func newProjectInCtx(t *testing.T, projects *repo.ProjectRepo, ctxID int64, title string) *model.Project {
	t.Helper()
	p, err := projects.Create(context.Background(), repo.CreateProject{ContextID: ctxID, Title: title, Color: "blue"})
	if err != nil {
		t.Fatalf("create project %q: %v", title, err)
	}
	return p
}

func TestTroikiService_SetCategory_PinsProjectTaskPriorities(t *testing.T) {
	svc, tasks, projects, ctxs, _ := setupTroikiService(t)
	ctx := context.Background()
	c, _ := ctxs.Create(ctx, "Work", "blue", false)

	cases := []struct {
		cat  model.TroikiCategory
		want model.Priority
	}{
		{model.TroikiCategoryImportant, model.PriorityHigh},
		{model.TroikiCategoryMedium, model.PriorityMedium},
		{model.TroikiCategoryRest, model.PriorityLow},
	}
	for _, tc := range cases {
		p := newProjectInCtx(t, projects, c.ID, string(tc.cat))
		root, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &c.ID, ProjectID: &p.ID}, Title: "root"})
		sub, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &c.ID, ProjectID: &p.ID, ParentID: &root.ID}, Title: "sub"})

		if _, err := svc.SetCategory(ctx, p.ID, ptrCat(tc.cat)); err != nil {
			t.Fatalf("set %s: %v", tc.cat, err)
		}
		// Both root and subtask must be pinned to derived priority.
		got, err := tasks.Get(ctx, root.ID)
		if err != nil {
			t.Fatalf("get root: %v", err)
		}
		if got.Priority != tc.want {
			t.Errorf("root priority for %s: got %s, want %s", tc.cat, got.Priority, tc.want)
		}
		gotSub, _ := tasks.Get(ctx, sub.ID)
		if gotSub.Priority != tc.want {
			t.Errorf("sub priority for %s: got %s, want %s", tc.cat, gotSub.Priority, tc.want)
		}
	}
}

func TestTroikiService_SetCategory_Important_HasCapacity(t *testing.T) {
	svc, _, projects, ctxs, _ := setupTroikiService(t)
	ctx := context.Background()
	c, _ := ctxs.Create(ctx, "Work", "blue", false)

	p := newProjectInCtx(t, projects, c.ID, "x")
	got, err := svc.SetCategory(ctx, p.ID, ptrCat(model.TroikiCategoryImportant))
	if err != nil {
		t.Fatalf("set important: %v", err)
	}
	if got.TroikiCategory == nil || *got.TroikiCategory != model.TroikiCategoryImportant {
		t.Errorf("category: got %v, want important", got.TroikiCategory)
	}
}

func TestTroikiService_SetCategory_Important_FullSlot(t *testing.T) {
	svc, _, projects, ctxs, _ := setupTroikiService(t)
	ctx := context.Background()
	c, _ := ctxs.Create(ctx, "Work", "blue", false)

	for i := 0; i < service.TroikiImportantCap; i++ {
		p := newProjectInCtx(t, projects, c.ID, "imp")
		if _, err := svc.SetCategory(ctx, p.ID, ptrCat(model.TroikiCategoryImportant)); err != nil {
			t.Fatalf("seed important %d: %v", i, err)
		}
	}
	extra := newProjectInCtx(t, projects, c.ID, "extra")
	_, err := svc.SetCategory(ctx, extra.ID, ptrCat(model.TroikiCategoryImportant))
	if !errors.Is(err, service.ErrTroikiSlotFull) {
		t.Fatalf("err: got %v, want ErrTroikiSlotFull", err)
	}
}

func TestTroikiService_SetCategory_Medium_NoCapacity_AfterStart(t *testing.T) {
	svc, _, projects, ctxs, _ := setupTroikiService(t)
	ctx := context.Background()
	c, _ := ctxs.Create(ctx, "Work", "blue", false)

	if err := svc.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	p := newProjectInCtx(t, projects, c.ID, "m")
	_, err := svc.SetCategory(ctx, p.ID, ptrCat(model.TroikiCategoryMedium))
	if !errors.Is(err, service.ErrTroikiSlotFull) {
		t.Fatalf("err: got %v, want ErrTroikiSlotFull", err)
	}
}

func TestTroikiService_SetCategory_Medium_BeforeStart_NoCap(t *testing.T) {
	svc, _, projects, ctxs, _ := setupTroikiService(t)
	ctx := context.Background()
	c, _ := ctxs.Create(ctx, "Work", "blue", false)

	p := newProjectInCtx(t, projects, c.ID, "m")
	got, err := svc.SetCategory(ctx, p.ID, ptrCat(model.TroikiCategoryMedium))
	if err != nil {
		t.Fatalf("set medium before start: %v", err)
	}
	if got.TroikiCategory == nil || *got.TroikiCategory != model.TroikiCategoryMedium {
		t.Errorf("category: got %v, want medium", got.TroikiCategory)
	}
}

func TestTroikiService_SetCategory_Important_BeforeStart_StillCapped(t *testing.T) {
	svc, _, projects, ctxs, _ := setupTroikiService(t)
	ctx := context.Background()
	c, _ := ctxs.Create(ctx, "Work", "blue", false)

	for i := 0; i < service.TroikiImportantCap; i++ {
		p := newProjectInCtx(t, projects, c.ID, "imp")
		if _, err := svc.SetCategory(ctx, p.ID, ptrCat(model.TroikiCategoryImportant)); err != nil {
			t.Fatalf("seed important %d: %v", i, err)
		}
	}
	extra := newProjectInCtx(t, projects, c.ID, "extra")
	if _, err := svc.SetCategory(ctx, extra.ID, ptrCat(model.TroikiCategoryImportant)); !errors.Is(err, service.ErrTroikiSlotFull) {
		t.Fatalf("important cap honored before start: got %v, want ErrTroikiSlotFull", err)
	}
}

func TestTroikiService_Start_SnapshotsCapacity(t *testing.T) {
	svc, _, projects, ctxs, users := setupTroikiService(t)
	ctx := context.Background()
	c, _ := ctxs.Create(ctx, "Work", "blue", false)

	for i := range 4 {
		_ = i
		p := newProjectInCtx(t, projects, c.ID, "m")
		if _, err := svc.SetCategory(ctx, p.ID, ptrCat(model.TroikiCategoryMedium)); err != nil {
			t.Fatalf("seed medium: %v", err)
		}
	}
	for i := range 2 {
		_ = i
		p := newProjectInCtx(t, projects, c.ID, "r")
		if _, err := svc.SetCategory(ctx, p.ID, ptrCat(model.TroikiCategoryRest)); err != nil {
			t.Fatalf("seed rest: %v", err)
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

	// Idempotent: second Start does not re-snapshot.
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("start (idempotent): %v", err)
	}
	cap2, _ := users.GetTroikiCapacity(ctx, service.SingleUserID)
	if cap2.Medium != 4 || cap2.Rest != 2 {
		t.Errorf("cap after second start: got medium=%d rest=%d, want 4/2", cap2.Medium, cap2.Rest)
	}
}

func TestTroikiService_SetCategory_Medium_AfterCapacityGranted(t *testing.T) {
	svc, _, projects, ctxs, users := setupTroikiService(t)
	ctx := context.Background()
	c, _ := ctxs.Create(ctx, "Work", "blue", false)

	if err := users.IncTroikiCapacity(ctx, service.SingleUserID, model.TroikiCategoryMedium); err != nil {
		t.Fatalf("inc medium: %v", err)
	}
	p := newProjectInCtx(t, projects, c.ID, "m")
	got, err := svc.SetCategory(ctx, p.ID, ptrCat(model.TroikiCategoryMedium))
	if err != nil {
		t.Fatalf("set medium: %v", err)
	}
	if got.TroikiCategory == nil || *got.TroikiCategory != model.TroikiCategoryMedium {
		t.Errorf("category: got %v, want medium", got.TroikiCategory)
	}
}

func TestTroikiService_SetCategory_Clear(t *testing.T) {
	svc, _, projects, ctxs, _ := setupTroikiService(t)
	ctx := context.Background()
	c, _ := ctxs.Create(ctx, "Work", "blue", false)

	p := newProjectInCtx(t, projects, c.ID, "x")
	if _, err := svc.SetCategory(ctx, p.ID, ptrCat(model.TroikiCategoryImportant)); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := svc.SetCategory(ctx, p.ID, nil)
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if got.TroikiCategory != nil {
		t.Errorf("category after clear: got %v, want nil", got.TroikiCategory)
	}
}

func TestTroikiService_SetCategory_RejectsClosedProject(t *testing.T) {
	svc, _, projects, ctxs, _ := setupTroikiService(t)
	ctx := context.Background()
	c, _ := ctxs.Create(ctx, "Work", "blue", false)

	p := newProjectInCtx(t, projects, c.ID, "closed")
	if err := projects.UpdateStatus(ctx, p.ID, model.ProjectStatusCompleted); err != nil {
		t.Fatalf("close project: %v", err)
	}
	_, err := svc.SetCategory(ctx, p.ID, ptrCat(model.TroikiCategoryImportant))
	if !errors.Is(err, service.ErrTroikiInvalidProject) {
		t.Fatalf("err: got %v, want ErrTroikiInvalidProject", err)
	}
}

func TestTroikiService_SetCategory_SameCategoryNoop(t *testing.T) {
	svc, _, projects, ctxs, _ := setupTroikiService(t)
	ctx := context.Background()
	c, _ := ctxs.Create(ctx, "Work", "blue", false)

	pids := []int64{}
	for i := 0; i < service.TroikiImportantCap; i++ {
		p := newProjectInCtx(t, projects, c.ID, "imp")
		if _, err := svc.SetCategory(ctx, p.ID, ptrCat(model.TroikiCategoryImportant)); err != nil {
			t.Fatalf("seed: %v", err)
		}
		pids = append(pids, p.ID)
	}
	// Re-set the first project to the same category. Slot is full but it's a no-op.
	if _, err := svc.SetCategory(ctx, pids[0], ptrCat(model.TroikiCategoryImportant)); err != nil {
		t.Errorf("re-set same: %v", err)
	}
}

func TestTroikiService_SetCategory_ConcurrentSameCategory_NoFalseSlotFull(t *testing.T) {
	svc, _, projects, ctxs, _ := setupTroikiService(t)
	ctx := context.Background()
	c, _ := ctxs.Create(ctx, "Work", "blue", false)

	// Pre-fill capacity-1 slots so the racing pair targets the last slot.
	for i := 0; i < service.TroikiImportantCap-1; i++ {
		p := newProjectInCtx(t, projects, c.ID, "imp")
		if _, err := svc.SetCategory(ctx, p.ID, ptrCat(model.TroikiCategoryImportant)); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	target := newProjectInCtx(t, projects, c.ID, "target")

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
	got, err := projects.Get(ctx, target.ID)
	if err != nil {
		t.Fatalf("re-read target: %v", err)
	}
	if got.TroikiCategory == nil || *got.TroikiCategory != model.TroikiCategoryImportant {
		t.Errorf("final category: got %v, want important", got.TroikiCategory)
	}
}

func TestTroikiService_View(t *testing.T) {
	svc, tasks, projects, ctxs, users := setupTroikiService(t)
	ctx := context.Background()
	c, _ := ctxs.Create(ctx, "Work", "blue", false)

	if err := users.IncTroikiCapacity(ctx, service.SingleUserID, model.TroikiCategoryMedium); err != nil {
		t.Fatalf("inc medium: %v", err)
	}
	imp := newProjectInCtx(t, projects, c.ID, "i")
	if _, err := svc.SetCategory(ctx, imp.ID, ptrCat(model.TroikiCategoryImportant)); err != nil {
		t.Fatalf("set imp: %v", err)
	}
	// Important project gets a root task and a subtask.
	root, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &c.ID, ProjectID: &imp.ID}, Title: "root"})
	if _, err := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &c.ID, ProjectID: &imp.ID, ParentID: &root.ID}, Title: "sub"}); err != nil {
		t.Fatalf("create sub: %v", err)
	}

	med := newProjectInCtx(t, projects, c.ID, "m")
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
	if len(view.Important.Projects) != 1 {
		t.Fatalf("important projects: got %d, want 1", len(view.Important.Projects))
	}
	if got := view.Important.Tasks[imp.ID]; len(got) != 2 {
		t.Errorf("important tasks for imp: got %d, want 2 (root+sub)", len(got))
	}
	if view.Medium.Capacity != 1 {
		t.Errorf("medium capacity: got %d, want 1", view.Medium.Capacity)
	}
	if len(view.Medium.Projects) != 1 {
		t.Errorf("medium projects: got %d, want 1", len(view.Medium.Projects))
	}
	if view.Rest.Capacity != 0 {
		t.Errorf("rest capacity: got %d, want 0", view.Rest.Capacity)
	}
	if len(view.Rest.Projects) != 0 {
		t.Errorf("rest projects: got %d, want 0", len(view.Rest.Projects))
	}
}

func TestTroikiService_EnforceProjectPriority(t *testing.T) {
	svc, tasks, projects, ctxs, _ := setupTroikiService(t)
	ctx := context.Background()
	c, _ := ctxs.Create(ctx, "Work", "blue", false)

	p := newProjectInCtx(t, projects, c.ID, "p")
	root, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &c.ID, ProjectID: &p.ID}, Title: "root", Priority: model.PriorityLow})
	sub, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &c.ID, ProjectID: &p.ID, ParentID: &root.ID}, Title: "sub", Priority: model.PriorityNone})

	if err := svc.EnforceProjectPriority(ctx, p.ID, model.PriorityHigh); err != nil {
		t.Fatalf("enforce: %v", err)
	}
	r, _ := tasks.Get(ctx, root.ID)
	if r.Priority != model.PriorityHigh {
		t.Errorf("root: got %s, want high", r.Priority)
	}
	s, _ := tasks.Get(ctx, sub.ID)
	if s.Priority != model.PriorityHigh {
		t.Errorf("sub: got %s, want high", s.Priority)
	}
}

func TestTroikiService_SetCategory_ResetsGrantFlagOnRecategorise(t *testing.T) {
	svc, tasks, projects, ctxs, _ := setupTroikiService(t)
	ctx := context.Background()
	c, _ := ctxs.Create(ctx, "Work", "blue", false)

	p := newProjectInCtx(t, projects, c.ID, "p")
	tk, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &c.ID, ProjectID: &p.ID}, Title: "t"})

	if _, err := svc.SetCategory(ctx, p.ID, ptrCat(model.TroikiCategoryImportant)); err != nil {
		t.Fatalf("set important: %v", err)
	}
	// Mark the task as having received its capacity grant (mirrors what a
	// successful Complete would set).
	granted, err := tasks.GrantAndBumpTroikiCapacity(ctx, tk.ID, service.SingleUserID, "troiki_medium_capacity")
	if err != nil || !granted {
		t.Fatalf("first grant: granted=%v err=%v", granted, err)
	}
	// Re-calling without a reset must return false (already granted) — proves
	// the flag persists across calls.
	if g, _ := tasks.GrantAndBumpTroikiCapacity(ctx, tk.ID, service.SingleUserID, "troiki_medium_capacity"); g {
		t.Fatal("second grant should be a no-op without recategorisation")
	}

	if _, err := svc.SetCategory(ctx, p.ID, nil); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if _, err := svc.SetCategory(ctx, p.ID, ptrCat(model.TroikiCategoryImportant)); err != nil {
		t.Fatalf("re-set important: %v", err)
	}
	// After recategorisation the flag must be reset, so a new grant succeeds.
	g, err := tasks.GrantAndBumpTroikiCapacity(ctx, tk.ID, service.SingleUserID, "troiki_medium_capacity")
	if err != nil {
		t.Fatalf("grant after recategorise: %v", err)
	}
	if !g {
		t.Errorf("grant after recategorise: got false, want true (flag must be reset)")
	}
}

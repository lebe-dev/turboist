package service_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.RunMigrations(context.Background(), d); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d
}

func setupCompleteService(t *testing.T) (*service.CompleteService, *repo.TaskRepo, *repo.ContextRepo, *repo.UserRepo) {
	t.Helper()
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels)
	ctxs := repo.NewContextRepo(d)
	users := repo.NewUserRepo(d)
	svc := service.NewCompleteService(tasks, users)
	return svc, tasks, ctxs, users
}

func TestCompleteService_SimpleTask(t *testing.T) {
	svc, tasks, ctxs, _ := setupCompleteService(t)
	ctx := context.Background()

	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID
	task, err := tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cid},
		Title:     "Simple task",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	result, err := svc.Complete(ctx, task.ID)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if result.Status != model.TaskStatusCompleted {
		t.Errorf("status: got %q, want %q", result.Status, model.TaskStatusCompleted)
	}
}

func TestCompleteService_Recurring_AdvancesDueAt(t *testing.T) {
	svc, tasks, ctxs, _ := setupCompleteService(t)
	ctx := context.Background()

	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID
	due := time.Now().Add(24 * time.Hour)
	rruleStr := "FREQ=DAILY;INTERVAL=1"
	task, err := tasks.Create(ctx, repo.CreateTask{
		Placement:      repo.Placement{ContextID: &cid},
		Title:          "Daily task",
		DueAt:          &due,
		RecurrenceRule: &rruleStr,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	result, err := svc.Complete(ctx, task.ID)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if result.Status != model.TaskStatusOpen {
		t.Errorf("status: got %q, want open (recurring task should stay open)", result.Status)
	}
	if result.DueAt == nil {
		t.Fatal("dueAt should not be nil after advancing")
	}
	if !result.DueAt.After(due) {
		t.Errorf("dueAt should advance: got %v, original was %v", result.DueAt, due)
	}
}

func TestCompleteService_Recurring_CountExhausted(t *testing.T) {
	svc, tasks, ctxs, _ := setupCompleteService(t)
	ctx := context.Background()

	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID
	due := time.Now().Add(24 * time.Hour)
	rruleStr := "FREQ=DAILY;COUNT=1"
	task, err := tasks.Create(ctx, repo.CreateTask{
		Placement:      repo.Placement{ContextID: &cid},
		Title:          "Once task",
		DueAt:          &due,
		RecurrenceRule: &rruleStr,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	result, err := svc.Complete(ctx, task.ID)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if result.Status != model.TaskStatusCompleted {
		t.Errorf("status: got %q, want completed (COUNT=1 should exhaust)", result.Status)
	}
}

func TestCompleteService_Uncomplete(t *testing.T) {
	svc, tasks, ctxs, _ := setupCompleteService(t)
	ctx := context.Background()

	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID
	task, _ := tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cid},
		Title:     "Task",
	})

	if _, err := svc.Complete(ctx, task.ID); err != nil {
		t.Fatalf("complete: %v", err)
	}
	result, err := svc.Uncomplete(ctx, task.ID)
	if err != nil {
		t.Fatalf("uncomplete: %v", err)
	}
	if result.Status != model.TaskStatusOpen {
		t.Errorf("status: got %q, want open", result.Status)
	}
}

func TestCompleteService_TroikiHook_ImportantGrantsMedium(t *testing.T) {
	svc, tasks, ctxs, users := setupCompleteService(t)
	ctx := context.Background()

	if _, err := users.Create(ctx, "admin", "h"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID
	cat := model.TroikiCategoryImportant
	tk, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "imp"})
	if _, err := tasks.Update(ctx, tk.ID, repo.TaskUpdate{TroikiCategory: &cat}); err != nil {
		t.Fatalf("set cat: %v", err)
	}

	if _, err := svc.Complete(ctx, tk.ID); err != nil {
		t.Fatalf("complete: %v", err)
	}
	cap, err := users.GetTroikiCapacity(ctx, service.SingleUserID)
	if err != nil {
		t.Fatalf("get capacity: %v", err)
	}
	if cap.Medium != 1 {
		t.Errorf("medium capacity: got %d, want 1", cap.Medium)
	}
	if cap.Rest != 0 {
		t.Errorf("rest capacity: got %d, want 0", cap.Rest)
	}
}

func TestCompleteService_TroikiHook_MediumGrantsRest(t *testing.T) {
	svc, tasks, ctxs, users := setupCompleteService(t)
	ctx := context.Background()

	if _, err := users.Create(ctx, "admin", "h"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID
	cat := model.TroikiCategoryMedium
	tk, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "med"})
	if _, err := tasks.Update(ctx, tk.ID, repo.TaskUpdate{TroikiCategory: &cat}); err != nil {
		t.Fatalf("set cat: %v", err)
	}
	if _, err := svc.Complete(ctx, tk.ID); err != nil {
		t.Fatalf("complete: %v", err)
	}
	cap, err := users.GetTroikiCapacity(ctx, service.SingleUserID)
	if err != nil {
		t.Fatalf("get capacity: %v", err)
	}
	if cap.Medium != 0 {
		t.Errorf("medium capacity: got %d, want 0", cap.Medium)
	}
	if cap.Rest != 1 {
		t.Errorf("rest capacity: got %d, want 1", cap.Rest)
	}
}

func TestCompleteService_TroikiHook_RestNoCapacity(t *testing.T) {
	svc, tasks, ctxs, users := setupCompleteService(t)
	ctx := context.Background()

	if _, err := users.Create(ctx, "admin", "h"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID
	cat := model.TroikiCategoryRest
	tk, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "rest"})
	if _, err := tasks.Update(ctx, tk.ID, repo.TaskUpdate{TroikiCategory: &cat}); err != nil {
		t.Fatalf("set cat: %v", err)
	}
	if _, err := svc.Complete(ctx, tk.ID); err != nil {
		t.Fatalf("complete: %v", err)
	}
	cap, _ := users.GetTroikiCapacity(ctx, service.SingleUserID)
	if cap.Medium != 0 || cap.Rest != 0 {
		t.Errorf("capacity: got %+v, want all zero", cap)
	}
}

func TestCompleteService_TroikiHook_NoCategoryNoEffect(t *testing.T) {
	svc, tasks, ctxs, users := setupCompleteService(t)
	ctx := context.Background()

	if _, err := users.Create(ctx, "admin", "h"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID
	tk, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "plain"})

	if _, err := svc.Complete(ctx, tk.ID); err != nil {
		t.Fatalf("complete: %v", err)
	}
	cap, _ := users.GetTroikiCapacity(ctx, service.SingleUserID)
	if cap.Medium != 0 || cap.Rest != 0 {
		t.Errorf("capacity: got %+v, want all zero", cap)
	}
}

func TestCompleteService_TroikiHook_RecurringNonTerminalNoBump(t *testing.T) {
	svc, tasks, ctxs, users := setupCompleteService(t)
	ctx := context.Background()

	if _, err := users.Create(ctx, "admin", "h"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID
	due := time.Now().Add(24 * time.Hour)
	rruleStr := "FREQ=DAILY;INTERVAL=1"
	cat := model.TroikiCategoryImportant
	tk, _ := tasks.Create(ctx, repo.CreateTask{
		Placement:      repo.Placement{ContextID: &cid},
		Title:          "daily imp",
		DueAt:          &due,
		RecurrenceRule: &rruleStr,
	})
	if _, err := tasks.Update(ctx, tk.ID, repo.TaskUpdate{TroikiCategory: &cat}); err != nil {
		t.Fatalf("set cat: %v", err)
	}
	if _, err := svc.Complete(ctx, tk.ID); err != nil {
		t.Fatalf("complete: %v", err)
	}
	cap, _ := users.GetTroikiCapacity(ctx, service.SingleUserID)
	if cap.Medium != 0 {
		t.Errorf("medium capacity: got %d, want 0 (recurring non-terminal)", cap.Medium)
	}
}

func TestCompleteService_TroikiHook_RecurringTerminalBumps(t *testing.T) {
	svc, tasks, ctxs, users := setupCompleteService(t)
	ctx := context.Background()

	if _, err := users.Create(ctx, "admin", "h"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID
	due := time.Now().Add(24 * time.Hour)
	rruleStr := "FREQ=DAILY;COUNT=1"
	cat := model.TroikiCategoryImportant
	tk, _ := tasks.Create(ctx, repo.CreateTask{
		Placement:      repo.Placement{ContextID: &cid},
		Title:          "once imp",
		DueAt:          &due,
		RecurrenceRule: &rruleStr,
	})
	if _, err := tasks.Update(ctx, tk.ID, repo.TaskUpdate{TroikiCategory: &cat}); err != nil {
		t.Fatalf("set cat: %v", err)
	}
	if _, err := svc.Complete(ctx, tk.ID); err != nil {
		t.Fatalf("complete: %v", err)
	}
	cap, _ := users.GetTroikiCapacity(ctx, service.SingleUserID)
	if cap.Medium != 1 {
		t.Errorf("medium capacity: got %d, want 1 (recurring terminal)", cap.Medium)
	}
}

func TestCompleteService_TroikiHook_DoubleCompleteNoBump(t *testing.T) {
	// Re-completing an already-completed task must not re-grant capacity.
	svc, tasks, ctxs, users := setupCompleteService(t)
	ctx := context.Background()

	if _, err := users.Create(ctx, "admin", "h"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID
	cat := model.TroikiCategoryImportant
	tk, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "imp"})
	if _, err := tasks.Update(ctx, tk.ID, repo.TaskUpdate{TroikiCategory: &cat}); err != nil {
		t.Fatalf("set cat: %v", err)
	}
	if _, err := svc.Complete(ctx, tk.ID); err != nil {
		t.Fatalf("complete 1: %v", err)
	}
	if _, err := svc.Complete(ctx, tk.ID); err != nil {
		t.Fatalf("complete 2: %v", err)
	}
	cap, _ := users.GetTroikiCapacity(ctx, service.SingleUserID)
	if cap.Medium != 1 {
		t.Errorf("medium capacity: got %d, want 1 (no double-bump)", cap.Medium)
	}
}

func TestCompleteService_TroikiHook_UncompleteRecompleteNoBump(t *testing.T) {
	// Completing then uncompleting then re-completing the same Important task
	// must not grant Medium capacity twice — the grant is bound to the current
	// categorisation, not to each status transition.
	svc, tasks, ctxs, users := setupCompleteService(t)
	ctx := context.Background()

	if _, err := users.Create(ctx, "admin", "h"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID
	cat := model.TroikiCategoryImportant
	tk, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "imp"})
	if _, err := tasks.Update(ctx, tk.ID, repo.TaskUpdate{TroikiCategory: &cat}); err != nil {
		t.Fatalf("set cat: %v", err)
	}
	if _, err := svc.Complete(ctx, tk.ID); err != nil {
		t.Fatalf("complete 1: %v", err)
	}
	if _, err := svc.Uncomplete(ctx, tk.ID); err != nil {
		t.Fatalf("uncomplete: %v", err)
	}
	if _, err := svc.Complete(ctx, tk.ID); err != nil {
		t.Fatalf("complete 2: %v", err)
	}
	cap, _ := users.GetTroikiCapacity(ctx, service.SingleUserID)
	if cap.Medium != 1 {
		t.Errorf("medium capacity: got %d, want 1 (no double-bump on uncomplete/recomplete)", cap.Medium)
	}
}

func TestCompleteService_TroikiHook_RecategoriseGrantsAgain(t *testing.T) {
	// Clearing the category and re-assigning should reset the grant flag, so
	// the next completion grants capacity again. This preserves the spec:
	// each (task, current-categorisation) earns one bump.
	svc, tasks, ctxs, users := setupCompleteService(t)
	ctx := context.Background()

	if _, err := users.Create(ctx, "admin", "h"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID
	cat := model.TroikiCategoryImportant
	tk, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "imp"})
	if _, err := tasks.Update(ctx, tk.ID, repo.TaskUpdate{TroikiCategory: &cat}); err != nil {
		t.Fatalf("set cat: %v", err)
	}
	if _, err := svc.Complete(ctx, tk.ID); err != nil {
		t.Fatalf("complete 1: %v", err)
	}
	if _, err := svc.Uncomplete(ctx, tk.ID); err != nil {
		t.Fatalf("uncomplete: %v", err)
	}
	if _, err := tasks.Update(ctx, tk.ID, repo.TaskUpdate{TroikiCategoryClear: true}); err != nil {
		t.Fatalf("clear cat: %v", err)
	}
	if _, err := tasks.Update(ctx, tk.ID, repo.TaskUpdate{TroikiCategory: &cat}); err != nil {
		t.Fatalf("re-set cat: %v", err)
	}
	if _, err := svc.Complete(ctx, tk.ID); err != nil {
		t.Fatalf("complete 2: %v", err)
	}
	cap, _ := users.GetTroikiCapacity(ctx, service.SingleUserID)
	if cap.Medium != 2 {
		t.Errorf("medium capacity: got %d, want 2 (recategorisation grants again)", cap.Medium)
	}
}

func TestCompleteService_Cancel(t *testing.T) {
	svc, tasks, ctxs, _ := setupCompleteService(t)
	ctx := context.Background()

	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID
	task, _ := tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cid},
		Title:     "Task",
	})

	result, err := svc.Cancel(ctx, task.ID)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if result.Status != model.TaskStatusCancelled {
		t.Errorf("status: got %q, want cancelled", result.Status)
	}
}

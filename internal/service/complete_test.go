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

type completeFixtures struct {
	svc      *service.CompleteService
	tasks    *repo.TaskRepo
	projects *repo.ProjectRepo
	ctxs     *repo.ContextRepo
	users    *repo.UserRepo
}

func setupCompleteService(t *testing.T) *completeFixtures {
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
	svc := service.NewCompleteService(tasks, projects, users)
	return &completeFixtures{svc: svc, tasks: tasks, projects: projects, ctxs: ctxs, users: users}
}

// projectInCategory creates an open project bound to ctxID with the given
// Troiki category set directly (bypassing capacity checks — these tests only
// exercise CompleteService behaviour).
func projectInCategory(t *testing.T, f *completeFixtures, ctxID int64, cat *model.TroikiCategory) *model.Project {
	t.Helper()
	p, err := f.projects.Create(context.Background(), repo.CreateProject{ContextID: ctxID, Title: "p", Color: "blue"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if cat != nil {
		if _, err := f.projects.Update(context.Background(), p.ID, repo.ProjectUpdate{TroikiCategory: cat}); err != nil {
			t.Fatalf("set project cat: %v", err)
		}
	}
	return p
}

func TestCompleteService_SimpleTask(t *testing.T) {
	f := setupCompleteService(t)
	ctx := context.Background()
	c, _ := f.ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID
	task, err := f.tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cid},
		Title:     "Simple task",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	result, err := f.svc.Complete(ctx, task.ID)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if result.Status != model.TaskStatusCompleted {
		t.Errorf("status: got %q, want %q", result.Status, model.TaskStatusCompleted)
	}
}

func TestCompleteService_Recurring_AdvancesDueAt(t *testing.T) {
	f := setupCompleteService(t)
	ctx := context.Background()
	c, _ := f.ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID
	due := time.Now().Add(24 * time.Hour)
	rruleStr := "FREQ=DAILY;INTERVAL=1"
	task, err := f.tasks.Create(ctx, repo.CreateTask{
		Placement:      repo.Placement{ContextID: &cid},
		Title:          "Daily task",
		DueAt:          &due,
		RecurrenceRule: &rruleStr,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	result, err := f.svc.Complete(ctx, task.ID)
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
	f := setupCompleteService(t)
	ctx := context.Background()
	c, _ := f.ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID
	due := time.Now().Add(24 * time.Hour)
	rruleStr := "FREQ=DAILY;COUNT=1"
	task, err := f.tasks.Create(ctx, repo.CreateTask{
		Placement:      repo.Placement{ContextID: &cid},
		Title:          "Once task",
		DueAt:          &due,
		RecurrenceRule: &rruleStr,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	result, err := f.svc.Complete(ctx, task.ID)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if result.Status != model.TaskStatusCompleted {
		t.Errorf("status: got %q, want completed (COUNT=1 should exhaust)", result.Status)
	}
}

func TestCompleteService_Uncomplete(t *testing.T) {
	f := setupCompleteService(t)
	ctx := context.Background()
	c, _ := f.ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID
	task, _ := f.tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cid},
		Title:     "Task",
	})

	if _, err := f.svc.Complete(ctx, task.ID); err != nil {
		t.Fatalf("complete: %v", err)
	}
	result, err := f.svc.Uncomplete(ctx, task.ID)
	if err != nil {
		t.Fatalf("uncomplete: %v", err)
	}
	if result.Status != model.TaskStatusOpen {
		t.Errorf("status: got %q, want open", result.Status)
	}
}

func TestCompleteService_TroikiHook_ImportantProject_GrantsMedium(t *testing.T) {
	f := setupCompleteService(t)
	ctx := context.Background()
	c, _ := f.ctxs.Create(ctx, "Work", "blue", false)
	cat := model.TroikiCategoryImportant
	p := projectInCategory(t, f, c.ID, &cat)
	tk, _ := f.tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &c.ID, ProjectID: &p.ID}, Title: "imp"})

	if _, err := f.svc.Complete(ctx, tk.ID); err != nil {
		t.Fatalf("complete: %v", err)
	}
	cap, err := f.users.GetTroikiCapacity(ctx, service.SingleUserID)
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

func TestCompleteService_TroikiHook_MediumProject_GrantsRest(t *testing.T) {
	f := setupCompleteService(t)
	ctx := context.Background()
	c, _ := f.ctxs.Create(ctx, "Work", "blue", false)
	cat := model.TroikiCategoryMedium
	p := projectInCategory(t, f, c.ID, &cat)
	tk, _ := f.tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &c.ID, ProjectID: &p.ID}, Title: "med"})

	if _, err := f.svc.Complete(ctx, tk.ID); err != nil {
		t.Fatalf("complete: %v", err)
	}
	cap, _ := f.users.GetTroikiCapacity(ctx, service.SingleUserID)
	if cap.Medium != 0 {
		t.Errorf("medium capacity: got %d, want 0", cap.Medium)
	}
	if cap.Rest != 1 {
		t.Errorf("rest capacity: got %d, want 1", cap.Rest)
	}
}

func TestCompleteService_TroikiHook_RestProject_NoBump(t *testing.T) {
	f := setupCompleteService(t)
	ctx := context.Background()
	c, _ := f.ctxs.Create(ctx, "Work", "blue", false)
	cat := model.TroikiCategoryRest
	p := projectInCategory(t, f, c.ID, &cat)
	tk, _ := f.tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &c.ID, ProjectID: &p.ID}, Title: "rest"})

	if _, err := f.svc.Complete(ctx, tk.ID); err != nil {
		t.Fatalf("complete: %v", err)
	}
	cap, _ := f.users.GetTroikiCapacity(ctx, service.SingleUserID)
	if cap.Medium != 0 || cap.Rest != 0 {
		t.Errorf("capacity: got %+v, want all zero", cap)
	}
}

func TestCompleteService_TroikiHook_NoProjectNoEffect(t *testing.T) {
	f := setupCompleteService(t)
	ctx := context.Background()
	c, _ := f.ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID
	tk, _ := f.tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "plain"})

	if _, err := f.svc.Complete(ctx, tk.ID); err != nil {
		t.Fatalf("complete: %v", err)
	}
	cap, _ := f.users.GetTroikiCapacity(ctx, service.SingleUserID)
	if cap.Medium != 0 || cap.Rest != 0 {
		t.Errorf("capacity: got %+v, want all zero", cap)
	}
}

func TestCompleteService_TroikiHook_UncategorisedProject_NoBump(t *testing.T) {
	f := setupCompleteService(t)
	ctx := context.Background()
	c, _ := f.ctxs.Create(ctx, "Work", "blue", false)
	p := projectInCategory(t, f, c.ID, nil)
	tk, _ := f.tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &c.ID, ProjectID: &p.ID}, Title: "x"})

	if _, err := f.svc.Complete(ctx, tk.ID); err != nil {
		t.Fatalf("complete: %v", err)
	}
	cap, _ := f.users.GetTroikiCapacity(ctx, service.SingleUserID)
	if cap.Medium != 0 || cap.Rest != 0 {
		t.Errorf("capacity: got %+v, want all zero", cap)
	}
}

func TestCompleteService_TroikiHook_RecurringNonTerminal_NoBump(t *testing.T) {
	f := setupCompleteService(t)
	ctx := context.Background()
	c, _ := f.ctxs.Create(ctx, "Work", "blue", false)
	cat := model.TroikiCategoryImportant
	p := projectInCategory(t, f, c.ID, &cat)
	due := time.Now().Add(24 * time.Hour)
	rruleStr := "FREQ=DAILY;INTERVAL=1"
	tk, _ := f.tasks.Create(ctx, repo.CreateTask{
		Placement:      repo.Placement{ContextID: &c.ID, ProjectID: &p.ID},
		Title:          "daily imp",
		DueAt:          &due,
		RecurrenceRule: &rruleStr,
	})
	if _, err := f.svc.Complete(ctx, tk.ID); err != nil {
		t.Fatalf("complete: %v", err)
	}
	cap, _ := f.users.GetTroikiCapacity(ctx, service.SingleUserID)
	if cap.Medium != 0 {
		t.Errorf("medium capacity: got %d, want 0 (recurring non-terminal)", cap.Medium)
	}
}

func TestCompleteService_TroikiHook_RecurringTerminal_Bumps(t *testing.T) {
	f := setupCompleteService(t)
	ctx := context.Background()
	c, _ := f.ctxs.Create(ctx, "Work", "blue", false)
	cat := model.TroikiCategoryImportant
	p := projectInCategory(t, f, c.ID, &cat)
	due := time.Now().Add(24 * time.Hour)
	rruleStr := "FREQ=DAILY;COUNT=1"
	tk, _ := f.tasks.Create(ctx, repo.CreateTask{
		Placement:      repo.Placement{ContextID: &c.ID, ProjectID: &p.ID},
		Title:          "once imp",
		DueAt:          &due,
		RecurrenceRule: &rruleStr,
	})
	if _, err := f.svc.Complete(ctx, tk.ID); err != nil {
		t.Fatalf("complete: %v", err)
	}
	cap, _ := f.users.GetTroikiCapacity(ctx, service.SingleUserID)
	if cap.Medium != 1 {
		t.Errorf("medium capacity: got %d, want 1 (recurring terminal)", cap.Medium)
	}
}

func TestCompleteService_TroikiHook_DoubleComplete_NoDoubleBump(t *testing.T) {
	f := setupCompleteService(t)
	ctx := context.Background()
	c, _ := f.ctxs.Create(ctx, "Work", "blue", false)
	cat := model.TroikiCategoryImportant
	p := projectInCategory(t, f, c.ID, &cat)
	tk, _ := f.tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &c.ID, ProjectID: &p.ID}, Title: "imp"})
	if _, err := f.svc.Complete(ctx, tk.ID); err != nil {
		t.Fatalf("complete 1: %v", err)
	}
	if _, err := f.svc.Complete(ctx, tk.ID); err != nil {
		t.Fatalf("complete 2: %v", err)
	}
	cap, _ := f.users.GetTroikiCapacity(ctx, service.SingleUserID)
	if cap.Medium != 1 {
		t.Errorf("medium capacity: got %d, want 1 (no double-bump)", cap.Medium)
	}
}

func TestCompleteService_TroikiHook_UncompleteRecomplete_NoDoubleBump(t *testing.T) {
	f := setupCompleteService(t)
	ctx := context.Background()
	c, _ := f.ctxs.Create(ctx, "Work", "blue", false)
	cat := model.TroikiCategoryImportant
	p := projectInCategory(t, f, c.ID, &cat)
	tk, _ := f.tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &c.ID, ProjectID: &p.ID}, Title: "imp"})
	if _, err := f.svc.Complete(ctx, tk.ID); err != nil {
		t.Fatalf("complete 1: %v", err)
	}
	if _, err := f.svc.Uncomplete(ctx, tk.ID); err != nil {
		t.Fatalf("uncomplete: %v", err)
	}
	if _, err := f.svc.Complete(ctx, tk.ID); err != nil {
		t.Fatalf("complete 2: %v", err)
	}
	cap, _ := f.users.GetTroikiCapacity(ctx, service.SingleUserID)
	if cap.Medium != 1 {
		t.Errorf("medium capacity: got %d, want 1 (no double-bump on uncomplete/recomplete)", cap.Medium)
	}
}

func TestCompleteService_TroikiHook_ProjectRecategorise_GrantsAgain(t *testing.T) {
	// Clearing the project's category and re-assigning should reset every task's
	// grant flag, so a subsequent completion grants capacity again.
	f := setupCompleteService(t)
	ctx := context.Background()
	c, _ := f.ctxs.Create(ctx, "Work", "blue", false)
	cat := model.TroikiCategoryImportant
	p := projectInCategory(t, f, c.ID, &cat)
	tk, _ := f.tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &c.ID, ProjectID: &p.ID}, Title: "imp"})

	if _, err := f.svc.Complete(ctx, tk.ID); err != nil {
		t.Fatalf("complete 1: %v", err)
	}
	if _, err := f.svc.Uncomplete(ctx, tk.ID); err != nil {
		t.Fatalf("uncomplete: %v", err)
	}
	// Reset the task's grant flag via re-categorising at the task level (the
	// repo.TaskUpdate API still resets troiki_capacity_granted on category
	// changes, which mirrors the project-recategorisation flow that Task 4 will
	// add as an explicit project-side reset).
	if _, err := f.tasks.Update(ctx, tk.ID, repo.TaskUpdate{TroikiCategoryClear: true}); err != nil {
		t.Fatalf("clear task cat: %v", err)
	}
	if _, err := f.tasks.Update(ctx, tk.ID, repo.TaskUpdate{TroikiCategory: &cat}); err != nil {
		t.Fatalf("re-set task cat: %v", err)
	}
	if _, err := f.svc.Complete(ctx, tk.ID); err != nil {
		t.Fatalf("complete 2: %v", err)
	}
	cap, _ := f.users.GetTroikiCapacity(ctx, service.SingleUserID)
	if cap.Medium != 2 {
		t.Errorf("medium capacity: got %d, want 2 (recategorisation grants again)", cap.Medium)
	}
}

func TestCompleteService_Cancel(t *testing.T) {
	f := setupCompleteService(t)
	ctx := context.Background()
	c, _ := f.ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID
	task, _ := f.tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cid},
		Title:     "Task",
	})

	result, err := f.svc.Cancel(ctx, task.ID)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if result.Status != model.TaskStatusCancelled {
		t.Errorf("status: got %q, want cancelled", result.Status)
	}
}

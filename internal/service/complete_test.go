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

func setupCompleteService(t *testing.T) (*service.CompleteService, *repo.TaskRepo, *repo.ContextRepo) {
	t.Helper()
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels)
	ctxs := repo.NewContextRepo(d)
	svc := service.NewCompleteService(tasks)
	return svc, tasks, ctxs
}

func TestCompleteService_SimpleTask(t *testing.T) {
	svc, tasks, ctxs := setupCompleteService(t)
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
	svc, tasks, ctxs := setupCompleteService(t)
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
	svc, tasks, ctxs := setupCompleteService(t)
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
	svc, tasks, ctxs := setupCompleteService(t)
	ctx := context.Background()

	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID
	task, _ := tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cid},
		Title:     "Task",
	})

	svc.Complete(ctx, task.ID) //nolint
	result, err := svc.Uncomplete(ctx, task.ID)
	if err != nil {
		t.Fatalf("uncomplete: %v", err)
	}
	if result.Status != model.TaskStatusOpen {
		t.Errorf("status: got %q, want open", result.Status)
	}
}

func TestCompleteService_Cancel(t *testing.T) {
	svc, tasks, ctxs := setupCompleteService(t)
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

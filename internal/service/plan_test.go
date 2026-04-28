package service_test

import (
	"context"
	"testing"

	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
)

func TestPlanService_SetWeek(t *testing.T) {
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels)
	ctxs := repo.NewContextRepo(d)
	svc := service.NewPlanService(tasks, ctxs, 5, 10)
	ctx := context.Background()

	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID
	task, _ := tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cid},
		Title:     "Plan task",
	})

	result, err := svc.SetPlanState(ctx, task.ID, model.PlanStateWeek)
	if err != nil {
		t.Fatalf("set plan state: %v", err)
	}
	if result.PlanState != model.PlanStateWeek {
		t.Errorf("planState: got %q, want %q", result.PlanState, model.PlanStateWeek)
	}
}

func TestPlanService_WeeklyLimitEnforced(t *testing.T) {
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels)
	ctxs := repo.NewContextRepo(d)
	svc := service.NewPlanService(tasks, ctxs, 2, 100) // limit = 2
	ctx := context.Background()

	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID

	for i := 0; i < 2; i++ {
		task, _ := tasks.Create(ctx, repo.CreateTask{
			Placement: repo.Placement{ContextID: &cid},
			Title:     "Task",
		})
		if _, err := svc.SetPlanState(ctx, task.ID, model.PlanStateWeek); err != nil {
			t.Fatalf("plan task %d: %v", i, err)
		}
	}

	task3, _ := tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cid},
		Title:     "Over limit",
	})
	_, err := svc.SetPlanState(ctx, task3.ID, model.PlanStateWeek)
	if err == nil {
		t.Fatal("expected error when weekly limit exceeded")
	}
	if err != service.ErrPlanLimitExceeded {
		t.Errorf("error: got %v, want %v", err, service.ErrPlanLimitExceeded)
	}
}

func TestPlanService_InboxTaskMovedToFirstContextOnPlan(t *testing.T) {
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels)
	ctxs := repo.NewContextRepo(d)
	svc := service.NewPlanService(tasks, ctxs, 5, 10)
	ctx := context.Background()

	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	inboxID := int64(1)
	task, _ := tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{InboxID: &inboxID},
		Title:     "Inbox task",
	})

	result, err := svc.SetPlanState(ctx, task.ID, model.PlanStateBacklog)
	if err != nil {
		t.Fatalf("set plan state: %v", err)
	}
	if result.PlanState != model.PlanStateBacklog {
		t.Errorf("planState: got %q, want %q", result.PlanState, model.PlanStateBacklog)
	}
	if result.InboxID != nil {
		t.Errorf("inboxId: got %v, want nil", *result.InboxID)
	}
	if result.ContextID == nil || *result.ContextID != c.ID {
		t.Errorf("contextId: got %v, want %d", result.ContextID, c.ID)
	}
}

func TestPlanService_InboxTaskRejectedWhenNoContexts(t *testing.T) {
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels)
	ctxs := repo.NewContextRepo(d)
	svc := service.NewPlanService(tasks, ctxs, 5, 10)
	ctx := context.Background()

	inboxID := int64(1)
	task, _ := tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{InboxID: &inboxID},
		Title:     "Inbox task",
	})

	_, err := svc.SetPlanState(ctx, task.ID, model.PlanStateBacklog)
	if err != service.ErrNoContextForInbox {
		t.Errorf("error: got %v, want %v", err, service.ErrNoContextForInbox)
	}
}

func TestPlanService_NoChangeIfSameState(t *testing.T) {
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels)
	ctxs := repo.NewContextRepo(d)
	svc := service.NewPlanService(tasks, ctxs, 1, 1) // limit = 1
	ctx := context.Background()

	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID
	task, _ := tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cid},
		Title:     "Task",
	})

	// Set to week first.
	svc.SetPlanState(ctx, task.ID, model.PlanStateWeek) //nolint

	// Setting to week again should succeed (no-op), even though limit=1.
	_, err := svc.SetPlanState(ctx, task.ID, model.PlanStateWeek)
	if err != nil {
		t.Errorf("re-setting same state: got error %v, want nil", err)
	}
}

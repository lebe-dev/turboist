package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
)

func TestPlanService_SetWeek(t *testing.T) {
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels, repo.NewTaskRelationsRepo(d))
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
	tasks := repo.NewTaskRepo(d, tlabels, repo.NewTaskRelationsRepo(d))
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
	tasks := repo.NewTaskRepo(d, tlabels, repo.NewTaskRelationsRepo(d))
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
	tasks := repo.NewTaskRepo(d, tlabels, repo.NewTaskRelationsRepo(d))
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

func TestPlanService_SetWeekClearsDue(t *testing.T) {
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels, repo.NewTaskRelationsRepo(d))
	ctxs := repo.NewContextRepo(d)
	svc := service.NewPlanService(tasks, ctxs, 5, 10)
	ctx := context.Background()

	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID
	due := time.Now().Add(24 * time.Hour)
	task, _ := tasks.Create(ctx, repo.CreateTask{
		Placement:  repo.Placement{ContextID: &cid},
		Title:      "Task with due",
		DueAt:      &due,
		DueHasTime: true,
	})

	result, err := svc.SetPlanState(ctx, task.ID, model.PlanStateWeek)
	if err != nil {
		t.Fatalf("set plan state: %v", err)
	}
	if result.PlanState != model.PlanStateWeek {
		t.Errorf("planState: got %q, want %q", result.PlanState, model.PlanStateWeek)
	}
	if result.DueAt != nil {
		t.Errorf("dueAt: got %v, want nil", *result.DueAt)
	}
	if result.DueHasTime {
		t.Errorf("dueHasTime: got %v, want false", result.DueHasTime)
	}
}

func TestPlanService_SetBacklogClearsDue(t *testing.T) {
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels, repo.NewTaskRelationsRepo(d))
	ctxs := repo.NewContextRepo(d)
	svc := service.NewPlanService(tasks, ctxs, 5, 10)
	ctx := context.Background()

	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID
	due := time.Now().Add(24 * time.Hour)
	task, _ := tasks.Create(ctx, repo.CreateTask{
		Placement:  repo.Placement{ContextID: &cid},
		Title:      "Task with due",
		DueAt:      &due,
		DueHasTime: true,
	})

	result, err := svc.SetPlanState(ctx, task.ID, model.PlanStateBacklog)
	if err != nil {
		t.Fatalf("set plan state: %v", err)
	}
	if result.PlanState != model.PlanStateBacklog {
		t.Errorf("planState: got %q, want %q", result.PlanState, model.PlanStateBacklog)
	}
	if result.DueAt != nil {
		t.Errorf("dueAt: got %v, want nil", *result.DueAt)
	}
	if result.DueHasTime {
		t.Errorf("dueHasTime: got %v, want false", result.DueHasTime)
	}
}

// A parent parked in the backlog takes its whole subtree with it: a subtask left
// scheduled would keep surfacing in the day views for work that was postponed.
func TestPlanService_SetBacklogCascadesToSubtasks(t *testing.T) {
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels, repo.NewTaskRelationsRepo(d))
	ctxs := repo.NewContextRepo(d)
	svc := service.NewPlanService(tasks, ctxs, 5, 10)
	ctx := context.Background()

	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID
	parent, _ := tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cid},
		Title:     "Parent",
	})
	due := time.Now().Add(24 * time.Hour)
	child, _ := tasks.Create(ctx, repo.CreateTask{
		Placement:  repo.Placement{ContextID: &cid, ParentID: &parent.ID},
		Title:      "Child",
		DueAt:      &due,
		DueHasTime: true,
	})
	grandchild, _ := tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cid, ParentID: &child.ID},
		Title:     "Grandchild",
	})
	completedChild, _ := tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cid, ParentID: &parent.ID},
		Title:     "Done child",
	})
	completed := model.TaskStatusCompleted
	if _, err := tasks.Update(ctx, completedChild.ID, repo.TaskUpdate{Status: &completed}); err != nil {
		t.Fatalf("complete child: %v", err)
	}

	if _, err := svc.SetPlanState(ctx, parent.ID, model.PlanStateBacklog); err != nil {
		t.Fatalf("set backlog: %v", err)
	}

	for _, id := range []int64{child.ID, grandchild.ID} {
		got, err := tasks.Get(ctx, id)
		if err != nil {
			t.Fatalf("get %d: %v", id, err)
		}
		if got.PlanState != model.PlanStateBacklog {
			t.Errorf("task %q planState: got %q, want %q", got.Title, got.PlanState, model.PlanStateBacklog)
		}
		if got.DueAt != nil {
			t.Errorf("task %q dueAt: got %v, want nil", got.Title, *got.DueAt)
		}
		if got.DueHasTime {
			t.Errorf("task %q dueHasTime: got true, want false", got.Title)
		}
	}

	// A finished subtask is history — parking the parent must not rewrite it.
	doneAfter, err := tasks.Get(ctx, completedChild.ID)
	if err != nil {
		t.Fatalf("get completed child: %v", err)
	}
	if doneAfter.PlanState != model.PlanStateNone {
		t.Errorf("completed child planState: got %q, want %q", doneAfter.PlanState, model.PlanStateNone)
	}
}

// The cascade follows the parent, so it must not be refused halfway by the backlog
// limit — that would leave the parent parked and its subtasks scheduled.
func TestPlanService_SetBacklogCascadeIgnoresBacklogLimit(t *testing.T) {
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels, repo.NewTaskRelationsRepo(d))
	ctxs := repo.NewContextRepo(d)
	svc := service.NewPlanService(tasks, ctxs, 5, 1) // backlog limit = 1
	ctx := context.Background()

	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID
	parent, _ := tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cid},
		Title:     "Parent",
	})
	child, _ := tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cid, ParentID: &parent.ID},
		Title:     "Child",
	})

	if _, err := svc.SetPlanState(ctx, parent.ID, model.PlanStateBacklog); err != nil {
		t.Fatalf("set backlog: %v", err)
	}
	got, err := tasks.Get(ctx, child.ID)
	if err != nil {
		t.Fatalf("get child: %v", err)
	}
	if got.PlanState != model.PlanStateBacklog {
		t.Errorf("child planState: got %q, want %q", got.PlanState, model.PlanStateBacklog)
	}
}

func TestPlanService_BacklogToWeekClearsBacklog(t *testing.T) {
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels, repo.NewTaskRelationsRepo(d))
	ctxs := repo.NewContextRepo(d)
	svc := service.NewPlanService(tasks, ctxs, 5, 10)
	ctx := context.Background()

	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID
	task, _ := tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cid},
		Title:     "Task",
	})

	if _, err := svc.SetPlanState(ctx, task.ID, model.PlanStateBacklog); err != nil {
		t.Fatalf("set backlog: %v", err)
	}

	result, err := svc.SetPlanState(ctx, task.ID, model.PlanStateWeek)
	if err != nil {
		t.Fatalf("set week: %v", err)
	}
	if result.PlanState != model.PlanStateWeek {
		t.Errorf("planState: got %q, want %q", result.PlanState, model.PlanStateWeek)
	}
}

func TestPlanService_SetNoneKeepsDue(t *testing.T) {
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels, repo.NewTaskRelationsRepo(d))
	ctxs := repo.NewContextRepo(d)
	svc := service.NewPlanService(tasks, ctxs, 5, 10)
	ctx := context.Background()

	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID
	due := time.Now().Add(24 * time.Hour)
	task, _ := tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cid},
		Title:     "Task with due",
		DueAt:     &due,
		PlanState: model.PlanStateWeek,
	})

	result, err := svc.SetPlanState(ctx, task.ID, model.PlanStateNone)
	if err != nil {
		t.Fatalf("set none: %v", err)
	}
	if result.PlanState != model.PlanStateNone {
		t.Errorf("planState: got %q, want %q", result.PlanState, model.PlanStateNone)
	}
	if result.DueAt == nil {
		t.Errorf("dueAt: got nil, want preserved")
	}
}

func TestPlanService_NoChangeIfSameState(t *testing.T) {
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels, repo.NewTaskRelationsRepo(d))
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

// A parent planned for the week takes its whole subtree with it: a subtask left in
// the backlog would keep showing there as a separate row, detached from the work it
// belongs to. Due dates are kept — a day inside the week is still valid planning.
func TestPlanService_SetWeekCascadesToSubtasks(t *testing.T) {
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels, repo.NewTaskRelationsRepo(d))
	ctxs := repo.NewContextRepo(d)
	svc := service.NewPlanService(tasks, ctxs, 5, 10)
	ctx := context.Background()

	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID
	parent, _ := tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cid},
		Title:     "Parent",
	})
	due := time.Now().Add(24 * time.Hour)
	child, _ := tasks.Create(ctx, repo.CreateTask{
		Placement:  repo.Placement{ContextID: &cid, ParentID: &parent.ID},
		Title:      "Child",
		PlanState:  model.PlanStateBacklog,
		DueAt:      &due,
		DueHasTime: true,
	})
	grandchild, _ := tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cid, ParentID: &child.ID},
		Title:     "Grandchild",
		PlanState: model.PlanStateBacklog,
	})
	completedChild, _ := tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cid, ParentID: &parent.ID},
		Title:     "Done child",
		PlanState: model.PlanStateBacklog,
	})
	completed := model.TaskStatusCompleted
	if _, err := tasks.Update(ctx, completedChild.ID, repo.TaskUpdate{Status: &completed}); err != nil {
		t.Fatalf("complete child: %v", err)
	}

	if _, err := svc.SetPlanState(ctx, parent.ID, model.PlanStateWeek); err != nil {
		t.Fatalf("set week: %v", err)
	}

	for _, id := range []int64{child.ID, grandchild.ID} {
		got, err := tasks.Get(ctx, id)
		if err != nil {
			t.Fatalf("get %d: %v", id, err)
		}
		if got.PlanState != model.PlanStateWeek {
			t.Errorf("task %q planState: got %q, want %q", got.Title, got.PlanState, model.PlanStateWeek)
		}
	}
	childAfter, _ := tasks.Get(ctx, child.ID)
	if childAfter.DueAt == nil {
		t.Error("child dueAt: got nil, want the scheduled day kept")
	}

	// A finished subtask is history — planning the parent must not rewrite it.
	doneAfter, err := tasks.Get(ctx, completedChild.ID)
	if err != nil {
		t.Fatalf("get completed child: %v", err)
	}
	if doneAfter.PlanState != model.PlanStateBacklog {
		t.Errorf("completed child planState: got %q, want %q", doneAfter.PlanState, model.PlanStateBacklog)
	}
}

// Cascaded subtasks ride along with their parent, so they must not eat slots of the
// weekly limit — otherwise one decomposed task would fill the whole week.
func TestPlanService_WeekCascadeDoesNotConsumeWeeklyLimit(t *testing.T) {
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels, repo.NewTaskRelationsRepo(d))
	ctxs := repo.NewContextRepo(d)
	svc := service.NewPlanService(tasks, ctxs, 2, 10) // weekly limit = 2
	ctx := context.Background()

	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID
	parent, _ := tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cid},
		Title:     "Parent",
	})
	for _, title := range []string{"Child A", "Child B", "Child C"} {
		if _, err := tasks.Create(ctx, repo.CreateTask{
			Placement: repo.Placement{ContextID: &cid, ParentID: &parent.ID},
			Title:     title,
		}); err != nil {
			t.Fatalf("create %s: %v", title, err)
		}
	}
	other, _ := tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cid},
		Title:     "Other",
	})

	if _, err := svc.SetPlanState(ctx, parent.ID, model.PlanStateWeek); err != nil {
		t.Fatalf("set week on parent: %v", err)
	}
	n, err := tasks.CountWeek(ctx)
	if err != nil {
		t.Fatalf("count week: %v", err)
	}
	if n != 1 {
		t.Errorf("week count after cascade: got %d, want 1 (only the hand-planned parent)", n)
	}
	// The second hand-planned task must still fit under the limit of 2.
	if _, err := svc.SetPlanState(ctx, other.ID, model.PlanStateWeek); err != nil {
		t.Fatalf("set week on second root task: %v", err)
	}
}

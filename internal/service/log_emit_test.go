package service_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
)

// TestPlanService_WarnOnWeeklyLimit asserts a WARN log record is emitted when
// the weekly plan limit is exceeded.
func TestPlanService_WarnOnWeeklyLimit(t *testing.T) {
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels, repo.NewTaskRelationsRepo(d))
	ctxs := repo.NewContextRepo(d)
	svc := service.NewPlanService(tasks, ctxs, 1, 10)

	ctx, h := ctxWithCapture(t)
	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID

	t1, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "a"})
	if _, err := svc.SetPlanState(ctx, t1.ID, model.PlanStateWeek); err != nil {
		t.Fatalf("set t1: %v", err)
	}
	t2, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "b"})
	if _, err := svc.SetPlanState(ctx, t2.ID, model.PlanStateWeek); err == nil {
		t.Fatal("expected limit exceeded")
	}

	if !hasMessageAtLevel(h.snapshot(), "service.PlanService.SetPlanState: weekly limit exceeded", slog.LevelWarn) {
		t.Errorf("expected WARN log for weekly limit, got records: %v", h.snapshot())
	}
}

// TestPinService_WarnOnLimit asserts a WARN log record is emitted when the pin
// limit is exceeded.
func TestPinService_WarnOnLimit(t *testing.T) {
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels, repo.NewTaskRelationsRepo(d))
	plabels := repo.NewProjectLabelsRepo(d)
	projects := repo.NewProjectRepo(d, plabels)
	ctxs := repo.NewContextRepo(d)
	users := seedPinUser(t, d)
	setPinnedCaps(t, users, 1, 1)
	svc := service.NewPinService(tasks, projects, users)

	ctx, h := ctxWithCapture(t)
	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	p1, _ := projects.Create(ctx, repo.CreateProject{ContextID: c.ID, Title: "p1", Color: "blue"})
	p2, _ := projects.Create(ctx, repo.CreateProject{ContextID: c.ID, Title: "p2", Color: "blue"})
	if err := svc.PinProject(ctx, p1.ID); err != nil {
		t.Fatalf("pin p1: %v", err)
	}
	if err := svc.PinProject(ctx, p2.ID); err == nil {
		t.Fatal("expected limit exceeded")
	}

	if !hasMessageAtLevel(h.snapshot(), "service.PinService.PinProject: pin limit exceeded", slog.LevelWarn) {
		t.Errorf("expected WARN log for pin limit, records: %v", h.snapshot())
	}
}

// TestGroupService_WarnOnEmptyChildren asserts a WARN log record is emitted
// when the group request is rejected for empty childIds.
func TestGroupService_WarnOnEmptyChildren(t *testing.T) {
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels, repo.NewTaskRelationsRepo(d))
	plabels := repo.NewProjectLabelsRepo(d)
	projects := repo.NewProjectRepo(d, plabels)
	labels := repo.NewLabelRepo(d)
	appSet := repo.NewAppSettingsRepo(d)
	auto := service.NewAutoLabelsService(labels, appSet)
	taskSvc := service.NewTaskService(tasks, projects, tlabels, auto)
	moveSvc := service.NewMoveService(tasks, projects)
	svc := service.NewGroupService(taskSvc, moveSvc, tasks, tlabels)

	ctx, h := ctxWithCapture(t)
	ctxs := repo.NewContextRepo(d)
	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID

	_, err := svc.Group(ctx, service.GroupInput{
		Parent:   repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "wrap"},
		ChildIDs: []int64{},
	})
	if err == nil {
		t.Fatal("expected error")
	}

	if !hasMessageAtLevel(h.snapshot(), "service.GroupService.Group: empty childIds", slog.LevelWarn) {
		t.Errorf("expected WARN log for empty childIds, records: %v", h.snapshot())
	}
}

// TestTroikiService_WarnOnSlotFull asserts a WARN log record when the Troiki
// slot is full.
func TestTroikiService_WarnOnSlotFull(t *testing.T) {
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	plabels := repo.NewProjectLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels, repo.NewTaskRelationsRepo(d))
	projects := repo.NewProjectRepo(d, plabels)
	users := repo.NewUserRepo(d)
	if _, err := users.Create(context.Background(), "admin", "h"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	svc := service.NewTroikiService(tasks, projects, users)

	ctx, h := ctxWithCapture(t)
	ctxs := repo.NewContextRepo(d)
	c, _ := ctxs.Create(ctx, "Work", "blue", false)

	cat := model.TroikiCategoryImportant
	for i := range service.TroikiImportantCap {
		p, _ := projects.Create(ctx, repo.CreateProject{ContextID: c.ID, Title: "imp", Color: "blue"})
		if _, err := svc.SetCategory(ctx, p.ID, &cat); err != nil {
			t.Fatalf("seed imp %d: %v", i, err)
		}
	}
	extra, _ := projects.Create(ctx, repo.CreateProject{ContextID: c.ID, Title: "extra", Color: "blue"})
	if _, err := svc.SetCategory(ctx, extra.ID, &cat); err == nil {
		t.Fatal("expected ErrTroikiSlotFull")
	}

	if !hasMessageAtLevel(h.snapshot(), "service.TroikiService.SetCategory: slot full", slog.LevelWarn) {
		t.Errorf("expected WARN log for slot full, records: %v", h.snapshot())
	}
}

// TestTaskService_WarnOnUnknownLabel asserts a WARN log record when an
// explicit label name does not exist.
func TestTaskService_WarnOnUnknownLabel(t *testing.T) {
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	plabels := repo.NewProjectLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels, repo.NewTaskRelationsRepo(d))
	projects := repo.NewProjectRepo(d, plabels)
	labels := repo.NewLabelRepo(d)
	appSet := repo.NewAppSettingsRepo(d)
	auto := service.NewAutoLabelsService(labels, appSet)
	svc := service.NewTaskService(tasks, projects, tlabels, auto)

	ctx, h := ctxWithCapture(t)
	ctxs := repo.NewContextRepo(d)
	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID

	_, err := svc.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cid},
		Title:     "t",
	}, []string{"nonexistent"}, nil)
	if err == nil {
		t.Fatal("expected unknown label error")
	}

	if !hasMessageAtLevel(h.snapshot(), "service.TaskService.Create: unknown label", slog.LevelWarn) {
		t.Errorf("expected WARN log for unknown label, records: %v", h.snapshot())
	}
}

// TestCompleteService_WarnOnInvalidRRULE asserts a WARN log record when the
// task's RRULE cannot be parsed.
func TestCompleteService_WarnOnInvalidRRULE(t *testing.T) {
	f := setupCompleteService(t)
	ctx, h := ctxWithCapture(t)
	c, _ := f.ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID
	bad := "NOT A REAL RRULE"
	task, err := f.tasks.Create(ctx, repo.CreateTask{
		Placement:      repo.Placement{ContextID: &cid},
		Title:          "rec",
		RecurrenceRule: &bad,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if _, err := f.svc.Complete(ctx, task.ID); err == nil {
		t.Fatal("expected RecurrenceError")
	}

	if !hasMessageAtLevel(h.snapshot(), "service.CompleteService.advanceRecurring: invalid RRULE", slog.LevelWarn) {
		t.Errorf("expected WARN log for invalid RRULE, records: %v", h.snapshot())
	}
}

// TestBackupService_WarnOnUnsupportedVersion asserts a WARN log record when
// restoring an unsupported payload version.
func TestBackupService_WarnOnUnsupportedVersion(t *testing.T) {
	d := setupTestDB(t)
	users := repo.NewUserRepo(d)
	if _, err := users.Create(context.Background(), "admin", "h"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	svc := service.NewBackupService(d)

	ctx, h := ctxWithCapture(t)
	payload := &service.BackupPayload{Version: 0}
	if err := svc.Restore(ctx, payload); err == nil {
		t.Fatal("expected error for unsupported version")
	}

	if !hasMessageAtLevel(h.snapshot(), "service.BackupService.Restore: unsupported version", slog.LevelWarn) {
		t.Errorf("expected WARN log for unsupported version, records: %v", h.snapshot())
	}
}

// TestMoveService_WarnOnInvalidPlacement asserts a WARN log record when Move
// receives an invalid placement.
func TestMoveService_WarnOnInvalidPlacement(t *testing.T) {
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels, repo.NewTaskRelationsRepo(d))
	plabels := repo.NewProjectLabelsRepo(d)
	projects := repo.NewProjectRepo(d, plabels)
	svc := service.NewMoveService(tasks, projects)

	ctx, h := ctxWithCapture(t)
	ctxs := repo.NewContextRepo(d)
	c, _ := ctxs.Create(ctx, "Work", "blue", false)
	cid := c.ID
	task, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "t"})

	if _, err := svc.Move(ctx, task.ID, repo.Placement{}); err == nil {
		t.Fatal("expected invalid placement")
	}

	if !hasMessageAtLevel(h.snapshot(), "service.MoveService.Move: invalid placement", slog.LevelWarn) {
		t.Errorf("expected WARN log for invalid placement, records: %v", h.snapshot())
	}
}

package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
)

func setupPinService(t *testing.T, maxPinned int) (*service.PinService, *repo.TaskRepo, *repo.ProjectRepo, *repo.ContextRepo) {
	t.Helper()
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels)
	plabels := repo.NewProjectLabelsRepo(d)
	projects := repo.NewProjectRepo(d, plabels)
	ctxs := repo.NewContextRepo(d)
	svc := service.NewPinService(tasks, projects, maxPinned)
	return svc, tasks, projects, ctxs
}

func TestPinService_PinProject(t *testing.T) {
	svc, _, projects, ctxs := setupPinService(t, 2)
	ctx := context.Background()

	c, _ := ctxs.Create(ctx, "work", "blue", false)
	p, _ := projects.Create(ctx, repo.CreateProject{ContextID: c.ID, Title: "p1", Color: "blue"})

	if err := svc.PinProject(ctx, p.ID); err != nil {
		t.Fatalf("pin: %v", err)
	}
	got, _ := projects.Get(ctx, p.ID)
	if !got.IsPinned {
		t.Errorf("isPinned: got false, want true")
	}
}

func TestPinService_PinProject_LimitExceeded(t *testing.T) {
	svc, _, projects, ctxs := setupPinService(t, 1)
	ctx := context.Background()

	c, _ := ctxs.Create(ctx, "work", "blue", false)
	p1, _ := projects.Create(ctx, repo.CreateProject{ContextID: c.ID, Title: "p1", Color: "blue"})
	p2, _ := projects.Create(ctx, repo.CreateProject{ContextID: c.ID, Title: "p2", Color: "blue"})

	if err := svc.PinProject(ctx, p1.ID); err != nil {
		t.Fatalf("pin p1: %v", err)
	}
	err := svc.PinProject(ctx, p2.ID)
	if !errors.Is(err, service.ErrPinLimitExceeded) {
		t.Errorf("err: got %v, want ErrPinLimitExceeded", err)
	}
}

func TestPinService_UnpinProject(t *testing.T) {
	svc, _, projects, ctxs := setupPinService(t, 2)
	ctx := context.Background()

	c, _ := ctxs.Create(ctx, "work", "blue", false)
	p, _ := projects.Create(ctx, repo.CreateProject{ContextID: c.ID, Title: "p1", Color: "blue"})

	if err := svc.PinProject(ctx, p.ID); err != nil {
		t.Fatalf("pin: %v", err)
	}
	if err := svc.UnpinProject(ctx, p.ID); err != nil {
		t.Fatalf("unpin: %v", err)
	}
	got, _ := projects.Get(ctx, p.ID)
	if got.IsPinned {
		t.Errorf("isPinned: got true, want false")
	}
}

func TestPinService_PinTask(t *testing.T) {
	svc, tasks, _, ctxs := setupPinService(t, 2)
	ctx := context.Background()

	c, _ := ctxs.Create(ctx, "work", "blue", false)
	cid := c.ID
	task, _ := tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cid},
		Title:     "t1",
	})

	if err := svc.PinTask(ctx, task.ID); err != nil {
		t.Fatalf("pin: %v", err)
	}
	got, _ := tasks.Get(ctx, task.ID)
	if !got.IsPinned {
		t.Errorf("isPinned: got false, want true")
	}
}

func TestPinService_PinTask_LimitExceeded(t *testing.T) {
	svc, tasks, _, ctxs := setupPinService(t, 1)
	ctx := context.Background()

	c, _ := ctxs.Create(ctx, "work", "blue", false)
	cid := c.ID
	t1, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "t1"})
	t2, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "t2"})

	if err := svc.PinTask(ctx, t1.ID); err != nil {
		t.Fatalf("pin t1: %v", err)
	}
	err := svc.PinTask(ctx, t2.ID)
	if !errors.Is(err, service.ErrPinLimitExceeded) {
		t.Errorf("err: got %v, want ErrPinLimitExceeded", err)
	}
}

func TestPinService_UnpinTask(t *testing.T) {
	svc, tasks, _, ctxs := setupPinService(t, 2)
	ctx := context.Background()

	c, _ := ctxs.Create(ctx, "work", "blue", false)
	cid := c.ID
	task, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "t1"})

	if err := svc.PinTask(ctx, task.ID); err != nil {
		t.Fatalf("pin: %v", err)
	}
	if err := svc.UnpinTask(ctx, task.ID); err != nil {
		t.Fatalf("unpin: %v", err)
	}
	got, _ := tasks.Get(ctx, task.ID)
	if got.IsPinned {
		t.Errorf("isPinned: got true, want false")
	}
}

package service_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
)

// setupPinService wires a PinService whose caps come from users.settings — the
// same path production uses — with both caps set to maxPinned.
func setupPinService(t *testing.T, maxPinned int) (*service.PinService, *repo.TaskRepo, *repo.ProjectRepo, *repo.ContextRepo) {
	t.Helper()
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels, repo.NewTaskRelationsRepo(d))
	plabels := repo.NewProjectLabelsRepo(d)
	projects := repo.NewProjectRepo(d, plabels)
	ctxs := repo.NewContextRepo(d)
	users := seedPinUser(t, d)
	setPinnedCaps(t, users, maxPinned, maxPinned)
	svc := service.NewPinService(tasks, projects, users)
	return svc, tasks, projects, ctxs
}

// seedPinUser creates the single user (migration 002 only creates the table),
// whose settings blob holds the pinned caps.
func seedPinUser(t *testing.T, d *sql.DB) *repo.UserRepo {
	t.Helper()
	users := repo.NewUserRepo(d)
	if _, err := users.Create(context.Background(), "admin", "h"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return users
}

func setPinnedCaps(t *testing.T, users *repo.UserRepo, tasks int, projects int) {
	t.Helper()
	ctx := context.Background()
	s, err := users.GetSettings(ctx, service.SingleUserID)
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}
	s.MaxPinnedTasks = tasks
	s.MaxPinnedProjects = projects
	if err := users.SetSettings(ctx, service.SingleUserID, s); err != nil {
		t.Fatalf("set settings: %v", err)
	}
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

// TestPinService_CapsAreIndependent asserts maxPinnedTasks and
// maxPinnedProjects are enforced separately: exhausting the task cap must not
// affect projects, which have their own, larger cap.
func TestPinService_CapsAreIndependent(t *testing.T) {
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels, repo.NewTaskRelationsRepo(d))
	projects := repo.NewProjectRepo(d, repo.NewProjectLabelsRepo(d))
	ctxs := repo.NewContextRepo(d)
	users := seedPinUser(t, d)
	setPinnedCaps(t, users, 1, 2)
	svc := service.NewPinService(tasks, projects, users)

	ctx := context.Background()
	c, _ := ctxs.Create(ctx, "work", "blue", false)
	cid := c.ID
	t1, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "t1"})
	t2, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "t2"})
	p1, _ := projects.Create(ctx, repo.CreateProject{ContextID: c.ID, Title: "p1", Color: "blue"})
	p2, _ := projects.Create(ctx, repo.CreateProject{ContextID: c.ID, Title: "p2", Color: "blue"})

	if err := svc.PinTask(ctx, t1.ID); err != nil {
		t.Fatalf("pin t1: %v", err)
	}
	if err := svc.PinTask(ctx, t2.ID); !errors.Is(err, service.ErrPinLimitExceeded) {
		t.Errorf("pin t2: got %v, want ErrPinLimitExceeded", err)
	}
	if err := svc.PinProject(ctx, p1.ID); err != nil {
		t.Fatalf("pin p1: %v", err)
	}
	if err := svc.PinProject(ctx, p2.ID); err != nil {
		t.Fatalf("pin p2: %v", err)
	}
}

// TestPinService_CapChangeTakesEffectImmediately asserts the cap is re-read on
// every pin, so raising it in Settings needs no restart.
func TestPinService_CapChangeTakesEffectImmediately(t *testing.T) {
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels, repo.NewTaskRelationsRepo(d))
	projects := repo.NewProjectRepo(d, repo.NewProjectLabelsRepo(d))
	ctxs := repo.NewContextRepo(d)
	users := seedPinUser(t, d)
	setPinnedCaps(t, users, 1, 1)
	svc := service.NewPinService(tasks, projects, users)

	ctx := context.Background()
	c, _ := ctxs.Create(ctx, "work", "blue", false)
	cid := c.ID
	t1, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "t1"})
	t2, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "t2"})

	if err := svc.PinTask(ctx, t1.ID); err != nil {
		t.Fatalf("pin t1: %v", err)
	}
	if err := svc.PinTask(ctx, t2.ID); !errors.Is(err, service.ErrPinLimitExceeded) {
		t.Fatalf("pin t2 at cap 1: got %v, want ErrPinLimitExceeded", err)
	}
	setPinnedCaps(t, users, 2, 1)
	if err := svc.PinTask(ctx, t2.ID); err != nil {
		t.Errorf("pin t2 after raising cap: %v", err)
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

// TestPinService_FailsClosedWithoutSettings asserts an unreadable settings row
// blocks the pin instead of silently falling back to some default cap — the
// count check must never be skipped.
func TestPinService_FailsClosedWithoutSettings(t *testing.T) {
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels, repo.NewTaskRelationsRepo(d))
	projects := repo.NewProjectRepo(d, repo.NewProjectLabelsRepo(d))
	ctxs := repo.NewContextRepo(d)
	// Deliberately no user row: migration 002 only creates the table.
	svc := service.NewPinService(tasks, projects, repo.NewUserRepo(d))

	ctx := context.Background()
	c, _ := ctxs.Create(ctx, "work", "blue", false)
	cid := c.ID
	task, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "t1"})
	project, _ := projects.Create(ctx, repo.CreateProject{ContextID: c.ID, Title: "p1", Color: "blue"})

	if err := svc.PinTask(ctx, task.ID); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("PinTask: got %v, want ErrNotFound", err)
	}
	if got, _ := tasks.Get(ctx, task.ID); got.IsPinned {
		t.Error("task pinned despite unreadable caps")
	}
	if err := svc.PinProject(ctx, project.ID); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("PinProject: got %v, want ErrNotFound", err)
	}
	if got, _ := projects.Get(ctx, project.ID); got.IsPinned {
		t.Error("project pinned despite unreadable caps")
	}
}

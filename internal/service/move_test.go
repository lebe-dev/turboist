package service_test

import (
	"context"
	"testing"

	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
)

func setupMoveService(t *testing.T) (*service.MoveService, *repo.TaskRepo, *repo.ContextRepo, *repo.ProjectRepo) {
	t.Helper()
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels)
	plabels := repo.NewProjectLabelsRepo(d)
	projects := repo.NewProjectRepo(d, plabels)
	ctxs := repo.NewContextRepo(d)
	return service.NewMoveService(tasks), tasks, ctxs, projects
}

func TestMoveService_BetweenContexts(t *testing.T) {
	svc, tasks, ctxs, _ := setupMoveService(t)
	ctx := context.Background()

	a, _ := ctxs.Create(ctx, "a", "blue", false)
	b, _ := ctxs.Create(ctx, "b", "blue", false)
	aid := a.ID
	task, _ := tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &aid},
		Title:     "t",
	})

	bid := b.ID
	moved, err := svc.Move(ctx, task.ID, repo.Placement{ContextID: &bid})
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	if moved.ContextID == nil || *moved.ContextID != bid {
		t.Errorf("contextID: got %v, want %d", moved.ContextID, bid)
	}
}

func TestMoveService_ToProject(t *testing.T) {
	svc, tasks, ctxs, projects := setupMoveService(t)
	ctx := context.Background()

	c, _ := ctxs.Create(ctx, "work", "blue", false)
	p, _ := projects.Create(ctx, repo.CreateProject{ContextID: c.ID, Title: "p", Color: "blue"})
	cid := c.ID
	task, _ := tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cid},
		Title:     "t",
	})

	pid := p.ID
	moved, err := svc.Move(ctx, task.ID, repo.Placement{ContextID: &cid, ProjectID: &pid})
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	if moved.ProjectID == nil || *moved.ProjectID != pid {
		t.Errorf("projectID: got %v, want %d", moved.ProjectID, pid)
	}
}

func TestMoveService_InvalidPlacement(t *testing.T) {
	svc, tasks, ctxs, _ := setupMoveService(t)
	ctx := context.Background()

	c, _ := ctxs.Create(ctx, "work", "blue", false)
	cid := c.ID
	task, _ := tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cid},
		Title:     "t",
	})

	// neither inbox nor context — invalid
	_, err := svc.Move(ctx, task.ID, repo.Placement{})
	if err == nil {
		t.Error("expected error for invalid placement")
	}
}

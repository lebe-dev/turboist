package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
)

func setupHarpoonService(t *testing.T) (*service.HarpoonService, *repo.TaskRepo, *repo.ProjectRepo, *repo.ContextRepo) {
	t.Helper()
	d := setupTestDB(t)
	users := repo.NewUserRepo(d)
	if _, err := users.Create(context.Background(), "admin", "hash"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	tlabels := repo.NewTaskLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels)
	plabels := repo.NewProjectLabelsRepo(d)
	projects := repo.NewProjectRepo(d, plabels)
	ctxs := repo.NewContextRepo(d)
	svc := service.NewHarpoonService(users, tasks, projects)
	return svc, tasks, projects, ctxs
}

func taskRef(id int64) model.HarpoonRef {
	return model.HarpoonRef{Kind: model.HarpoonKindTask, ID: id}
}

func projectRef(id int64) model.HarpoonRef {
	return model.HarpoonRef{Kind: model.HarpoonKindProject, ID: id}
}

func TestHarpoonService_AttachMixedPair(t *testing.T) {
	svc, tasks, projects, ctxs := setupHarpoonService(t)
	ctx := context.Background()

	c, _ := ctxs.Create(ctx, "work", "blue", false)
	cid := c.ID
	task, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "do thing"})
	proj, _ := projects.Create(ctx, repo.CreateProject{ContextID: c.ID, Title: "my project", Color: "blue"})

	if _, err := svc.Attach(ctx, 1, taskRef(task.ID)); err != nil {
		t.Fatalf("attach task: %v", err)
	}
	slots, err := svc.Attach(ctx, 1, projectRef(proj.ID))
	if err != nil {
		t.Fatalf("attach project: %v", err)
	}
	if len(slots) != 2 {
		t.Fatalf("slots: got %d, want 2", len(slots))
	}
	if slots[0].Kind != model.HarpoonKindTask || slots[0].ID != task.ID || slots[0].Title != "do thing" {
		t.Errorf("slot0: got %+v, want task %d 'do thing'", slots[0], task.ID)
	}
	if slots[1].Kind != model.HarpoonKindProject || slots[1].ID != proj.ID || slots[1].Title != "my project" {
		t.Errorf("slot1: got %+v, want project %d 'my project'", slots[1], proj.ID)
	}
}

func TestHarpoonService_AttachIsIdempotent(t *testing.T) {
	svc, tasks, _, ctxs := setupHarpoonService(t)
	ctx := context.Background()

	c, _ := ctxs.Create(ctx, "work", "blue", false)
	cid := c.ID
	task, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "t1"})

	if _, err := svc.Attach(ctx, 1, taskRef(task.ID)); err != nil {
		t.Fatalf("attach: %v", err)
	}
	slots, err := svc.Attach(ctx, 1, taskRef(task.ID))
	if err != nil {
		t.Fatalf("re-attach: %v", err)
	}
	if len(slots) != 1 {
		t.Errorf("slots: got %d, want 1 (idempotent)", len(slots))
	}
}

func TestHarpoonService_AttachThirdEvictsOldest(t *testing.T) {
	svc, tasks, _, ctxs := setupHarpoonService(t)
	ctx := context.Background()

	c, _ := ctxs.Create(ctx, "work", "blue", false)
	cid := c.ID
	t1, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "t1"})
	t2, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "t2"})
	t3, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "t3"})

	for _, id := range []int64{t1.ID, t2.ID} {
		if _, err := svc.Attach(ctx, 1, taskRef(id)); err != nil {
			t.Fatalf("attach %d: %v", id, err)
		}
	}
	slots, err := svc.Attach(ctx, 1, taskRef(t3.ID))
	if err != nil {
		t.Fatalf("attach t3: %v", err)
	}
	if len(slots) != 2 {
		t.Fatalf("slots: got %d, want 2", len(slots))
	}
	if slots[0].ID != t2.ID || slots[1].ID != t3.ID {
		t.Errorf("order: got [%d %d], want [%d %d] (oldest evicted)", slots[0].ID, slots[1].ID, t2.ID, t3.ID)
	}
}

func TestHarpoonService_AttachUnknownTarget(t *testing.T) {
	svc, _, _, _ := setupHarpoonService(t)
	ctx := context.Background()

	_, err := svc.Attach(ctx, 1, taskRef(999))
	if !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("err: got %v, want ErrNotFound", err)
	}
}

func TestHarpoonService_Detach(t *testing.T) {
	svc, tasks, _, ctxs := setupHarpoonService(t)
	ctx := context.Background()

	c, _ := ctxs.Create(ctx, "work", "blue", false)
	cid := c.ID
	t1, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "t1"})
	t2, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "t2"})

	for _, id := range []int64{t1.ID, t2.ID} {
		if _, err := svc.Attach(ctx, 1, taskRef(id)); err != nil {
			t.Fatalf("attach %d: %v", id, err)
		}
	}
	slots, err := svc.Detach(ctx, 1, taskRef(t1.ID))
	if err != nil {
		t.Fatalf("detach: %v", err)
	}
	if len(slots) != 1 || slots[0].ID != t2.ID {
		t.Errorf("slots after detach: got %+v, want only t2 (%d)", slots, t2.ID)
	}
}

func TestHarpoonService_GetSelfHealsDeleted(t *testing.T) {
	svc, tasks, _, ctxs := setupHarpoonService(t)
	ctx := context.Background()

	c, _ := ctxs.Create(ctx, "work", "blue", false)
	cid := c.ID
	t1, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "t1"})
	t2, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "t2"})

	for _, id := range []int64{t1.ID, t2.ID} {
		if _, err := svc.Attach(ctx, 1, taskRef(id)); err != nil {
			t.Fatalf("attach %d: %v", id, err)
		}
	}
	if err := tasks.Delete(ctx, t1.ID); err != nil {
		t.Fatalf("delete t1: %v", err)
	}
	slots, err := svc.Get(ctx, 1)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(slots) != 1 || slots[0].ID != t2.ID {
		t.Errorf("slots: got %+v, want only t2 (%d) after self-heal", slots, t2.ID)
	}
}

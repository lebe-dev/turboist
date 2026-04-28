package service_test

import (
	"context"
	"testing"

	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
)

func setupTaskService(t *testing.T, autoLabels []config.AutoLabel) (*service.TaskService, *repo.TaskRepo, *repo.ContextRepo, *repo.LabelRepo) {
	t.Helper()
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels)
	labels := repo.NewLabelRepo(d)
	ctxs := repo.NewContextRepo(d)
	cfg := &config.Config{AutoLabels: autoLabels}
	auto := service.NewAutoLabelsService(labels, cfg)
	svc := service.NewTaskService(tasks, tlabels, auto)
	return svc, tasks, ctxs, labels
}

func TestTaskService_Create_NoLabels(t *testing.T) {
	svc, _, ctxs, _ := setupTaskService(t, nil)
	ctx := context.Background()

	c, _ := ctxs.Create(ctx, "work", "blue", false)
	cid := c.ID

	task, err := svc.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cid},
		Title:     "simple",
	}, nil, nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if task.Title != "simple" {
		t.Errorf("title: got %q", task.Title)
	}
	if len(task.Labels) != 0 {
		t.Errorf("labels: got %v, want empty", task.Labels)
	}
}

func TestTaskService_Create_WithExplicitLabels(t *testing.T) {
	svc, _, ctxs, labels := setupTaskService(t, nil)
	ctx := context.Background()

	_, _ = labels.Create(ctx, "x", "blue", false)
	c, _ := ctxs.Create(ctx, "work", "blue", false)
	cid := c.ID

	task, err := svc.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cid},
		Title:     "t",
	}, []string{"x"}, nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(task.Labels) != 1 || task.Labels[0].Name != "x" {
		t.Errorf("labels: got %v, want [x]", task.Labels)
	}
}

func TestTaskService_Create_WithAutoLabel(t *testing.T) {
	svc, _, ctxs, _ := setupTaskService(t, []config.AutoLabel{
		{Mask: "urgent", Label: "urgent"},
	})
	ctx := context.Background()

	c, _ := ctxs.Create(ctx, "work", "blue", false)
	cid := c.ID

	task, err := svc.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cid},
		Title:     "urgent thing",
	}, nil, nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(task.Labels) != 1 || task.Labels[0].Name != "urgent" {
		t.Errorf("labels: got %v, want [urgent]", task.Labels)
	}
}

func TestTaskService_PatchLabels(t *testing.T) {
	svc, tasks, ctxs, labels := setupTaskService(t, nil)
	ctx := context.Background()

	a, _ := labels.Create(ctx, "a", "blue", false)
	b, _ := labels.Create(ctx, "b", "blue", false)
	c, _ := ctxs.Create(ctx, "work", "blue", false)
	cid := c.ID

	task, err := svc.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &cid},
		Title:     "t",
	}, []string{"a"}, nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(task.Labels) != 1 || task.Labels[0].ID != a.ID {
		t.Errorf("initial labels: got %v", task.Labels)
	}

	newLabels := []string{"b"}
	if err := svc.PatchLabels(ctx, task, "t", &newLabels, nil); err != nil {
		t.Fatalf("patch: %v", err)
	}
	got, _ := tasks.Get(ctx, task.ID)
	if len(got.Labels) != 1 || got.Labels[0].ID != b.ID {
		t.Errorf("after patch: got %v, want [b]", got.Labels)
	}
}

package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
)

func setupGroupService(t *testing.T) (*service.GroupService, *repo.TaskRepo, *repo.ContextRepo, *repo.ProjectRepo, *repo.LabelRepo) {
	t.Helper()
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels)
	plabels := repo.NewProjectLabelsRepo(d)
	projects := repo.NewProjectRepo(d, plabels)
	labels := repo.NewLabelRepo(d)
	ctxs := repo.NewContextRepo(d)
	appSettings := repo.NewAppSettingsRepo(d)
	auto := service.NewAutoLabelsService(labels, appSettings)
	taskSvc := service.NewTaskService(tasks, projects, tlabels, auto)
	moveSvc := service.NewMoveService(tasks, projects)
	groupSvc := service.NewGroupService(taskSvc, moveSvc, tasks, tlabels)
	return groupSvc, tasks, ctxs, projects, labels
}

func TestGroupService_Group_HappyPath(t *testing.T) {
	svc, tasks, ctxs, _, labels := setupGroupService(t)
	ctx := context.Background()

	_, _ = labels.Create(ctx, "umbrella", "blue", false)
	c, _ := ctxs.Create(ctx, "work", "blue", false)
	cid := c.ID

	a, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "a"})
	b, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "b"})

	res, err := svc.Group(ctx, service.GroupInput{
		Parent: repo.CreateTask{
			Placement: repo.Placement{ContextID: &cid},
			Title:     "wrap",
			Priority:  model.PriorityHigh,
		},
		ExplicitLabels: []string{"umbrella"},
		ChildIDs:       []int64{a.ID, b.ID},
	})
	if err != nil {
		t.Fatalf("group: %v", err)
	}
	if res.Parent == nil || res.Parent.Title != "wrap" {
		t.Fatalf("parent: got %+v", res.Parent)
	}
	if len(res.SucceededIDs) != 2 || len(res.Failed) != 0 {
		t.Fatalf("outcomes: succeeded=%v failed=%v", res.SucceededIDs, res.Failed)
	}

	for _, id := range []int64{a.ID, b.ID} {
		got, err := tasks.Get(ctx, id)
		if err != nil {
			t.Fatalf("get child %d: %v", id, err)
		}
		if got.ParentID == nil || *got.ParentID != res.Parent.ID {
			t.Errorf("child %d parentID: got %v, want %d", id, got.ParentID, res.Parent.ID)
		}
		if got.Priority != model.PriorityHigh {
			t.Errorf("child %d priority: got %s, want high", id, got.Priority)
		}
		if len(got.Labels) != 1 || got.Labels[0].Name != "umbrella" {
			t.Errorf("child %d labels: got %v, want [umbrella]", id, got.Labels)
		}
	}
}

func TestGroupService_Group_CrossProjectChildrenRelocate(t *testing.T) {
	svc, tasks, ctxs, projects, _ := setupGroupService(t)
	ctx := context.Background()

	c, _ := ctxs.Create(ctx, "work", "blue", false)
	cid := c.ID
	pSrc, _ := projects.Create(ctx, repo.CreateProject{ContextID: cid, Title: "src", Color: "blue"})
	pDst, _ := projects.Create(ctx, repo.CreateProject{ContextID: cid, Title: "dst", Color: "blue"})

	srcID := pSrc.ID
	a, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid, ProjectID: &srcID}, Title: "a"})

	dstID := pDst.ID
	res, err := svc.Group(ctx, service.GroupInput{
		Parent: repo.CreateTask{
			Placement: repo.Placement{ContextID: &cid, ProjectID: &dstID},
			Title:     "wrap",
		},
		ChildIDs: []int64{a.ID},
	})
	if err != nil {
		t.Fatalf("group: %v", err)
	}
	if len(res.Failed) != 0 {
		t.Fatalf("unexpected failures: %v", res.Failed)
	}
	got, _ := tasks.Get(ctx, a.ID)
	if got.ProjectID == nil || *got.ProjectID != dstID {
		t.Errorf("child projectID: got %v, want %d", got.ProjectID, dstID)
	}
}

func TestGroupService_Group_RejectsInboxTarget(t *testing.T) {
	svc, _, _, _, _ := setupGroupService(t)
	ctx := context.Background()
	inboxID := int64(2)

	_, err := svc.Group(ctx, service.GroupInput{
		Parent:   repo.CreateTask{Placement: repo.Placement{InboxID: &inboxID}, Title: "wrap"},
		ChildIDs: []int64{1},
	})
	if !errors.Is(err, repo.ErrInvalidPlacement) {
		t.Fatalf("err: got %v, want ErrInvalidPlacement", err)
	}
}

func TestGroupService_Group_RejectsRootlessTarget(t *testing.T) {
	svc, _, _, _, _ := setupGroupService(t)
	ctx := context.Background()

	_, err := svc.Group(ctx, service.GroupInput{
		Parent:   repo.CreateTask{Title: "wrap"},
		ChildIDs: []int64{1},
	})
	if !errors.Is(err, repo.ErrInvalidPlacement) {
		t.Fatalf("err: got %v, want ErrInvalidPlacement", err)
	}
}

func TestGroupService_Group_EmptyChildIDs(t *testing.T) {
	svc, _, ctxs, _, _ := setupGroupService(t)
	ctx := context.Background()
	c, _ := ctxs.Create(ctx, "work", "blue", false)
	cid := c.ID

	_, err := svc.Group(ctx, service.GroupInput{
		Parent:   repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "wrap"},
		ChildIDs: []int64{},
	})
	if !errors.Is(err, service.ErrInvalidGroupRequest) {
		t.Fatalf("err: got %v, want ErrInvalidGroupRequest", err)
	}
}

func TestGroupService_Group_DuplicateChildIDs(t *testing.T) {
	svc, _, ctxs, _, _ := setupGroupService(t)
	ctx := context.Background()
	c, _ := ctxs.Create(ctx, "work", "blue", false)
	cid := c.ID

	_, err := svc.Group(ctx, service.GroupInput{
		Parent:   repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "wrap"},
		ChildIDs: []int64{1, 1},
	})
	if !errors.Is(err, service.ErrInvalidGroupRequest) {
		t.Fatalf("err: got %v, want ErrInvalidGroupRequest", err)
	}
}

func TestGroupService_Group_MissingChildRecorded(t *testing.T) {
	svc, tasks, ctxs, _, _ := setupGroupService(t)
	ctx := context.Background()
	c, _ := ctxs.Create(ctx, "work", "blue", false)
	cid := c.ID
	a, _ := tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "a"})

	const missingID int64 = 99999
	res, err := svc.Group(ctx, service.GroupInput{
		Parent:   repo.CreateTask{Placement: repo.Placement{ContextID: &cid}, Title: "wrap"},
		ChildIDs: []int64{a.ID, missingID},
	})
	if err != nil {
		t.Fatalf("group: %v", err)
	}
	if len(res.SucceededIDs) != 1 || res.SucceededIDs[0] != a.ID {
		t.Errorf("succeeded: got %v, want [%d]", res.SucceededIDs, a.ID)
	}
	if len(res.Failed) != 1 || res.Failed[0].ID != missingID {
		t.Errorf("failed: got %v, want one for %d", res.Failed, missingID)
	}
}

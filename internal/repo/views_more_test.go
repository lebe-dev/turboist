package repo

import (
	"context"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

func TestTaskRepo_ListInbox(t *testing.T) {
	d := setupTestDB(t)
	tlabels := NewTaskLabelsRepo(d)
	tasks := NewTaskRepo(d, tlabels)
	ctx := context.Background()

	inboxID := int64(1)
	if _, err := tasks.Create(ctx, CreateTask{
		Placement: Placement{InboxID: &inboxID},
		Title:     "inbox-task",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	out, total, err := tasks.ListInbox(ctx, TaskFilter{}, Page{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 || len(out) != 1 {
		t.Errorf("got total=%d items=%d, want 1/1", total, len(out))
	}
}

func TestTaskRepo_ListByProject_AndSection(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()

	if _, err := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID, ProjectID: &f.projectID, SectionID: &f.sectionID},
		Title:     "in-section",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID, ProjectID: &f.projectID},
		Title:     "in-project",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	pTasks, total, err := f.tasks.ListByProject(ctx, f.projectID, TaskFilter{}, Page{})
	if err != nil {
		t.Fatalf("by project: %v", err)
	}
	if total != 2 || len(pTasks) != 2 {
		t.Errorf("project list: total=%d items=%d want 2/2", total, len(pTasks))
	}

	sTasks, sTotal, err := f.tasks.ListBySection(ctx, f.sectionID, TaskFilter{}, Page{})
	if err != nil {
		t.Fatalf("by section: %v", err)
	}
	if sTotal != 1 || len(sTasks) != 1 {
		t.Errorf("section list: total=%d items=%d want 1/1", sTotal, len(sTasks))
	}
}

func TestTaskRepo_ListByLabel(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()

	lab, _ := f.labels.Create(ctx, "lbl", "blue", false)
	task, _ := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "t",
	})
	if err := f.tlabels.SetForTask(ctx, task.ID, []int64{lab.ID}); err != nil {
		t.Fatalf("set labels: %v", err)
	}
	out, total, err := f.tasks.ListByLabel(ctx, lab.ID, TaskFilter{}, Page{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 || len(out) != 1 {
		t.Errorf("got total=%d items=%d, want 1/1", total, len(out))
	}
}

func TestTaskRepo_ListSubtasks(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()

	parent, _ := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "parent",
	})
	pid := parent.ID
	if _, err := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID, ParentID: &pid},
		Title:     "child",
	}); err != nil {
		t.Fatalf("create child: %v", err)
	}
	out, err := f.tasks.ListSubtasks(ctx, parent.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(out) != 1 || out[0].Title != "child" {
		t.Errorf("got %v, want [child]", out)
	}
}

func TestTaskRepo_SubtreeIDs(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()

	root, _ := f.tasks.Create(ctx, CreateTask{Placement: Placement{ContextID: &f.contextID}, Title: "root"})
	rid := root.ID
	child, _ := f.tasks.Create(ctx, CreateTask{Placement: Placement{ContextID: &f.contextID, ParentID: &rid}, Title: "child"})
	chid := child.ID
	if _, err := f.tasks.Create(ctx, CreateTask{Placement: Placement{ContextID: &f.contextID, ParentID: &chid}, Title: "grand"}); err != nil {
		t.Fatalf("grand: %v", err)
	}

	ids, err := f.tasks.SubtreeIDs(ctx, root.ID)
	if err != nil {
		t.Fatalf("subtree: %v", err)
	}
	if len(ids) != 3 {
		t.Errorf("got %v, want 3 ids", ids)
	}
}

func TestTaskRepo_CountPinnedProjects(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()

	if err := f.projects.SetPinned(ctx, f.projectID, true); err != nil {
		t.Fatalf("pin: %v", err)
	}
	got, err := f.tasks.CountPinnedProjects(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if got != 1 {
		t.Errorf("got %d, want 1", got)
	}
}

func TestProjectRepo_Update_AndSetLabels_AndListByLabel(t *testing.T) {
	d := setupTestDB(t)
	plabels := NewProjectLabelsRepo(d)
	projects := NewProjectRepo(d, plabels)
	ctxs := NewContextRepo(d)
	labels := NewLabelRepo(d)
	ctx := context.Background()

	c, _ := ctxs.Create(ctx, "work", "blue", false)
	p, _ := projects.Create(ctx, CreateProject{ContextID: c.ID, Title: "old", Color: "blue"})

	newTitle := "new"
	newDesc := "desc"
	updated, err := projects.Update(ctx, p.ID, ProjectUpdate{Title: &newTitle, Description: &newDesc})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Title != "new" || updated.Description != "desc" {
		t.Errorf("got %+v", updated)
	}

	got, err := projects.Update(ctx, p.ID, ProjectUpdate{})
	if err != nil {
		t.Fatalf("noop update: %v", err)
	}
	if got.Title != "new" {
		t.Errorf("noop changed title: %q", got.Title)
	}

	if _, err := projects.Update(ctx, 99999, ProjectUpdate{Title: &newTitle}); err == nil {
		t.Error("expected error for missing project")
	}

	lab, _ := labels.Create(ctx, "lbl", "blue", false)
	if err := projects.SetLabels(ctx, p.ID, []int64{lab.ID}); err != nil {
		t.Fatalf("set labels: %v", err)
	}

	out, total, err := projects.ListByLabel(ctx, lab.ID, Page{})
	if err != nil {
		t.Fatalf("list by label: %v", err)
	}
	if total != 1 || len(out) != 1 || out[0].ID != p.ID {
		t.Errorf("got total=%d items=%v", total, out)
	}
}

func TestSessionRepo_TouchLastUsed(t *testing.T) {
	d := setupTestDB(t)
	users := NewUserRepo(d)
	sessions := NewSessionRepo(d)
	ctx := context.Background()

	u, err := users.Create(ctx, "u@test", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	expires := time.Now().Add(time.Hour)
	s, err := sessions.Create(ctx, CreateSessionParams{
		UserID:     u.ID,
		TokenHash:  "h1",
		ClientKind: model.ClientWeb,
		ExpiresAt:  expires,
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := sessions.TouchLastUsed(ctx, s.ID); err != nil {
		t.Fatalf("touch: %v", err)
	}
}

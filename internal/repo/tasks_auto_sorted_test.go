package repo

import (
	"context"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

func TestTaskRepo_Update_AutoSortedAtRoundTrip(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	task, err := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID, ProjectID: &f.projectID},
		Title:     "filed",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if task.AutoSortedAt != nil {
		t.Fatalf("fresh task: got marker %v, want nil", task.AutoSortedAt)
	}
	at := time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC)
	got, err := f.tasks.Update(ctx, task.ID, TaskUpdate{AutoSortedAt: &at})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.AutoSortedAt == nil || !got.AutoSortedAt.Equal(at) {
		t.Fatalf("marker: got %v, want %v", got.AutoSortedAt, at)
	}
	cleared, err := f.tasks.Update(ctx, task.ID, TaskUpdate{AutoSortedAtClear: true})
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if cleared.AutoSortedAt != nil {
		t.Errorf("cleared marker: got %v, want nil", cleared.AutoSortedAt)
	}
}

func TestTaskRepo_Move_ClearsAutoSortedAt(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	other, err := f.projects.Create(ctx, CreateProject{ContextID: f.contextID, Title: "other", Color: "red"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	task, err := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID, ProjectID: &f.projectID},
		Title:     "filed",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	at := time.Now()
	if _, err := f.tasks.Update(ctx, task.ID, TaskUpdate{AutoSortedAt: &at}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := f.tasks.Move(ctx, task.ID, Placement{ContextID: &f.contextID, ProjectID: &other.ID}); err != nil {
		t.Fatalf("move: %v", err)
	}
	got, err := f.tasks.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.AutoSortedAt != nil {
		t.Errorf("marker after manual move: got %v, want nil", got.AutoSortedAt)
	}
}

func TestTaskRepo_Update_AutoSortUndecidedRoundTrip(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	inboxID := int64(1)
	task, err := f.tasks.Create(ctx, CreateTask{Placement: Placement{InboxID: &inboxID}, Title: "hmm"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	at := time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC)
	got, err := f.tasks.Update(ctx, task.ID, TaskUpdate{AutoSortUndecidedAt: &at})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.AutoSortUndecidedAt == nil || !got.AutoSortUndecidedAt.Equal(at) {
		t.Fatalf("mark: got %v, want %v", got.AutoSortUndecidedAt, at)
	}
}

func TestTaskRepo_Update_EditingWordingClearsUndecided(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	inboxID := int64(1)
	mark := func(t *testing.T, id int64) {
		t.Helper()
		at := time.Now()
		if _, err := f.tasks.Update(ctx, id, TaskUpdate{AutoSortUndecidedAt: &at}); err != nil {
			t.Fatalf("mark: %v", err)
		}
	}
	task, err := f.tasks.Create(ctx, CreateTask{Placement: Placement{InboxID: &inboxID}, Title: "hmm", Description: "d"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	mark(t, task.ID)
	same := "hmm"
	priority := model.PriorityHigh
	got, err := f.tasks.Update(ctx, task.ID, TaskUpdate{Title: &same, Priority: &priority})
	if err != nil {
		t.Fatalf("update same title: %v", err)
	}
	if got.AutoSortUndecidedAt == nil {
		t.Error("an unchanged title or another field must keep the mark")
	}

	newTitle := "buy seeds for the garden"
	got, err = f.tasks.Update(ctx, task.ID, TaskUpdate{Title: &newTitle})
	if err != nil {
		t.Fatalf("update title: %v", err)
	}
	if got.AutoSortUndecidedAt != nil {
		t.Errorf("a new title must clear the mark, got %v", got.AutoSortUndecidedAt)
	}

	mark(t, task.ID)
	newDescription := "tulips"
	got, err = f.tasks.Update(ctx, task.ID, TaskUpdate{Description: &newDescription})
	if err != nil {
		t.Fatalf("update description: %v", err)
	}
	if got.AutoSortUndecidedAt != nil {
		t.Errorf("a new description must clear the mark, got %v", got.AutoSortUndecidedAt)
	}

	mark(t, task.ID)
	if err := f.tasks.Move(ctx, task.ID, Placement{ContextID: &f.contextID}); err != nil {
		t.Fatalf("move: %v", err)
	}
	got, _ = f.tasks.Get(ctx, task.ID)
	if got.AutoSortUndecidedAt != nil {
		t.Errorf("a move must clear the mark, got %v", got.AutoSortUndecidedAt)
	}
}

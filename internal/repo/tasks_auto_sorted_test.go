package repo

import (
	"context"
	"testing"
	"time"
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

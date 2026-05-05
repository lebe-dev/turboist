package repo

import (
	"context"
	"testing"
)

func TestSearchRepo_Tasks(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	if _, err := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "buy milk",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "call mom",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	s := NewSearchRepo(f.tasks, f.projects)
	items, total, err := s.SearchTasks(ctx, "milk", Page{})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].Title != "buy milk" {
		t.Errorf("got %+v / total=%d", items, total)
	}

	// empty query → empty result, no error.
	items, total, err = s.SearchTasks(ctx, "", Page{})
	if err != nil {
		t.Fatalf("empty: %v", err)
	}
	if total != 0 || len(items) != 0 {
		t.Errorf("expected empty, got %+v", items)
	}
}

func TestSearchRepo_Projects(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	if _, err := f.projects.Create(ctx, CreateProject{ContextID: f.contextID, Title: "Garden", Color: "green"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	s := NewSearchRepo(f.tasks, f.projects)
	items, total, err := s.SearchProjects(ctx, "gard", Page{})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	// existing project from fixture is "alpha"
	if total != 1 || len(items) != 1 || items[0].Title != "Garden" {
		t.Errorf("got %+v / total=%d", items, total)
	}
}

package repo

import (
	"context"
	"testing"

	"github.com/lebe-dev/turboist/internal/model"
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

func TestSearchRepo_Tasks_FoldsCyrillicCase(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	if _, err := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "Отпуск в горах",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	s := NewSearchRepo(f.tasks, f.projects)
	items, total, err := s.SearchTasks(ctx, "отп", Page{})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("expected one hit, got %+v / total=%d", items, total)
	}
}

func TestSearchRepo_Tasks_RanksTitleThenLabelThenDescription(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	mk := func(title, desc string) int64 {
		t.Helper()
		task, err := f.tasks.Create(ctx, CreateTask{
			Placement:   Placement{ContextID: &f.contextID},
			Title:       title,
			Description: desc,
		})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		return task.ID
	}
	// Created in reverse of the expected order, so the default created_at sort
	// alone would put them the other way round.
	byDescription := mk("call bank", "about the garden fence")
	byLabel := mk("buy seeds", "")
	byTitle := mk("Garden plan", "")
	label, err := f.labels.Create(ctx, "garden", "green", false)
	if err != nil {
		t.Fatalf("label: %v", err)
	}
	if err := f.tlabels.SetForTask(ctx, byLabel, []int64{label.ID}); err != nil {
		t.Fatalf("tag: %v", err)
	}

	s := NewSearchRepo(f.tasks, f.projects)
	items, total, err := s.SearchTasks(ctx, "GARDEN", Page{})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if total != 3 || len(items) != 3 {
		t.Fatalf("expected three hits, got %+v / total=%d", items, total)
	}
	want := []int64{byTitle, byLabel, byDescription}
	for i, id := range want {
		if items[i].ID != id {
			t.Errorf("position %d: got task %d (%q), want %d", i, items[i].ID, items[i].Title, id)
		}
	}
}

func TestSearchRepo_Tasks_OpenBeforeCompleted(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	open, err := f.tasks.Create(ctx, CreateTask{Placement: Placement{ContextID: &f.contextID}, Title: "renew passport"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	done, err := f.tasks.Create(ctx, CreateTask{Placement: Placement{ContextID: &f.contextID}, Title: "renew passport photo"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	completed := model.TaskStatusCompleted
	if _, err := f.tasks.Update(ctx, done.ID, TaskUpdate{Status: &completed}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	s := NewSearchRepo(f.tasks, f.projects)
	items, _, err := s.SearchTasks(ctx, "passport", Page{})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(items) != 2 || items[0].ID != open.ID {
		t.Fatalf("expected the open task first, got %+v", items)
	}
}

func TestSearchRepo_Projects_MatchesLabelAfterTitle(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	plr := NewProjectLabelsRepo(f.db)
	tagged, err := f.projects.Create(ctx, CreateProject{ContextID: f.contextID, Title: "Dacha", Color: "green"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	titled, err := f.projects.Create(ctx, CreateProject{ContextID: f.contextID, Title: "Огород", Color: "green"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	label, err := f.labels.Create(ctx, "огород", "green", false)
	if err != nil {
		t.Fatalf("label: %v", err)
	}
	if err := plr.SetForProject(ctx, tagged.ID, []int64{label.ID}); err != nil {
		t.Fatalf("tag: %v", err)
	}
	s := NewSearchRepo(f.tasks, f.projects)
	items, total, err := s.SearchProjects(ctx, "ОГОР", Page{})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if total != 2 || len(items) != 2 || items[0].ID != titled.ID || items[1].ID != tagged.ID {
		t.Fatalf("expected title hit then label hit, got %+v / total=%d", items, total)
	}
}

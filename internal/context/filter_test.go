package context_test

import (
	"testing"

	"github.com/lebe-dev/turboist/internal/config"
	appctx "github.com/lebe-dev/turboist/internal/context"
	"github.com/lebe-dev/turboist/internal/todoist"
)

func ptr(s string) *string { return &s }

func makeTask(id, projectID string, sectionID *string, labels []string) *todoist.Task {
	return &todoist.Task{
		ID:        id,
		ProjectID: projectID,
		SectionID: sectionID,
		Labels:    labels,
	}
}

var (
	projWork     = &todoist.Project{ID: "p1", Name: "Work"}
	projPersonal = &todoist.Project{ID: "p2", Name: "Personal"}

	secDev    = &todoist.Section{ID: "s1", Name: "Dev", ProjectID: "p1"}
	secDesign = &todoist.Section{ID: "s2", Name: "Design", ProjectID: "p1"}

	projects = []*todoist.Project{projWork, projPersonal}
	sections = []*todoist.Section{secDev, secDesign}
)

func TestFilterTasks_EmptyFilters_ReturnsAll(t *testing.T) {
	tasks := []*todoist.Task{
		makeTask("1", "p1", nil, nil),
		makeTask("2", "p2", nil, nil),
	}
	got := appctx.FilterTasks(tasks, config.ContextFilters{}, projects, sections)
	if len(got) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(got))
	}
}

func TestFilterTasks_ByProject(t *testing.T) {
	tasks := []*todoist.Task{
		makeTask("1", "p1", nil, nil),
		makeTask("2", "p2", nil, nil),
		makeTask("3", "p1", nil, nil),
	}
	filters := config.ContextFilters{Projects: []string{"Work"}}
	got := appctx.FilterTasks(tasks, filters, projects, sections)
	if len(got) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(got))
	}
	for _, task := range got {
		if task.ProjectID != "p1" {
			t.Errorf("unexpected project ID %q", task.ProjectID)
		}
	}
}

func TestFilterTasks_BySection(t *testing.T) {
	tasks := []*todoist.Task{
		makeTask("1", "p1", ptr("s1"), nil),
		makeTask("2", "p1", ptr("s2"), nil),
		makeTask("3", "p1", nil, nil),
	}
	filters := config.ContextFilters{Sections: []string{"Dev"}}
	got := appctx.FilterTasks(tasks, filters, projects, sections)
	if len(got) != 1 {
		t.Fatalf("expected 1 task, got %d", len(got))
	}
	if got[0].ID != "1" {
		t.Errorf("expected task id=1, got %q", got[0].ID)
	}
}

func TestFilterTasks_ByLabel(t *testing.T) {
	tasks := []*todoist.Task{
		makeTask("1", "p1", nil, []string{"urgent", "work"}),
		makeTask("2", "p1", nil, []string{"someday"}),
		makeTask("3", "p2", nil, []string{"urgent"}),
	}
	filters := config.ContextFilters{Labels: []string{"urgent"}}
	got := appctx.FilterTasks(tasks, filters, projects, sections)
	if len(got) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(got))
	}
}

func TestFilterTasks_ProjectAndSection_AND(t *testing.T) {
	tasks := []*todoist.Task{
		makeTask("1", "p1", ptr("s1"), nil), // Work + Dev ✓
		makeTask("2", "p1", ptr("s2"), nil), // Work + Design ✗
		makeTask("3", "p2", ptr("s1"), nil), // Personal + Dev ✗
	}
	filters := config.ContextFilters{
		Projects: []string{"Work"},
		Sections: []string{"Dev"},
	}
	got := appctx.FilterTasks(tasks, filters, projects, sections)
	if len(got) != 1 {
		t.Fatalf("expected 1 task, got %d", len(got))
	}
	if got[0].ID != "1" {
		t.Errorf("expected task id=1, got %q", got[0].ID)
	}
}

func TestFilterTasks_MultipleProjectsOR(t *testing.T) {
	tasks := []*todoist.Task{
		makeTask("1", "p1", nil, nil),
		makeTask("2", "p2", nil, nil),
		makeTask("3", "p3", nil, nil), // unknown project
	}
	filters := config.ContextFilters{Projects: []string{"Work", "Personal"}}
	got := appctx.FilterTasks(tasks, filters, projects, sections)
	if len(got) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(got))
	}
}

func TestFilterTasks_IncludesSubtasksOfMatchedParents(t *testing.T) {
	tasks := []*todoist.Task{
		makeTask("1", "p1", nil, nil),                  // Work parent ✓
		{ID: "2", ProjectID: "p2", ParentID: ptr("1")}, // child of #1, different project
		{ID: "3", ProjectID: "p2", ParentID: ptr("2")}, // grandchild via #2
		makeTask("4", "p2", nil, nil),                  // Personal, no parent ✗
	}
	filters := config.ContextFilters{Projects: []string{"Work"}}
	got := appctx.FilterTasks(tasks, filters, projects, sections)
	if len(got) != 3 {
		t.Fatalf("got %d tasks, want 3 (parent + child + grandchild)", len(got))
	}
	ids := make(map[string]bool)
	for _, task := range got {
		ids[task.ID] = true
	}
	for _, id := range []string{"1", "2", "3"} {
		if !ids[id] {
			t.Errorf("expected task %q in result", id)
		}
	}
}

func TestFilterTasks_UnknownProjectName_ReturnsNone(t *testing.T) {
	tasks := []*todoist.Task{
		makeTask("1", "p1", nil, nil),
	}
	filters := config.ContextFilters{Projects: []string{"Nonexistent"}}
	got := appctx.FilterTasks(tasks, filters, projects, sections)
	if len(got) != 0 {
		t.Fatalf("expected 0 tasks, got %d", len(got))
	}
}

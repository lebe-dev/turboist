package handler

import (
	"testing"

	"github.com/lebe-dev/turboist/internal/todoist"
)

func ptr(s string) *string { return &s }

func TestBuildTree_flat(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Content: "a", Children: []*todoist.Task{}},
		{ID: "2", Content: "b", Children: []*todoist.Task{}},
	}
	roots := buildTree(tasks)
	if len(roots) != 2 {
		t.Fatalf("expected 2 roots, got %d", len(roots))
	}
}

func TestBuildTree_parentChild(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Content: "parent", Children: []*todoist.Task{}},
		{ID: "2", Content: "child", ParentID: ptr("1"), Children: []*todoist.Task{}},
		{ID: "3", Content: "child2", ParentID: ptr("1"), Children: []*todoist.Task{}},
	}
	roots := buildTree(tasks)
	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}
	if roots[0].SubTaskCount != 2 {
		t.Errorf("expected SubTaskCount=2, got %d", roots[0].SubTaskCount)
	}
	if len(roots[0].Children) != 2 {
		t.Errorf("expected 2 children, got %d", len(roots[0].Children))
	}
}

func TestBuildTree_orphanBecomesRoot(t *testing.T) {
	tasks := []*todoist.Task{
		// parent not in set
		{ID: "2", Content: "child", ParentID: ptr("999"), Children: []*todoist.Task{}},
	}
	roots := buildTree(tasks)
	if len(roots) != 1 {
		t.Fatalf("expected orphan to be root, got %d roots", len(roots))
	}
}

func TestBuildTree_doesNotMutateCached(t *testing.T) {
	original := &todoist.Task{ID: "1", Content: "p", Children: []*todoist.Task{}}
	child := &todoist.Task{ID: "2", Content: "c", ParentID: ptr("1"), Children: []*todoist.Task{}}

	buildTree([]*todoist.Task{original, child})

	if len(original.Children) != 0 {
		t.Error("buildTree mutated cached task Children")
	}
}

func TestFilterByLabel(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Labels: []string{"на неделе", "другой"}},
		{ID: "2", Labels: []string{"другой"}},
		{ID: "3", Labels: []string{"на неделе"}},
	}
	got := filterByLabel(tasks, "на неделе")
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
}

func TestFilterByLabel_emptyLabel(t *testing.T) {
	tasks := []*todoist.Task{{ID: "1"}, {ID: "2"}}
	got := filterByLabel(tasks, "")
	if len(got) != 2 {
		t.Fatalf("expected all tasks returned for empty label, got %d", len(got))
	}
}

func TestCountWithLabel(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Labels: []string{"на неделе"}},
		{ID: "2", Labels: []string{}},
		{ID: "3", Labels: []string{"на неделе"}},
	}
	if n := countWithLabel(tasks, "на неделе"); n != 2 {
		t.Errorf("expected 2, got %d", n)
	}
	if n := countWithLabel(tasks, ""); n != 0 {
		t.Errorf("expected 0 for empty label, got %d", n)
	}
}

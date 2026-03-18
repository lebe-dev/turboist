package taskview

import (
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/todoist"
)

func ptr(s string) *string { return &s }

func TestBuildTree_flat(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Content: "a", Children: []*todoist.Task{}},
		{ID: "2", Content: "b", Children: []*todoist.Task{}},
	}
	roots := BuildTree(tasks)
	if len(roots) != 2 {
		t.Fatalf("got %d roots, want 2", len(roots))
	}
}

func TestBuildTree_parentChild(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Content: "parent", Children: []*todoist.Task{}},
		{ID: "2", Content: "child", ParentID: ptr("1"), Children: []*todoist.Task{}},
		{ID: "3", Content: "child2", ParentID: ptr("1"), Children: []*todoist.Task{}},
	}
	roots := BuildTree(tasks)
	if len(roots) != 1 {
		t.Fatalf("got %d roots, want 1", len(roots))
	}
	if roots[0].SubTaskCount != 2 {
		t.Errorf("got SubTaskCount=%d, want 2", roots[0].SubTaskCount)
	}
	if len(roots[0].Children) != 2 {
		t.Errorf("got %d children, want 2", len(roots[0].Children))
	}
}

func TestBuildTree_orphanBecomesRoot(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "2", Content: "child", ParentID: ptr("999"), Children: []*todoist.Task{}},
	}
	roots := BuildTree(tasks)
	if len(roots) != 1 {
		t.Fatalf("got %d roots, want 1 (orphan)", len(roots))
	}
}

func TestBuildTree_doesNotMutateCached(t *testing.T) {
	original := &todoist.Task{ID: "1", Content: "p", Children: []*todoist.Task{}}
	child := &todoist.Task{ID: "2", Content: "c", ParentID: ptr("1"), Children: []*todoist.Task{}}
	BuildTree([]*todoist.Task{original, child})
	if len(original.Children) != 0 {
		t.Error("BuildTree mutated cached task Children")
	}
}

func TestFilterByLabel(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Labels: []string{"weekly", "other"}},
		{ID: "2", Labels: []string{"other"}},
		{ID: "3", Labels: []string{"weekly"}},
	}
	got := FilterByLabel(tasks, "weekly")
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
}

func TestFilterByLabel_emptyLabel(t *testing.T) {
	tasks := []*todoist.Task{{ID: "1"}, {ID: "2"}}
	got := FilterByLabel(tasks, "")
	if len(got) != 2 {
		t.Fatalf("got %d, want 2 (all tasks for empty label)", len(got))
	}
}

func TestCountWithLabel(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Labels: []string{"weekly"}},
		{ID: "2", Labels: []string{}},
		{ID: "3", Labels: []string{"weekly"}},
	}
	if n := CountWithLabel(tasks, "weekly"); n != 2 {
		t.Errorf("got %d, want 2", n)
	}
	if n := CountWithLabel(tasks, ""); n != 0 {
		t.Errorf("got %d, want 0 for empty label", n)
	}
}

func TestExcludeByLabel(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Labels: []string{"weekly", "other"}},
		{ID: "2", Labels: []string{"other"}},
		{ID: "3", Labels: []string{"weekly"}},
		{ID: "4", Labels: []string{}},
	}
	got := ExcludeByLabel(tasks, "weekly")
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
	if got[0].ID != "2" || got[1].ID != "4" {
		t.Errorf("got IDs [%s,%s], want [2,4]", got[0].ID, got[1].ID)
	}
}

func TestFilterByDueDate_exactMatch(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Due: &todoist.Due{Date: "2026-03-13"}},
		{ID: "2", Due: &todoist.Due{Date: "2026-03-14"}},
		{ID: "3"},
	}
	target, _ := time.Parse("2006-01-02", "2026-03-13")
	got := FilterByDueDate(tasks, target, false)
	if len(got) != 1 || got[0].ID != "1" {
		t.Fatalf("got %d tasks, want 1 (id=1)", len(got))
	}
}

func TestFilterByDueDate_includeOverdue(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Due: &todoist.Due{Date: "2026-03-11"}},
		{ID: "2", Due: &todoist.Due{Date: "2026-03-13"}},
		{ID: "3", Due: &todoist.Due{Date: "2026-03-14"}},
		{ID: "4"},
	}
	target, _ := time.Parse("2006-01-02", "2026-03-13")
	got := FilterByDueDate(tasks, target, true)
	if len(got) != 2 {
		t.Fatalf("got %d tasks, want 2 (overdue + today)", len(got))
	}
}

func TestSortTasks_Priority(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Content: "low", Priority: 1, Children: []*todoist.Task{}},
		{ID: "2", Content: "high", Priority: 4, Children: []*todoist.Task{}},
		{ID: "3", Content: "med", Priority: 2, Children: []*todoist.Task{}},
	}
	SortTasks(tasks, "priority")
	if tasks[0].ID != "2" || tasks[1].ID != "3" || tasks[2].ID != "1" {
		t.Errorf("got order [%s,%s,%s], want [2,3,1]", tasks[0].ID, tasks[1].ID, tasks[2].ID)
	}
}

func TestSortTasks_DueDate(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Content: "a", Due: &todoist.Due{Date: "2026-03-15"}, Children: []*todoist.Task{}},
		{ID: "2", Content: "b", Children: []*todoist.Task{}},
		{ID: "3", Content: "c", Due: &todoist.Due{Date: "2026-03-10"}, Children: []*todoist.Task{}},
	}
	SortTasks(tasks, "due_date")
	if tasks[0].ID != "3" || tasks[1].ID != "1" || tasks[2].ID != "2" {
		t.Errorf("got order [%s,%s,%s], want [3,1,2]", tasks[0].ID, tasks[1].ID, tasks[2].ID)
	}
}

func TestSortTasks_Content(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Content: "Charlie", Children: []*todoist.Task{}},
		{ID: "2", Content: "alpha", Children: []*todoist.Task{}},
		{ID: "3", Content: "Bravo", Children: []*todoist.Task{}},
	}
	SortTasks(tasks, "content")
	if tasks[0].ID != "2" || tasks[1].ID != "3" || tasks[2].ID != "1" {
		t.Errorf("got order [%s,%s,%s], want [2,3,1]", tasks[0].ID, tasks[1].ID, tasks[2].ID)
	}
}

func TestSortTasks_Recursive(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Content: "parent", Priority: 1, Children: []*todoist.Task{
			{ID: "c1", Content: "child-low", Priority: 1, Children: []*todoist.Task{}},
			{ID: "c2", Content: "child-high", Priority: 4, Children: []*todoist.Task{}},
		}},
	}
	SortTasks(tasks, "priority")
	if tasks[0].Children[0].ID != "c2" {
		t.Errorf("got first child %s, want c2", tasks[0].Children[0].ID)
	}
}

func TestSortTasksByAddedAt(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Content: "oldest", AddedAt: "2026-03-01T00:00:00Z", Children: []*todoist.Task{}},
		{ID: "2", Content: "newest", AddedAt: "2026-03-15T00:00:00Z", Children: []*todoist.Task{}},
		{ID: "3", Content: "middle", AddedAt: "2026-03-10T00:00:00Z", Children: []*todoist.Task{}},
	}
	SortTasksByAddedAt(tasks)
	if tasks[0].ID != "2" || tasks[1].ID != "3" || tasks[2].ID != "1" {
		t.Errorf("got order [%s,%s,%s], want [2,3,1] (newest first)", tasks[0].ID, tasks[1].ID, tasks[2].ID)
	}
}

func TestSortTasksByAddedAt_Recursive(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Content: "parent", AddedAt: "2026-03-15T00:00:00Z", Children: []*todoist.Task{
			{ID: "c1", Content: "old child", AddedAt: "2026-03-01T00:00:00Z", Children: []*todoist.Task{}},
			{ID: "c2", Content: "new child", AddedAt: "2026-03-10T00:00:00Z", Children: []*todoist.Task{}},
		}},
	}
	SortTasksByAddedAt(tasks)
	if tasks[0].Children[0].ID != "c2" {
		t.Errorf("got first child %s, want c2 (newest first)", tasks[0].Children[0].ID)
	}
}

func TestSortBacklogTasks(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Content: "oldest", AddedAt: "2026-03-01T00:00:00Z", Children: []*todoist.Task{}},
		{ID: "2", Content: "newest", AddedAt: "2026-03-15T00:00:00Z", Children: []*todoist.Task{}},
	}
	SortBacklogTasks(tasks, "added_at")
	if tasks[0].ID != "2" || tasks[1].ID != "1" {
		t.Errorf("got order [%s,%s], want [2,1]", tasks[0].ID, tasks[1].ID)
	}
}

func TestFindInTree(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Content: "parent", Children: []*todoist.Task{
			{ID: "2", Content: "child", Children: []*todoist.Task{}},
		}},
		{ID: "3", Content: "other", Children: []*todoist.Task{}},
	}
	if found := FindInTree(tasks, "2"); found == nil || found.ID != "2" {
		t.Error("expected to find task 2 in tree")
	}
	if found := FindInTree(tasks, "999"); found != nil {
		t.Error("expected nil for missing task")
	}
}

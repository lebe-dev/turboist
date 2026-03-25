package handler

import (
	"strings"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/taskview"
	"github.com/lebe-dev/turboist/internal/todoist"
)

func ptr(s string) *string { return &s }

func TestBuildTree_flat(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Content: "a", Children: []*todoist.Task{}},
		{ID: "2", Content: "b", Children: []*todoist.Task{}},
	}
	roots := taskview.BuildTree(tasks)
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
	roots := taskview.BuildTree(tasks)
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
	roots := taskview.BuildTree(tasks)
	if len(roots) != 1 {
		t.Fatalf("expected orphan to be root, got %d roots", len(roots))
	}
}

func TestBuildTree_doesNotMutateCached(t *testing.T) {
	original := &todoist.Task{ID: "1", Content: "p", Children: []*todoist.Task{}}
	child := &todoist.Task{ID: "2", Content: "c", ParentID: ptr("1"), Children: []*todoist.Task{}}

	taskview.BuildTree([]*todoist.Task{original, child})

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
	got := taskview.FilterByLabel(tasks, "на неделе")
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
}

func TestFilterByLabel_emptyLabel(t *testing.T) {
	tasks := []*todoist.Task{{ID: "1"}, {ID: "2"}}
	got := taskview.FilterByLabel(tasks, "")
	if len(got) != 2 {
		t.Fatalf("expected all tasks returned for empty label, got %d", len(got))
	}
}

func TestFilterByDueDate_exactMatch(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Due: &todoist.Due{Date: "2026-03-13"}},
		{ID: "2", Due: &todoist.Due{Date: "2026-03-14"}},
		{ID: "3"},
	}
	target, _ := time.Parse("2006-01-02", "2026-03-13")
	got := taskview.FilterByDueDate(tasks, target, false)
	if len(got) != 1 || got[0].ID != "1" {
		t.Fatalf("expected 1 task (id=1), got %d", len(got))
	}
}

func TestFilterByDueDate_includeOverdue(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Due: &todoist.Due{Date: "2026-03-11"}}, // overdue
		{ID: "2", Due: &todoist.Due{Date: "2026-03-13"}}, // today
		{ID: "3", Due: &todoist.Due{Date: "2026-03-14"}}, // future
		{ID: "4"}, // no due
	}
	target, _ := time.Parse("2006-01-02", "2026-03-13")
	got := taskview.FilterByDueDate(tasks, target, true)
	if len(got) != 2 {
		t.Fatalf("expected 2 tasks (overdue + today), got %d", len(got))
	}
}

func TestSortTasks_Priority(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Content: "low", Priority: 1, Children: []*todoist.Task{}},
		{ID: "2", Content: "high", Priority: 4, Children: []*todoist.Task{}},
		{ID: "3", Content: "med", Priority: 2, Children: []*todoist.Task{}},
	}
	taskview.SortTasks(tasks, "priority")
	if tasks[0].ID != "2" || tasks[1].ID != "3" || tasks[2].ID != "1" {
		t.Errorf("expected order [2,3,1], got [%s,%s,%s]", tasks[0].ID, tasks[1].ID, tasks[2].ID)
	}
}

func TestSortTasks_PriorityThenDueDate(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Content: "a", Priority: 4, Due: &todoist.Due{Date: "2026-03-15"}, Children: []*todoist.Task{}},
		{ID: "2", Content: "b", Priority: 4, Due: &todoist.Due{Date: "2026-03-10"}, Children: []*todoist.Task{}},
		{ID: "3", Content: "c", Priority: 4, Children: []*todoist.Task{}}, // no due date
	}
	taskview.SortTasks(tasks, "priority")
	if tasks[0].ID != "2" || tasks[1].ID != "1" || tasks[2].ID != "3" {
		t.Errorf("expected order [2,1,3], got [%s,%s,%s]", tasks[0].ID, tasks[1].ID, tasks[2].ID)
	}
}

func TestSortTasks_DueDate(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Content: "a", Due: &todoist.Due{Date: "2026-03-15"}, Children: []*todoist.Task{}},
		{ID: "2", Content: "b", Children: []*todoist.Task{}}, // no due
		{ID: "3", Content: "c", Due: &todoist.Due{Date: "2026-03-10"}, Children: []*todoist.Task{}},
	}
	taskview.SortTasks(tasks, "due_date")
	if tasks[0].ID != "3" || tasks[1].ID != "1" || tasks[2].ID != "2" {
		t.Errorf("expected order [3,1,2], got [%s,%s,%s]", tasks[0].ID, tasks[1].ID, tasks[2].ID)
	}
}

func TestSortTasks_Content(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Content: "Charlie", Children: []*todoist.Task{}},
		{ID: "2", Content: "alpha", Children: []*todoist.Task{}},
		{ID: "3", Content: "Bravo", Children: []*todoist.Task{}},
	}
	taskview.SortTasks(tasks, "content")
	if tasks[0].ID != "2" || tasks[1].ID != "3" || tasks[2].ID != "1" {
		t.Errorf("expected order [2,3,1], got [%s,%s,%s]", tasks[0].ID, tasks[1].ID, tasks[2].ID)
	}
}

func TestSortTasks_Recursive(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Content: "parent", Priority: 1, Children: []*todoist.Task{
			{ID: "c1", Content: "child-low", Priority: 1, Children: []*todoist.Task{}},
			{ID: "c2", Content: "child-high", Priority: 4, Children: []*todoist.Task{}},
		}},
	}
	taskview.SortTasks(tasks, "priority")
	if tasks[0].Children[0].ID != "c2" {
		t.Errorf("expected children sorted, got first child %s", tasks[0].Children[0].ID)
	}
}

func TestExcludeByLabel(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Labels: []string{"weekly", "other"}},
		{ID: "2", Labels: []string{"other"}},
		{ID: "3", Labels: []string{"weekly"}},
		{ID: "4", Labels: []string{}},
	}
	got := taskview.ExcludeByLabel(tasks, "weekly")
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
	if got[0].ID != "2" || got[1].ID != "4" {
		t.Errorf("expected IDs [2,4], got [%s,%s]", got[0].ID, got[1].ID)
	}
}

func TestExcludeByLabel_emptyLabel(t *testing.T) {
	tasks := []*todoist.Task{{ID: "1"}, {ID: "2"}}
	got := taskview.ExcludeByLabel(tasks, "")
	if len(got) != 2 {
		t.Fatalf("expected all tasks returned for empty label, got %d", len(got))
	}
}

func autoLabel(mask, label string, ignoreCase bool) config.CompiledAutoLabel {
	m := mask
	if ignoreCase {
		m = strings.ToLower(m)
	}
	return config.CompiledAutoLabel{Label: label, Mask: m, IgnoreCase: ignoreCase}
}

func TestApplyAutoLabels_Match(t *testing.T) {
	tags := []config.CompiledAutoLabel{autoLabel("купить", "покупки", true)}
	got := applyAutoLabels("Купить молоко", []string{}, tags)
	if len(got) != 1 || got[0] != "покупки" {
		t.Errorf("expected [покупки], got %v", got)
	}
}

func TestApplyAutoLabels_NoMatch(t *testing.T) {
	tags := []config.CompiledAutoLabel{autoLabel("купить", "покупки", true)}
	got := applyAutoLabels("Позвонить другу", []string{}, tags)
	if len(got) != 0 {
		t.Errorf("expected no labels, got %v", got)
	}
}

func TestApplyAutoLabels_NoDuplicate(t *testing.T) {
	tags := []config.CompiledAutoLabel{autoLabel("купить", "покупки", true)}
	got := applyAutoLabels("Купить молоко", []string{"покупки"}, tags)
	if len(got) != 1 {
		t.Errorf("expected 1 label (no duplicate), got %v", got)
	}
}

func TestApplyAutoLabels_CaseInsensitive(t *testing.T) {
	tags := []config.CompiledAutoLabel{autoLabel("купить", "покупки", true)}
	got := applyAutoLabels("КУПИТЬ ХЛЕБ", []string{}, tags)
	if len(got) != 1 || got[0] != "покупки" {
		t.Errorf("expected [покупки], got %v", got)
	}
}

func TestApplyAutoLabels_CaseSensitive(t *testing.T) {
	tags := []config.CompiledAutoLabel{autoLabel("купить", "покупки", false)}
	if got := applyAutoLabels("купить молоко", []string{}, tags); len(got) != 1 {
		t.Errorf("expected match for exact case, got %v", got)
	}
	if got := applyAutoLabels("КУПИТЬ молоко", []string{}, tags); len(got) != 0 {
		t.Errorf("expected no match for wrong case, got %v", got)
	}
}

func TestApplyAutoLabels_MultipleMatches(t *testing.T) {
	tags := []config.CompiledAutoLabel{
		autoLabel("купить", "покупки", true),
		autoLabel("встреча", "работа", true),
	}
	got := applyAutoLabels("Встреча и купить кофе", []string{}, tags)
	if len(got) != 2 {
		t.Errorf("expected 2 labels, got %v", got)
	}
}

func TestApplyAutoLabels_PreservesExisting(t *testing.T) {
	tags := []config.CompiledAutoLabel{autoLabel("купить", "покупки", true)}
	got := applyAutoLabels("Купить молоко", []string{"важное"}, tags)
	if len(got) != 2 {
		t.Errorf("expected 2 labels, got %v", got)
	}
	if got[0] != "важное" {
		t.Errorf("expected existing label first, got %v", got[0])
	}
}

func TestCountWithLabel(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Labels: []string{"на неделе"}},
		{ID: "2", Labels: []string{}},
		{ID: "3", Labels: []string{"на неделе"}},
	}
	if n := taskview.CountWithLabel(tasks, "на неделе"); n != 2 {
		t.Errorf("expected 2, got %d", n)
	}
	if n := taskview.CountWithLabel(tasks, ""); n != 0 {
		t.Errorf("expected 0 for empty label, got %d", n)
	}
}

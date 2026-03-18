package ws

import (
	"testing"

	"github.com/lebe-dev/turboist/internal/todoist"
)

func ptr(s string) *string { return &s }

func TestBuildSnapshot_FlatTasks(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Content: "a", Labels: []string{}, Children: []*todoist.Task{}},
		{ID: "2", Content: "b", Labels: []string{}, Children: []*todoist.Task{}},
	}
	snap := buildSnapshot(tasks)
	if len(snap) != 2 {
		t.Fatalf("got %d entries, want 2", len(snap))
	}
	if snap["1"] == 0 {
		t.Error("got zero hash for task 1")
	}
	if snap["2"] == 0 {
		t.Error("got zero hash for task 2")
	}
}

func TestBuildSnapshot_NestedTasks(t *testing.T) {
	tasks := []*todoist.Task{
		{
			ID: "1", Content: "parent", Labels: []string{},
			Children: []*todoist.Task{
				{ID: "2", Content: "child", ParentID: ptr("1"), Labels: []string{}, Children: []*todoist.Task{}},
			},
		},
	}
	snap := buildSnapshot(tasks)
	if len(snap) != 2 {
		t.Fatalf("got %d entries, want 2 (parent + child)", len(snap))
	}
	if _, ok := snap["2"]; !ok {
		t.Error("child task missing from snapshot")
	}
}

func TestComputeTasksDelta_NoChanges(t *testing.T) {
	tasks := []*todoist.Task{
		{ID: "1", Content: "a", Labels: []string{}, Children: []*todoist.Task{}},
	}
	oldSnap := buildSnapshot(tasks)
	delta, newSnap := computeTasksDelta(oldSnap, tasks, nil)
	if delta != nil {
		t.Fatalf("got delta %+v, want nil (no changes)", delta)
	}
	if len(newSnap) != 1 {
		t.Errorf("got %d entries in new snap, want 1", len(newSnap))
	}
}

func TestComputeTasksDelta_NewTask(t *testing.T) {
	old := []*todoist.Task{
		{ID: "1", Content: "a", Labels: []string{}, Children: []*todoist.Task{}},
	}
	oldSnap := buildSnapshot(old)

	newTasks := []*todoist.Task{
		{ID: "1", Content: "a", Labels: []string{}, Children: []*todoist.Task{}},
		{ID: "2", Content: "new", Labels: []string{}, Children: []*todoist.Task{}},
	}
	delta, _ := computeTasksDelta(oldSnap, newTasks, nil)
	if delta == nil {
		t.Fatal("got nil delta, want non-nil")
	}
	if len(delta.Upserted) != 1 || delta.Upserted[0].ID != "2" {
		t.Errorf("got upserted %v, want [2]", delta.Upserted)
	}
	if len(delta.Removed) != 0 {
		t.Errorf("got %d removed, want 0", len(delta.Removed))
	}
}

func TestComputeTasksDelta_RemovedTask(t *testing.T) {
	old := []*todoist.Task{
		{ID: "1", Content: "a", Labels: []string{}, Children: []*todoist.Task{}},
		{ID: "2", Content: "b", Labels: []string{}, Children: []*todoist.Task{}},
	}
	oldSnap := buildSnapshot(old)

	newTasks := []*todoist.Task{
		{ID: "1", Content: "a", Labels: []string{}, Children: []*todoist.Task{}},
	}
	delta, _ := computeTasksDelta(oldSnap, newTasks, nil)
	if delta == nil {
		t.Fatal("got nil delta, want non-nil")
	}
	if len(delta.Removed) != 1 || delta.Removed[0] != "2" {
		t.Errorf("got removed %v, want [2]", delta.Removed)
	}
}

func TestComputeTasksDelta_UpdatedTask(t *testing.T) {
	old := []*todoist.Task{
		{ID: "1", Content: "old content", Labels: []string{}, Children: []*todoist.Task{}},
	}
	oldSnap := buildSnapshot(old)

	newTasks := []*todoist.Task{
		{ID: "1", Content: "new content", Labels: []string{}, Children: []*todoist.Task{}},
	}
	delta, _ := computeTasksDelta(oldSnap, newTasks, nil)
	if delta == nil {
		t.Fatal("got nil delta, want non-nil")
	}
	if len(delta.Upserted) != 1 || delta.Upserted[0].ID != "1" {
		t.Errorf("got upserted %v, want [1]", delta.Upserted)
	}
	if len(delta.Removed) != 0 {
		t.Errorf("got %d removed, want 0", len(delta.Removed))
	}
}

func TestComputeTasksDelta_Meta(t *testing.T) {
	oldSnap := make(TasksSnapshot)
	newTasks := []*todoist.Task{
		{ID: "1", Content: "a", Labels: []string{}, Children: []*todoist.Task{}},
	}
	meta := map[string]int{"count": 42}
	delta, _ := computeTasksDelta(oldSnap, newTasks, meta)
	if delta == nil {
		t.Fatal("got nil delta, want non-nil")
	}
	m, ok := delta.Meta.(map[string]int)
	if !ok || m["count"] != 42 {
		t.Errorf("got meta %v, want {count: 42}", delta.Meta)
	}
}

func TestHashTask_IgnoresChildren(t *testing.T) {
	task1 := &todoist.Task{ID: "1", Content: "same", Labels: []string{}, Children: []*todoist.Task{}}
	task2 := &todoist.Task{
		ID: "1", Content: "same", Labels: []string{},
		Children: []*todoist.Task{
			{ID: "2", Content: "child", Labels: []string{}, Children: []*todoist.Task{}},
		},
	}
	h1 := hashTask(task1)
	h2 := hashTask(task2)
	if h1 != h2 {
		t.Errorf("got different hashes (%d vs %d), want same (children should be ignored)", h1, h2)
	}
}

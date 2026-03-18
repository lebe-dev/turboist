package todoist

import (
	"testing"
	"time"

	"github.com/CnTeng/todoist-api-go/sync"
)

func ptr(s string) *string { return &s }
func boolPtr(b bool) *bool { return &b }

func TestTaskFromSync_Full(t *testing.T) {
	addedAt := time.Date(2026, 3, 15, 10, 30, 0, 0, time.UTC)
	dueDate := time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)

	st := &sync.Task{
		ID:          "123",
		Content:     "Buy groceries",
		Description: "Milk, eggs, bread",
		ProjectID:   "proj1",
		SectionID:   ptr("sec1"),
		ParentID:    ptr("parent1"),
		Labels:      []string{"errand", "weekly"},
		Priority:    4,
		CompletedAt: ptr("2026-03-18T12:00:00Z"),
		AddedAt:     addedAt,
		Due: &sync.Due{
			Date:        &dueDate,
			IsRecurring: boolPtr(false),
		},
	}

	task := TaskFromSync(st)

	if task.ID != "123" {
		t.Errorf("got ID %q, want %q", task.ID, "123")
	}
	if task.Content != "Buy groceries" {
		t.Errorf("got Content %q, want %q", task.Content, "Buy groceries")
	}
	if task.Description != "Milk, eggs, bread" {
		t.Errorf("got Description %q, want %q", task.Description, "Milk, eggs, bread")
	}
	if task.ProjectID != "proj1" {
		t.Errorf("got ProjectID %q, want %q", task.ProjectID, "proj1")
	}
	if task.SectionID == nil || *task.SectionID != "sec1" {
		t.Errorf("got SectionID %v, want %q", task.SectionID, "sec1")
	}
	if task.ParentID == nil || *task.ParentID != "parent1" {
		t.Errorf("got ParentID %v, want %q", task.ParentID, "parent1")
	}
	if len(task.Labels) != 2 || task.Labels[0] != "errand" || task.Labels[1] != "weekly" {
		t.Errorf("got Labels %v, want [errand, weekly]", task.Labels)
	}
	if task.Priority != 4 {
		t.Errorf("got Priority %d, want 4", task.Priority)
	}
	if task.CompletedAt == nil || *task.CompletedAt != "2026-03-18T12:00:00Z" {
		t.Errorf("got CompletedAt %v, want %q", task.CompletedAt, "2026-03-18T12:00:00Z")
	}
	if task.AddedAt != "2026-03-15T10:30:00Z" {
		t.Errorf("got AddedAt %q, want %q", task.AddedAt, "2026-03-15T10:30:00Z")
	}
	if task.Due == nil {
		t.Fatal("got Due nil, want non-nil")
	}
	if task.Due.Date != "2026-03-20" {
		t.Errorf("got Due.Date %q, want %q", task.Due.Date, "2026-03-20")
	}
	if task.Due.Recurring {
		t.Error("got Due.Recurring true, want false")
	}
	if task.Children == nil || len(task.Children) != 0 {
		t.Errorf("got Children %v, want empty slice", task.Children)
	}
}

func TestTaskFromSync_NilDue(t *testing.T) {
	st := &sync.Task{
		ID:      "1",
		Content: "No due date",
		AddedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	task := TaskFromSync(st)
	if task.Due != nil {
		t.Errorf("got Due %v, want nil", task.Due)
	}
}

func TestTaskFromSync_NilLabels(t *testing.T) {
	st := &sync.Task{
		ID:      "1",
		Content: "No labels",
		Labels:  nil,
		AddedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	task := TaskFromSync(st)
	if task.Labels == nil {
		t.Fatal("got Labels nil, want empty slice")
	}
	if len(task.Labels) != 0 {
		t.Errorf("got %d labels, want 0", len(task.Labels))
	}
}

func TestTaskFromSync_RecurringDue(t *testing.T) {
	dueDate := time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)
	st := &sync.Task{
		ID:      "1",
		Content: "Recurring task",
		AddedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Due: &sync.Due{
			Date:        &dueDate,
			IsRecurring: boolPtr(true),
		},
	}

	task := TaskFromSync(st)
	if task.Due == nil {
		t.Fatal("got Due nil, want non-nil")
	}
	if !task.Due.Recurring {
		t.Error("got Due.Recurring false, want true")
	}
}

func TestTaskFromSync_DueWithNilIsRecurring(t *testing.T) {
	dueDate := time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)
	st := &sync.Task{
		ID:      "1",
		Content: "Due without recurring flag",
		AddedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Due: &sync.Due{
			Date:        &dueDate,
			IsRecurring: nil,
		},
	}

	task := TaskFromSync(st)
	if task.Due == nil {
		t.Fatal("got Due nil, want non-nil")
	}
	if task.Due.Recurring {
		t.Error("got Due.Recurring true, want false (nil defaults to false)")
	}
}

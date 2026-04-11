package todoist

import (
	"testing"
)

func ptr(s string) *string { return &s }

func TestTaskFromSync_Full(t *testing.T) {
	st := &syncItem{
		ID:          "123",
		Content:     "Buy groceries",
		Description: "Milk, eggs, bread",
		ProjectID:   "proj1",
		SectionID:   ptr("sec1"),
		ParentID:    ptr("parent1"),
		Labels:      []string{"errand", "weekly"},
		Priority:    4,
		CompletedAt: ptr("2026-03-18T12:00:00Z"),
		AddedAt:     "2026-03-15T10:30:00Z",
		Due: &syncDue{
			Date:        "2026-03-20",
			IsRecurring: false,
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
	st := &syncItem{
		ID:      "1",
		Content: "No due date",
		AddedAt: "2026-01-01T00:00:00Z",
	}

	task := TaskFromSync(st)
	if task.Due != nil {
		t.Errorf("got Due %v, want nil", task.Due)
	}
}

func TestTaskFromSync_NilLabels(t *testing.T) {
	st := &syncItem{
		ID:      "1",
		Content: "No labels",
		Labels:  nil,
		AddedAt: "2026-01-01T00:00:00Z",
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
	st := &syncItem{
		ID:      "1",
		Content: "Recurring task",
		AddedAt: "2026-01-01T00:00:00Z",
		Due: &syncDue{
			Date:        "2026-03-20",
			IsRecurring: true,
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

func TestTaskFromSync_DueDateTruncation(t *testing.T) {
	st := &syncItem{
		ID:      "1",
		Content: "Task with datetime due",
		AddedAt: "2026-01-01T00:00:00Z",
		Due: &syncDue{
			Date: "2026-03-20T12:00:00",
		},
	}

	task := TaskFromSync(st)
	if task.Due == nil {
		t.Fatal("got Due nil, want non-nil")
	}
	if task.Due.Date != "2026-03-20" {
		t.Errorf("got Due.Date %q, want %q", task.Due.Date, "2026-03-20")
	}
}

func TestTaskFromSync_EmptyDueDate(t *testing.T) {
	st := &syncItem{
		ID:      "1",
		Content: "Empty due date",
		AddedAt: "2026-01-01T00:00:00Z",
		Due:     &syncDue{Date: ""},
	}

	task := TaskFromSync(st)
	if task.Due != nil {
		t.Errorf("got Due %v, want nil (empty date treated as no due)", task.Due)
	}
}

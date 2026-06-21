package model

import "time"

// TaskTemplate is a reusable blueprint for creating a task together with its
// subtasks. The template's Name doubles as the title of the root task created
// from it. Captured fields mirror the editable task fields a template supports.
type TaskTemplate struct {
	ID          int64
	Name        string
	Description string
	Priority    Priority
	DayPart     DayPart
	Position    int
	Labels      []Label
	Subtasks    []TaskTemplateSubtask
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// TaskTemplateSubtask is one child task captured in a template.
type TaskTemplateSubtask struct {
	ID          int64
	TemplateID  int64
	Position    int
	Title       string
	Description string
	Priority    Priority
	DayPart     DayPart
	Labels      []Label
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

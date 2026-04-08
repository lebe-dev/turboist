package todoist

import (
	"github.com/CnTeng/todoist-api-go/sync"
)

// Due represents task due date.
type Due struct {
	Date      string `json:"date"`
	Recurring bool   `json:"recurring"`
}

// Task is an internal representation of a Todoist task.
type Task struct {
	ID                    string   `json:"id"`
	Content               string   `json:"content"`
	Description           string   `json:"description"`
	ProjectID             string   `json:"project_id"`
	SectionID             *string  `json:"section_id"`
	ParentID              *string  `json:"parent_id"`
	Labels                []string `json:"labels"`
	Priority              int      `json:"priority"`
	Due                   *Due     `json:"due"`
	SubTaskCount          int      `json:"sub_task_count"`
	CompletedSubTaskCount int      `json:"completed_sub_task_count"`
	CompletedAt           *string  `json:"completed_at"`
	AddedAt               string   `json:"added_at"`
	Children              []*Task  `json:"children"`
	PostponeCount         int      `json:"postpone_count"`
	ExpiresAt             *string  `json:"expires_at,omitempty"`
}

// Project is an internal representation of a Todoist project.
type Project struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	IsInbox bool   `json:"is_inbox,omitempty"`
}

// Section is an internal representation of a Todoist section.
type Section struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ProjectID string `json:"project_id"`
}

// MoveTarget describes where a task should be moved.
// When SectionID is non-empty, the task moves to that section (project is implicit).
// When SectionID is empty, the task moves to ProjectID only.
type MoveTarget struct {
	ProjectID string
	SectionID string
}

// Label is an internal representation of a Todoist label.
type Label struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// DeltaResult holds incremental sync changes.
// If FullSync is true, the server returned a full dataset (token expired) and
// Result contains the complete state — delta fields should be ignored.
type DeltaResult struct {
	FullSync          bool
	Result            *SyncResult
	UpsertedTasks     []*Task
	RemovedTaskIDs    []string
	UpsertedProjects  []*Project
	RemovedProjectIDs []string
	UpsertedSections  []*Section
	RemovedSectionIDs []string
	UpsertedLabels    []*Label
	RemovedLabelIDs   []string
}

// TaskFromSync maps a sync.Task to our internal Task model.
func TaskFromSync(t *sync.Task) *Task {
	task := &Task{
		ID:          t.ID,
		Content:     t.Content,
		Description: t.Description,
		ProjectID:   t.ProjectID,
		SectionID:   t.SectionID,
		ParentID:    t.ParentID,
		Labels:      t.Labels,
		Priority:    t.Priority,
		CompletedAt: t.CompletedAt,
		AddedAt:     t.AddedAt.Format("2006-01-02T15:04:05Z"),
		Children:    []*Task{},
	}

	if t.Due != nil && t.Due.Date != nil {
		recurring := false
		if t.Due.IsRecurring != nil {
			recurring = *t.Due.IsRecurring
		}
		task.Due = &Due{
			Date:      t.Due.Date.Format("2006-01-02"),
			Recurring: recurring,
		}
	}

	if task.Labels == nil {
		task.Labels = []string{}
	}

	return task
}

// ProjectFromSync maps a sync.Project to our internal Project model.
func ProjectFromSync(p *sync.Project) *Project {
	return &Project{
		ID:      p.ID,
		Name:    p.Name,
		IsInbox: p.InboxProject,
	}
}

// SectionFromSync maps a sync.Section to our internal Section model.
func SectionFromSync(s *sync.Section) *Section {
	return &Section{
		ID:        s.ID,
		Name:      s.Name,
		ProjectID: s.ProjectID,
	}
}

// LabelFromSync maps a sync.Label to our internal Label model.
func LabelFromSync(l *sync.Label) *Label {
	return &Label{
		ID:   l.ID,
		Name: l.Name,
	}
}

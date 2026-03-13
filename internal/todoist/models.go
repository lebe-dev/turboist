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
	Children              []*Task  `json:"children"`
}

// Project is an internal representation of a Todoist project.
type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Section is an internal representation of a Todoist section.
type Section struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ProjectID string `json:"project_id"`
}

// Label is an internal representation of a Todoist label.
type Label struct {
	ID   string `json:"id"`
	Name string `json:"name"`
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
		ID:   p.ID,
		Name: p.Name,
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

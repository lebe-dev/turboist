package dto

import "github.com/lebe-dev/turboist/internal/model"

// TaskTemplateSubtaskDTO is one subtask captured in a template.
type TaskTemplateSubtaskDTO struct {
	ID          int64      `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Priority    string     `json:"priority"`
	DayPart     string     `json:"dayPart"`
	Labels      []LabelDTO `json:"labels"`
}

// TaskTemplateDTO is the wire shape of a task template (root task + subtasks).
type TaskTemplateDTO struct {
	ID          int64                    `json:"id"`
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	Priority    string                   `json:"priority"`
	DayPart     string                   `json:"dayPart"`
	Position    int                      `json:"position"`
	Labels      []LabelDTO               `json:"labels"`
	Subtasks    []TaskTemplateSubtaskDTO `json:"subtasks"`
	CreatedAt   string                   `json:"createdAt"`
	UpdatedAt   string                   `json:"updatedAt"`
}

func labelDTOs(labels []model.Label) []LabelDTO {
	out := make([]LabelDTO, len(labels))
	for i, l := range labels {
		out[i] = LabelFromModel(l)
	}
	return out
}

func TaskTemplateFromModel(t model.TaskTemplate) TaskTemplateDTO {
	subtasks := make([]TaskTemplateSubtaskDTO, len(t.Subtasks))
	for i, st := range t.Subtasks {
		subtasks[i] = TaskTemplateSubtaskDTO{
			ID:          st.ID,
			Title:       st.Title,
			Description: st.Description,
			Priority:    string(st.Priority),
			DayPart:     string(st.DayPart),
			Labels:      labelDTOs(st.Labels),
		}
	}
	return TaskTemplateDTO{
		ID:          t.ID,
		Name:        t.Name,
		Description: t.Description,
		Priority:    string(t.Priority),
		DayPart:     string(t.DayPart),
		Position:    t.Position,
		Labels:      labelDTOs(t.Labels),
		Subtasks:    subtasks,
		CreatedAt:   FormatTime(t.CreatedAt),
		UpdatedAt:   FormatTime(t.UpdatedAt),
	}
}

// TemplateSubtaskRequest is one subtask in a create/update payload.
type TemplateSubtaskRequest struct {
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Priority    string  `json:"priority"`
	DayPart     string  `json:"dayPart"`
	LabelIDs    []int64 `json:"labelIds"`
}

// TaskTemplateRequest is the body for both POST (create) and PATCH (full
// replace) — the editor always submits the complete template structure.
type TaskTemplateRequest struct {
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	Priority    string                   `json:"priority"`
	DayPart     string                   `json:"dayPart"`
	LabelIDs    []int64                  `json:"labelIds"`
	Subtasks    []TemplateSubtaskRequest `json:"subtasks"`
}

// InstantiateTemplateRequest is the body for POST /task-templates/:id/instantiate.
type InstantiateTemplateRequest struct {
	ProjectID int64 `json:"projectId"`
}

// InstantiateTemplateResponse returns the created root task and its subtasks.
type InstantiateTemplateResponse struct {
	Root     TaskDTO   `json:"root"`
	Subtasks []TaskDTO `json:"subtasks"`
}

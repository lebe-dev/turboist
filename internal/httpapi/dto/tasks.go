package dto

import (
	"github.com/lebe-dev/turboist/internal/model"
)

type TaskDTO struct {
	ID              int64      `json:"id"`
	Title           string     `json:"title"`
	Description     string     `json:"description"`
	InboxID         *int64     `json:"inboxId"`
	ContextID       *int64     `json:"contextId"`
	ProjectID       *int64     `json:"projectId"`
	SectionID       *int64     `json:"sectionId"`
	ParentID        *int64     `json:"parentId"`
	Priority        string     `json:"priority"`
	Status          string     `json:"status"`
	DueAt           *string    `json:"dueAt"`
	DueHasTime      bool       `json:"dueHasTime"`
	DeadlineAt      *string    `json:"deadlineAt"`
	DeadlineHasTime bool       `json:"deadlineHasTime"`
	DayPart         string     `json:"dayPart"`
	PlanState       string     `json:"planState"`
	IsPinned        bool       `json:"isPinned"`
	PinnedAt        *string    `json:"pinnedAt"`
	CompletedAt     *string    `json:"completedAt"`
	RecurrenceRule  *string    `json:"recurrenceRule"`
	PostponeCount   int        `json:"postponeCount"`
	TroikiCategory  *string    `json:"troikiCategory"`
	Labels          []LabelDTO `json:"labels"`
	URL             string     `json:"url"`
	CreatedAt       string     `json:"createdAt"`
	UpdatedAt       string     `json:"updatedAt"`
}

func TaskFromModel(t model.Task, baseURL string) TaskDTO {
	labels := make([]LabelDTO, len(t.Labels))
	for i, l := range t.Labels {
		labels[i] = LabelFromModel(l)
	}
	var troikiCat *string
	if t.TroikiCategory != nil {
		s := string(*t.TroikiCategory)
		troikiCat = &s
	}
	return TaskDTO{
		ID:              t.ID,
		Title:           t.Title,
		Description:     t.Description,
		InboxID:         t.InboxID,
		ContextID:       t.ContextID,
		ProjectID:       t.ProjectID,
		SectionID:       t.SectionID,
		ParentID:        t.ParentID,
		Priority:        string(t.Priority),
		Status:          string(t.Status),
		DueAt:           FormatTimePtr(t.DueAt),
		DueHasTime:      t.DueHasTime,
		DeadlineAt:      FormatTimePtr(t.DeadlineAt),
		DeadlineHasTime: t.DeadlineHasTime,
		DayPart:         string(t.DayPart),
		PlanState:       string(t.PlanState),
		IsPinned:        t.IsPinned,
		PinnedAt:        FormatTimePtr(t.PinnedAt),
		CompletedAt:     FormatTimePtr(t.CompletedAt),
		RecurrenceRule:  t.RecurrenceRule,
		PostponeCount:   t.PostponeCount,
		TroikiCategory:  troikiCat,
		Labels:          labels,
		URL:             t.URL(baseURL),
		CreatedAt:       FormatTime(t.CreatedAt),
		UpdatedAt:       FormatTime(t.UpdatedAt),
	}
}

// CreateTaskRequest is the shared body for all task creation endpoints.
type CreateTaskRequest struct {
	Title             string   `json:"title"`
	Description       string   `json:"description"`
	Priority          string   `json:"priority"`
	DueAt             *string  `json:"dueAt"`
	DueHasTime        bool     `json:"dueHasTime"`
	DeadlineAt        *string  `json:"deadlineAt"`
	DeadlineHasTime   bool     `json:"deadlineHasTime"`
	DayPart           string   `json:"dayPart"`
	PlanState         string   `json:"planState"`
	RecurrenceRule    *string  `json:"recurrenceRule"`
	Labels            []string `json:"labels"`
	RemovedAutoLabels []string `json:"removedAutoLabels"`
}

// PatchTaskRequest is the body for PATCH /tasks/:id.
// Only editable fields are accepted; placement, status, and pin are managed via action endpoints.
// Optional[string] distinguishes absent, null (clear), and set value.
type PatchTaskRequest struct {
	Title             *string          `json:"title"`
	Description       *string          `json:"description"`
	Priority          *string          `json:"priority"`
	DueAt             Optional[string] `json:"dueAt"`
	DueHasTime        *bool            `json:"dueHasTime"`
	DeadlineAt        Optional[string] `json:"deadlineAt"`
	DeadlineHasTime   *bool            `json:"deadlineHasTime"`
	DayPart           *string          `json:"dayPart"`
	PlanState         *string          `json:"planState"`
	RecurrenceRule    Optional[string] `json:"recurrenceRule"`
	Labels            *[]string        `json:"labels"`
	RemovedAutoLabels []string         `json:"removedAutoLabels"`
}

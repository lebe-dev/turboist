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
	IsPrivate       bool       `json:"isPrivate"`
	CompletedAt     *string    `json:"completedAt"`
	RecurrenceRule  *string    `json:"recurrenceRule"`
	PostponeCount   int        `json:"postponeCount"`
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
		IsPrivate:       t.IsPrivate,
		CompletedAt:     FormatTimePtr(t.CompletedAt),
		RecurrenceRule:  t.RecurrenceRule,
		PostponeCount:   t.PostponeCount,
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

// GroupTasksRequest is the body for POST /tasks/group: it creates a new parent
// task in the given scope and re-parents the listed child tasks under it,
// overwriting their labels and priority with the new parent's values.
type GroupTasksRequest struct {
	CreateTaskRequest
	ProjectID *int64  `json:"projectId"`
	SectionID *int64  `json:"sectionId"`
	ContextID *int64  `json:"contextId"`
	ChildIDs  []int64 `json:"childIds"`
}

// DecomposeTaskRequest is the body for POST /tasks/:id/decompose: replaces an
// existing task with N sibling tasks created from the supplied titles. New
// tasks inherit the original's placement, priority, due/deadline, labels,
// description, day part, plan state, recurrence and privacy.
type DecomposeTaskRequest struct {
	Titles []string `json:"titles"`
}

// DecomposeTaskResponse is the body returned by POST /tasks/:id/decompose.
type DecomposeTaskResponse struct {
	Created []TaskDTO `json:"created"`
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
	IsPrivate         *bool            `json:"isPrivate"`
}

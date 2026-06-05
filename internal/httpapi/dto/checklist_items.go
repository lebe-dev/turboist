package dto

import "github.com/lebe-dev/turboist/internal/model"

// ChecklistItemDTO is the wire shape of a task checklist item (Federation v1
// F0.2). FracPosition carries the fractional-index key federation uses for
// conflict-free ordering; it is empty until the federated ordering path writes
// it (§5.6 / R9).
type ChecklistItemDTO struct {
	ID           int64   `json:"id"`
	TaskID       int64   `json:"taskId"`
	Title        string  `json:"title"`
	IsCompleted  bool    `json:"isCompleted"`
	Position     int     `json:"position"`
	FracPosition string  `json:"fracPosition"`
	ClientID     string  `json:"clientId"`
	DeletedAt    *string `json:"deletedAt"`
	CreatedAt    string  `json:"createdAt"`
	UpdatedAt    string  `json:"updatedAt"`
}

func ChecklistItemFromModel(it model.ChecklistItem) ChecklistItemDTO {
	return ChecklistItemDTO{
		ID:           it.ID,
		TaskID:       it.TaskID,
		Title:        it.Title,
		IsCompleted:  it.IsCompleted,
		Position:     it.Position,
		FracPosition: it.FracPosition,
		ClientID:     it.ClientID,
		DeletedAt:    FormatTimePtr(it.DeletedAt),
		CreatedAt:    FormatTime(it.CreatedAt),
		UpdatedAt:    FormatTime(it.UpdatedAt),
	}
}

// CreateChecklistItemRequest is the body for POST /tasks/:id/checklist.
type CreateChecklistItemRequest struct {
	Title string `json:"title"`
}

// PatchChecklistItemRequest is the body for PATCH /tasks/:id/checklist/:itemId.
// Nil fields are left unchanged.
type PatchChecklistItemRequest struct {
	Title       *string `json:"title"`
	IsCompleted *bool   `json:"isCompleted"`
}

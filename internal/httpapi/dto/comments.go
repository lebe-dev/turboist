package dto

import "github.com/lebe-dev/turboist/internal/model"

// CommentDTO is the wire shape of a task comment (Federation v1 F0.2).
//
// AuthorDisplayName / AuthorInstance render the federated author line
// "display_name @ origin" (US-3.5 AC4). The display name is sourced from
// federated_instances.display_name (landed in F0.3); until federation is wired
// these are nil for locally-authored comments, and the UI shows the local user.
// They are present on the wire now so the comment carriage is forward-compatible
// with the snapshot (F2.3) and inbox apply (F3.1).
type CommentDTO struct {
	ID                int64   `json:"id"`
	TaskID            int64   `json:"taskId"`
	Body              string  `json:"body"`
	AuthorDisplayName *string `json:"authorDisplayName"`
	AuthorInstance    *string `json:"authorInstance"`
	ClientID          string  `json:"clientId"`
	DeletedAt         *string `json:"deletedAt"`
	CreatedAt         string  `json:"createdAt"`
	UpdatedAt         string  `json:"updatedAt"`
}

// CommentFromModel maps a model.Comment to its DTO. The author fields are left
// nil here — they are populated by the federation layer when a comment
// originates from a peer (F3.1); a local comment has no remote author.
func CommentFromModel(c model.Comment) CommentDTO {
	return CommentDTO{
		ID:        c.ID,
		TaskID:    c.TaskID,
		Body:      c.Body,
		ClientID:  c.ClientID,
		DeletedAt: FormatTimePtr(c.DeletedAt),
		CreatedAt: FormatTime(c.CreatedAt),
		UpdatedAt: FormatTime(c.UpdatedAt),
	}
}

// CreateCommentRequest is the body for POST /tasks/:id/comments. Comments are
// immutable, so there is no patch request shape.
type CreateCommentRequest struct {
	Body string `json:"body"`
}

package dto

import "github.com/lebe-dev/turboist/internal/model"

type CreateProjectRequest struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Color       string   `json:"color"`
	Labels      []string `json:"labels"`
}

type PatchProjectRequest struct {
	Title       *string   `json:"title"`
	Description *string   `json:"description"`
	Color       *string   `json:"color"`
	Labels      *[]string `json:"labels"`
}

type ProjectDTO struct {
	ID          int64      `json:"id"`
	ContextID   int64      `json:"contextId"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Color       string     `json:"color"`
	Status      string     `json:"status"`
	IsPinned    bool       `json:"isPinned"`
	PinnedAt    *string    `json:"pinnedAt"`
	Labels      []LabelDTO `json:"labels"`
	CreatedAt   string     `json:"createdAt"`
	UpdatedAt   string     `json:"updatedAt"`
}

func ProjectFromModel(p model.Project) ProjectDTO {
	labels := make([]LabelDTO, len(p.Labels))
	for i, l := range p.Labels {
		labels[i] = LabelFromModel(l)
	}
	return ProjectDTO{
		ID:          p.ID,
		ContextID:   p.ContextID,
		Title:       p.Title,
		Description: p.Description,
		Color:       p.Color,
		Status:      string(p.Status),
		IsPinned:    p.IsPinned,
		PinnedAt:    FormatTimePtr(p.PinnedAt),
		Labels:      labels,
		CreatedAt:   FormatTime(p.CreatedAt),
		UpdatedAt:   FormatTime(p.UpdatedAt),
	}
}

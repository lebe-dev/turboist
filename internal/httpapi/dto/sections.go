package dto

import "github.com/lebe-dev/turboist/internal/model"

type SectionDTO struct {
	ID        int64   `json:"id"`
	ProjectID int64   `json:"projectId"`
	Title     string  `json:"title"`
	Position  int     `json:"position"`
	ClientID  *string `json:"clientId"`
	CreatedAt string  `json:"createdAt"`
	UpdatedAt string  `json:"updatedAt"`
}

func SectionFromModel(s model.ProjectSection) SectionDTO {
	return SectionDTO{
		ID:        s.ID,
		ProjectID: s.ProjectID,
		Title:     s.Title,
		Position:  s.Position,
		ClientID:  s.ClientID,
		CreatedAt: FormatTime(s.CreatedAt),
		UpdatedAt: FormatTime(s.UpdatedAt),
	}
}

type ReorderSectionRequest struct {
	Position int `json:"position"`
}

type CreateSectionRequest struct {
	Title    string  `json:"title"`
	ClientID *string `json:"clientId"`
}

type PatchSectionRequest struct {
	Title    *string `json:"title"`
	ClientID *string `json:"clientId"`
}

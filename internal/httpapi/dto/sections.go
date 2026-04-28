package dto

import "github.com/lebe-dev/turboist/internal/model"

type SectionDTO struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"projectId"`
	Title     string `json:"title"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func SectionFromModel(s model.ProjectSection) SectionDTO {
	return SectionDTO{
		ID:        s.ID,
		ProjectID: s.ProjectID,
		Title:     s.Title,
		CreatedAt: FormatTime(s.CreatedAt),
		UpdatedAt: FormatTime(s.UpdatedAt),
	}
}

type CreateSectionRequest struct {
	Title string `json:"title"`
}

type PatchSectionRequest struct {
	Title *string `json:"title"`
}

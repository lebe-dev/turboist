package dto

import "github.com/lebe-dev/turboist/internal/model"

type LabelDTO struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	IsFavourite bool   `json:"isFavourite"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

func LabelFromModel(l model.Label) LabelDTO {
	return LabelDTO{
		ID:          l.ID,
		Name:        l.Name,
		Color:       l.Color,
		IsFavourite: l.IsFavourite,
		CreatedAt:   FormatTime(l.CreatedAt),
		UpdatedAt:   FormatTime(l.UpdatedAt),
	}
}

type CreateLabelRequest struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	IsFavourite bool   `json:"isFavourite"`
}

type PatchLabelRequest struct {
	Name        *string `json:"name"`
	Color       *string `json:"color"`
	IsFavourite *bool   `json:"isFavourite"`
}

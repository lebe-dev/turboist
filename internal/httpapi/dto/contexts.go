package dto

import "github.com/lebe-dev/turboist/internal/model"

type ContextDTO struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Color       string  `json:"color"`
	IsFavourite bool    `json:"isFavourite"`
	ClientID    string  `json:"clientId"`
	DeletedAt   *string `json:"deletedAt"`
	CreatedAt   string  `json:"createdAt"`
	UpdatedAt   string  `json:"updatedAt"`
}

func ContextFromModel(c model.Context) ContextDTO {
	return ContextDTO{
		ID:          c.ID,
		Name:        c.Name,
		Color:       c.Color,
		IsFavourite: c.IsFavourite,
		ClientID:    c.ClientID,
		DeletedAt:   FormatTimePtr(c.DeletedAt),
		CreatedAt:   FormatTime(c.CreatedAt),
		UpdatedAt:   FormatTime(c.UpdatedAt),
	}
}

type CreateContextRequest struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	IsFavourite bool   `json:"isFavourite"`
}

type PatchContextRequest struct {
	Name        *string `json:"name"`
	Color       *string `json:"color"`
	IsFavourite *bool   `json:"isFavourite"`
}

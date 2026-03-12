package handler

import (
	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/todoist"
)

type LabelsHandler struct {
	cache *todoist.Cache
}

func NewLabelsHandler(cache *todoist.Cache) *LabelsHandler {
	return &LabelsHandler{cache: cache}
}

type labelsResponse struct {
	Labels []*todoist.Label `json:"labels"`
}

// Labels handles GET /api/labels
func (h *LabelsHandler) Labels(c fiber.Ctx) error {
	return c.JSON(labelsResponse{Labels: h.cache.Labels()})
}

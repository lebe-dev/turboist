package handler

import (
	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/config"
)

type ContextsHandler struct {
	cfg *config.AppConfig
}

func NewContextsHandler(cfg *config.AppConfig) *ContextsHandler {
	return &ContextsHandler{cfg: cfg}
}

type contextFiltersResponse struct {
	Projects []string `json:"projects"`
	Sections []string `json:"sections"`
	Labels   []string `json:"labels"`
}

type contextItem struct {
	ID            string                 `json:"id"`
	DisplayName   string                 `json:"display_name"`
	Color         string                 `json:"color,omitempty"`
	InheritLabels bool                   `json:"inherit_labels"`
	Filters       contextFiltersResponse `json:"filters"`
}

// Contexts handles GET /api/contexts
func (h *ContextsHandler) Contexts(c fiber.Ctx) error {
	items := make([]contextItem, 0, len(h.cfg.Contexts))
	for _, ctx := range h.cfg.Contexts {
		filters := contextFiltersResponse{
			Projects: ctx.Filters.Projects,
			Sections: ctx.Filters.Sections,
			Labels:   ctx.Filters.Labels,
		}
		if filters.Projects == nil {
			filters.Projects = []string{}
		}
		if filters.Sections == nil {
			filters.Sections = []string{}
		}
		if filters.Labels == nil {
			filters.Labels = []string{}
		}
		items = append(items, contextItem{
			ID:            ctx.ID,
			DisplayName:   ctx.DisplayName,
			Color:         ctx.Color,
			InheritLabels: ctx.ShouldInheritLabels(),
			Filters:       filters,
		})
	}
	return c.JSON(items)
}

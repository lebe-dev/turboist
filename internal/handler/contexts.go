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

type contextItem struct {
	Key         string `json:"key"`
	DisplayName string `json:"display_name"`
}

type contextsResponse struct {
	Contexts []contextItem `json:"contexts"`
}

// Contexts handles GET /api/contexts
func (h *ContextsHandler) Contexts(c fiber.Ctx) error {
	items := make([]contextItem, 0, len(h.cfg.Contexts))
	for key, ctx := range h.cfg.Contexts {
		items = append(items, contextItem{
			Key:         key,
			DisplayName: ctx.DisplayName,
		})
	}
	return c.JSON(contextsResponse{Contexts: items})
}

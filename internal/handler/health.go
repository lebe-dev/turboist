package handler

import (
	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/todoist"
)

type HealthHandler struct {
	cache *todoist.Cache
}

func NewHealthHandler(cache *todoist.Cache) *HealthHandler {
	return &HealthHandler{cache: cache}
}

func (h *HealthHandler) Health(c fiber.Ctx) error {
	ready := h.cache.Warmed()
	lastSyncedAt := h.cache.LastSyncedAt()

	status := fiber.StatusOK
	if !ready {
		status = fiber.StatusServiceUnavailable
	}

	return c.Status(status).JSON(fiber.Map{
		"cache_ready":    ready,
		"last_synced_at": lastSyncedAt,
	})
}

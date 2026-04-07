package handler

import (
	"context"

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

// ResetCache forces a full cache refresh from the Todoist API and clears evicted entries.
func (h *HealthHandler) ResetCache(c fiber.Ctx) error {
	h.cache.ClearEvicted()
	if err := h.cache.Refresh(context.Background()); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"ok":             true,
		"last_synced_at": h.cache.LastSyncedAt(),
	})
}

package handler

import (
	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/todoist"
)

type QuickCaptureHandler struct {
	cache *todoist.Cache
	cfg   *config.AppConfig
}

func NewQuickCaptureHandler(cache *todoist.Cache, cfg *config.AppConfig) *QuickCaptureHandler {
	return &QuickCaptureHandler{cache: cache, cfg: cfg}
}

type quickCaptureResponse struct {
	ParentTaskID string `json:"parent_task_id"`
}

// QuickCapture handles GET /api/quick-capture.
// Resolves the parent task ID from cache based on config title.
func (h *QuickCaptureHandler) QuickCapture(c fiber.Ctx) error {
	if h.cfg.QuickCapture == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "quick_capture not configured"})
	}

	qc := h.cfg.QuickCapture

	for _, t := range h.cache.Tasks() {
		if t.Content == qc.Title {
			return c.JSON(quickCaptureResponse{
				ParentTaskID: t.ID,
			})
		}
	}

	return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "parent task not found"})
}

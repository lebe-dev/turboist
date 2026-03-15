package handler

import (
	"encoding/json"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/storage"
)

var validViews = map[string]bool{
	"all":       true,
	"inbox":     true,
	"today":     true,
	"tomorrow":  true,
	"weekly":    true,
	"backlog":   true,
	"completed": true,
}

// StateHandler handles PATCH /api/state for updating user UI state.
type StateHandler struct {
	store *storage.Store
	cfg   *config.AppConfig
}

// NewStateHandler creates a new StateHandler.
func NewStateHandler(store *storage.Store, cfg *config.AppConfig) *StateHandler {
	return &StateHandler{store: store, cfg: cfg}
}

type stateUpdateRequest struct {
	PinnedTasks      *[]storage.PinnedTask `json:"pinned_tasks"`
	ActiveContextID  *string               `json:"active_context_id"`
	ActiveView       *string               `json:"active_view"`
	CollapsedIDs     *[]string             `json:"collapsed_ids"`
	SidebarCollapsed *bool                 `json:"sidebar_collapsed"`
	PlanningOpen     *bool                 `json:"planning_open"`
}

// Update handles PATCH /api/state.
func (h *StateHandler) Update(c fiber.Ctx) error {
	var req stateUpdateRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON"})
	}

	if req.ActiveView != nil {
		if !validViews[*req.ActiveView] {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid active_view"})
		}
	}

	if req.ActiveContextID != nil && *req.ActiveContextID != "" {
		if h.cfg.FindContext(*req.ActiveContextID) == nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "unknown context"})
		}
	}

	if req.PinnedTasks != nil {
		if len(*req.PinnedTasks) > h.cfg.MaxPinned {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "too many pinned tasks"})
		}
	}

	if req.PinnedTasks != nil {
		data, _ := json.Marshal(*req.PinnedTasks)
		if err := h.store.SetValue("pinned_tasks", string(data)); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "save failed"})
		}
	}

	if req.ActiveContextID != nil {
		if err := h.store.SetValue("active_context_id", *req.ActiveContextID); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "save failed"})
		}
	}

	if req.ActiveView != nil {
		if err := h.store.SetValue("active_view", *req.ActiveView); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "save failed"})
		}
	}

	if req.CollapsedIDs != nil {
		data, _ := json.Marshal(*req.CollapsedIDs)
		if err := h.store.SetValue("collapsed_ids", string(data)); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "save failed"})
		}
	}

	if req.SidebarCollapsed != nil {
		v := "false"
		if *req.SidebarCollapsed {
			v = "true"
		}
		if err := h.store.SetValue("sidebar_collapsed", v); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "save failed"})
		}
	}

	if req.PlanningOpen != nil {
		v := "false"
		if *req.PlanningOpen {
			v = "true"
		}
		if err := h.store.SetValue("planning_open", v); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "save failed"})
		}
	}

	return c.SendStatus(fiber.StatusNoContent)
}

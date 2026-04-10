package handler

import (
	"encoding/json"
	"unicode/utf8"

	"github.com/charmbracelet/log"
	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/storage"
)

var validLocales = map[string]bool{
	"en": true,
	"ru": true,
}

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
	PinnedTasks         *[]storage.PinnedTask    `json:"pinned_tasks"`
	ActiveContextID     *string                  `json:"active_context_id"`
	ActiveView          *string                  `json:"active_view"`
	CollapsedIDs        *[]string                `json:"collapsed_ids"`
	SidebarCollapsed    *bool                    `json:"sidebar_collapsed"`
	PlanningOpen        *bool                    `json:"planning_open"`
	DayPartNotes        *map[string]string       `json:"day_part_notes"`
	Locale              *string                  `json:"locale"`
	AllFilters          *storage.AllFiltersState `json:"all_filters"`
	BannerText          *string                  `json:"banner_text"`
	BannerDismissedText *string                  `json:"banner_dismissed_text"`
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
			log.Error("state save failed", "field", "pinned_tasks", "err", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "save failed: pinned_tasks", "detail": err.Error()})
		}
	}

	if req.ActiveContextID != nil {
		if err := h.store.SetValue("active_context_id", *req.ActiveContextID); err != nil {
			log.Error("state save failed", "field", "active_context_id", "value", *req.ActiveContextID, "err", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "save failed: active_context_id", "detail": err.Error()})
		}
	}

	if req.ActiveView != nil {
		if err := h.store.SetValue("active_view", *req.ActiveView); err != nil {
			log.Error("state save failed", "field", "active_view", "value", *req.ActiveView, "err", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "save failed: active_view", "detail": err.Error()})
		}
	}

	if req.CollapsedIDs != nil {
		data, _ := json.Marshal(*req.CollapsedIDs)
		if err := h.store.SetValue("collapsed_ids", string(data)); err != nil {
			log.Error("state save failed", "field", "collapsed_ids", "err", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "save failed: collapsed_ids", "detail": err.Error()})
		}
	}

	if req.SidebarCollapsed != nil {
		v := "false"
		if *req.SidebarCollapsed {
			v = "true"
		}
		if err := h.store.SetValue("sidebar_collapsed", v); err != nil {
			log.Error("state save failed", "field", "sidebar_collapsed", "value", v, "err", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "save failed: sidebar_collapsed", "detail": err.Error()})
		}
	}

	if req.PlanningOpen != nil {
		v := "false"
		if *req.PlanningOpen {
			v = "true"
		}
		if err := h.store.SetValue("planning_open", v); err != nil {
			log.Error("state save failed", "field", "planning_open", "value", v, "err", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "save failed: planning_open", "detail": err.Error()})
		}
	}

	if req.Locale != nil {
		if *req.Locale != "" && !validLocales[*req.Locale] {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid locale"})
		}
		if err := h.store.SetValue("locale", *req.Locale); err != nil {
			log.Error("state save failed", "field", "locale", "err", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "save failed: locale", "detail": err.Error()})
		}
	}

	if req.AllFilters != nil {
		data, _ := json.Marshal(*req.AllFilters)
		if err := h.store.SetValue("all_filters", string(data)); err != nil {
			log.Error("state save failed", "field", "all_filters", "err", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "save failed: all_filters", "detail": err.Error()})
		}
	}

	if req.BannerText != nil {
		if utf8.RuneCountInString(*req.BannerText) > 200 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "banner_text too long"})
		}
		if err := h.store.SetValue("banner_text", *req.BannerText); err != nil {
			log.Error("state save failed", "field", "banner_text", "err", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "save failed: banner_text", "detail": err.Error()})
		}
	}

	if req.BannerDismissedText != nil {
		if utf8.RuneCountInString(*req.BannerDismissedText) > 200 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "banner_dismissed_text too long"})
		}
		if err := h.store.SetValue("banner_dismissed_text", *req.BannerDismissedText); err != nil {
			log.Error("state save failed", "field", "banner_dismissed_text", "err", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "save failed: banner_dismissed_text", "detail": err.Error()})
		}
	}

	if req.DayPartNotes != nil {
		validLabels := make(map[string]bool, len(h.cfg.Today.DayParts))
		for _, dp := range h.cfg.Today.DayParts {
			validLabels[dp.Label] = true
		}
		for label, note := range *req.DayPartNotes {
			if !validLabels[label] {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "unknown day_part label: " + label})
			}
			if utf8.RuneCountInString(note) > h.cfg.Today.MaxDayPartNoteLength {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "note too long for " + label})
			}
		}
		data, _ := json.Marshal(*req.DayPartNotes)
		if err := h.store.SetValue("day_part_notes", string(data)); err != nil {
			log.Error("state save failed", "field", "day_part_notes", "err", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "save failed: day_part_notes", "detail": err.Error()})
		}
	}

	return c.SendStatus(fiber.StatusNoContent)
}

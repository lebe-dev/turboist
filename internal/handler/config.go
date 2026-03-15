package handler

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/todoist"
)

type ConfigHandler struct {
	cache *todoist.Cache
	cfg   *config.AppConfig
}

func NewConfigHandler(cache *todoist.Cache, cfg *config.AppConfig) *ConfigHandler {
	return &ConfigHandler{cache: cache, cfg: cfg}
}

type dayPartResponse struct {
	Label string `json:"label"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

type configResponse struct {
	PollInterval  int               `json:"poll_interval"`
	Timezone      string            `json:"timezone"`
	WeeklyLabel   string            `json:"weekly_label"`
	NextWeekLabel string            `json:"next_week_label"`
	WeeklyLimit   int               `json:"weekly_limit"`
	CompletedDays int               `json:"completed_days"`
	MaxPinned     int               `json:"max_pinned"`
	LastSyncedAt  time.Time         `json:"last_synced_at"`
	DayParts      []dayPartResponse `json:"day_parts"`
}

// Config handles GET /api/config
func (h *ConfigHandler) Config(c fiber.Ctx) error {
	dayParts := make([]dayPartResponse, 0, len(h.cfg.Today.DayParts))
	for _, dp := range h.cfg.Today.DayParts {
		dayParts = append(dayParts, dayPartResponse{
			Label: dp.Label,
			Start: dp.Start,
			End:   dp.End,
		})
	}

	return c.JSON(configResponse{
		PollInterval:  int(h.cfg.PollInterval.Seconds()),
		Timezone:      h.cfg.Timezone,
		WeeklyLabel:   h.cfg.Weekly.Label,
		NextWeekLabel: h.cfg.NextWeek.Label,
		WeeklyLimit:   h.cfg.Weekly.MaxTasks,
		CompletedDays: h.cfg.Completed.Days,
		MaxPinned:     h.cfg.MaxPinned,
		LastSyncedAt:  h.cache.LastSyncedAt(),
		DayParts:      dayParts,
	})
}

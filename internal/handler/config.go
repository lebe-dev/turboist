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

type configResponse struct {
	PollInterval string    `json:"poll_interval"`
	WeeklyLabel  string    `json:"weekly_label"`
	WeeklyLimit  int       `json:"weekly_limit"`
	LastSyncedAt time.Time `json:"last_synced_at"`
}

// Config handles GET /api/config
func (h *ConfigHandler) Config(c fiber.Ctx) error {
	return c.JSON(configResponse{
		PollInterval: h.cfg.PollInterval.String(),
		WeeklyLabel:  h.cfg.Weekly.Label,
		WeeklyLimit:  h.cfg.Weekly.MaxTasks,
		LastSyncedAt: h.cache.LastSyncedAt(),
	})
}

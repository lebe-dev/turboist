package handlers

import (
	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/config"
)

// MetaHandler handles public meta and config endpoints.
// /healthz and /version are registered inline in server.go.
// This handler exposes /api/v1/config (requires auth).
type MetaHandler struct {
	cfg *config.Config
}

// NewMetaHandler constructs a MetaHandler.
func NewMetaHandler(cfg *config.Config) *MetaHandler {
	return &MetaHandler{cfg: cfg}
}

// Register wires /config onto the authenticated API group r.
func (h *MetaHandler) Register(r fiber.Router) {
	r.Get("/config", h.config)
}

type dayPartResp struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type autoLabelResp struct {
	Mask       string `json:"mask"`
	Label      string `json:"label"`
	IgnoreCase bool   `json:"ignoreCase"`
}

type overflowTaskResp struct {
	Title    string `json:"title"`
	Priority string `json:"priority"`
}

type inboxResp struct {
	WarnThreshold int              `json:"warnThreshold"`
	OverflowTask  overflowTaskResp `json:"overflowTask"`
}

type limitResp struct {
	Limit int `json:"limit"`
}

type configResp struct {
	Timezone   string                 `json:"timezone"`
	MaxPinned  int                    `json:"maxPinned"`
	Weekly     limitResp              `json:"weekly"`
	Backlog    limitResp              `json:"backlog"`
	Inbox      inboxResp              `json:"inbox"`
	DayParts   map[string]dayPartResp `json:"dayParts"`
	AutoLabels []autoLabelResp        `json:"autoLabels"`
}

func (h *MetaHandler) config(c fiber.Ctx) error {
	cfg := h.cfg
	dayParts := make(map[string]dayPartResp, len(cfg.DayParts))
	for name, dp := range cfg.DayParts {
		dayParts[name] = dayPartResp{Start: dp.Start, End: dp.End}
	}
	autoLabels := make([]autoLabelResp, len(cfg.AutoLabels))
	for i, al := range cfg.AutoLabels {
		autoLabels[i] = autoLabelResp{
			Mask:       al.Mask,
			Label:      al.Label,
			IgnoreCase: al.IgnoreCaseValue(),
		}
	}
	return c.JSON(configResp{
		Timezone:  cfg.Timezone,
		MaxPinned: cfg.MaxPinned,
		Weekly:    limitResp{Limit: cfg.Weekly.Limit},
		Backlog:   limitResp{Limit: cfg.Backlog.Limit},
		Inbox: inboxResp{
			WarnThreshold: cfg.Inbox.WarnThreshold,
			OverflowTask: overflowTaskResp{
				Title:    cfg.Inbox.OverflowTask.Title,
				Priority: cfg.Inbox.OverflowTask.Priority,
			},
		},
		DayParts:   dayParts,
		AutoLabels: autoLabels,
	})
}

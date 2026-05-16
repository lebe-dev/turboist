package handlers

import (
	"context"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// AppSettingsHandler exposes global application settings.
//
//	GET /api/v1/app-settings              -> returns AppSettings
//	PUT /api/v1/app-settings/auto-labels  -> replaces the auto-label rules list
type AppSettingsHandler struct {
	repo   *repo.AppSettingsRepo
	labels *repo.LabelRepo
}

func NewAppSettingsHandler(r *repo.AppSettingsRepo, labels *repo.LabelRepo) *AppSettingsHandler {
	return &AppSettingsHandler{repo: r, labels: labels}
}

func (h *AppSettingsHandler) Register(r fiber.Router) {
	r.Get("/app-settings", h.get)
	r.Put("/app-settings/auto-labels", h.putAutoLabels)
}

type autoLabelDTO struct {
	Mask       string  `json:"mask"`
	LabelIDs   []int64 `json:"labelIds"`
	IgnoreCase bool    `json:"ignoreCase"`
}

type appSettingsResp struct {
	AutoLabels []autoLabelDTO `json:"autoLabels"`
}

type autoLabelsPutReq struct {
	AutoLabels []autoLabelDTO `json:"autoLabels"`
}

func toAppSettingsResp(s *model.AppSettings) appSettingsResp {
	rules := make([]autoLabelDTO, len(s.AutoLabels))
	for i, r := range s.AutoLabels {
		ids := r.LabelIDs
		if ids == nil {
			ids = []int64{}
		}
		rules[i] = autoLabelDTO{Mask: r.Mask, LabelIDs: ids, IgnoreCase: r.IgnoreCase}
	}
	return appSettingsResp{AutoLabels: rules}
}

func (h *AppSettingsHandler) get(c fiber.Ctx) error {
	s, err := h.repo.Get(c.Context())
	if err != nil {
		return httpapi.ErrInternal("load app settings")
	}
	return c.JSON(toAppSettingsResp(s))
}

func (h *AppSettingsHandler) putAutoLabels(c fiber.Ctx) error {
	var req autoLabelsPutReq
	if err := c.Bind().JSON(&req); err != nil {
		return httpapi.ErrValidation("invalid JSON")
	}
	rules := make([]model.AutoLabelRule, 0, len(req.AutoLabels))
	for i, r := range req.AutoLabels {
		mask := strings.TrimSpace(r.Mask)
		if mask == "" {
			return httpapi.ErrValidation("auto-labels mask must not be empty", map[string]any{"index": i})
		}
		if len(r.LabelIDs) == 0 {
			return httpapi.ErrValidation("auto-labels labelIds must not be empty", map[string]any{"index": i})
		}
		seen := make(map[int64]struct{}, len(r.LabelIDs))
		ids := make([]int64, 0, len(r.LabelIDs))
		for _, id := range r.LabelIDs {
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			if err := h.ensureLabelExists(c.Context(), id); err != nil {
				return err
			}
			ids = append(ids, id)
		}
		rules = append(rules, model.AutoLabelRule{Mask: mask, LabelIDs: ids, IgnoreCase: r.IgnoreCase})
	}
	current, err := h.repo.Get(c.Context())
	if err != nil {
		return httpapi.ErrInternal("load app settings")
	}
	current.AutoLabels = rules
	if err := h.repo.Set(c.Context(), current); err != nil {
		return httpapi.ErrInternal("save app settings")
	}
	return c.JSON(toAppSettingsResp(current))
}

func (h *AppSettingsHandler) ensureLabelExists(ctx context.Context, id int64) error {
	if _, err := h.labels.Get(ctx, id); err != nil {
		return httpapi.ErrValidation("auto-labels: label not found", map[string]any{"labelId": id})
	}
	return nil
}

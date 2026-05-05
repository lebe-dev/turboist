package handlers

import (
	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// SettingsHandler exposes user application settings.
//
//	GET   /api/v1/settings  -> returns UserSettings
//	PATCH /api/v1/settings  -> partial-merges fields and returns updated UserSettings
type SettingsHandler struct {
	users *repo.UserRepo
}

func NewSettingsHandler(users *repo.UserRepo) *SettingsHandler {
	return &SettingsHandler{users: users}
}

func (h *SettingsHandler) Register(r fiber.Router) {
	r.Get("/settings", h.get)
	r.Patch("/settings", h.patch)
}

type settingsResp struct {
	WeeklyUnplannedExcludedLabelIDs []int64 `json:"weeklyUnplannedExcludedLabelIds"`
}

type settingsPatchReq struct {
	WeeklyUnplannedExcludedLabelIDs *[]int64 `json:"weeklyUnplannedExcludedLabelIds"`
}

func toResp(s *model.UserSettings) settingsResp {
	ids := s.WeeklyUnplannedExcludedLabelIDs
	if ids == nil {
		ids = []int64{}
	}
	return settingsResp{WeeklyUnplannedExcludedLabelIDs: ids}
}

func (h *SettingsHandler) get(c fiber.Ctx) error {
	claims := httpapi.GetClaims(c)
	if claims == nil {
		return httpapi.ErrAuthInvalid("missing auth claims")
	}
	s, err := h.users.GetSettings(c.Context(), claims.UserID)
	if err != nil {
		return httpapi.ErrInternal("load settings")
	}
	return c.JSON(toResp(s))
}

func (h *SettingsHandler) patch(c fiber.Ctx) error {
	claims := httpapi.GetClaims(c)
	if claims == nil {
		return httpapi.ErrAuthInvalid("missing auth claims")
	}
	var req settingsPatchReq
	if err := c.Bind().JSON(&req); err != nil {
		return httpapi.ErrValidation("invalid JSON")
	}
	current, err := h.users.GetSettings(c.Context(), claims.UserID)
	if err != nil {
		return httpapi.ErrInternal("load settings")
	}
	if req.WeeklyUnplannedExcludedLabelIDs != nil {
		current.WeeklyUnplannedExcludedLabelIDs = *req.WeeklyUnplannedExcludedLabelIDs
	}
	if err := h.users.SetSettings(c.Context(), claims.UserID, current); err != nil {
		return httpapi.ErrInternal("save settings")
	}
	return c.JSON(toResp(current))
}

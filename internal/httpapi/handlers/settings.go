package handlers

import (
	"fmt"
	"log/slog"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

const opSettingsPatch = "handler.Settings.Patch"

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
	r.Get("/settings", httpapi.RequireScope("settings:read"), h.get)
	r.Patch("/settings", httpapi.RequireScope("settings:write"), h.patch)
}

type settingsResp struct {
	WeeklyUnplannedExcludedLabelIDs []int64 `json:"weeklyUnplannedExcludedLabelIds"`
	BugLabelIDs                     []int64 `json:"bugLabelIds"`
	Locale                          string  `json:"locale"`
	PublicView                      bool    `json:"publicView"`
	BannerText                      string  `json:"bannerText"`
	BannerPublished                 bool    `json:"bannerPublished"`
	BannerDayPart                   string  `json:"bannerDayPart"`
	CalendarEnabled                 bool    `json:"calendarEnabled"`
	CalendarHidePastEvents          bool    `json:"calendarHidePastEvents"`
	TroikiEnabled                   bool    `json:"troikiEnabled"`
	MaxPinnedTasks                  int     `json:"maxPinnedTasks"`
	MaxPinnedProjects               int     `json:"maxPinnedProjects"`
}

type settingsPatchReq struct {
	WeeklyUnplannedExcludedLabelIDs *[]int64 `json:"weeklyUnplannedExcludedLabelIds"`
	BugLabelIDs                     *[]int64 `json:"bugLabelIds"`
	Locale                          *string  `json:"locale"`
	PublicView                      *bool    `json:"publicView"`
	BannerText                      *string  `json:"bannerText"`
	BannerPublished                 *bool    `json:"bannerPublished"`
	BannerDayPart                   *string  `json:"bannerDayPart"`
	CalendarEnabled                 *bool    `json:"calendarEnabled"`
	CalendarHidePastEvents          *bool    `json:"calendarHidePastEvents"`
	TroikiEnabled                   *bool    `json:"troikiEnabled"`
	MaxPinnedTasks                  *int     `json:"maxPinnedTasks"`
	MaxPinnedProjects               *int     `json:"maxPinnedProjects"`
}

// supportedLocales is the whitelist accepted by PATCH /settings. Empty string
// is allowed and means "let the client decide" (e.g., from navigator.language).
var supportedLocales = map[string]struct{}{
	"":   {},
	"en": {},
	"ru": {},
}

// bannerDayParts is the whitelist accepted for bannerDayPart. Empty string means
// "all day"; model.DayPartNone is deliberately excluded — a banner is either
// unrestricted or bound to one of the three real phases.
var bannerDayParts = map[string]struct{}{
	"":                             {},
	string(model.DayPartMorning):   {},
	string(model.DayPartAfternoon): {},
	string(model.DayPartEvening):   {},
}

// validateMaxPinned bounds a pinned cap to [model.MinMaxPinned, model.MaxMaxPinned].
// Zero is rejected rather than treated as "default" — the client always sends an
// explicit number, and a silent 0 would read as "pinning disabled".
func validateMaxPinned(c fiber.Ctx, field string, v int) error {
	if v < model.MinMaxPinned || v > model.MaxMaxPinned {
		logValidation(c, opSettingsPatch, "pinned cap out of range", slog.String("field", field), slog.Int("value", v))
		return httpapi.ErrValidation(fmt.Sprintf("%s must be between %d and %d", field, model.MinMaxPinned, model.MaxMaxPinned))
	}
	return nil
}

func toResp(s *model.UserSettings) settingsResp {
	ids := s.WeeklyUnplannedExcludedLabelIDs
	if ids == nil {
		ids = []int64{}
	}
	bugIDs := s.BugLabelIDs
	if bugIDs == nil {
		bugIDs = []int64{}
	}
	return settingsResp{
		WeeklyUnplannedExcludedLabelIDs: ids,
		BugLabelIDs:                     bugIDs,
		Locale:                          s.Locale,
		PublicView:                      s.PublicView,
		BannerText:                      s.BannerText,
		BannerPublished:                 s.BannerPublished,
		BannerDayPart:                   string(s.BannerDayPart),
		CalendarEnabled:                 s.CalendarEnabled,
		CalendarHidePastEvents:          s.CalendarHidePastEvents,
		TroikiEnabled:                   s.TroikiEnabled,
		MaxPinnedTasks:                  s.MaxPinnedTasks,
		MaxPinnedProjects:               s.MaxPinnedProjects,
	}
}

func (h *SettingsHandler) get(c fiber.Ctx) error {
	userID := httpapi.GetUserID(c)
	if userID == 0 {
		return httpapi.ErrAuthInvalid("missing auth claims")
	}
	s, err := h.users.GetSettings(c.Context(), userID)
	if err != nil {
		return httpapi.ErrInternal("load settings").WithCause(err)
	}
	return c.JSON(toResp(s))
}

func (h *SettingsHandler) patch(c fiber.Ctx) error {
	userID := httpapi.GetUserID(c)
	if userID == 0 {
		return httpapi.ErrAuthInvalid("missing auth claims")
	}
	logEntry(c, opSettingsPatch, slog.Int64("user_id", userID))
	var req settingsPatchReq
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, opSettingsPatch, "invalid JSON")
		return httpapi.ErrValidation("invalid JSON")
	}
	if req.Locale != nil {
		if _, ok := supportedLocales[*req.Locale]; !ok {
			logValidation(c, opSettingsPatch, "unsupported locale", slog.String("locale", *req.Locale))
			return httpapi.ErrValidation("unsupported locale")
		}
	}
	if req.BannerDayPart != nil {
		if _, ok := bannerDayParts[*req.BannerDayPart]; !ok {
			logValidation(c, opSettingsPatch, "unsupported banner day part", slog.String("day_part", *req.BannerDayPart))
			return httpapi.ErrValidation("unsupported banner day part")
		}
	}
	if req.MaxPinnedTasks != nil {
		if err := validateMaxPinned(c, "maxPinnedTasks", *req.MaxPinnedTasks); err != nil {
			return err
		}
	}
	if req.MaxPinnedProjects != nil {
		if err := validateMaxPinned(c, "maxPinnedProjects", *req.MaxPinnedProjects); err != nil {
			return err
		}
	}
	current, err := h.users.GetSettings(c.Context(), userID)
	if err != nil {
		return httpapi.ErrInternal("load settings").WithCause(err)
	}
	if req.WeeklyUnplannedExcludedLabelIDs != nil {
		current.WeeklyUnplannedExcludedLabelIDs = *req.WeeklyUnplannedExcludedLabelIDs
	}
	if req.BugLabelIDs != nil {
		current.BugLabelIDs = *req.BugLabelIDs
	}
	if req.Locale != nil {
		current.Locale = *req.Locale
	}
	if req.PublicView != nil {
		current.PublicView = *req.PublicView
	}
	if req.BannerText != nil {
		current.BannerText = *req.BannerText
	}
	if req.BannerPublished != nil {
		current.BannerPublished = *req.BannerPublished
	}
	if req.BannerDayPart != nil {
		current.BannerDayPart = model.DayPart(*req.BannerDayPart)
	}
	if req.CalendarEnabled != nil {
		current.CalendarEnabled = *req.CalendarEnabled
	}
	if req.CalendarHidePastEvents != nil {
		current.CalendarHidePastEvents = *req.CalendarHidePastEvents
	}
	if req.TroikiEnabled != nil {
		current.TroikiEnabled = *req.TroikiEnabled
	}
	if req.MaxPinnedTasks != nil {
		current.MaxPinnedTasks = *req.MaxPinnedTasks
	}
	if req.MaxPinnedProjects != nil {
		current.MaxPinnedProjects = *req.MaxPinnedProjects
	}
	if err := h.users.SetSettings(c.Context(), userID, current); err != nil {
		return httpapi.ErrInternal("save settings").WithCause(err)
	}
	logMutation(c, opSettingsPatch, slog.Int64("user_id", userID))
	return c.JSON(toResp(current))
}

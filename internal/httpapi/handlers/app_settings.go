package handlers

import (
	"context"
	"log/slog"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service/linktitle"
)

const (
	opAppSettingsPutAutoLabels         = "handler.AppSettings.PutAutoLabels"
	opAppSettingsPutProjectSuggestions = "handler.AppSettings.PutProjectSuggestions"
	opAppSettingsPutLinkTitles         = "handler.AppSettings.PutLinkTitles"
)

// AppSettingsHandler exposes global application settings.
//
//	GET /api/v1/app-settings                      -> returns AppSettings
//	PUT /api/v1/app-settings/auto-labels          -> replaces the auto-label rules list
//	PUT /api/v1/app-settings/project-suggestions  -> replaces the project-suggestion rules list
//	PUT /api/v1/app-settings/link-titles          -> link titles: toggle, default template, domain rules
type AppSettingsHandler struct {
	repo     *repo.AppSettingsRepo
	labels   *repo.LabelRepo
	projects *repo.ProjectRepo
}

func NewAppSettingsHandler(r *repo.AppSettingsRepo, labels *repo.LabelRepo, projects *repo.ProjectRepo) *AppSettingsHandler {
	return &AppSettingsHandler{repo: r, labels: labels, projects: projects}
}

func (h *AppSettingsHandler) Register(r fiber.Router) {
	r.Get("/app-settings", httpapi.RequireScope("settings:read"), h.get)
	r.Put("/app-settings/auto-labels", httpapi.RequireScope("settings:write"), h.putAutoLabels)
	r.Put("/app-settings/project-suggestions", httpapi.RequireScope("settings:write"), h.putProjectSuggestions)
	r.Put("/app-settings/link-titles", httpapi.RequireScope("settings:write"), h.putLinkTitles)
}

type autoLabelDTO struct {
	Mask       string  `json:"mask"`
	LabelIDs   []int64 `json:"labelIds"`
	IgnoreCase bool    `json:"ignoreCase"`
}

type projectSuggestionDTO struct {
	Mask       string  `json:"mask"`
	ProjectIDs []int64 `json:"projectIds"`
	IgnoreCase bool    `json:"ignoreCase"`
}

type inboxProcessingSettingsDTO struct {
	Prompt string `json:"prompt"`
	Paused bool   `json:"paused"`
}

type linkTitleRuleDTO struct {
	Domain   string `json:"domain"`
	Template string `json:"template"`
}

type linkTitlesSettingsDTO struct {
	Enabled bool `json:"enabled"`
	// Template is the default title template; empty = built-in default for the
	// user's language.
	Template string             `json:"template"`
	Rules    []linkTitleRuleDTO `json:"rules"`
}

type appSettingsResp struct {
	AutoLabels         []autoLabelDTO             `json:"autoLabels"`
	ProjectSuggestions []projectSuggestionDTO     `json:"projectSuggestions"`
	InboxProcessing    inboxProcessingSettingsDTO `json:"inboxProcessing"`
	LinkTitles         linkTitlesSettingsDTO      `json:"linkTitles"`
}

// linkTitlesPutReq updates any subset of the link-title settings; a field left
// out is kept as stored. Rules, when present, replace the whole list.
type linkTitlesPutReq struct {
	Enabled  *bool               `json:"enabled"`
	Template *string             `json:"template"`
	Rules    *[]linkTitleRuleDTO `json:"rules"`
}

type autoLabelsPutReq struct {
	AutoLabels []autoLabelDTO `json:"autoLabels"`
}

type projectSuggestionsPutReq struct {
	ProjectSuggestions []projectSuggestionDTO `json:"projectSuggestions"`
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
	suggestions := make([]projectSuggestionDTO, len(s.ProjectSuggestions))
	for i, r := range s.ProjectSuggestions {
		ids := r.ProjectIDs
		if ids == nil {
			ids = []int64{}
		}
		suggestions[i] = projectSuggestionDTO{Mask: r.Mask, ProjectIDs: ids, IgnoreCase: r.IgnoreCase}
	}
	linkRules := make([]linkTitleRuleDTO, len(s.LinkTitles.Rules))
	for i, r := range s.LinkTitles.Rules {
		linkRules[i] = linkTitleRuleDTO{Domain: r.Domain, Template: r.Template}
	}
	return appSettingsResp{
		AutoLabels:         rules,
		ProjectSuggestions: suggestions,
		InboxProcessing:    inboxProcessingSettingsDTO{Prompt: s.InboxProcessing.Prompt, Paused: s.InboxProcessing.Paused},
		LinkTitles: linkTitlesSettingsDTO{
			Enabled:  s.LinkTitles.Enabled,
			Template: s.LinkTitles.Template,
			Rules:    linkRules,
		},
	}
}

func (h *AppSettingsHandler) get(c fiber.Ctx) error {
	s, err := h.repo.Get(c.Context())
	if err != nil {
		return httpapi.ErrInternal("load app settings").WithCause(err)
	}
	return c.JSON(toAppSettingsResp(s))
}

func (h *AppSettingsHandler) putAutoLabels(c fiber.Ctx) error {
	logEntry(c, opAppSettingsPutAutoLabels)
	var req autoLabelsPutReq
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, opAppSettingsPutAutoLabels, "invalid JSON")
		return httpapi.ErrValidation("invalid JSON")
	}
	rules := make([]model.AutoLabelRule, 0, len(req.AutoLabels))
	for i, r := range req.AutoLabels {
		mask := strings.TrimSpace(r.Mask)
		if mask == "" {
			logValidation(c, opAppSettingsPutAutoLabels, "empty mask", slog.Int("index", i))
			return httpapi.ErrValidation("auto-labels mask must not be empty", map[string]any{"index": i})
		}
		if len(r.LabelIDs) == 0 {
			logValidation(c, opAppSettingsPutAutoLabels, "empty labelIds", slog.Int("index", i))
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
		return httpapi.ErrInternal("load app settings").WithCause(err)
	}
	current.AutoLabels = rules
	if err := h.repo.Set(c.Context(), current); err != nil {
		return httpapi.ErrInternal("save app settings").WithCause(err)
	}
	logMutation(c, opAppSettingsPutAutoLabels, slog.Int("rules", len(rules)))
	return c.JSON(toAppSettingsResp(current))
}

func (h *AppSettingsHandler) putProjectSuggestions(c fiber.Ctx) error {
	logEntry(c, opAppSettingsPutProjectSuggestions)
	var req projectSuggestionsPutReq
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, opAppSettingsPutProjectSuggestions, "invalid JSON")
		return httpapi.ErrValidation("invalid JSON")
	}
	rules := make([]model.ProjectSuggestionRule, 0, len(req.ProjectSuggestions))
	for i, r := range req.ProjectSuggestions {
		mask := strings.TrimSpace(r.Mask)
		if mask == "" {
			logValidation(c, opAppSettingsPutProjectSuggestions, "empty mask", slog.Int("index", i))
			return httpapi.ErrValidation("project-suggestions mask must not be empty", map[string]any{"index": i})
		}
		if len(r.ProjectIDs) == 0 {
			logValidation(c, opAppSettingsPutProjectSuggestions, "empty projectIds", slog.Int("index", i))
			return httpapi.ErrValidation("project-suggestions projectIds must not be empty", map[string]any{"index": i})
		}
		seen := make(map[int64]struct{}, len(r.ProjectIDs))
		ids := make([]int64, 0, len(r.ProjectIDs))
		for _, id := range r.ProjectIDs {
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			if err := h.ensureProjectExists(c.Context(), id); err != nil {
				return err
			}
			ids = append(ids, id)
		}
		rules = append(rules, model.ProjectSuggestionRule{Mask: mask, ProjectIDs: ids, IgnoreCase: r.IgnoreCase})
	}
	current, err := h.repo.Get(c.Context())
	if err != nil {
		return httpapi.ErrInternal("load app settings").WithCause(err)
	}
	current.ProjectSuggestions = rules
	if err := h.repo.Set(c.Context(), current); err != nil {
		return httpapi.ErrInternal("save app settings").WithCause(err)
	}
	logMutation(c, opAppSettingsPutProjectSuggestions, slog.Int("rules", len(rules)))
	return c.JSON(toAppSettingsResp(current))
}

func (h *AppSettingsHandler) putLinkTitles(c fiber.Ctx) error {
	logEntry(c, opAppSettingsPutLinkTitles)
	var req linkTitlesPutReq
	if err := c.Bind().JSON(&req); err != nil || (req.Enabled == nil && req.Template == nil && req.Rules == nil) {
		logValidation(c, opAppSettingsPutLinkTitles, msgInvalidBody)
		return httpapi.ErrValidation(msgInvalidRequestBody)
	}
	var template string
	if req.Template != nil {
		t, err := linktitle.NormalizeTemplate(*req.Template)
		if err != nil {
			logValidation(c, opAppSettingsPutLinkTitles, "invalid template")
			return httpapi.ErrValidation(err.Error(), map[string]any{"field": "template"})
		}
		template = t
	}
	var rules []model.LinkTitleRule
	if req.Rules != nil {
		in := make([]model.LinkTitleRule, len(*req.Rules))
		for i, r := range *req.Rules {
			in[i] = model.LinkTitleRule{Domain: r.Domain, Template: r.Template}
		}
		out, idx, err := linktitle.NormalizeRules(in)
		if err != nil {
			logValidation(c, opAppSettingsPutLinkTitles, "invalid rule", slog.Int("index", idx))
			details := map[string]any{"field": "rules"}
			if idx >= 0 {
				details["index"] = idx
			}
			return httpapi.ErrValidation(err.Error(), details)
		}
		rules = out
	}
	current, err := h.repo.Get(c.Context())
	if err != nil {
		return httpapi.ErrInternal("load app settings").WithCause(err)
	}
	if req.Enabled != nil {
		current.LinkTitles.Enabled = *req.Enabled
	}
	if req.Template != nil {
		current.LinkTitles.Template = template
	}
	if req.Rules != nil {
		current.LinkTitles.Rules = rules
	}
	if err := h.repo.Set(c.Context(), current); err != nil {
		return httpapi.ErrInternal("save app settings").WithCause(err)
	}
	logMutation(c, opAppSettingsPutLinkTitles, slog.Bool("enabled", current.LinkTitles.Enabled),
		slog.Int("rules", len(current.LinkTitles.Rules)))
	return c.JSON(toAppSettingsResp(current))
}

func (h *AppSettingsHandler) ensureLabelExists(ctx context.Context, id int64) error {
	if _, err := h.labels.Get(ctx, id); err != nil {
		return httpapi.ErrValidation("auto-labels: label not found", map[string]any{"labelId": id})
	}
	return nil
}

func (h *AppSettingsHandler) ensureProjectExists(ctx context.Context, id int64) error {
	if _, err := h.projects.Get(ctx, id); err != nil {
		return httpapi.ErrValidation("project-suggestions: project not found", map[string]any{"projectId": id})
	}
	return nil
}

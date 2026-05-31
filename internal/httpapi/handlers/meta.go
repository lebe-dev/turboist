package handlers

import (
	"encoding/json"

	"github.com/gofiber/fiber/v3"
	"golang.org/x/sync/errgroup"

	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
)

// configListLimit caps the number of contexts/projects/labels returned in the
// embedded bootstrap sections. Frontend used a 500-row limit for the same
// lists when they were fetched via dedicated endpoints; keep parity.
const configListLimit = 500

// MetaHandler handles public meta and config endpoints.
// /healthz and /version are registered inline in server.go.
// This handler exposes /api/v1/config (requires auth) which doubles as the
// workspace-bootstrap endpoint: a single round-trip returns the static config
// values plus the user's contexts, projects, labels, settings, app settings,
// UI state and troiki view.
type MetaHandler struct {
	cfg             *config.Config
	totpAvailable   bool
	contexts        *repo.ContextRepo
	projects        *repo.ProjectRepo
	labels          *repo.LabelRepo
	tasks           *repo.TaskRepo
	users           *repo.UserRepo
	appSettingsRepo *repo.AppSettingsRepo
	troikiSvc       *service.TroikiService
	baseURL         string
}

// NewMetaHandler constructs a MetaHandler. totpAvailable reports whether
// the TOTP feature is wired up on this deploy (TOTP_SECRET_KEY non-empty);
// the frontend uses it to hide the 2FA UI when the routes are not mounted.
func NewMetaHandler(
	cfg *config.Config,
	totpAvailable bool,
	contexts *repo.ContextRepo,
	projects *repo.ProjectRepo,
	labels *repo.LabelRepo,
	tasks *repo.TaskRepo,
	users *repo.UserRepo,
	appSettingsRepo *repo.AppSettingsRepo,
	troikiSvc *service.TroikiService,
	baseURL string,
) *MetaHandler {
	return &MetaHandler{
		cfg:             cfg,
		totpAvailable:   totpAvailable,
		contexts:        contexts,
		projects:        projects,
		labels:          labels,
		tasks:           tasks,
		users:           users,
		appSettingsRepo: appSettingsRepo,
		troikiSvc:       troikiSvc,
		baseURL:         baseURL,
	}
}

// Register wires /config onto the authenticated API group r.
func (h *MetaHandler) Register(r fiber.Router) {
	r.Get("/config", httpapi.RequireScope("settings:read"), h.config)
}

type dayPartResp struct {
	Start int `json:"start"`
	End   int `json:"end"`
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

type planStatsResp struct {
	Week    int `json:"week"`
	Backlog int `json:"backlog"`
}

type inboxStatsResp struct {
	Count                 int  `json:"count"`
	WarnThresholdExceeded bool `json:"warnThresholdExceeded"`
}

type configResp struct {
	Timezone      string                 `json:"timezone"`
	MaxPinned     int                    `json:"maxPinned"`
	Weekly        limitResp              `json:"weekly"`
	Backlog       limitResp              `json:"backlog"`
	Inbox         inboxResp              `json:"inbox"`
	DayParts      map[string]dayPartResp `json:"dayParts"`
	TOTPAvailable bool                   `json:"totpAvailable"`

	Contexts    []dto.ContextDTO `json:"contexts"`
	Projects    []dto.ProjectDTO `json:"projects"`
	Labels      []dto.LabelDTO   `json:"labels"`
	Settings    settingsResp     `json:"settings"`
	AppSettings appSettingsResp  `json:"appSettings"`
	UserState   json.RawMessage  `json:"userState"`
	Troiki      any              `json:"troiki"`
	PlanStats   planStatsResp    `json:"planStats"`
	InboxStats  inboxStatsResp   `json:"inboxStats"`
	PinnedTasks []dto.TaskDTO    `json:"pinnedTasks"`
}

func (h *MetaHandler) config(c fiber.Ctx) error {
	cfg := h.cfg
	dayParts := make(map[string]dayPartResp, len(cfg.DayParts))
	for name, dp := range cfg.DayParts {
		dayParts[name] = dayPartResp{Start: dp.Start, End: dp.End}
	}

	userID := httpapi.GetUserID(c)
	if userID == 0 {
		return httpapi.ErrAuthInvalid("missing auth claims")
	}

	ctx := c.Context()
	page := repo.Page{Limit: configListLimit, Offset: 0}

	var (
		contexts    []dto.ContextDTO
		projects    []dto.ProjectDTO
		labels      []dto.LabelDTO
		settings    settingsResp
		appSettings appSettingsResp
		userState   json.RawMessage
		troiki      any
		planStats   planStatsResp
		inboxStats  inboxStatsResp
		pinnedTasks []dto.TaskDTO
	)

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		items, _, err := h.contexts.List(gctx, page)
		if err != nil {
			return err
		}
		contexts = make([]dto.ContextDTO, len(items))
		for i, ctx := range items {
			contexts[i] = dto.ContextFromModel(ctx)
		}
		return nil
	})

	g.Go(func() error {
		items, _, err := h.projects.List(gctx, repo.ProjectListFilter{}, page)
		if err != nil {
			return err
		}
		projects = make([]dto.ProjectDTO, len(items))
		for i, p := range items {
			projects[i] = dto.ProjectFromModel(p)
		}
		return nil
	})

	g.Go(func() error {
		items, _, err := h.labels.List(gctx, repo.LabelListFilter{}, page)
		if err != nil {
			return err
		}
		labels = make([]dto.LabelDTO, len(items))
		for i, l := range items {
			labels[i] = dto.LabelFromModel(l)
		}
		return nil
	})

	g.Go(func() error {
		s, err := h.users.GetSettings(gctx, userID)
		if err != nil {
			return err
		}
		settings = toResp(s)
		return nil
	})

	g.Go(func() error {
		s, err := h.appSettingsRepo.Get(gctx)
		if err != nil {
			return err
		}
		appSettings = toAppSettingsResp(s)
		return nil
	})

	g.Go(func() error {
		raw, err := h.users.GetState(gctx, userID)
		if err != nil {
			return err
		}
		if raw == "" {
			raw = "{}"
		}
		userState = json.RawMessage(raw)
		return nil
	})

	g.Go(func() error {
		v, err := h.troikiSvc.View(gctx)
		if err != nil {
			return err
		}
		troiki = RenderTroikiView(v, h.baseURL)
		return nil
	})

	g.Go(func() error {
		week, err := h.tasks.CountWeek(gctx)
		if err != nil {
			return err
		}
		backlog, err := h.tasks.CountBacklog(gctx)
		if err != nil {
			return err
		}
		planStats = planStatsResp{Week: week, Backlog: backlog}
		return nil
	})

	g.Go(func() error {
		_, total, err := h.tasks.ListInbox(gctx, repo.TaskFilter{}, repo.Page{Limit: 0})
		if err != nil {
			return err
		}
		inboxStats = inboxStatsResp{
			Count:                 total,
			WarnThresholdExceeded: total > h.cfg.Inbox.WarnThreshold,
		}
		return nil
	})

	g.Go(func() error {
		items, _, err := h.tasks.ListPinned(gctx, repo.TaskFilter{})
		if err != nil {
			return err
		}
		pinnedTasks = make([]dto.TaskDTO, len(items))
		for i, t := range items {
			pinnedTasks[i] = dto.TaskFromModel(t, h.baseURL)
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		return httpapi.ErrInternal("load config").WithCause(err)
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
		DayParts:      dayParts,
		TOTPAvailable: h.totpAvailable,
		Contexts:      contexts,
		Projects:      projects,
		Labels:        labels,
		Settings:      settings,
		AppSettings:   appSettings,
		UserState:     userState,
		Troiki:        troiki,
		PlanStats:     planStats,
		InboxStats:    inboxStats,
		PinnedTasks:   pinnedTasks,
	})
}

package handler

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/storage"
	"github.com/lebe-dev/turboist/internal/todoist"
)

// ConfigHandler handles GET /api/config — returns consolidated app config, metadata, and user state.
type ConfigHandler struct {
	cache *todoist.Cache
	cfg   *config.AppConfig
	store *storage.Store
}

// NewConfigHandler creates a new ConfigHandler.
func NewConfigHandler(cache *todoist.Cache, cfg *config.AppConfig, store *storage.Store) *ConfigHandler {
	return &ConfigHandler{cache: cache, cfg: cfg, store: store}
}

type dayPartResponse struct {
	Label string `json:"label"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

type settingsResponse struct {
	PollInterval  int               `json:"poll_interval"`
	Timezone      string            `json:"timezone"`
	WeeklyLabel   string            `json:"weekly_label"`
	BacklogLabel  string            `json:"backlog_label"`
	WeeklyLimit   int               `json:"weekly_limit"`
	BacklogLimit  int               `json:"backlog_limit"`
	CompletedDays int               `json:"completed_days"`
	MaxPinned     int               `json:"max_pinned"`
	LastSyncedAt  time.Time         `json:"last_synced_at"`
	DayParts      []dayPartResponse `json:"day_parts"`
}

type contextFiltersResponse struct {
	Projects []string `json:"projects"`
	Sections []string `json:"sections"`
	Labels   []string `json:"labels"`
}

type contextItem struct {
	ID            string                 `json:"id"`
	DisplayName   string                 `json:"display_name"`
	Color         string                 `json:"color,omitempty"`
	InheritLabels bool                   `json:"inherit_labels"`
	Filters       contextFiltersResponse `json:"filters"`
}

type projectWithSections struct {
	ID       string             `json:"id"`
	Name     string             `json:"name"`
	Sections []*todoist.Section `json:"sections"`
}

type quickCaptureResponse struct {
	ParentTaskID string `json:"parent_task_id"`
}

type labelConfigResponse struct {
	Name              string `json:"name"`
	InheritToSubtasks bool   `json:"inherit_to_subtasks"`
}

type appConfigResponse struct {
	Settings     settingsResponse      `json:"settings"`
	Contexts     []contextItem         `json:"contexts"`
	Projects     []projectWithSections `json:"projects"`
	Labels       []*todoist.Label      `json:"labels"`
	LabelConfigs []labelConfigResponse `json:"label_configs"`
	QuickCapture *quickCaptureResponse `json:"quick_capture"`
	State        *storage.UserState    `json:"state"`
}

// Config handles GET /api/config — consolidated response with settings, contexts, projects, labels, quick_capture, and state.
func (h *ConfigHandler) Config(c fiber.Ctx) error {
	// Settings
	dayParts := make([]dayPartResponse, 0, len(h.cfg.Today.DayParts))
	for _, dp := range h.cfg.Today.DayParts {
		dayParts = append(dayParts, dayPartResponse{
			Label: dp.Label,
			Start: dp.Start,
			End:   dp.End,
		})
	}
	settings := settingsResponse{
		PollInterval:  int(h.cfg.PollInterval.Seconds()),
		Timezone:      h.cfg.Timezone,
		WeeklyLabel:   h.cfg.Weekly.Label,
		BacklogLabel:  h.cfg.Backlog.Label,
		WeeklyLimit:   h.cfg.Weekly.MaxTasks,
		BacklogLimit:  h.cfg.Backlog.MaxLimit,
		CompletedDays: h.cfg.Completed.Days,
		MaxPinned:     h.cfg.MaxPinned,
		LastSyncedAt:  h.cache.LastSyncedAt(),
		DayParts:      dayParts,
	}

	// Contexts
	contexts := make([]contextItem, 0, len(h.cfg.Contexts))
	for _, ctx := range h.cfg.Contexts {
		filters := contextFiltersResponse{
			Projects: ctx.Filters.Projects,
			Sections: ctx.Filters.Sections,
			Labels:   ctx.Filters.Labels,
		}
		if filters.Projects == nil {
			filters.Projects = []string{}
		}
		if filters.Sections == nil {
			filters.Sections = []string{}
		}
		if filters.Labels == nil {
			filters.Labels = []string{}
		}
		contexts = append(contexts, contextItem{
			ID:            ctx.ID,
			DisplayName:   ctx.DisplayName,
			Color:         ctx.Color,
			InheritLabels: ctx.ShouldInheritLabels(),
			Filters:       filters,
		})
	}

	// Projects with sections
	projects := h.cache.Projects()
	sections := h.cache.Sections()
	sectionsByProject := make(map[string][]*todoist.Section)
	for _, s := range sections {
		sectionsByProject[s.ProjectID] = append(sectionsByProject[s.ProjectID], s)
	}
	projectItems := make([]projectWithSections, len(projects))
	for i, p := range projects {
		secs := sectionsByProject[p.ID]
		if secs == nil {
			secs = []*todoist.Section{}
		}
		projectItems[i] = projectWithSections{
			ID:       p.ID,
			Name:     p.Name,
			Sections: secs,
		}
	}

	// Labels
	labels := h.cache.Labels()

	// Quick capture
	var qc *quickCaptureResponse
	if h.cfg.QuickCapture != nil {
		for _, t := range h.cache.Tasks() {
			if t.Content == h.cfg.QuickCapture.Title {
				qc = &quickCaptureResponse{ParentTaskID: t.ID}
				break
			}
		}
	}

	// Label configs
	labelConfigs := make([]labelConfigResponse, 0, len(h.cfg.Labels))
	for _, lc := range h.cfg.Labels {
		labelConfigs = append(labelConfigs, labelConfigResponse{
			Name:              lc.Name,
			InheritToSubtasks: lc.ShouldInheritToSubtasks(),
		})
	}

	// User state
	state, err := h.store.GetState()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to load state"})
	}

	return c.JSON(appConfigResponse{
		Settings:     settings,
		Contexts:     contexts,
		Projects:     projectItems,
		Labels:       labels,
		LabelConfigs: labelConfigs,
		QuickCapture: qc,
		State:        state,
	})
}

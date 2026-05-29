package handlers

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
	"golang.org/x/sync/errgroup"

	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// TaskViewHandler serves the named view endpoints (today/tomorrow/overdue/week/backlog).
type TaskViewHandler struct {
	tasks   *repo.TaskRepo
	cfg     *config.Config
	baseURL string
}

func NewTaskViewHandler(tasks *repo.TaskRepo, cfg *config.Config, baseURL string) *TaskViewHandler {
	return &TaskViewHandler{tasks: tasks, cfg: cfg, baseURL: baseURL}
}

func (h *TaskViewHandler) Register(r fiber.Router) {
	r.Get("/tasks/today", httpapi.RequireScope("tasks:read"), h.today)
	r.Get("/tasks/tomorrow", httpapi.RequireScope("tasks:read"), h.tomorrow)
	r.Get("/tasks/overdue", httpapi.RequireScope("tasks:read"), h.overdue)
	r.Get("/tasks/week", httpapi.RequireScope("tasks:read"), h.week)
	r.Get("/tasks/backlog", httpapi.RequireScope("tasks:read"), h.backlog)
	r.Get("/tasks/pinned", httpapi.RequireScope("tasks:read"), h.pinned)
	r.Get("/tasks/completed", httpapi.RequireScope("tasks:read"), h.completed)
	r.Get("/stats/plan", httpapi.RequireScope("tasks:read"), h.statsPlan)
	r.Get("/stats/sidebar", httpapi.RequireScope("tasks:read"), h.statsSidebar)
}

// todayStart returns the start of the current day in the configured timezone.
func (h *TaskViewHandler) todayStart() time.Time {
	now := time.Now().In(h.cfg.Location)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, h.cfg.Location).UTC()
}

// currentWeekRange returns [Mon 00:00, next Mon 00:00) in the configured
// timezone, expressed as UTC instants. Week starts on Monday.
func (h *TaskViewHandler) currentWeekRange() (time.Time, time.Time) {
	now := time.Now().In(h.cfg.Location)
	daysFromMonday := (int(now.Weekday()) + 6) % 7
	start := time.Date(now.Year(), now.Month(), now.Day()-daysFromMonday, 0, 0, 0, 0, h.cfg.Location).UTC()
	return start, start.AddDate(0, 0, 7)
}

func parseViewFilter(c fiber.Ctx) repo.TaskFilter {
	f := repo.TaskFilter{}
	if v := c.Query("contextId"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			f.ContextID = &n
		}
	}
	if v := c.Query("projectId"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			f.ProjectID = &n
		}
	}
	if v := c.Query("labelId"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			f.LabelID = &n
		}
	}
	if v := c.Query("priority"); v != "" {
		p := model.Priority(v)
		if p.IsValid() {
			f.Priority = &p
		}
	}
	return f
}

// todayBundleResponse groups every list the Today page needs into one
// response. Previously the page issued three parallel requests (today,
// overdue, completed?limit=1) — now it is one round-trip.
type todayBundleResponse struct {
	Today          dto.PagedResponse[dto.TaskDTO] `json:"today"`
	Overdue        dto.PagedResponse[dto.TaskDTO] `json:"overdue"`
	CompletedToday dto.PagedResponse[dto.TaskDTO] `json:"completedToday"`
}

func (h *TaskViewHandler) today(c fiber.Ctx) error {
	pp := dto.ParsePageParams(c.Query("limit"), c.Query("offset"))
	filter := parseViewFilter(c)
	todayStart := h.todayStart()
	ctx := c.Context()

	var (
		todayResp     dto.PagedResponse[dto.TaskDTO]
		overdueResp   dto.PagedResponse[dto.TaskDTO]
		completedResp dto.PagedResponse[dto.TaskDTO]
	)

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		items, total, err := h.tasks.ListToday(gctx, todayStart, filter, repo.Page{Limit: pp.Limit, Offset: pp.Offset})
		if err != nil {
			return err
		}
		todayResp = dto.NewPagedResponse(tasksToDTO(items, h.baseURL), total, pp.Limit, pp.Offset)
		return nil
	})

	g.Go(func() error {
		items, total, err := h.tasks.ListOverdue(gctx, todayStart, filter, repo.Page{Limit: pp.Limit, Offset: pp.Offset})
		if err != nil {
			return err
		}
		overdueResp = dto.NewPagedResponse(tasksToDTO(items, h.baseURL), total, pp.Limit, pp.Offset)
		return nil
	})

	g.Go(func() error {
		// Completed window is the same one /tasks/completed?days=1 returns:
		// from start-of-today (in the configured timezone) through start-of-tomorrow.
		end := todayStart.Add(24 * time.Hour)
		items, total, err := h.tasks.ListCompletedInRange(gctx, todayStart, end, filter, repo.Page{Limit: 1, Offset: 0})
		if err != nil {
			return err
		}
		completedResp = dto.NewPagedResponse(tasksToDTO(items, h.baseURL), total, 1, 0)
		return nil
	})

	if err := g.Wait(); err != nil {
		return httpapi.ErrInternal("list today bundle").WithCause(err)
	}

	return c.JSON(todayBundleResponse{
		Today:          todayResp,
		Overdue:        overdueResp,
		CompletedToday: completedResp,
	})
}

func (h *TaskViewHandler) tomorrow(c fiber.Ctx) error {
	pp := dto.ParsePageParams(c.Query("limit"), c.Query("offset"))
	filter := parseViewFilter(c)
	items, total, err := h.tasks.ListTomorrow(c.Context(), h.todayStart(), filter, repo.Page{Limit: pp.Limit, Offset: pp.Offset})
	if err != nil {
		return httpapi.ErrInternal("list tomorrow").WithCause(err)
	}
	return c.JSON(dto.NewPagedResponse(tasksToDTO(items, h.baseURL), total, pp.Limit, pp.Offset))
}

// completed returns tasks completed within the last `days` days (clamped to
// [1, 90]). The window ends at the start of tomorrow in the configured
// timezone, so today is always included. `days=1` keeps the original
// today-only behavior.
func (h *TaskViewHandler) completed(c fiber.Ctx) error {
	pp := dto.ParsePageParams(c.Query("limit"), c.Query("offset"))
	filter := parseViewFilter(c)
	days := 1
	if v := c.Query("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 {
			days = min(n, 90)
		}
	}
	todayStart := h.todayStart()
	start := todayStart.Add(-time.Duration(days-1) * 24 * time.Hour)
	end := todayStart.Add(24 * time.Hour)
	items, total, err := h.tasks.ListCompletedInRange(c.Context(), start, end, filter, repo.Page{Limit: pp.Limit, Offset: pp.Offset})
	if err != nil {
		return httpapi.ErrInternal("list completed").WithCause(err)
	}
	return c.JSON(dto.NewPagedResponse(tasksToDTO(items, h.baseURL), total, pp.Limit, pp.Offset))
}

func (h *TaskViewHandler) overdue(c fiber.Ctx) error {
	pp := dto.ParsePageParams(c.Query("limit"), c.Query("offset"))
	filter := parseViewFilter(c)
	items, total, err := h.tasks.ListOverdue(c.Context(), h.todayStart(), filter, repo.Page{Limit: pp.Limit, Offset: pp.Offset})
	if err != nil {
		return httpapi.ErrInternal("list overdue").WithCause(err)
	}
	return c.JSON(dto.NewPagedResponse(tasksToDTO(items, h.baseURL), total, pp.Limit, pp.Offset))
}

// viewResponse is returned by week/backlog (no pagination params).
type viewResponse struct {
	Items []dto.TaskDTO `json:"items"`
	Total int           `json:"total"`
}

func (h *TaskViewHandler) week(c fiber.Ctx) error {
	filter := parseViewFilter(c)
	start, end := h.currentWeekRange()
	items, total, err := h.tasks.ListWeek(c.Context(), start, end, filter)
	if err != nil {
		return httpapi.ErrInternal("list week").WithCause(err)
	}
	return c.JSON(viewResponse{Items: tasksToDTO(items, h.baseURL), Total: total})
}

func (h *TaskViewHandler) backlog(c fiber.Ctx) error {
	filter := parseViewFilter(c)
	items, total, err := h.tasks.ListBacklog(c.Context(), filter)
	if err != nil {
		return httpapi.ErrInternal("list backlog").WithCause(err)
	}
	return c.JSON(viewResponse{Items: tasksToDTO(items, h.baseURL), Total: total})
}

func (h *TaskViewHandler) pinned(c fiber.Ctx) error {
	filter := parseViewFilter(c)
	items, total, err := h.tasks.ListPinned(c.Context(), filter)
	if err != nil {
		return httpapi.ErrInternal("list pinned").WithCause(err)
	}
	return c.JSON(viewResponse{Items: tasksToDTO(items, h.baseURL), Total: total})
}

type statsPlanResponse struct {
	Week    int `json:"week"`
	Backlog int `json:"backlog"`
}

func (h *TaskViewHandler) statsPlan(c fiber.Ctx) error {
	week, err := h.tasks.CountWeek(c.Context())
	if err != nil {
		return httpapi.ErrInternal("count week").WithCause(err)
	}
	backlog, err := h.tasks.CountBacklog(c.Context())
	if err != nil {
		return httpapi.ErrInternal("count backlog").WithCause(err)
	}
	return c.JSON(statsPlanResponse{Week: week, Backlog: backlog})
}

type sidebarInboxStats struct {
	Count                 int  `json:"count"`
	WarnThresholdExceeded bool `json:"warnThresholdExceeded"`
}

// sidebarStatsResponse bundles every aggregate the app shell shows in the
// sidebar: plan counters, the inbox badge and the pinned list. A client that
// just mutated its own data refetches this once (its own SSE echo is
// suppressed) instead of issuing the three separate /stats/plan + /inbox +
// /tasks/pinned requests the SSE handlers would otherwise trigger.
type sidebarStatsResponse struct {
	PlanStats  statsPlanResponse `json:"planStats"`
	InboxStats sidebarInboxStats `json:"inboxStats"`
	Pinned     viewResponse      `json:"pinned"`
}

func (h *TaskViewHandler) statsSidebar(c fiber.Ctx) error {
	ctx := c.Context()

	var (
		planStats  statsPlanResponse
		inboxStats sidebarInboxStats
		pinned     viewResponse
	)

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		week, err := h.tasks.CountWeek(gctx)
		if err != nil {
			return err
		}
		backlog, err := h.tasks.CountBacklog(gctx)
		if err != nil {
			return err
		}
		planStats = statsPlanResponse{Week: week, Backlog: backlog}
		return nil
	})

	g.Go(func() error {
		_, total, err := h.tasks.ListInbox(gctx, repo.TaskFilter{}, repo.Page{Limit: 0})
		if err != nil {
			return err
		}
		inboxStats = sidebarInboxStats{
			Count:                 total,
			WarnThresholdExceeded: total > h.cfg.Inbox.WarnThreshold,
		}
		return nil
	})

	g.Go(func() error {
		items, total, err := h.tasks.ListPinned(gctx, repo.TaskFilter{})
		if err != nil {
			return err
		}
		pinned = viewResponse{Items: tasksToDTO(items, h.baseURL), Total: total}
		return nil
	})

	if err := g.Wait(); err != nil {
		return httpapi.ErrInternal("load sidebar stats").WithCause(err)
	}

	return c.JSON(sidebarStatsResponse{
		PlanStats:  planStats,
		InboxStats: inboxStats,
		Pinned:     pinned,
	})
}

func tasksToDTO(tasks []model.Task, baseURL string) []dto.TaskDTO {
	result := make([]dto.TaskDTO, len(tasks))
	for i, t := range tasks {
		result[i] = dto.TaskFromModel(t, baseURL)
	}
	return result
}

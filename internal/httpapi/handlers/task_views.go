package handlers

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
	"golang.org/x/sync/errgroup"

	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
)

// TaskViewHandler serves the named view endpoints (today/tomorrow/overdue/week/backlog).
type TaskViewHandler struct {
	tasks   *repo.TaskRepo
	users   *repo.UserRepo
	troiki  *service.TroikiService
	cfg     *config.Config
	baseURL string
}

func NewTaskViewHandler(tasks *repo.TaskRepo, users *repo.UserRepo, troiki *service.TroikiService, cfg *config.Config, baseURL string) *TaskViewHandler {
	return &TaskViewHandler{tasks: tasks, users: users, troiki: troiki, cfg: cfg, baseURL: baseURL}
}

func (h *TaskViewHandler) Register(r fiber.Router) {
	r.Get("/tasks/today", httpapi.RequireScope(auth.ScopeTasksRead), h.today)
	r.Get("/tasks/tomorrow", httpapi.RequireScope(auth.ScopeTasksRead), h.tomorrow)
	r.Get("/tasks/overdue", httpapi.RequireScope(auth.ScopeTasksRead), h.overdue)
	r.Get("/tasks/week", httpapi.RequireScope(auth.ScopeTasksRead), h.week)
	r.Get("/tasks/backlog", httpapi.RequireScope(auth.ScopeTasksRead), h.backlog)
	r.Get("/tasks/pinned", httpapi.RequireScope(auth.ScopeTasksRead), h.pinned)
	r.Get("/tasks/completed", httpapi.RequireScope(auth.ScopeTasksRead), h.completed)
	r.Get("/stats/plan", httpapi.RequireScope(auth.ScopeTasksRead), h.statsPlan)
	r.Get("/stats/sidebar", httpapi.RequireScope(auth.ScopeTasksRead), h.statsSidebar)
	r.Get("/stats/week-summary", httpapi.RequireScope(auth.ScopeTasksRead), h.weekSummary)
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

// weekSummaryCompletedLimit caps the completed-task list the weekly summary
// returns. Breakdowns (by priority/project/context) are computed client-side
// from this list, so the cap is generous; a single week realistically stays
// well below it.
const weekSummaryCompletedLimit = 500

type weekSummaryRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type weekSummaryStats struct {
	// CompletedCount is the total tasks completed this week (incl. subtasks and
	// recurrence-completion snapshots — every row marked completed in range).
	CompletedCount int `json:"completedCount"`
	// PlannedOpen is the count of open tasks still on the week board.
	PlannedOpen int `json:"plannedOpen"`
	// Overdue is the count of open tasks past their due date.
	Overdue int `json:"overdue"`
}

// weekSummaryTroikiSlot is the per-category progress block: how full the slot is
// (Projects/Capacity), how many open tasks remain in its projects, and how many
// of its tasks were completed this week.
type weekSummaryTroikiSlot struct {
	Category  string `json:"category"`
	Capacity  int    `json:"capacity"`
	Projects  int    `json:"projects"`
	Open      int    `json:"open"`
	Completed int    `json:"completed"`
}

// weekSummaryTroiki is the Troiki-system progress section. It is present only
// when the user has the Troiki system enabled; the frontend renders it above the
// generic breakdowns because the methodology takes priority over plain projects.
type weekSummaryTroiki struct {
	Started bool                    `json:"started"`
	Slots   []weekSummaryTroikiSlot `json:"slots"`
}

// weekSummaryResponse powers the /week/summary review page: the current-week
// range, headline counters, the full completed-task list the frontend groups by
// priority/project/context, and (when enabled) the prioritised Troiki progress.
type weekSummaryResponse struct {
	Range     weekSummaryRange   `json:"range"`
	Stats     weekSummaryStats   `json:"stats"`
	Completed []dto.TaskDTO      `json:"completed"`
	Troiki    *weekSummaryTroiki `json:"troiki"`
}

func (h *TaskViewHandler) weekSummary(c fiber.Ctx) error {
	start, end := h.currentWeekRange()
	todayStart := h.todayStart()
	ctx := c.Context()

	var (
		completed      []model.Task
		completedTotal int
		plannedOpen    int
		overdue        int
		troikiEnabled  bool
		troikiView     service.TroikiView
	)

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		items, total, err := h.tasks.ListCompletedInRange(gctx, start, end, repo.TaskFilter{}, repo.Page{Limit: weekSummaryCompletedLimit})
		if err != nil {
			return err
		}
		completed = items
		completedTotal = total
		return nil
	})

	g.Go(func() error {
		n, err := h.tasks.CountWeek(gctx)
		if err != nil {
			return err
		}
		plannedOpen = n
		return nil
	})

	g.Go(func() error {
		n, err := h.tasks.CountOverdue(gctx, todayStart)
		if err != nil {
			return err
		}
		overdue = n
		return nil
	})

	g.Go(func() error {
		if h.users == nil {
			return nil
		}
		s, err := h.users.GetSettings(gctx, service.SingleUserID)
		if err != nil {
			return err
		}
		troikiEnabled = s.TroikiEnabled
		return nil
	})

	g.Go(func() error {
		if h.troiki == nil {
			return nil
		}
		v, err := h.troiki.View(gctx)
		if err != nil {
			return err
		}
		troikiView = v
		return nil
	})

	if err := g.Wait(); err != nil {
		return httpapi.ErrInternal("load week summary").WithCause(err)
	}

	var troiki *weekSummaryTroiki
	if troikiEnabled && h.troiki != nil {
		troiki = buildTroikiProgress(troikiView, completed)
	}

	return c.JSON(weekSummaryResponse{
		Range:     weekSummaryRange{Start: model.FormatUTC(start), End: model.FormatUTC(end)},
		Stats:     weekSummaryStats{CompletedCount: completedTotal, PlannedOpen: plannedOpen, Overdue: overdue},
		Completed: tasksToDTO(completed, h.baseURL),
		Troiki:    troiki,
	})
}

// buildTroikiProgress derives the per-category progress block from the Troiki
// view (open/capacity/projects) and the week's completed-task list (completed
// per category, by mapping each completed task's project to its slot). Slots are
// emitted in methodology order: important → medium → rest.
func buildTroikiProgress(v service.TroikiView, completed []model.Task) *weekSummaryTroiki {
	ordered := []struct {
		cat  model.TroikiCategory
		slot service.TroikiSlotProject
	}{
		{model.TroikiCategoryImportant, v.Important},
		{model.TroikiCategoryMedium, v.Medium},
		{model.TroikiCategoryRest, v.Rest},
	}

	slots := make([]weekSummaryTroikiSlot, len(ordered))
	catIndex := make(map[model.TroikiCategory]int, len(ordered))
	projectCat := make(map[int64]model.TroikiCategory)

	for i, o := range ordered {
		catIndex[o.cat] = i
		open := 0
		for _, p := range o.slot.Projects {
			projectCat[p.ID] = o.cat
			for _, t := range o.slot.Tasks[p.ID] {
				if t.Status == model.TaskStatusOpen {
					open++
				}
			}
		}
		slots[i] = weekSummaryTroikiSlot{
			Category: string(o.cat),
			Capacity: o.slot.Capacity,
			Projects: len(o.slot.Projects),
			Open:     open,
		}
	}

	for _, t := range completed {
		if t.ProjectID == nil {
			continue
		}
		if cat, ok := projectCat[*t.ProjectID]; ok {
			slots[catIndex[cat]].Completed++
		}
	}

	return &weekSummaryTroiki{Started: v.Started, Slots: slots}
}

func tasksToDTO(tasks []model.Task, baseURL string) []dto.TaskDTO {
	result := make([]dto.TaskDTO, len(tasks))
	for i, t := range tasks {
		result[i] = dto.TaskFromModel(t, baseURL)
	}
	return result
}

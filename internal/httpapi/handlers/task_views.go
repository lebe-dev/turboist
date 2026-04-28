package handlers

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
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
	r.Get("/tasks/today", h.today)
	r.Get("/tasks/tomorrow", h.tomorrow)
	r.Get("/tasks/overdue", h.overdue)
	r.Get("/tasks/week", h.week)
	r.Get("/tasks/backlog", h.backlog)
	r.Get("/tasks/completed", h.completed)
}

// todayStart returns the start of the current day in the configured timezone.
func (h *TaskViewHandler) todayStart() time.Time {
	now := time.Now().In(h.cfg.Location)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, h.cfg.Location).UTC()
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

func (h *TaskViewHandler) today(c fiber.Ctx) error {
	pp := dto.ParsePageParams(c.Query("limit"), c.Query("offset"))
	filter := parseViewFilter(c)
	items, total, err := h.tasks.ListToday(c.Context(), h.todayStart(), filter, repo.Page{Limit: pp.Limit, Offset: pp.Offset})
	if err != nil {
		return httpapi.ErrInternal("list today")
	}
	return c.JSON(dto.NewPagedResponse(tasksToDTO(items, h.baseURL), total, pp.Limit, pp.Offset))
}

func (h *TaskViewHandler) tomorrow(c fiber.Ctx) error {
	pp := dto.ParsePageParams(c.Query("limit"), c.Query("offset"))
	filter := parseViewFilter(c)
	items, total, err := h.tasks.ListTomorrow(c.Context(), h.todayStart(), filter, repo.Page{Limit: pp.Limit, Offset: pp.Offset})
	if err != nil {
		return httpapi.ErrInternal("list tomorrow")
	}
	return c.JSON(dto.NewPagedResponse(tasksToDTO(items, h.baseURL), total, pp.Limit, pp.Offset))
}

// completed returns tasks completed within the requested date window.
// Currently only date=today is supported; expand if other windows are needed.
func (h *TaskViewHandler) completed(c fiber.Ctx) error {
	pp := dto.ParsePageParams(c.Query("limit"), c.Query("offset"))
	filter := parseViewFilter(c)
	start := h.todayStart()
	end := start.Add(24 * time.Hour)
	items, total, err := h.tasks.ListCompletedInRange(c.Context(), start, end, filter, repo.Page{Limit: pp.Limit, Offset: pp.Offset})
	if err != nil {
		return httpapi.ErrInternal("list completed")
	}
	return c.JSON(dto.NewPagedResponse(tasksToDTO(items, h.baseURL), total, pp.Limit, pp.Offset))
}

func (h *TaskViewHandler) overdue(c fiber.Ctx) error {
	pp := dto.ParsePageParams(c.Query("limit"), c.Query("offset"))
	filter := parseViewFilter(c)
	items, total, err := h.tasks.ListOverdue(c.Context(), h.todayStart(), filter, repo.Page{Limit: pp.Limit, Offset: pp.Offset})
	if err != nil {
		return httpapi.ErrInternal("list overdue")
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
	items, total, err := h.tasks.ListWeek(c.Context(), filter)
	if err != nil {
		return httpapi.ErrInternal("list week")
	}
	return c.JSON(viewResponse{Items: tasksToDTO(items, h.baseURL), Total: total})
}

func (h *TaskViewHandler) backlog(c fiber.Ctx) error {
	filter := parseViewFilter(c)
	items, total, err := h.tasks.ListBacklog(c.Context(), filter)
	if err != nil {
		return httpapi.ErrInternal("list backlog")
	}
	return c.JSON(viewResponse{Items: tasksToDTO(items, h.baseURL), Total: total})
}

func tasksToDTO(tasks []model.Task, baseURL string) []dto.TaskDTO {
	result := make([]dto.TaskDTO, len(tasks))
	for i, t := range tasks {
		result[i] = dto.TaskFromModel(t, baseURL)
	}
	return result
}

package handler

import (
	ctxfilter "github.com/lebe-dev/turboist/internal/context"
	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/todoist"

	"github.com/gofiber/fiber/v3"
)

type TasksHandler struct {
	cache *todoist.Cache
	cfg   *config.AppConfig
}

func NewTasksHandler(cache *todoist.Cache, cfg *config.AppConfig) *TasksHandler {
	return &TasksHandler{cache: cache, cfg: cfg}
}

type tasksMeta struct {
	Context     string `json:"context"`
	WeeklyLimit int    `json:"weekly_limit"`
	WeeklyCount int    `json:"weekly_count"`
}

type tasksResponse struct {
	Tasks []*todoist.Task `json:"tasks"`
	Meta  tasksMeta       `json:"meta"`
}

// Tasks handles GET /api/tasks?context=...
func (h *TasksHandler) Tasks(c fiber.Ctx) error {
	contextKey := c.Query("context")
	tasks := h.filterByContext(contextKey)
	weeklyCount := countWithLabel(tasks, h.cfg.Weekly.Label)
	tree := buildTree(tasks)

	return c.JSON(tasksResponse{
		Tasks: tree,
		Meta: tasksMeta{
			Context:     contextKey,
			WeeklyLimit: h.cfg.Weekly.MaxTasks,
			WeeklyCount: weeklyCount,
		},
	})
}

// Weekly handles GET /api/tasks/weekly?context=...
func (h *TasksHandler) Weekly(c fiber.Ctx) error {
	contextKey := c.Query("context")
	tasks := h.filterByContext(contextKey)
	weekly := filterByLabel(tasks, h.cfg.Weekly.Label)
	tree := buildTree(weekly)

	return c.JSON(tasksResponse{
		Tasks: tree,
		Meta: tasksMeta{
			Context:     contextKey,
			WeeklyLimit: h.cfg.Weekly.MaxTasks,
			WeeklyCount: len(weekly),
		},
	})
}

// NextWeek handles GET /api/tasks/next-week?context=...
func (h *TasksHandler) NextWeek(c fiber.Ctx) error {
	contextKey := c.Query("context")
	tasks := h.filterByContext(contextKey)
	nextWeek := filterByLabel(tasks, h.cfg.NextWeek.Label)
	weeklyCount := countWithLabel(tasks, h.cfg.Weekly.Label)
	tree := buildTree(nextWeek)

	return c.JSON(tasksResponse{
		Tasks: tree,
		Meta: tasksMeta{
			Context:     contextKey,
			WeeklyLimit: h.cfg.Weekly.MaxTasks,
			WeeklyCount: weeklyCount,
		},
	})
}

// Complete handles POST /api/tasks/:id/complete
func (h *TasksHandler) Complete(c fiber.Ctx) error {
	id := c.Params("id")
	if err := h.cache.CompleteTask(c.Context(), id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusOK)
}

func (h *TasksHandler) filterByContext(contextKey string) []*todoist.Task {
	tasks := h.cache.Tasks()
	if contextKey == "" {
		return tasks
	}
	ctx, ok := h.cfg.Contexts[contextKey]
	if !ok {
		return tasks
	}
	return ctxfilter.FilterTasks(tasks, ctx.Filters, h.cache.Projects(), h.cache.Sections())
}

// buildTree builds a parent/child tree from a flat task list.
// Tasks are cloned to avoid mutating shared cache state.
// Children whose parent is not in the set are treated as roots.
func buildTree(tasks []*todoist.Task) []*todoist.Task {
	byID := make(map[string]*todoist.Task, len(tasks))
	clones := make([]*todoist.Task, len(tasks))
	for i, t := range tasks {
		c := *t
		c.Children = make([]*todoist.Task, 0)
		clones[i] = &c
		byID[t.ID] = &c
	}

	roots := make([]*todoist.Task, 0)
	for _, t := range clones {
		if t.ParentID == nil {
			roots = append(roots, t)
			continue
		}
		if parent, ok := byID[*t.ParentID]; ok {
			parent.Children = append(parent.Children, t)
		} else {
			roots = append(roots, t)
		}
	}

	for _, t := range roots {
		populateSubtaskCounts(t)
	}

	return roots
}

func populateSubtaskCounts(t *todoist.Task) {
	t.SubTaskCount = len(t.Children)
	for _, child := range t.Children {
		populateSubtaskCounts(child)
	}
}

func filterByLabel(tasks []*todoist.Task, label string) []*todoist.Task {
	if label == "" {
		return tasks
	}
	result := make([]*todoist.Task, 0)
	for _, t := range tasks {
		for _, l := range t.Labels {
			if l == label {
				result = append(result, t)
				break
			}
		}
	}
	return result
}

func countWithLabel(tasks []*todoist.Task, label string) int {
	if label == "" {
		return 0
	}
	count := 0
	for _, t := range tasks {
		for _, l := range t.Labels {
			if l == label {
				count++
				break
			}
		}
	}
	return count
}

package handler

import (
	synctodoist "github.com/CnTeng/todoist-api-go/sync"
	"github.com/lebe-dev/turboist/internal/config"
	ctxfilter "github.com/lebe-dev/turboist/internal/context"
	"github.com/lebe-dev/turboist/internal/todoist"

	"github.com/charmbracelet/log"
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

type createTaskRequest struct {
	Content     string   `json:"content"`
	Description string   `json:"description"`
	Labels      []string `json:"labels"`
	Priority    int      `json:"priority"`
}

// Create handles POST /api/tasks?context=...
func (h *TasksHandler) Create(c fiber.Ctx) error {
	var req createTaskRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.Content == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "content is required"})
	}

	args := &synctodoist.TaskAddArgs{
		Content: req.Content,
	}

	if req.Description != "" {
		args.Description = &req.Description
	}
	if req.Priority >= 1 && req.Priority <= 4 {
		args.Priority = &req.Priority
	}

	labels := make([]string, 0)
	if len(req.Labels) > 0 {
		labels = append(labels, req.Labels...)
	}

	// Resolve context defaults: project, section, labels
	contextKey := c.Query("context")
	if contextKey != "" {
		ctx := h.cfg.FindContext(contextKey)
		if ctx != nil {
			// Set project from context filter (first match)
			if len(ctx.Filters.Projects) > 0 {
				projectID := resolveProjectID(ctx.Filters.Projects[0], h.cache.Projects())
				if projectID != "" {
					args.ProjectID = &projectID
				}
			}
			// Set section from context filter (first match)
			if len(ctx.Filters.Sections) > 0 {
				sectionID := resolveSectionID(ctx.Filters.Sections[0], h.cache.Sections())
				if sectionID != "" {
					args.SectionID = &sectionID
				}
			}
			// Merge context labels (avoid duplicates)
			existing := make(map[string]struct{}, len(labels))
			for _, l := range labels {
				existing[l] = struct{}{}
			}
			for _, l := range ctx.Filters.Labels {
				if _, ok := existing[l]; !ok {
					labels = append(labels, l)
				}
			}
		}
	}

	if len(labels) > 0 {
		args.Labels = labels
	}

	log.Debug("create task", "content", req.Content, "context", contextKey, "labels", labels)
	if err := h.cache.AddTask(c.Context(), args); err != nil {
		log.Error("create task failed", "err", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"ok": true})
}

func resolveProjectID(name string, projects []*todoist.Project) string {
	for _, p := range projects {
		if p.Name == name {
			return p.ID
		}
	}
	return ""
}

func resolveSectionID(name string, sections []*todoist.Section) string {
	for _, s := range sections {
		if s.Name == name {
			return s.ID
		}
	}
	return ""
}

// Complete handles POST /api/tasks/:id/complete
func (h *TasksHandler) Complete(c fiber.Ctx) error {
	id := c.Params("id")
	log.Debug("complete task", "id", id)
	if err := h.cache.CompleteTask(c.Context(), id); err != nil {
		log.Error("complete task failed", "id", id, "err", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusOK)
}

func (h *TasksHandler) filterByContext(contextKey string) []*todoist.Task {
	tasks := h.cache.Tasks()
	if contextKey == "" {
		return tasks
	}
	ctx := h.cfg.FindContext(contextKey)
	if ctx == nil {
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

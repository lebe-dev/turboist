package handler

import (
	"cmp"
	"slices"
	"strings"
	"time"

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
	sortTasks(tree, h.cfg.TaskSort)

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
	sortTasks(tree, h.cfg.TaskSort)

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
	sortTasks(tree, h.cfg.TaskSort)

	return c.JSON(tasksResponse{
		Tasks: tree,
		Meta: tasksMeta{
			Context:     contextKey,
			WeeklyLimit: h.cfg.Weekly.MaxTasks,
			WeeklyCount: weeklyCount,
		},
	})
}

// Today handles GET /api/tasks/today?context=...
func (h *TasksHandler) Today(c fiber.Ctx) error {
	contextKey := c.Query("context")
	tasks := h.filterByContext(contextKey)
	today := filterByDueDate(tasks, time.Now(), h.cfg.Today.IncludeOverdue)
	weeklyCount := countWithLabel(tasks, h.cfg.Weekly.Label)
	tree := buildTree(today)
	sortTasks(tree, h.cfg.TaskSort)

	return c.JSON(tasksResponse{
		Tasks: tree,
		Meta: tasksMeta{
			Context:     contextKey,
			WeeklyLimit: h.cfg.Weekly.MaxTasks,
			WeeklyCount: weeklyCount,
		},
	})
}

// Tomorrow handles GET /api/tasks/tomorrow?context=...
func (h *TasksHandler) Tomorrow(c fiber.Ctx) error {
	contextKey := c.Query("context")
	tasks := h.filterByContext(contextKey)
	tomorrow := filterByDueDate(tasks, time.Now().AddDate(0, 0, 1), false)
	weeklyCount := countWithLabel(tasks, h.cfg.Weekly.Label)
	tree := buildTree(tomorrow)
	sortTasks(tree, h.cfg.TaskSort)

	return c.JSON(tasksResponse{
		Tasks: tree,
		Meta: tasksMeta{
			Context:     contextKey,
			WeeklyLimit: h.cfg.Weekly.MaxTasks,
			WeeklyCount: weeklyCount,
		},
	})
}

// Inbox handles GET /api/tasks/inbox?context=...
func (h *TasksHandler) Inbox(c fiber.Ctx) error {
	contextKey := c.Query("context")
	tasks := h.filterByContext(contextKey)

	inboxProjectID := h.cache.InboxProjectID()
	inbox := make([]*todoist.Task, 0)
	for _, t := range tasks {
		if t.ProjectID == inboxProjectID {
			inbox = append(inbox, t)
		}
	}

	weeklyCount := countWithLabel(tasks, h.cfg.Weekly.Label)
	tree := buildTree(inbox)
	sortTasksByAddedAt(tree)

	return c.JSON(tasksResponse{
		Tasks: tree,
		Meta: tasksMeta{
			Context:     contextKey,
			WeeklyLimit: h.cfg.Weekly.MaxTasks,
			WeeklyCount: weeklyCount,
		},
	})
}

// Completed handles GET /api/tasks/completed
func (h *TasksHandler) Completed(c fiber.Ctx) error {
	now := time.Now()
	since := now.AddDate(0, 0, -h.cfg.Completed.Days)

	tasks, err := h.cache.Client().FetchCompletedTasks(c.Context(), since, now)
	if err != nil {
		log.Error("fetch completed tasks failed", "err", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// Sort by completed_at descending (newest first)
	slices.SortStableFunc(tasks, func(a, b *todoist.Task) int {
		ca, cb := "", ""
		if a.CompletedAt != nil {
			ca = *a.CompletedAt
		}
		if b.CompletedAt != nil {
			cb = *b.CompletedAt
		}
		return cmp.Compare(cb, ca)
	})

	return c.JSON(tasksResponse{
		Tasks: tasks,
		Meta:  tasksMeta{},
	})
}

type createTaskRequest struct {
	Content     string   `json:"content"`
	Description string   `json:"description"`
	Labels      []string `json:"labels"`
	Priority    int      `json:"priority"`
	ParentID    string   `json:"parent_id"`
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

	if req.ParentID != "" {
		args.ParentID = &req.ParentID
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

// GetByID handles GET /api/tasks/:id
func (h *TasksHandler) GetByID(c fiber.Ctx) error {
	id := c.Params("id")
	all := h.cache.Tasks()
	tree := buildTree(all)
	sortTasks(tree, h.cfg.TaskSort)
	if t := findInTree(tree, id); t != nil {
		return c.JSON(t)
	}
	return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "task not found"})
}

func findInTree(tasks []*todoist.Task, id string) *todoist.Task {
	for _, t := range tasks {
		if t.ID == id {
			return t
		}
		if found := findInTree(t.Children, id); found != nil {
			return found
		}
	}
	return nil
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

type updateTaskRequest struct {
	Content     *string  `json:"content"`
	Description *string  `json:"description"`
	Labels      []string `json:"labels"`
	Priority    *int     `json:"priority"`
	DueDate     *string  `json:"due_date"`
}

// Update handles PATCH /api/tasks/:id
func (h *TasksHandler) Update(c fiber.Ctx) error {
	id := c.Params("id")
	var req updateTaskRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	args := &synctodoist.TaskUpdateArgs{ID: id}

	if req.Content != nil {
		args.Content = req.Content
	}
	if req.Description != nil {
		args.Description = req.Description
	}
	if req.Priority != nil {
		args.Priority = req.Priority
	}
	if req.Labels != nil {
		args.Labels = req.Labels
	}
	if req.DueDate != nil {
		if *req.DueDate == "" {
			// Clear the due date
			noDate := "no date"
			args.Due = &synctodoist.Due{String: &noDate}
		} else {
			dueDate, err := time.Parse("2006-01-02", *req.DueDate)
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid due_date format, expected YYYY-MM-DD"})
			}
			args.Due = &synctodoist.Due{Date: &dueDate}
		}
	}

	log.Debug("update task", "id", id)
	if err := h.cache.UpdateTask(c.Context(), args); err != nil {
		log.Error("update task failed", "id", id, "err", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"ok": true})
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

// sortTasks sorts tasks in place according to the configured sort mode.
// Also recursively sorts children.
func sortTasks(tasks []*todoist.Task, mode config.TaskSort) {
	slices.SortStableFunc(tasks, func(a, b *todoist.Task) int {
		switch mode {
		case config.TaskSortDueDate:
			return compareDueDate(a, b)
		case config.TaskSortContent:
			return cmp.Compare(strings.ToLower(a.Content), strings.ToLower(b.Content))
		default: // priority
			// Todoist priority: 4 = highest, 1 = lowest; sort descending
			if c := cmp.Compare(b.Priority, a.Priority); c != 0 {
				return c
			}
			return compareDueDate(a, b)
		}
	})
	for _, t := range tasks {
		if len(t.Children) > 1 {
			sortTasks(t.Children, mode)
		}
	}
}

// sortTasksByAddedAt sorts tasks by creation date descending (newest first).
// Also recursively sorts children.
func sortTasksByAddedAt(tasks []*todoist.Task) {
	slices.SortStableFunc(tasks, func(a, b *todoist.Task) int {
		return cmp.Compare(b.AddedAt, a.AddedAt)
	})
	for _, t := range tasks {
		if len(t.Children) > 1 {
			sortTasksByAddedAt(t.Children)
		}
	}
}

// compareDueDate compares two tasks by due date. Tasks without due date go last.
func compareDueDate(a, b *todoist.Task) int {
	switch {
	case a.Due == nil && b.Due == nil:
		return 0
	case a.Due == nil:
		return 1
	case b.Due == nil:
		return -1
	default:
		return cmp.Compare(a.Due.Date, b.Due.Date)
	}
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

// filterByDueDate returns tasks due on the given date.
// If includeOverdue is true, tasks with due dates before the target date are also included.
func filterByDueDate(tasks []*todoist.Task, target time.Time, includeOverdue bool) []*todoist.Task {
	targetDate := target.Format("2006-01-02")
	result := make([]*todoist.Task, 0)
	for _, t := range tasks {
		if t.Due == nil {
			continue
		}
		if t.Due.Date == targetDate {
			result = append(result, t)
			continue
		}
		if includeOverdue && t.Due.Date < targetDate {
			result = append(result, t)
		}
	}
	return result
}

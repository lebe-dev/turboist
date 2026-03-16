package handler

import (
	"cmp"
	"slices"
	"time"

	synctodoist "github.com/CnTeng/todoist-api-go/sync"
	"github.com/lebe-dev/turboist/internal/config"
	ctxfilter "github.com/lebe-dev/turboist/internal/context"
	"github.com/lebe-dev/turboist/internal/taskview"
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

type tasksMeta = taskview.TasksMeta

type tasksResponse struct {
	Tasks []*todoist.Task `json:"tasks"`
	Meta  tasksMeta       `json:"meta"`
}

func resultToResponse(r taskview.TasksResult) tasksResponse {
	return tasksResponse{Tasks: r.Tasks, Meta: r.Meta}
}

// Tasks handles GET /api/tasks?context=...
func (h *TasksHandler) Tasks(c fiber.Ctx) error {
	r := taskview.ComputeTasks(h.cache, h.cfg, taskview.ViewParams{
		View: "all", Context: c.Query("context"),
	})
	return c.JSON(resultToResponse(r))
}

// Weekly handles GET /api/tasks/weekly?context=...
func (h *TasksHandler) Weekly(c fiber.Ctx) error {
	r := taskview.ComputeTasks(h.cache, h.cfg, taskview.ViewParams{
		View: "weekly", Context: c.Query("context"),
	})
	return c.JSON(resultToResponse(r))
}

// NextWeek handles GET /api/tasks/next-week?context=...
// Returns tasks with the backlog label, sorted per backlog config.
func (h *TasksHandler) NextWeek(c fiber.Ctx) error {
	contextKey := c.Query("context")
	r := taskview.ComputeTasks(h.cache, h.cfg, taskview.ViewParams{
		View: "backlog", Context: contextKey,
	})
	// NextWeek adds backlog limit/count to meta
	r.Meta.BacklogLimit = h.cfg.Backlog.MaxLimit
	tasks := h.filterByContext(contextKey)
	r.Meta.BacklogCount = len(taskview.FilterByLabel(tasks, h.cfg.Backlog.Label))
	return c.JSON(resultToResponse(r))
}

// Today handles GET /api/tasks/today?context=...
func (h *TasksHandler) Today(c fiber.Ctx) error {
	r := taskview.ComputeTasks(h.cache, h.cfg, taskview.ViewParams{
		View: "today", Context: c.Query("context"),
	})
	return c.JSON(resultToResponse(r))
}

// Tomorrow handles GET /api/tasks/tomorrow?context=...
func (h *TasksHandler) Tomorrow(c fiber.Ctx) error {
	r := taskview.ComputeTasks(h.cache, h.cfg, taskview.ViewParams{
		View: "tomorrow", Context: c.Query("context"),
	})
	return c.JSON(resultToResponse(r))
}

// Inbox handles GET /api/tasks/inbox?context=...
func (h *TasksHandler) Inbox(c fiber.Ctx) error {
	r := taskview.ComputeTasks(h.cache, h.cfg, taskview.ViewParams{
		View: "inbox", Context: c.Query("context"),
	})
	return c.JSON(resultToResponse(r))
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
	DueDate     string   `json:"due_date"`
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
			// Merge context labels (avoid duplicates) if inherit_labels is enabled
			if ctx.ShouldInheritLabels() {
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
	}

	if len(labels) > 0 {
		args.Labels = labels
	}

	if req.ParentID != "" {
		args.ParentID = &req.ParentID
	}

	if req.DueDate != "" {
		dueDate, err := time.Parse("2006-01-02", req.DueDate)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid due_date format, expected YYYY-MM-DD"})
		}
		args.Due = &synctodoist.Due{Date: &dueDate}
	}

	log.Debug("create task", "content", req.Content, "context", contextKey, "labels", labels)
	if _, err := h.cache.AddTask(c.Context(), args); err != nil {
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
	tree := taskview.BuildTree(all)
	taskview.SortTasks(tree, h.cfg.TaskSort)
	if t := taskview.FindInTree(tree, id); t != nil {
		return c.JSON(t)
	}
	return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "task not found"})
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

// Delete handles DELETE /api/tasks/:id
func (h *TasksHandler) Delete(c fiber.Ctx) error {
	id := c.Params("id")
	log.Debug("delete task", "id", id)
	if err := h.cache.DeleteTask(c.Context(), id); err != nil {
		log.Error("delete task failed", "id", id, "err", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusOK)
}

// Duplicate handles POST /api/tasks/:id/duplicate
func (h *TasksHandler) Duplicate(c fiber.Ctx) error {
	id := c.Params("id")
	all := h.cache.Tasks()

	var src *todoist.Task
	for _, t := range all {
		if t.ID == id {
			src = t
			break
		}
	}
	if src == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "task not found"})
	}

	args := &synctodoist.TaskAddArgs{
		Content:   src.Content,
		ProjectID: &src.ProjectID,
		SectionID: src.SectionID,
		ParentID:  src.ParentID,
	}
	if src.Description != "" {
		args.Description = &src.Description
	}
	if src.Priority >= 1 && src.Priority <= 4 {
		args.Priority = &src.Priority
	}
	if len(src.Labels) > 0 {
		args.Labels = src.Labels
	}
	if src.Due != nil {
		dueDate, err := time.Parse("2006-01-02", src.Due.Date)
		if err == nil {
			args.Due = &synctodoist.Due{Date: &dueDate}
		}
	}

	log.Debug("duplicate task", "id", id, "content", src.Content)
	newID, err := h.cache.AddTask(c.Context(), args)
	if err != nil {
		log.Error("duplicate task failed", "id", id, "err", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"ok": true, "task_id": newID})
}

// CompletedSubtasks handles GET /api/tasks/:id/completed-subtasks
func (h *TasksHandler) CompletedSubtasks(c fiber.Ctx) error {
	id := c.Params("id")
	tasks, err := h.cache.Client().FetchCompletedSubtasks(c.Context(), id)
	if err != nil {
		log.Error("fetch completed subtasks failed", "id", id, "err", err)
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

	return c.JSON(fiber.Map{"tasks": tasks})
}

// ResetWeekly handles POST /api/tasks/reset-weekly
// Removes the weekly label from all tasks that have it.
func (h *TasksHandler) ResetWeekly(c fiber.Ctx) error {
	label := h.cfg.Weekly.Label
	if label == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "weekly label not configured"})
	}

	tasks := taskview.FilterByLabel(h.cache.Tasks(), label)
	if len(tasks) == 0 {
		return c.JSON(fiber.Map{"ok": true, "updated": 0})
	}

	updates := make(map[string][]string, len(tasks))
	for _, t := range tasks {
		newLabels := make([]string, 0, len(t.Labels))
		for _, l := range t.Labels {
			if l != label {
				newLabels = append(newLabels, l)
			}
		}
		updates[t.ID] = newLabels
	}

	if err := h.cache.Client().SetTasksLabels(c.Context(), updates); err != nil {
		log.Error("reset weekly: batch update failed", "err", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	if err := h.cache.RefreshAfterMutation(c.Context()); err != nil {
		log.Error("reset weekly: cache refresh failed", "err", err)
	}

	log.Info("reset weekly labels", "updated", len(tasks))
	return c.JSON(fiber.Map{"ok": true, "updated": len(tasks)})
}

// Backlog handles GET /api/tasks/backlog?context=...
func (h *TasksHandler) Backlog(c fiber.Ctx) error {
	r := taskview.ComputeTasks(h.cache, h.cfg, taskview.ViewParams{
		View: "backlog", Context: c.Query("context"),
	})
	return c.JSON(resultToResponse(r))
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

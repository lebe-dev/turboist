package handler

import (
	"cmp"
	"slices"
	"strings"
	"time"

	synctodoist "github.com/CnTeng/todoist-api-go/sync"
	"github.com/lebe-dev/turboist/internal/config"
	ctxfilter "github.com/lebe-dev/turboist/internal/context"
	"github.com/lebe-dev/turboist/internal/storage"
	"github.com/lebe-dev/turboist/internal/taskview"
	"github.com/lebe-dev/turboist/internal/todoist"

	"github.com/charmbracelet/log"
	"github.com/gofiber/fiber/v3"
)

type TasksHandler struct {
	cache *todoist.Cache
	cfg   *config.AppConfig
	store *storage.Store
}

func NewTasksHandler(cache *todoist.Cache, cfg *config.AppConfig, store *storage.Store) *TasksHandler {
	return &TasksHandler{cache: cache, cfg: cfg, store: store}
}

// todoistErrorResponse maps Todoist API errors to appropriate HTTP status codes.
// Returns 429 for rate-limited responses, 500 otherwise.
func todoistErrorResponse(c fiber.Ctx, err error) error {
	status := fiber.StatusInternalServerError
	if todoist.IsRateLimited(err) {
		status = fiber.StatusTooManyRequests
	}
	return c.Status(status).JSON(fiber.Map{"error": err.Error()})
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
		return todoistErrorResponse(c, err)
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

	// Apply auto-labels based on task title
	labels = applyAutoLabels(req.Content, labels, h.cfg.CompiledAutoLabels)

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
	newID, err := h.cache.AddTask(c.Context(), args)
	if err != nil {
		log.Error("create task failed", "err", err)
		return todoistErrorResponse(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"ok": true, "id": newID})
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
		return todoistErrorResponse(c, err)
	}
	return c.SendStatus(fiber.StatusOK)
}

type updateTaskRequest struct {
	Content     *string  `json:"content"`
	Description *string  `json:"description"`
	Labels      []string `json:"labels"`
	Priority    *int     `json:"priority"`
	DueDate     *string  `json:"due_date"`
	DueString   *string  `json:"due_string"`
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
	// Use SetTasksLabels for label updates to work around omitempty on
	// TaskUpdateArgs.Labels which silently drops empty slices.
	var labelsViaSync bool
	if req.Labels != nil {
		if len(req.Labels) == 0 {
			labelsViaSync = true
		} else {
			args.Labels = req.Labels
		}
	}
	if req.DueString != nil {
		// due_string takes precedence — pass the human-readable string (e.g. "every day")
		// directly to the Todoist sync API which will parse it.
		// Also preserve the existing due date so Todoist doesn't skip to the next
		// occurrence (e.g. setting "every month on the 1st" on April 1st should
		// keep April 1st, not jump to May 1st).
		lang := "en"
		due := &synctodoist.Due{String: req.DueString, Lang: &lang}
		if existing := h.findTask(id); existing != nil && existing.Due != nil {
			date, err := time.Parse("2006-01-02", existing.Due.Date)
			if err == nil {
				due.Date = &date
			}
		}
		args.Due = due
	} else if req.DueDate != nil {
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

	// Detect postpone: task had a due date and it's being changed to a different date.
	if req.DueDate != nil && *req.DueDate != "" {
		if existing := h.findTask(id); existing != nil && existing.Due != nil && existing.Due.Date != *req.DueDate {
			if err := h.store.IncrementPostponeCount(id); err != nil {
				log.Error("increment postpone count failed", "id", id, "err", err)
			}
		}
	}

	log.Debug("update task", "id", id)

	// When clearing labels, send via raw sync command (SetTasksLabels)
	// because the library's omitempty tag drops empty slices.
	if labelsViaSync {
		if err := h.cache.Client().SetTasksLabels(c.Context(), map[string][]string{id: req.Labels}); err != nil {
			log.Error("update task labels failed", "id", id, "err", err)
			return todoistErrorResponse(c, err)
		}
	}

	// Send remaining field updates (if any) via the standard UpdateTask path.
	hasOtherFields := req.Content != nil || req.Description != nil || req.Priority != nil || req.DueDate != nil || req.DueString != nil || (!labelsViaSync && req.Labels != nil)
	if hasOtherFields {
		if err := h.cache.UpdateTask(c.Context(), args); err != nil {
			log.Error("update task failed", "id", id, "err", err)
			return todoistErrorResponse(c, err)
		}
	} else if labelsViaSync {
		// Only labels were updated via sync — still refresh the cache.
		h.cache.RefreshAfterMutation()
	}

	return c.JSON(fiber.Map{"ok": true})
}

type moveTaskRequest struct {
	ParentID string `json:"parent_id"`
}

// Move handles POST /api/tasks/:id/move — moves a task to be a subtask of another task.
func (h *TasksHandler) Move(c fiber.Ctx) error {
	id := c.Params("id")
	var req moveTaskRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.ParentID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "parent_id is required"})
	}

	log.Debug("move task", "id", id, "parent_id", req.ParentID)
	if err := h.cache.MoveTask(c.Context(), id, req.ParentID); err != nil {
		log.Error("move task failed", "id", id, "parent_id", req.ParentID, "err", err)
		return todoistErrorResponse(c, err)
	}

	return c.JSON(fiber.Map{"ok": true})
}

// Delete handles DELETE /api/tasks/:id
func (h *TasksHandler) Delete(c fiber.Ctx) error {
	id := c.Params("id")
	log.Debug("delete task", "id", id)
	if err := h.cache.DeleteTask(c.Context(), id); err != nil {
		log.Error("delete task failed", "id", id, "err", err)
		return todoistErrorResponse(c, err)
	}
	return c.SendStatus(fiber.StatusOK)
}

type decomposeTaskRequest struct {
	Tasks []string `json:"tasks"`
}

// Decompose handles POST /api/tasks/:id/decompose
// Creates new tasks inheriting properties from the source task, then deletes the original.
func (h *TasksHandler) Decompose(c fiber.Ctx) error {
	id := c.Params("id")
	var req decomposeTaskRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if len(req.Tasks) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "tasks list is required"})
	}
	if slices.Contains(req.Tasks, "") {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "task content must not be empty"})
	}

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

	log.Debug("decompose task", "id", id, "content", src.Content, "new_tasks", len(req.Tasks))
	if err := h.cache.DecomposeTask(c.Context(), src, req.Tasks); err != nil {
		log.Error("decompose task failed", "id", id, "err", err)
		return todoistErrorResponse(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"ok": true})
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
		return todoistErrorResponse(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"ok": true, "task_id": newID})
}

// CompletedSubtasks handles GET /api/tasks/:id/completed-subtasks
func (h *TasksHandler) CompletedSubtasks(c fiber.Ctx) error {
	id := c.Params("id")
	tasks, err := h.cache.Client().FetchCompletedSubtasks(c.Context(), id)
	if err != nil {
		log.Error("fetch completed subtasks failed", "id", id, "err", err)
		return todoistErrorResponse(c, err)
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
		return todoistErrorResponse(c, err)
	}

	h.cache.RefreshAfterMutation()

	log.Info("reset weekly labels", "updated", len(tasks))
	return c.JSON(fiber.Map{"ok": true, "updated": len(tasks)})
}

type batchUpdateLabelsRequest struct {
	Updates map[string][]string `json:"updates"` // taskID → new labels
}

// BatchUpdateLabels handles POST /api/tasks/batch-update-labels
// Updates labels for multiple tasks in a single Todoist sync call.
func (h *TasksHandler) BatchUpdateLabels(c fiber.Ctx) error {
	var req batchUpdateLabelsRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if len(req.Updates) == 0 {
		return c.JSON(fiber.Map{"ok": true, "updated": 0})
	}

	if err := h.cache.Client().SetTasksLabels(c.Context(), req.Updates); err != nil {
		log.Error("batch update labels failed", "err", err)
		return todoistErrorResponse(c, err)
	}

	h.cache.RefreshAfterMutation()

	log.Info("batch update labels", "updated", len(req.Updates))
	return c.JSON(fiber.Map{"ok": true, "updated": len(req.Updates)})
}

// Backlog handles GET /api/tasks/backlog?context=...
func (h *TasksHandler) Backlog(c fiber.Ctx) error {
	r := taskview.ComputeTasks(h.cache, h.cfg, taskview.ViewParams{
		View: "backlog", Context: c.Query("context"),
	})
	return c.JSON(resultToResponse(r))
}

// applyAutoLabels checks whether content contains each mask and appends matching
// labels (deduplicating against already-present labels).
func applyAutoLabels(content string, labels []string, autoLabels []config.CompiledAutoLabel) []string {
	existing := make(map[string]struct{}, len(labels))
	for _, l := range labels {
		existing[l] = struct{}{}
	}
	for _, at := range autoLabels {
		haystack := content
		if at.IgnoreCase {
			haystack = strings.ToLower(content)
		}
		if strings.Contains(haystack, at.Mask) {
			if _, ok := existing[at.Label]; !ok {
				labels = append(labels, at.Label)
				existing[at.Label] = struct{}{}
			}
		}
	}
	return labels
}

func (h *TasksHandler) findTask(id string) *todoist.Task {
	for _, t := range h.cache.Tasks() {
		if t.ID == id {
			return t
		}
	}
	return nil
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

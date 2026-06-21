package handlers

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
	rrule "github.com/teambition/rrule-go"
)

var reTrailingCounter = regexp.MustCompile(`^(.*) \((\d+)\)$`)

func duplicateTitle(title string) string {
	if m := reTrailingCounter.FindStringSubmatch(title); m != nil {
		n, _ := strconv.Atoi(m[2])
		return m[1] + " (" + strconv.Itoa(n+1) + ")"
	}
	return title + " (2)"
}

// TaskHandler implements GET/PATCH/DELETE /tasks/:id and POST /tasks/:id/subtasks.
type TaskHandler struct {
	tasks    *repo.TaskRepo
	projects *repo.ProjectRepo
	taskSvc  *service.TaskService
	baseURL  string
}

// NewTaskHandler constructs a TaskHandler.
func NewTaskHandler(tasks *repo.TaskRepo, projects *repo.ProjectRepo, taskSvc *service.TaskService, baseURL string) *TaskHandler {
	return &TaskHandler{tasks: tasks, projects: projects, taskSvc: taskSvc, baseURL: baseURL}
}

// Register wires task routes onto r (the /api/v1 group).
func (h *TaskHandler) Register(r fiber.Router) {
	r.Get("/tasks/:id", httpapi.RequireScope("tasks:read"), h.get)
	r.Patch("/tasks/:id", httpapi.RequireScope("tasks:write"), h.patch)
	r.Delete("/tasks/:id", httpapi.RequireScope("tasks:write"), h.delete)
	r.Get("/tasks/:id/subtasks", httpapi.RequireScope("tasks:read"), h.listSubtasks)
	r.Get("/tasks/:id/template-draft", httpapi.RequireScope("tasks:read"), h.templateDraft)
	r.Post("/tasks/:id/subtasks", httpapi.RequireScope("tasks:write"), h.createSubtask)
	r.Post("/tasks/:id/duplicate", httpapi.RequireScope("tasks:write"), h.duplicate)
	r.Post("/tasks/:id/decompose", httpapi.RequireScope("tasks:write"), h.decompose)
}

func (h *TaskHandler) get(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	includeSubtasks := c.Query("subtasks") == "true"
	logEntry(c, "handler.Task.Get", slog.Int64("task_id", id), slog.Bool("subtasks", includeSubtasks))
	t, err := h.tasks.Get(c.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("task not found")
		}
		return httpapi.ErrInternal("get task").WithCause(err)
	}
	out := dto.TaskFromModel(*t, h.baseURL)
	if includeSubtasks {
		items, err := h.tasks.ListSubtasks(c.Context(), id)
		if err != nil {
			return httpapi.ErrInternal("list subtasks").WithCause(err)
		}
		dtos := make([]dto.TaskDTO, len(items))
		for i, st := range items {
			dtos[i] = dto.TaskFromModel(st, h.baseURL)
		}
		page := dto.NewPagedResponse(dtos, len(dtos), len(dtos), 0)
		out.Subtasks = &page
	}
	return c.JSON(out)
}

func (h *TaskHandler) patch(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.Task.Patch", slog.Int64("task_id", id))
	t, err := h.tasks.Get(c.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("task not found")
		}
		return httpapi.ErrInternal("get task").WithCause(err)
	}
	var req dto.PatchTaskRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, "handler.Task.Patch", "invalid request body")
		return httpapi.ErrValidation("invalid request body")
	}

	u := repo.TaskUpdate{}
	if req.Title != nil {
		if strings.TrimSpace(*req.Title) == "" {
			logValidation(c, "handler.Task.Patch", "empty title")
			return httpapi.ErrValidation("title must not be empty")
		}
		u.Title = req.Title
	}
	if req.Description != nil {
		u.Description = req.Description
	}
	if req.Priority != nil {
		p := model.Priority(*req.Priority)
		if !p.IsValid() {
			logValidation(c, "handler.Task.Patch", "invalid priority")
			return httpapi.ErrValidation("invalid priority")
		}
		// Tasks in a Troiki-bound project have priority pinned by the project's
		// category — reject direct priority edits so a stale client or CLI can't
		// desync the two.
		if t.ProjectID != nil {
			proj, err := h.projects.Get(c.Context(), *t.ProjectID)
			if err != nil && !errors.Is(err, repo.ErrNotFound) {
				return httpapi.ErrInternal("get project").WithCause(err)
			}
			if proj != nil && proj.TroikiCategory != nil && p != service.PriorityForCategory(*proj.TroikiCategory) {
				return httpapi.ErrValidation("priority is managed by Troiki category")
			}
		}
		u.Priority = &p
	}
	if req.DueAt.IsNull() {
		u.DueAtClear = true
	} else if v, ok := req.DueAt.Value(); ok {
		ts, err := model.ParseUTC(v)
		if err != nil {
			return httpapi.ErrValidation("invalid dueAt format")
		}
		u.DueAt = &ts
	}
	if req.DueHasTime != nil {
		u.DueHasTime = req.DueHasTime
	}
	if req.DeadlineAt.IsNull() {
		u.DeadlineAtClear = true
	} else if v, ok := req.DeadlineAt.Value(); ok {
		ts, err := model.ParseUTC(v)
		if err != nil {
			return httpapi.ErrValidation("invalid deadlineAt format")
		}
		u.DeadlineAt = &ts
	}
	if req.DeadlineHasTime != nil {
		u.DeadlineHasTime = req.DeadlineHasTime
	}
	if req.DayPart != nil {
		dp := model.DayPart(*req.DayPart)
		if !dp.IsValid() {
			return httpapi.ErrValidation("invalid dayPart")
		}
		u.DayPart = &dp
	}
	if req.PlanState != nil {
		ps := model.PlanState(*req.PlanState)
		if !ps.IsValid() {
			return httpapi.ErrValidation("invalid planState")
		}
		u.PlanState = &ps
	}

	// Assigning a due date pulls a task out of the backlog: a scheduled task no
	// longer belongs to the unscheduled backlog. This is the inverse of planning
	// into backlog, which clears due (see service.PlanService.SetPlanState). An
	// explicit planState in the same request wins.
	if u.DueAt != nil && req.PlanState == nil && t.PlanState == model.PlanStateBacklog {
		none := model.PlanStateNone
		u.PlanState = &none
	}
	if req.RecurrenceRule.IsNull() {
		u.RecurrenceClear = true
	} else if v, ok := req.RecurrenceRule.Value(); ok {
		if _, err := rrule.StrToRRule(v); err != nil {
			return httpapi.ErrValidation("invalid recurrenceRule")
		}
		u.RecurrenceRule = &v
	}

	// Reject hasTime=true without a corresponding date — DB CHECK would
	// otherwise surface as a generic 500.
	if u.DueHasTime != nil && *u.DueHasTime {
		hasDue := u.DueAt != nil || (!u.DueAtClear && t.DueAt != nil)
		if !hasDue {
			return httpapi.ErrValidation("dueHasTime requires dueAt")
		}
	}
	if u.DeadlineHasTime != nil && *u.DeadlineHasTime {
		hasDeadline := u.DeadlineAt != nil || (!u.DeadlineAtClear && t.DeadlineAt != nil)
		if !hasDeadline {
			return httpapi.ErrValidation("deadlineHasTime requires deadlineAt")
		}
	}

	if req.IsPrivate != nil {
		u.IsPrivate = req.IsPrivate
	}

	u.IncPostponeCount = shouldIncPostpone(t, u, time.Now())

	updated, err := h.tasks.Update(c.Context(), id, u)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("task not found")
		}
		return httpapi.ErrInternal("update task").WithCause(err)
	}

	needsLabelUpdate := req.Title != nil || req.Labels != nil || len(req.RemovedAutoLabels) > 0
	if needsLabelUpdate {
		if err := h.taskSvc.PatchLabels(c.Context(), t, updated.Title, req.Labels, req.RemovedAutoLabels); err != nil {
			var ule *service.UnknownLabelError
			if errors.As(err, &ule) {
				return httpapi.ErrValidation("unknown label: " + ule.Name)
			}
			return httpapi.ErrInternal("apply labels").WithCause(err)
		}
		updated, err = h.tasks.Get(c.Context(), id)
		if err != nil {
			return httpapi.ErrInternal("get task after patch").WithCause(err)
		}
	}

	logMutation(c, "handler.Task.Patch", slog.Int64("task_id", updated.ID))
	return c.JSON(dto.TaskFromModel(*updated, h.baseURL))
}

func (h *TaskHandler) delete(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.Task.Delete", slog.Int64("task_id", id))
	if err := h.tasks.Delete(c.Context(), id); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("task not found")
		}
		return httpapi.ErrInternal("delete task").WithCause(err)
	}
	logMutation(c, "handler.Task.Delete", slog.Int64("task_id", id))
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *TaskHandler) listSubtasks(c fiber.Ctx) error {
	parentID, err := parseID(c)
	if err != nil {
		return err
	}
	if _, err := h.tasks.Get(c.Context(), parentID); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("task not found")
		}
		return httpapi.ErrInternal("get task").WithCause(err)
	}
	items, err := h.tasks.ListSubtasks(c.Context(), parentID)
	if err != nil {
		return httpapi.ErrInternal("list subtasks").WithCause(err)
	}
	dtos := make([]dto.TaskDTO, len(items))
	for i, t := range items {
		dtos[i] = dto.TaskFromModel(t, h.baseURL)
	}
	return c.JSON(dto.NewPagedResponse(dtos, len(dtos), len(dtos), 0))
}

// templateDraft assembles an unsaved task-template draft from a task and its
// whole subtree, flattened into a single subtask level. The frontend uses it to
// prefill the template editor; nothing is persisted here.
func (h *TaskHandler) templateDraft(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.Task.TemplateDraft", slog.Int64("task_id", id))
	root, err := h.tasks.Get(c.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("task not found")
		}
		return httpapi.ErrInternal("get task").WithCause(err)
	}
	descendants, err := h.flattenSubtree(c.Context(), id)
	if err != nil {
		return httpapi.ErrInternal("flatten subtree").WithCause(err)
	}
	return c.JSON(dto.TaskTemplateDraftFromTask(*root, descendants))
}

// flattenSubtree returns every descendant of parentID in depth-first pre-order.
// Each task is re-fetched via Get so its labels are hydrated (ListSubtasks does
// not hydrate labels) — same reason cloneTask re-fetches.
func (h *TaskHandler) flattenSubtree(ctx context.Context, parentID int64) ([]model.Task, error) {
	children, err := h.tasks.ListSubtasks(ctx, parentID)
	if err != nil {
		return nil, err
	}
	out := make([]model.Task, 0, len(children))
	for _, child := range children {
		full, err := h.tasks.Get(ctx, child.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, *full)
		nested, err := h.flattenSubtree(ctx, child.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, nested...)
	}
	return out, nil
}

func (h *TaskHandler) createSubtask(c fiber.Ctx) error {
	parentID, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.Task.CreateSubtask", slog.Int64("parent_id", parentID))
	parent, err := h.tasks.Get(c.Context(), parentID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("parent task not found")
		}
		return httpapi.ErrInternal("get parent task").WithCause(err)
	}
	if parent.InboxID != nil {
		logValidation(c, "handler.Task.CreateSubtask", "subtask in inbox")
		return httpapi.ErrForbiddenPlacement("subtasks cannot be placed in inbox")
	}
	var req dto.CreateTaskRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, "handler.Task.CreateSubtask", "invalid body")
		return httpapi.ErrValidation("invalid request body")
	}
	if req.Title == "" {
		logValidation(c, "handler.Task.CreateSubtask", "title required")
		return httpapi.ErrValidation("title is required")
	}
	placement := repo.Placement{
		ContextID: parent.ContextID,
		ProjectID: parent.ProjectID,
		SectionID: parent.SectionID,
		ParentID:  &parentID,
	}
	in, appErr := buildTaskCreate(req, placement)
	if appErr != nil {
		return appErr
	}
	// Inherit parent's labels when caller omits the field. Explicit empty array
	// (req.Labels == []) decodes to non-nil empty slice and is treated as
	// "no labels", so users can still create unlabelled subtasks.
	labels := req.Labels
	if labels == nil && len(parent.Labels) > 0 {
		labels = make([]string, len(parent.Labels))
		for i, l := range parent.Labels {
			labels[i] = l.Name
		}
	}
	t, err := h.taskSvc.Create(c.Context(), in, labels, req.RemovedAutoLabels)
	if err != nil {
		return handleTaskCreateErr(c, err)
	}
	logMutation(c, "handler.Task.CreateSubtask", slog.Int64("task_id", t.ID), slog.Int64("parent_id", parentID))
	return c.Status(fiber.StatusCreated).JSON(dto.TaskFromModel(*t, h.baseURL))
}

func (h *TaskHandler) duplicate(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.Task.Duplicate", slog.Int64("task_id", id))
	src, err := h.tasks.Get(c.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("task not found")
		}
		return httpapi.ErrInternal("get task").WithCause(err)
	}
	t, err := h.cloneTask(c.Context(), src, src.ParentID, duplicateTitle(src.Title))
	if err != nil {
		return handleTaskCreateErr(c, err)
	}
	out := dto.TaskFromModel(*t, h.baseURL)
	// Surface the cloned subtasks so the client can render them without a reload.
	subtasks, err := h.tasks.ListSubtasks(c.Context(), t.ID)
	if err != nil {
		return httpapi.ErrInternal("list subtasks").WithCause(err)
	}
	dtos := make([]dto.TaskDTO, len(subtasks))
	for i, st := range subtasks {
		dtos[i] = dto.TaskFromModel(st, h.baseURL)
	}
	page := dto.NewPagedResponse(dtos, len(dtos), len(dtos), 0)
	out.Subtasks = &page
	logMutation(c, "handler.Task.Duplicate", slog.Int64("task_id", t.ID), slog.Int64("source_id", id))
	return c.Status(fiber.StatusCreated).JSON(out)
}

// cloneTask deep-copies src as a new task placed under parentID (nil for a
// top-level task) with the given title, then recursively clones src's subtasks
// under the freshly created task. Only the top-level clone is renamed; subtasks
// keep their original titles. Each subtask is re-fetched via Get so its labels
// are hydrated (ListSubtasks does not hydrate labels).
func (h *TaskHandler) cloneTask(ctx context.Context, src *model.Task, parentID *int64, title string) (*model.Task, error) {
	in := repo.CreateTask{
		Placement: repo.Placement{
			InboxID:   src.InboxID,
			ContextID: src.ContextID,
			ProjectID: src.ProjectID,
			SectionID: src.SectionID,
			ParentID:  parentID,
		},
		Title:           title,
		Description:     src.Description,
		Priority:        src.Priority,
		DueAt:           src.DueAt,
		DueHasTime:      src.DueHasTime,
		DeadlineAt:      src.DeadlineAt,
		DeadlineHasTime: src.DeadlineHasTime,
		DayPart:         src.DayPart,
		PlanState:       src.PlanState,
		RecurrenceRule:  src.RecurrenceRule,
	}
	labelNames := make([]string, len(src.Labels))
	for i, l := range src.Labels {
		labelNames[i] = l.Name
	}
	t, err := h.taskSvc.Create(ctx, in, labelNames, nil)
	if err != nil {
		return nil, err
	}
	subtasks, err := h.tasks.ListSubtasks(ctx, src.ID)
	if err != nil {
		return nil, err
	}
	for i := range subtasks {
		sub, err := h.tasks.Get(ctx, subtasks[i].ID)
		if err != nil {
			return nil, err
		}
		if _, err := h.cloneTask(ctx, sub, &t.ID, sub.Title); err != nil {
			return nil, err
		}
	}
	return t, nil
}

func (h *TaskHandler) decompose(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.Task.Decompose", slog.Int64("task_id", id))
	var req dto.DecomposeTaskRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, "handler.Task.Decompose", "invalid body")
		return httpapi.ErrValidation("invalid request body")
	}
	titles := make([]string, 0, len(req.Titles))
	for _, raw := range req.Titles {
		s := strings.TrimSpace(raw)
		if s != "" {
			titles = append(titles, s)
		}
	}
	if len(titles) == 0 {
		logValidation(c, "handler.Task.Decompose", "empty titles")
		return httpapi.ErrValidation("titles must not be empty")
	}

	src, err := h.tasks.Get(c.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("task not found")
		}
		return httpapi.ErrInternal("get task").WithCause(err)
	}
	subs, err := h.tasks.ListSubtasks(c.Context(), id)
	if err != nil {
		return httpapi.ErrInternal("list subtasks").WithCause(err)
	}
	if len(subs) > 0 {
		return httpapi.ErrConflict("task has subtasks")
	}

	labelNames := make([]string, len(src.Labels))
	for i, l := range src.Labels {
		labelNames[i] = l.Name
	}
	placement := repo.Placement{
		InboxID:   src.InboxID,
		ContextID: src.ContextID,
		ProjectID: src.ProjectID,
		SectionID: src.SectionID,
		ParentID:  src.ParentID,
	}

	created := make([]model.Task, 0, len(titles))
	createdIDs := make([]int64, 0, len(titles))
	rollback := func() {
		for _, cid := range createdIDs {
			_ = h.tasks.Delete(c.Context(), cid)
		}
	}
	for _, title := range titles {
		in := repo.CreateTask{
			Placement:       placement,
			Title:           title,
			Description:     src.Description,
			Priority:        src.Priority,
			DueAt:           src.DueAt,
			DueHasTime:      src.DueHasTime,
			DeadlineAt:      src.DeadlineAt,
			DeadlineHasTime: src.DeadlineHasTime,
			DayPart:         src.DayPart,
			PlanState:       src.PlanState,
			RecurrenceRule:  src.RecurrenceRule,
		}
		t, err := h.taskSvc.Create(c.Context(), in, labelNames, nil)
		if err != nil {
			rollback()
			return handleTaskCreateErr(c, err)
		}
		if src.IsPrivate {
			isPriv := true
			if updated, uerr := h.tasks.Update(c.Context(), t.ID, repo.TaskUpdate{IsPrivate: &isPriv}); uerr == nil {
				t = updated
			}
		}
		created = append(created, *t)
		createdIDs = append(createdIDs, t.ID)
	}

	if err := h.tasks.Delete(c.Context(), id); err != nil {
		rollback()
		return httpapi.ErrInternal("delete original task").WithCause(err)
	}

	out := dto.DecomposeTaskResponse{Created: make([]dto.TaskDTO, len(created))}
	for i, t := range created {
		out.Created[i] = dto.TaskFromModel(t, h.baseURL)
	}
	logMutation(c, "handler.Task.Decompose", slog.Int64("source_id", id), slog.Int("created", len(created)))
	return c.Status(fiber.StatusCreated).JSON(out)
}

// handleTaskCreateErr maps TaskService.Create errors to API errors.
func handleTaskCreateErr(c fiber.Ctx, err error) *httpapi.AppError {
	var ule *service.UnknownLabelError
	if errors.As(err, &ule) {
		logValidation(c, "handler.Task.Create", "unknown label", slog.String("label", ule.Name))
		return httpapi.ErrValidation("unknown label: " + ule.Name)
	}
	if errors.Is(err, repo.ErrInvalidPlacement) {
		logValidation(c, "handler.Task.Create", "invalid placement")
		return httpapi.ErrForbiddenPlacement("invalid task placement")
	}
	return httpapi.ErrInternal("create task").WithCause(err)
}

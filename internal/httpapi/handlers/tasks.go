package handlers

import (
	"errors"
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
	tasks   *repo.TaskRepo
	taskSvc *service.TaskService
	baseURL string
}

// NewTaskHandler constructs a TaskHandler.
func NewTaskHandler(tasks *repo.TaskRepo, taskSvc *service.TaskService, baseURL string) *TaskHandler {
	return &TaskHandler{tasks: tasks, taskSvc: taskSvc, baseURL: baseURL}
}

// Register wires task routes onto r (the /api/v1 group).
func (h *TaskHandler) Register(r fiber.Router) {
	r.Get("/tasks/:id", h.get)
	r.Patch("/tasks/:id", h.patch)
	r.Delete("/tasks/:id", h.delete)
	r.Get("/tasks/:id/subtasks", h.listSubtasks)
	r.Post("/tasks/:id/subtasks", h.createSubtask)
	r.Post("/tasks/:id/duplicate", h.duplicate)
}

func (h *TaskHandler) get(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	t, err := h.tasks.Get(c.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("task not found")
		}
		return httpapi.ErrInternal("get task")
	}
	return c.JSON(dto.TaskFromModel(*t, h.baseURL))
}

func (h *TaskHandler) patch(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	t, err := h.tasks.Get(c.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("task not found")
		}
		return httpapi.ErrInternal("get task")
	}
	var req dto.PatchTaskRequest
	if err := c.Bind().JSON(&req); err != nil {
		return httpapi.ErrValidation("invalid request body")
	}

	u := repo.TaskUpdate{}
	if req.Title != nil {
		if strings.TrimSpace(*req.Title) == "" {
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
			return httpapi.ErrValidation("invalid priority")
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

	u.IncPostponeCount = shouldIncPostpone(t, u, time.Now())

	updated, err := h.tasks.Update(c.Context(), id, u)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("task not found")
		}
		return httpapi.ErrInternal("update task")
	}

	needsLabelUpdate := req.Title != nil || req.Labels != nil || len(req.RemovedAutoLabels) > 0
	if needsLabelUpdate {
		if err := h.taskSvc.PatchLabels(c.Context(), t, updated.Title, req.Labels, req.RemovedAutoLabels); err != nil {
			var ule *service.UnknownLabelError
			if errors.As(err, &ule) {
				return httpapi.ErrValidation("unknown label: " + ule.Name)
			}
			return httpapi.ErrInternal("apply labels")
		}
		updated, err = h.tasks.Get(c.Context(), id)
		if err != nil {
			return httpapi.ErrInternal("get task after patch")
		}
	}

	return c.JSON(dto.TaskFromModel(*updated, h.baseURL))
}

func (h *TaskHandler) delete(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	if err := h.tasks.Delete(c.Context(), id); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("task not found")
		}
		return httpapi.ErrInternal("delete task")
	}
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
		return httpapi.ErrInternal("get task")
	}
	items, err := h.tasks.ListSubtasks(c.Context(), parentID)
	if err != nil {
		return httpapi.ErrInternal("list subtasks")
	}
	dtos := make([]dto.TaskDTO, len(items))
	for i, t := range items {
		dtos[i] = dto.TaskFromModel(t, h.baseURL)
	}
	return c.JSON(dto.NewPagedResponse(dtos, len(dtos), len(dtos), 0))
}

func (h *TaskHandler) createSubtask(c fiber.Ctx) error {
	parentID, err := parseID(c)
	if err != nil {
		return err
	}
	parent, err := h.tasks.Get(c.Context(), parentID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("parent task not found")
		}
		return httpapi.ErrInternal("get parent task")
	}
	if parent.InboxID != nil {
		return httpapi.ErrForbiddenPlacement("subtasks cannot be placed in inbox")
	}
	var req dto.CreateTaskRequest
	if err := c.Bind().JSON(&req); err != nil {
		return httpapi.ErrValidation("invalid request body")
	}
	if req.Title == "" {
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
		return handleTaskCreateErr(err)
	}
	return c.Status(fiber.StatusCreated).JSON(dto.TaskFromModel(*t, h.baseURL))
}

func (h *TaskHandler) duplicate(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	src, err := h.tasks.Get(c.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("task not found")
		}
		return httpapi.ErrInternal("get task")
	}
	in := repo.CreateTask{
		Placement: repo.Placement{
			InboxID:   src.InboxID,
			ContextID: src.ContextID,
			ProjectID: src.ProjectID,
			SectionID: src.SectionID,
			ParentID:  src.ParentID,
		},
		Title:           duplicateTitle(src.Title),
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
	t, err := h.taskSvc.Create(c.Context(), in, labelNames, nil)
	if err != nil {
		return handleTaskCreateErr(err)
	}
	return c.Status(fiber.StatusCreated).JSON(dto.TaskFromModel(*t, h.baseURL))
}

// handleTaskCreateErr maps TaskService.Create errors to API errors.
func handleTaskCreateErr(err error) *httpapi.AppError {
	var ule *service.UnknownLabelError
	if errors.As(err, &ule) {
		return httpapi.ErrValidation("unknown label: " + ule.Name)
	}
	if errors.Is(err, repo.ErrInvalidPlacement) {
		return httpapi.ErrForbiddenPlacement("invalid task placement")
	}
	return httpapi.ErrInternal("create task")
}

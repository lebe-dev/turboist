package handlers

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
)

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
	r.Post("/tasks/:id/subtasks", h.createSubtask)
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
	t, err := h.taskSvc.Create(c.Context(), in, req.Labels, req.RemovedAutoLabels)
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

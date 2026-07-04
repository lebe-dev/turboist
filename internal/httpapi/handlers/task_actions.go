package handlers

import (
	"errors"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
)

const (
	opTaskComplete = "handler.Task.Complete"
	opTaskMove     = "handler.Task.Move"
	opTaskPlan     = "handler.Task.Plan"
)

// TaskActionHandler handles action endpoints for tasks (complete, move, plan, pin).
type TaskActionHandler struct {
	tasks       *repo.TaskRepo
	completeSvc *service.CompleteService
	planSvc     *service.PlanService
	pinSvc      *service.PinService
	moveSvc     *service.MoveService
	baseURL     string
}

func NewTaskActionHandler(
	tasks *repo.TaskRepo,
	completeSvc *service.CompleteService,
	planSvc *service.PlanService,
	pinSvc *service.PinService,
	moveSvc *service.MoveService,
	baseURL string,
) *TaskActionHandler {
	return &TaskActionHandler{
		tasks:       tasks,
		completeSvc: completeSvc,
		planSvc:     planSvc,
		pinSvc:      pinSvc,
		moveSvc:     moveSvc,
		baseURL:     baseURL,
	}
}

func (h *TaskActionHandler) Register(r fiber.Router) {
	r.Post("/tasks/:id/complete", httpapi.RequireScope(auth.ScopeTasksWrite), h.complete)
	r.Post("/tasks/:id/uncomplete", httpapi.RequireScope(auth.ScopeTasksWrite), h.uncomplete)
	r.Post("/tasks/:id/cancel", httpapi.RequireScope(auth.ScopeTasksWrite), h.cancel)
	r.Post("/tasks/:id/pin", httpapi.RequireScope(auth.ScopeTasksWrite), h.pin)
	r.Post("/tasks/:id/unpin", httpapi.RequireScope(auth.ScopeTasksWrite), h.unpin)
	r.Post("/tasks/:id/move", httpapi.RequireScope(auth.ScopeTasksWrite), h.move)
	r.Post("/tasks/:id/plan", httpapi.RequireScope(auth.ScopeTasksWrite), h.plan)
}

// CompleteRequest is the optional body for POST /tasks/:id/complete. When
// completedAt is provided, the task is marked completed with that timestamp
// instead of the server's current time. Used by the "complete overdue task"
// flow on the Today page so users can record the actual completion day.
type CompleteRequest struct {
	CompletedAt *string `json:"completedAt"`
}

func (h *TaskActionHandler) complete(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, opTaskComplete, slog.Int64("task_id", id))
	var completedAt *time.Time
	if len(c.Body()) > 0 {
		var req CompleteRequest
		if err := c.Bind().JSON(&req); err != nil {
			logValidation(c, opTaskComplete, msgInvalidBody)
			return httpapi.ErrValidation(msgInvalidRequestBody)
		}
		if req.CompletedAt != nil && *req.CompletedAt != "" {
			ts, err := time.Parse(time.RFC3339Nano, *req.CompletedAt)
			if err != nil {
				logValidation(c, opTaskComplete, "invalid completedAt")
				return httpapi.ErrValidation("invalid completedAt timestamp")
			}
			completedAt = &ts
		}
	}
	var t *model.Task
	if completedAt != nil {
		t, err = h.completeSvc.CompleteAt(c.Context(), id, *completedAt)
	} else {
		t, err = h.completeSvc.Complete(c.Context(), id)
	}
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound(msgTaskNotFound)
		}
		var re *service.RecurrenceError
		if errors.As(err, &re) {
			return httpapi.ErrRecurrenceInvalid(re.Err.Error())
		}
		return httpapi.ErrInternal("complete task").WithCause(err)
	}
	logMutation(c, opTaskComplete, slog.Int64("task_id", t.ID))
	return c.JSON(dto.TaskFromModel(*t, h.baseURL))
}

func (h *TaskActionHandler) uncomplete(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.Task.Uncomplete", slog.Int64("task_id", id))
	t, err := h.completeSvc.Uncomplete(c.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound(msgTaskNotFound)
		}
		if errors.Is(err, service.ErrTroikiSlotFull) {
			return httpapi.ErrTroikiSlotFull("troiki slot is full")
		}
		return httpapi.ErrInternal("uncomplete task").WithCause(err)
	}
	logMutation(c, "handler.Task.Uncomplete", slog.Int64("task_id", t.ID))
	return c.JSON(dto.TaskFromModel(*t, h.baseURL))
}

func (h *TaskActionHandler) cancel(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.Task.Cancel", slog.Int64("task_id", id))
	t, err := h.completeSvc.Cancel(c.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound(msgTaskNotFound)
		}
		return httpapi.ErrInternal("cancel task").WithCause(err)
	}
	logMutation(c, "handler.Task.Cancel", slog.Int64("task_id", t.ID))
	return c.JSON(dto.TaskFromModel(*t, h.baseURL))
}

// MoveRequest is the body for POST /tasks/:id/move.
type MoveRequest struct {
	InboxID   *int64 `json:"inboxId"`
	ContextID *int64 `json:"contextId"`
	ProjectID *int64 `json:"projectId"`
	SectionID *int64 `json:"sectionId"`
	ParentID  *int64 `json:"parentId"`
}

func (h *TaskActionHandler) move(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, opTaskMove, slog.Int64("task_id", id))
	var req MoveRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, opTaskMove, msgInvalidBody)
		return httpapi.ErrValidation(msgInvalidRequestBody)
	}
	target := repo.Placement{
		InboxID:   req.InboxID,
		ContextID: req.ContextID,
		ProjectID: req.ProjectID,
		SectionID: req.SectionID,
		ParentID:  req.ParentID,
	}
	t, err := h.moveSvc.Move(c.Context(), id, target)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound(msgTaskNotFound)
		}
		if errors.Is(err, repo.ErrInvalidPlacement) || errors.Is(err, repo.ErrCycle) {
			logValidation(c, opTaskMove, "invalid placement")
			return httpapi.ErrForbiddenPlacement("invalid task placement")
		}
		return httpapi.ErrInternal("move task").WithCause(err)
	}
	logMutation(c, opTaskMove, slog.Int64("task_id", t.ID))
	return c.JSON(dto.TaskFromModel(*t, h.baseURL))
}

// PlanRequest is the body for POST /tasks/:id/plan.
type PlanRequest struct {
	State string `json:"state"`
}

func (h *TaskActionHandler) plan(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, opTaskPlan, slog.Int64("task_id", id))
	var req PlanRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, opTaskPlan, msgInvalidBody)
		return httpapi.ErrValidation(msgInvalidRequestBody)
	}
	state := model.PlanState(req.State)
	if !state.IsValid() {
		logValidation(c, opTaskPlan, "invalid plan state", slog.String("state", req.State))
		return httpapi.ErrValidation("invalid plan state")
	}
	t, err := h.planSvc.SetPlanState(c.Context(), id, state)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound(msgTaskNotFound)
		}
		if errors.Is(err, service.ErrPlanLimitExceeded) {
			return httpapi.ErrLimitExceeded("plan limit exceeded")
		}
		if errors.Is(err, service.ErrNoContextForInbox) {
			logValidation(c, opTaskPlan, "no context for inbox")
			return httpapi.ErrValidation("create a context before planning inbox tasks")
		}
		return httpapi.ErrInternal("set plan state").WithCause(err)
	}
	logMutation(c, opTaskPlan, slog.Int64("task_id", t.ID), slog.String("state", string(state)))
	return c.JSON(dto.TaskFromModel(*t, h.baseURL))
}

func (h *TaskActionHandler) pin(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.Task.Pin", slog.Int64("task_id", id))
	if err := h.pinSvc.PinTask(c.Context(), id); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound(msgTaskNotFound)
		}
		if errors.Is(err, service.ErrPinLimitExceeded) {
			return httpapi.ErrLimitExceeded("max-pinned limit reached")
		}
		return httpapi.ErrInternal("pin task").WithCause(err)
	}
	t, err := h.tasks.Get(c.Context(), id)
	if err != nil {
		return httpapi.ErrInternal("get task after pin").WithCause(err)
	}
	logMutation(c, "handler.Task.Pin", slog.Int64("task_id", t.ID))
	return c.JSON(dto.TaskFromModel(*t, h.baseURL))
}

func (h *TaskActionHandler) unpin(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.Task.Unpin", slog.Int64("task_id", id))
	if err := h.pinSvc.UnpinTask(c.Context(), id); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound(msgTaskNotFound)
		}
		return httpapi.ErrInternal("unpin task").WithCause(err)
	}
	t, err := h.tasks.Get(c.Context(), id)
	if err != nil {
		return httpapi.ErrInternal("get task after unpin").WithCause(err)
	}
	logMutation(c, "handler.Task.Unpin", slog.Int64("task_id", t.ID))
	return c.JSON(dto.TaskFromModel(*t, h.baseURL))
}

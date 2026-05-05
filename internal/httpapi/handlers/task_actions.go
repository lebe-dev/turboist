package handlers

import (
	"errors"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
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
	r.Post("/tasks/:id/complete", h.complete)
	r.Post("/tasks/:id/uncomplete", h.uncomplete)
	r.Post("/tasks/:id/cancel", h.cancel)
	r.Post("/tasks/:id/pin", h.pin)
	r.Post("/tasks/:id/unpin", h.unpin)
	r.Post("/tasks/:id/move", h.move)
	r.Post("/tasks/:id/plan", h.plan)
}

func (h *TaskActionHandler) complete(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	t, err := h.completeSvc.Complete(c.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("task not found")
		}
		var re *service.RecurrenceError
		if errors.As(err, &re) {
			return httpapi.ErrRecurrenceInvalid(re.Err.Error())
		}
		return httpapi.ErrInternal("complete task")
	}
	return c.JSON(dto.TaskFromModel(*t, h.baseURL))
}

func (h *TaskActionHandler) uncomplete(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	t, err := h.completeSvc.Uncomplete(c.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("task not found")
		}
		if errors.Is(err, service.ErrTroikiSlotFull) {
			return httpapi.ErrTroikiSlotFull("troiki slot is full")
		}
		return httpapi.ErrInternal("uncomplete task")
	}
	return c.JSON(dto.TaskFromModel(*t, h.baseURL))
}

func (h *TaskActionHandler) cancel(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	t, err := h.completeSvc.Cancel(c.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("task not found")
		}
		return httpapi.ErrInternal("cancel task")
	}
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
	var req MoveRequest
	if err := c.Bind().JSON(&req); err != nil {
		return httpapi.ErrValidation("invalid request body")
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
			return httpapi.ErrNotFound("task not found")
		}
		if errors.Is(err, repo.ErrInvalidPlacement) || errors.Is(err, repo.ErrCycle) {
			return httpapi.ErrForbiddenPlacement("invalid task placement")
		}
		return httpapi.ErrInternal("move task")
	}
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
	var req PlanRequest
	if err := c.Bind().JSON(&req); err != nil {
		return httpapi.ErrValidation("invalid request body")
	}
	state := model.PlanState(req.State)
	if !state.IsValid() {
		return httpapi.ErrValidation("invalid plan state")
	}
	t, err := h.planSvc.SetPlanState(c.Context(), id, state)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("task not found")
		}
		if errors.Is(err, service.ErrPlanLimitExceeded) {
			return httpapi.ErrLimitExceeded("plan limit exceeded")
		}
		if errors.Is(err, service.ErrNoContextForInbox) {
			return httpapi.ErrValidation("create a context before planning inbox tasks")
		}
		return httpapi.ErrInternal("set plan state")
	}
	return c.JSON(dto.TaskFromModel(*t, h.baseURL))
}

func (h *TaskActionHandler) pin(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	if err := h.pinSvc.PinTask(c.Context(), id); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("task not found")
		}
		if errors.Is(err, service.ErrPinLimitExceeded) {
			return httpapi.ErrLimitExceeded("max-pinned limit reached")
		}
		return httpapi.ErrInternal("pin task")
	}
	t, err := h.tasks.Get(c.Context(), id)
	if err != nil {
		return httpapi.ErrInternal("get task after pin")
	}
	return c.JSON(dto.TaskFromModel(*t, h.baseURL))
}

func (h *TaskActionHandler) unpin(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	if err := h.pinSvc.UnpinTask(c.Context(), id); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("task not found")
		}
		return httpapi.ErrInternal("unpin task")
	}
	t, err := h.tasks.Get(c.Context(), id)
	if err != nil {
		return httpapi.ErrInternal("get task after unpin")
	}
	return c.JSON(dto.TaskFromModel(*t, h.baseURL))
}

package handlers

import (
	"errors"
	"log/slog"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
)

const (
	opTaskRelationAdd    = "handler.TaskRelation.Add"
	opTaskRelationRemove = "handler.TaskRelation.Remove"
	msgRelationNotFound  = "relation not found"
)

// TaskRelationHandler serves the task relation graph.
//
// Write-only by design: there is no GET /tasks/:id/relations. Relations ride
// inside GET /tasks/:id?relations=true, and both mutations answer with the updated
// task, so the SPA never needs a follow-up read (see the "one aggregate for reads"
// invariant in CLAUDE.md, and selfRefresh.ts which deliberately does not
// re-dispatch the `tasks` scope for this client's own mutation).
type TaskRelationHandler struct {
	relationSvc *service.RelationService
	baseURL     string
}

func NewTaskRelationHandler(relationSvc *service.RelationService, baseURL string) *TaskRelationHandler {
	return &TaskRelationHandler{relationSvc: relationSvc, baseURL: baseURL}
}

func (h *TaskRelationHandler) Register(r fiber.Router) {
	r.Post("/tasks/:id/relations", httpapi.RequireScope(auth.ScopeTasksWrite), h.add)
	r.Delete("/tasks/:id/relations/:relationId", httpapi.RequireScope(auth.ScopeTasksWrite), h.remove)
}

func (h *TaskRelationHandler) add(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, opTaskRelationAdd, slog.Int64("task_id", id))

	var req dto.CreateTaskRelationRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, opTaskRelationAdd, msgInvalidBody)
		return httpapi.ErrValidation(msgInvalidRequestBody)
	}
	if req.TargetTaskID <= 0 {
		logValidation(c, opTaskRelationAdd, "invalid targetTaskId")
		return httpapi.ErrValidation("invalid targetTaskId")
	}
	relType := model.RelationType(req.Type)
	if !relType.IsValid() {
		logValidation(c, opTaskRelationAdd, "invalid type", slog.String("type", req.Type))
		return httpapi.ErrValidation("type must be related or blocks")
	}
	// `related` is symmetric, so an absent direction is fine there; `blocks` needs
	// one to know which end holds the other back.
	direction := model.RelationDirection(req.Direction)
	if req.Direction == "" {
		direction = model.RelationDirectionOutgoing
	} else if !direction.IsValid() {
		logValidation(c, opTaskRelationAdd, "invalid direction", slog.String("direction", req.Direction))
		return httpapi.ErrValidation("direction must be outgoing or incoming")
	}

	t, err := h.relationSvc.Add(c.Context(), id, req.TargetTaskID, relType, direction)
	if err != nil {
		return h.mapErr(err, opTaskRelationAdd)
	}
	logMutation(c, opTaskRelationAdd,
		slog.Int64("task_id", t.ID),
		slog.Int64("other_task_id", req.TargetTaskID),
		slog.String("type", req.Type))
	return c.JSON(dto.TaskFromModel(*t, h.baseURL))
}

func (h *TaskRelationHandler) remove(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	relationID, err := parseNamedID(c, "relationId")
	if err != nil {
		return err
	}
	logEntry(c, opTaskRelationRemove, slog.Int64("task_id", id), slog.Int64("relation_id", relationID))

	t, err := h.relationSvc.Remove(c.Context(), id, relationID)
	if err != nil {
		return h.mapErr(err, opTaskRelationRemove)
	}
	logMutation(c, opTaskRelationRemove, slog.Int64("task_id", t.ID), slog.Int64("relation_id", relationID))
	return c.JSON(dto.TaskFromModel(*t, h.baseURL))
}

func (h *TaskRelationHandler) mapErr(err error, op string) error {
	switch {
	case errors.Is(err, repo.ErrNotFound):
		// Covers both a missing task and a relation that does not touch this task —
		// the repo scopes its delete by task id precisely so the latter is a 404
		// rather than a silent success.
		if op == opTaskRelationRemove {
			return httpapi.ErrNotFound(msgRelationNotFound)
		}
		return httpapi.ErrNotFound(msgTaskNotFound)
	case errors.Is(err, repo.ErrConflict):
		return httpapi.ErrConflict("relation already exists")
	case errors.Is(err, service.ErrRelationSelf):
		return httpapi.ErrValidation("a task cannot be related to itself")
	case errors.Is(err, service.ErrRelationCycle):
		return httpapi.ErrValidation("relation would create a blocking cycle")
	default:
		return httpapi.ErrInternal(op).WithCause(err)
	}
}

package handlers

import (
	"errors"
	"log/slog"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/repo"
)

// ChecklistHandler implements the task checklist sub-resource (Federation v1
// F0.2): list / create / patch (title + toggle) / delete.
type ChecklistHandler struct {
	items *repo.ChecklistItemRepo
	tasks *repo.TaskRepo
}

// NewChecklistHandler constructs a ChecklistHandler.
func NewChecklistHandler(items *repo.ChecklistItemRepo, tasks *repo.TaskRepo) *ChecklistHandler {
	return &ChecklistHandler{items: items, tasks: tasks}
}

// Register wires checklist routes onto r (the /api/v1 group).
func (h *ChecklistHandler) Register(r fiber.Router) {
	r.Get("/tasks/:id/checklist", httpapi.RequireScope("tasks:read"), h.list)
	r.Post("/tasks/:id/checklist", httpapi.RequireScope("tasks:write"), h.create)
	r.Patch("/tasks/:id/checklist/:itemId", httpapi.RequireScope("tasks:write"), h.patch)
	r.Delete("/tasks/:id/checklist/:itemId", httpapi.RequireScope("tasks:write"), h.delete)
}

func (h *ChecklistHandler) list(c fiber.Ctx) error {
	taskID, err := parseID(c)
	if err != nil {
		return err
	}
	if appErr := h.ensureTask(c, taskID); appErr != nil {
		return appErr
	}
	pp := dto.ParsePageParams(c.Query("limit"), c.Query("offset"))
	items, total, err := h.items.ListByTask(c.Context(), taskID, repo.Page{Limit: pp.Limit, Offset: pp.Offset})
	if err != nil {
		return httpapi.ErrInternal("list checklist items").WithCause(err)
	}
	dtos := make([]dto.ChecklistItemDTO, len(items))
	for i, item := range items {
		dtos[i] = dto.ChecklistItemFromModel(item)
	}
	return c.JSON(dto.NewPagedResponse(dtos, total, pp.Limit, pp.Offset))
}

func (h *ChecklistHandler) create(c fiber.Ctx) error {
	taskID, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.Checklist.Create", slog.Int64("task_id", taskID))
	if appErr := h.ensureTask(c, taskID); appErr != nil {
		return appErr
	}
	var req dto.CreateChecklistItemRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, "handler.Checklist.Create", "invalid body")
		return httpapi.ErrValidation("invalid request body")
	}
	if req.Title == "" {
		logValidation(c, "handler.Checklist.Create", "title required")
		return httpapi.ErrValidation("checklist item title is required")
	}
	item, err := h.items.Create(c.Context(), taskID, req.Title)
	if err != nil {
		return httpapi.ErrInternal("create checklist item").WithCause(err)
	}
	logMutation(c, "handler.Checklist.Create", slog.Int64("checklist_item_id", item.ID), slog.Int64("task_id", taskID))
	return c.Status(fiber.StatusCreated).JSON(dto.ChecklistItemFromModel(*item))
}

func (h *ChecklistHandler) patch(c fiber.Ctx) error {
	taskID, err := parseID(c)
	if err != nil {
		return err
	}
	itemID, err := parseSubID(c, "itemId")
	if err != nil {
		return err
	}
	logEntry(c, "handler.Checklist.Patch", slog.Int64("task_id", taskID), slog.Int64("checklist_item_id", itemID))
	if appErr := h.ensureTask(c, taskID); appErr != nil {
		return appErr
	}
	var req dto.PatchChecklistItemRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, "handler.Checklist.Patch", "invalid body")
		return httpapi.ErrValidation("invalid request body")
	}
	item, err := h.items.Update(c.Context(), taskID, itemID, repo.ChecklistItemUpdate{Title: req.Title, IsCompleted: req.IsCompleted})
	if err != nil {
		if appErr := mutationErr(err, "checklist item not found"); appErr != nil {
			return appErr
		}
		return httpapi.ErrInternal("update checklist item").WithCause(err)
	}
	logMutation(c, "handler.Checklist.Patch", slog.Int64("checklist_item_id", item.ID))
	return c.JSON(dto.ChecklistItemFromModel(*item))
}

func (h *ChecklistHandler) delete(c fiber.Ctx) error {
	taskID, err := parseID(c)
	if err != nil {
		return err
	}
	itemID, err := parseSubID(c, "itemId")
	if err != nil {
		return err
	}
	logEntry(c, "handler.Checklist.Delete", slog.Int64("task_id", taskID), slog.Int64("checklist_item_id", itemID))
	if appErr := h.ensureTask(c, taskID); appErr != nil {
		return appErr
	}
	if err := h.items.Delete(c.Context(), taskID, itemID); err != nil {
		if appErr := mutationErr(err, "checklist item not found"); appErr != nil {
			return appErr
		}
		return httpapi.ErrInternal("delete checklist item").WithCause(err)
	}
	logMutation(c, "handler.Checklist.Delete", slog.Int64("checklist_item_id", itemID))
	return c.SendStatus(fiber.StatusNoContent)
}

// ensureTask resolves the parent task, mapping a tombstone to 410 and a missing
// task to 404.
func (h *ChecklistHandler) ensureTask(c fiber.Ctx, taskID int64) *httpapi.AppError {
	if _, err := h.tasks.Get(c.Context(), taskID); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return notFoundOrGoneTask(c, h.tasks, taskID, "task not found")
		}
		return httpapi.ErrInternal("get task").WithCause(err)
	}
	return nil
}

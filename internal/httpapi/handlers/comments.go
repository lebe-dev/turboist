package handlers

import (
	"errors"
	"log/slog"
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/repo"
)

// CommentHandler implements the immutable task-comment sub-resource
// (Federation v1 F0.2): list / create / delete only. There is deliberately no
// PATCH — a comment body never changes (US-3.5 AC2).
type CommentHandler struct {
	comments *repo.CommentRepo
	tasks    *repo.TaskRepo
}

// NewCommentHandler constructs a CommentHandler.
func NewCommentHandler(comments *repo.CommentRepo, tasks *repo.TaskRepo) *CommentHandler {
	return &CommentHandler{comments: comments, tasks: tasks}
}

// Register wires comment routes onto r (the /api/v1 group).
func (h *CommentHandler) Register(r fiber.Router) {
	r.Get("/tasks/:id/comments", httpapi.RequireScope("tasks:read"), h.list)
	r.Post("/tasks/:id/comments", httpapi.RequireScope("tasks:write"), h.create)
	r.Delete("/tasks/:id/comments/:commentId", httpapi.RequireScope("tasks:write"), h.delete)
}

func (h *CommentHandler) list(c fiber.Ctx) error {
	taskID, err := parseID(c)
	if err != nil {
		return err
	}
	if appErr := h.ensureTask(c, taskID); appErr != nil {
		return appErr
	}
	pp := dto.ParsePageParams(c.Query("limit"), c.Query("offset"))
	items, total, err := h.comments.ListByTask(c.Context(), taskID, repo.Page{Limit: pp.Limit, Offset: pp.Offset})
	if err != nil {
		return httpapi.ErrInternal("list comments").WithCause(err)
	}
	dtos := make([]dto.CommentDTO, len(items))
	for i, item := range items {
		dtos[i] = dto.CommentFromModel(item)
	}
	return c.JSON(dto.NewPagedResponse(dtos, total, pp.Limit, pp.Offset))
}

func (h *CommentHandler) create(c fiber.Ctx) error {
	taskID, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.Comment.Create", slog.Int64("task_id", taskID))
	if appErr := h.ensureTask(c, taskID); appErr != nil {
		return appErr
	}
	var req dto.CreateCommentRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, "handler.Comment.Create", "invalid body")
		return httpapi.ErrValidation("invalid request body")
	}
	if req.Body == "" {
		logValidation(c, "handler.Comment.Create", "body required")
		return httpapi.ErrValidation("comment body is required")
	}
	comment, err := h.comments.Create(c.Context(), taskID, req.Body)
	if err != nil {
		return httpapi.ErrInternal("create comment").WithCause(err)
	}
	logMutation(c, "handler.Comment.Create", slog.Int64("comment_id", comment.ID), slog.Int64("task_id", taskID))
	return c.Status(fiber.StatusCreated).JSON(dto.CommentFromModel(*comment))
}

func (h *CommentHandler) delete(c fiber.Ctx) error {
	taskID, err := parseID(c)
	if err != nil {
		return err
	}
	commentID, err := parseSubID(c, "commentId")
	if err != nil {
		return err
	}
	logEntry(c, "handler.Comment.Delete", slog.Int64("task_id", taskID), slog.Int64("comment_id", commentID))
	if appErr := h.ensureTask(c, taskID); appErr != nil {
		return appErr
	}
	if err := h.comments.Delete(c.Context(), taskID, commentID); err != nil {
		if appErr := mutationErr(err, "comment not found"); appErr != nil {
			return appErr
		}
		return httpapi.ErrInternal("delete comment").WithCause(err)
	}
	logMutation(c, "handler.Comment.Delete", slog.Int64("comment_id", commentID))
	return c.SendStatus(fiber.StatusNoContent)
}

// ensureTask resolves the parent task, mapping a tombstone to 410 and a missing
// task to 404 so the sub-resource never operates on a dead task.
func (h *CommentHandler) ensureTask(c fiber.Ctx, taskID int64) *httpapi.AppError {
	if _, err := h.tasks.Get(c.Context(), taskID); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return notFoundOrGoneTask(c, h.tasks, taskID, "task not found")
		}
		return httpapi.ErrInternal("get task").WithCause(err)
	}
	return nil
}

// parseSubID parses a non-:id path parameter as a positive int64.
func parseSubID(c fiber.Ctx, name string) (int64, error) {
	id, err := strconv.ParseInt(c.Params(name), 10, 64)
	if err != nil || id <= 0 {
		logValidation(c, "handler.parseSubID", "invalid id", slog.String("param", name), slog.String("raw", c.Params(name)))
		return 0, httpapi.ErrValidation("invalid id")
	}
	return id, nil
}

// notFoundOrGoneTask classifies a missing parent task as 404 or 410 (tombstone).
func notFoundOrGoneTask(c fiber.Ctx, tasks *repo.TaskRepo, taskID int64, msg string) *httpapi.AppError {
	if appErr := mutationErr(tasks.NotFoundOrGone(c.Context(), taskID), msg); appErr != nil {
		return appErr
	}
	return httpapi.ErrNotFound(msg)
}

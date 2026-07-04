package handlers

import (
	"errors"
	"log/slog"
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
)

const (
	opContextCreate     = "handler.Context.Create"
	opContextPatch      = "handler.Context.Patch"
	opContextCreateTask = "handler.Context.CreateTask"
	msgGetContext       = "get context"
	msgInvalidStatus    = "invalid status"
)

// ContextHandler implements CRUD and sub-resource routes for /api/v1/contexts.
type ContextHandler struct {
	ctxs     *repo.ContextRepo
	projects *repo.ProjectRepo
	tasks    *repo.TaskRepo
	taskSvc  *service.TaskService
	baseURL  string
}

// NewContextHandler constructs a ContextHandler.
func NewContextHandler(ctxs *repo.ContextRepo, projects *repo.ProjectRepo, tasks *repo.TaskRepo, taskSvc *service.TaskService, baseURL string) *ContextHandler {
	return &ContextHandler{ctxs: ctxs, projects: projects, tasks: tasks, taskSvc: taskSvc, baseURL: baseURL}
}

// Register wires context routes onto r.
func (h *ContextHandler) Register(r fiber.Router) {
	r.Get("/", httpapi.RequireScope(auth.ScopeContextsRead), h.list)
	r.Post("/", httpapi.RequireScope(auth.ScopeContextsWrite), h.create)
	r.Get("/:id", httpapi.RequireScope(auth.ScopeContextsRead), h.get)
	r.Patch("/:id", httpapi.RequireScope(auth.ScopeContextsWrite), h.patch)
	r.Delete("/:id", httpapi.RequireScope(auth.ScopeContextsWrite), h.delete)
	r.Get("/:id/projects", httpapi.RequireScope(auth.ScopeProjectsRead), h.listProjects)
	r.Get("/:id/tasks", httpapi.RequireScope(auth.ScopeTasksRead), h.listTasks)
	r.Post("/:id/tasks", httpapi.RequireScope(auth.ScopeTasksWrite), h.createTask)
}

func (h *ContextHandler) list(c fiber.Ctx) error {
	pp := dto.ParsePageParams(c.Query("limit"), c.Query("offset"))
	items, total, err := h.ctxs.List(c.Context(), repo.Page{Limit: pp.Limit, Offset: pp.Offset})
	if err != nil {
		return httpapi.ErrInternal("list contexts").WithCause(err)
	}
	dtos := make([]dto.ContextDTO, len(items))
	for i, ctx := range items {
		dtos[i] = dto.ContextFromModel(ctx)
	}
	return c.JSON(dto.NewPagedResponse(dtos, total, pp.Limit, pp.Offset))
}

func (h *ContextHandler) create(c fiber.Ctx) error {
	logEntry(c, opContextCreate)
	var req dto.CreateContextRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, opContextCreate, msgInvalidBody)
		return httpapi.ErrValidation(msgInvalidRequestBody)
	}
	if req.Name == "" {
		logValidation(c, opContextCreate, "name required")
		return httpapi.ErrValidation("name is required")
	}
	if req.Color != "" && !isValidColor(req.Color) {
		logValidation(c, opContextCreate, msgInvalidColor)
		return httpapi.ErrValidation(msgInvalidColor)
	}
	ctx, err := h.ctxs.Create(c.Context(), req.Name, req.Color, req.IsFavourite)
	if err != nil {
		if errors.Is(err, repo.ErrConflict) {
			return httpapi.ErrConflict("context name already exists")
		}
		return httpapi.ErrInternal("create context").WithCause(err)
	}
	logMutation(c, opContextCreate, slog.Int64("context_id", ctx.ID))
	return c.Status(fiber.StatusCreated).JSON(dto.ContextFromModel(*ctx))
}

func (h *ContextHandler) get(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	ctx, err := h.ctxs.Get(c.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound(msgContextNotFound)
		}
		return httpapi.ErrInternal(msgGetContext).WithCause(err)
	}
	return c.JSON(dto.ContextFromModel(*ctx))
}

func (h *ContextHandler) patch(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, opContextPatch, slog.Int64("context_id", id))
	var req dto.PatchContextRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, opContextPatch, msgInvalidBody)
		return httpapi.ErrValidation(msgInvalidRequestBody)
	}
	if req.Color != nil && !isValidColor(*req.Color) {
		logValidation(c, opContextPatch, msgInvalidColor)
		return httpapi.ErrValidation(msgInvalidColor)
	}
	ctx, err := h.ctxs.Update(c.Context(), id, repo.ContextUpdate{
		Name:        req.Name,
		Color:       req.Color,
		IsFavourite: req.IsFavourite,
	})
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound(msgContextNotFound)
		}
		if errors.Is(err, repo.ErrConflict) {
			return httpapi.ErrConflict("context name already exists")
		}
		return httpapi.ErrInternal("update context").WithCause(err)
	}
	logMutation(c, opContextPatch, slog.Int64("context_id", ctx.ID))
	return c.JSON(dto.ContextFromModel(*ctx))
}

func (h *ContextHandler) delete(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.Context.Delete", slog.Int64("context_id", id))
	if err := h.ctxs.Delete(c.Context(), id); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound(msgContextNotFound)
		}
		return httpapi.ErrInternal("delete context").WithCause(err)
	}
	logMutation(c, "handler.Context.Delete", slog.Int64("context_id", id))
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *ContextHandler) listProjects(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	if _, err := h.ctxs.Get(c.Context(), id); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound(msgContextNotFound)
		}
		return httpapi.ErrInternal(msgGetContext).WithCause(err)
	}
	pp := dto.ParsePageParams(c.Query("limit"), c.Query("offset"))
	filter := repo.ProjectListFilter{ContextID: &id}
	if s := c.Query("status"); s != "" {
		ps := model.ProjectStatus(s)
		if !ps.IsValid() {
			logValidation(c, "handler.Context.ListProjects", msgInvalidStatus)
			return httpapi.ErrValidation(msgInvalidStatus)
		}
		filter.Status = &ps
	}
	items, total, err := h.projects.List(c.Context(), filter, repo.Page{Limit: pp.Limit, Offset: pp.Offset})
	if err != nil {
		return httpapi.ErrInternal("list projects").WithCause(err)
	}
	dtos := make([]dto.ProjectDTO, len(items))
	for i, p := range items {
		dtos[i] = dto.ProjectFromModel(p)
	}
	return c.JSON(dto.NewPagedResponse(dtos, total, pp.Limit, pp.Offset))
}

func (h *ContextHandler) listTasks(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	if _, err := h.ctxs.Get(c.Context(), id); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound(msgContextNotFound)
		}
		return httpapi.ErrInternal(msgGetContext).WithCause(err)
	}
	pp := dto.ParsePageParams(c.Query("limit"), c.Query("offset"))
	filter := repo.TaskFilter{}
	if s := c.Query("status"); s != "" {
		ts := model.TaskStatus(s)
		if !ts.IsValid() {
			return httpapi.ErrValidation(msgInvalidStatus)
		}
		filter.Status = &ts
	}
	if p := c.Query("priority"); p != "" {
		pr := model.Priority(p)
		if !pr.IsValid() {
			return httpapi.ErrValidation("invalid priority")
		}
		filter.Priority = &pr
	}
	if q := c.Query("q"); q != "" {
		filter.Query = q
	}
	if lid := c.Query("labelId"); lid != "" {
		n, err := strconv.ParseInt(lid, 10, 64)
		if err != nil {
			return httpapi.ErrValidation("invalid labelId")
		}
		filter.LabelID = &n
	}
	items, total, err := h.tasks.ListByContext(c.Context(), id, true, filter, repo.Page{Limit: pp.Limit, Offset: pp.Offset})
	if err != nil {
		return httpapi.ErrInternal("list tasks").WithCause(err)
	}
	dtos := make([]dto.TaskDTO, len(items))
	for i, t := range items {
		dtos[i] = dto.TaskFromModel(t, h.baseURL)
	}
	return c.JSON(dto.NewPagedResponse(dtos, total, pp.Limit, pp.Offset))
}

func (h *ContextHandler) createTask(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, opContextCreateTask, slog.Int64("context_id", id))
	if _, err := h.ctxs.Get(c.Context(), id); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound(msgContextNotFound)
		}
		return httpapi.ErrInternal(msgGetContext).WithCause(err)
	}
	var req dto.CreateTaskRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, opContextCreateTask, msgInvalidBody)
		return httpapi.ErrValidation(msgInvalidRequestBody)
	}
	if req.Title == "" {
		logValidation(c, opContextCreateTask, "title required")
		return httpapi.ErrValidation("title is required")
	}
	return doCreateTask(c, h.taskSvc, repo.Placement{ContextID: &id}, req, h.baseURL)
}

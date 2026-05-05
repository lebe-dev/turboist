package handlers

import (
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
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
	r.Get("/", h.list)
	r.Post("/", h.create)
	r.Get("/:id", h.get)
	r.Patch("/:id", h.patch)
	r.Delete("/:id", h.delete)
	r.Get("/:id/projects", h.listProjects)
	r.Get("/:id/tasks", h.listTasks)
	r.Post("/:id/tasks", h.createTask)
}

func (h *ContextHandler) list(c fiber.Ctx) error {
	pp := dto.ParsePageParams(c.Query("limit"), c.Query("offset"))
	items, total, err := h.ctxs.List(c.Context(), repo.Page{Limit: pp.Limit, Offset: pp.Offset})
	if err != nil {
		return httpapi.ErrInternal("list contexts")
	}
	dtos := make([]dto.ContextDTO, len(items))
	for i, ctx := range items {
		dtos[i] = dto.ContextFromModel(ctx)
	}
	return c.JSON(dto.NewPagedResponse(dtos, total, pp.Limit, pp.Offset))
}

func (h *ContextHandler) create(c fiber.Ctx) error {
	var req dto.CreateContextRequest
	if err := c.Bind().JSON(&req); err != nil {
		return httpapi.ErrValidation("invalid request body")
	}
	if req.Name == "" {
		return httpapi.ErrValidation("name is required")
	}
	if req.Color != "" && !isValidColor(req.Color) {
		return httpapi.ErrValidation("invalid color")
	}
	ctx, err := h.ctxs.Create(c.Context(), req.Name, req.Color, req.IsFavourite)
	if err != nil {
		if errors.Is(err, repo.ErrConflict) {
			return httpapi.ErrConflict("context name already exists")
		}
		return httpapi.ErrInternal("create context")
	}
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
			return httpapi.ErrNotFound("context not found")
		}
		return httpapi.ErrInternal("get context")
	}
	return c.JSON(dto.ContextFromModel(*ctx))
}

func (h *ContextHandler) patch(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	var req dto.PatchContextRequest
	if err := c.Bind().JSON(&req); err != nil {
		return httpapi.ErrValidation("invalid request body")
	}
	if req.Color != nil && !isValidColor(*req.Color) {
		return httpapi.ErrValidation("invalid color")
	}
	ctx, err := h.ctxs.Update(c.Context(), id, repo.ContextUpdate{
		Name:        req.Name,
		Color:       req.Color,
		IsFavourite: req.IsFavourite,
	})
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("context not found")
		}
		if errors.Is(err, repo.ErrConflict) {
			return httpapi.ErrConflict("context name already exists")
		}
		return httpapi.ErrInternal("update context")
	}
	return c.JSON(dto.ContextFromModel(*ctx))
}

func (h *ContextHandler) delete(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	if err := h.ctxs.Delete(c.Context(), id); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("context not found")
		}
		return httpapi.ErrInternal("delete context")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *ContextHandler) listProjects(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	if _, err := h.ctxs.Get(c.Context(), id); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("context not found")
		}
		return httpapi.ErrInternal("get context")
	}
	pp := dto.ParsePageParams(c.Query("limit"), c.Query("offset"))
	filter := repo.ProjectListFilter{ContextID: &id}
	if s := c.Query("status"); s != "" {
		ps := model.ProjectStatus(s)
		if !ps.IsValid() {
			return httpapi.ErrValidation("invalid status")
		}
		filter.Status = &ps
	}
	items, total, err := h.projects.List(c.Context(), filter, repo.Page{Limit: pp.Limit, Offset: pp.Offset})
	if err != nil {
		return httpapi.ErrInternal("list projects")
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
			return httpapi.ErrNotFound("context not found")
		}
		return httpapi.ErrInternal("get context")
	}
	pp := dto.ParsePageParams(c.Query("limit"), c.Query("offset"))
	filter := repo.TaskFilter{}
	if s := c.Query("status"); s != "" {
		ts := model.TaskStatus(s)
		if !ts.IsValid() {
			return httpapi.ErrValidation("invalid status")
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
		return httpapi.ErrInternal("list tasks")
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
	if _, err := h.ctxs.Get(c.Context(), id); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("context not found")
		}
		return httpapi.ErrInternal("get context")
	}
	var req dto.CreateTaskRequest
	if err := c.Bind().JSON(&req); err != nil {
		return httpapi.ErrValidation("invalid request body")
	}
	if req.Title == "" {
		return httpapi.ErrValidation("title is required")
	}
	return doCreateTask(c, h.taskSvc, repo.Placement{ContextID: &id}, req, h.baseURL)
}

package handlers

import (
	"errors"
	"log/slog"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/repo"
)

// LabelHandler implements CRUD and sub-resource routes for /api/v1/labels.
type LabelHandler struct {
	labels   *repo.LabelRepo
	projects *repo.ProjectRepo
	tasks    *repo.TaskRepo
	baseURL  string
}

// NewLabelHandler constructs a LabelHandler.
func NewLabelHandler(labels *repo.LabelRepo, projects *repo.ProjectRepo, tasks *repo.TaskRepo, baseURL string) *LabelHandler {
	return &LabelHandler{labels: labels, projects: projects, tasks: tasks, baseURL: baseURL}
}

// Register wires label routes onto r.
func (h *LabelHandler) Register(r fiber.Router) {
	r.Get("/", httpapi.RequireScope("labels:read"), h.list)
	r.Post("/", httpapi.RequireScope("labels:write"), h.create)
	r.Get("/:id", httpapi.RequireScope("labels:read"), h.get)
	r.Patch("/:id", httpapi.RequireScope("labels:write"), h.patch)
	r.Delete("/:id", httpapi.RequireScope("labels:write"), h.delete)
	r.Get("/:id/tasks", httpapi.RequireScope("tasks:read"), h.listTasks)
	r.Get("/:id/projects", httpapi.RequireScope("projects:read"), h.listProjects)
}

func (h *LabelHandler) list(c fiber.Ctx) error {
	pp := dto.ParsePageParams(c.Query("limit"), c.Query("offset"))
	filter := repo.LabelListFilter{Query: c.Query("q")}
	items, total, err := h.labels.List(c.Context(), filter, repo.Page{Limit: pp.Limit, Offset: pp.Offset})
	if err != nil {
		return httpapi.ErrInternal("list labels").WithCause(err)
	}
	dtos := make([]dto.LabelDTO, len(items))
	for i, l := range items {
		dtos[i] = dto.LabelFromModel(l)
	}
	return c.JSON(dto.NewPagedResponse(dtos, total, pp.Limit, pp.Offset))
}

func (h *LabelHandler) create(c fiber.Ctx) error {
	logEntry(c, "handler.Label.Create")
	var req dto.CreateLabelRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, "handler.Label.Create", "invalid body")
		return httpapi.ErrValidation("invalid request body")
	}
	if req.Name == "" {
		logValidation(c, "handler.Label.Create", "name required")
		return httpapi.ErrValidation("name is required")
	}
	if req.Color != "" && !isValidColor(req.Color) {
		logValidation(c, "handler.Label.Create", "invalid color")
		return httpapi.ErrValidation("invalid color")
	}
	l, err := h.labels.Create(c.Context(), req.Name, req.Color, req.IsFavourite)
	if err != nil {
		if errors.Is(err, repo.ErrConflict) {
			return httpapi.ErrConflict("label name already exists")
		}
		return httpapi.ErrInternal("create label").WithCause(err)
	}
	logMutation(c, "handler.Label.Create", slog.Int64("label_id", l.ID))
	return c.Status(fiber.StatusCreated).JSON(dto.LabelFromModel(*l))
}

func (h *LabelHandler) get(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	l, err := h.labels.Get(c.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("label not found")
		}
		return httpapi.ErrInternal("get label").WithCause(err)
	}
	return c.JSON(dto.LabelFromModel(*l))
}

func (h *LabelHandler) patch(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.Label.Patch", slog.Int64("label_id", id))
	var req dto.PatchLabelRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, "handler.Label.Patch", "invalid body")
		return httpapi.ErrValidation("invalid request body")
	}
	if req.Color != nil && !isValidColor(*req.Color) {
		logValidation(c, "handler.Label.Patch", "invalid color")
		return httpapi.ErrValidation("invalid color")
	}
	l, err := h.labels.Update(c.Context(), id, repo.LabelUpdate{
		Name:        req.Name,
		Color:       req.Color,
		IsFavourite: req.IsFavourite,
		IsPrivate:   req.IsPrivate,
	})
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("label not found")
		}
		if errors.Is(err, repo.ErrConflict) {
			return httpapi.ErrConflict("label name already exists")
		}
		return httpapi.ErrInternal("update label").WithCause(err)
	}
	logMutation(c, "handler.Label.Patch", slog.Int64("label_id", l.ID))
	return c.JSON(dto.LabelFromModel(*l))
}

func (h *LabelHandler) delete(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.Label.Delete", slog.Int64("label_id", id))
	if err := h.labels.Delete(c.Context(), id); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("label not found")
		}
		return httpapi.ErrInternal("delete label").WithCause(err)
	}
	logMutation(c, "handler.Label.Delete", slog.Int64("label_id", id))
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *LabelHandler) listTasks(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	if _, err := h.labels.Get(c.Context(), id); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("label not found")
		}
		return httpapi.ErrInternal("get label").WithCause(err)
	}
	pp := dto.ParsePageParams(c.Query("limit"), c.Query("offset"))
	items, total, err := h.tasks.ListByLabel(c.Context(), id, repo.TaskFilter{}, repo.Page{Limit: pp.Limit, Offset: pp.Offset})
	if err != nil {
		return httpapi.ErrInternal("list tasks by label").WithCause(err)
	}
	dtos := make([]dto.TaskDTO, len(items))
	for i, t := range items {
		dtos[i] = dto.TaskFromModel(t, h.baseURL)
	}
	return c.JSON(dto.NewPagedResponse(dtos, total, pp.Limit, pp.Offset))
}

func (h *LabelHandler) listProjects(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	if _, err := h.labels.Get(c.Context(), id); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("label not found")
		}
		return httpapi.ErrInternal("get label").WithCause(err)
	}
	pp := dto.ParsePageParams(c.Query("limit"), c.Query("offset"))
	items, total, err := h.projects.ListByLabel(c.Context(), id, repo.Page{Limit: pp.Limit, Offset: pp.Offset})
	if err != nil {
		return httpapi.ErrInternal("list projects by label").WithCause(err)
	}
	dtos := make([]dto.ProjectDTO, len(items))
	for i, p := range items {
		dtos[i] = dto.ProjectFromModel(p)
	}
	return c.JSON(dto.NewPagedResponse(dtos, total, pp.Limit, pp.Offset))
}

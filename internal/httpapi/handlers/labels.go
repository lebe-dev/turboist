package handlers

import (
	"errors"
	"log/slog"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/repo"
)

const (
	opLabelCreate    = "handler.Label.Create"
	opLabelPatch     = "handler.Label.Patch"
	msgLabelNotFound = "label not found"
	msgGetLabel      = "get label"
)

// LabelHandler implements CRUD and sub-resource routes for /api/v1/labels.
type LabelHandler struct {
	labels   *repo.LabelRepo
	projects *repo.ProjectRepo
	tasks    *repo.TaskRepo
	cfg      *config.Config
	baseURL  string
}

// NewLabelHandler constructs a LabelHandler. cfg supplies the timezone the
// usage-stats windows are anchored to.
func NewLabelHandler(labels *repo.LabelRepo, projects *repo.ProjectRepo, tasks *repo.TaskRepo, cfg *config.Config, baseURL string) *LabelHandler {
	return &LabelHandler{labels: labels, projects: projects, tasks: tasks, cfg: cfg, baseURL: baseURL}
}

// Register wires label routes onto r.
func (h *LabelHandler) Register(r fiber.Router) {
	r.Get("/", httpapi.RequireScope(auth.ScopeLabelsRead), h.list)
	r.Post("/", httpapi.RequireScope(auth.ScopeLabelsWrite), h.create)
	// Static before parameterized: /stats must not be swallowed by /:id.
	r.Get("/stats", httpapi.RequireScope(auth.ScopeLabelsRead), h.stats)
	r.Get("/:id", httpapi.RequireScope(auth.ScopeLabelsRead), h.get)
	r.Patch("/:id", httpapi.RequireScope(auth.ScopeLabelsWrite), h.patch)
	r.Delete("/:id", httpapi.RequireScope(auth.ScopeLabelsWrite), h.delete)
	r.Get("/:id/tasks", httpapi.RequireScope(auth.ScopeTasksRead), h.listTasks)
	r.Get("/:id/projects", httpapi.RequireScope(auth.ScopeProjectsRead), h.listProjects)
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
	logEntry(c, opLabelCreate)
	var req dto.CreateLabelRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, opLabelCreate, msgInvalidBody)
		return httpapi.ErrValidation(msgInvalidRequestBody)
	}
	if req.Name == "" {
		logValidation(c, opLabelCreate, "name required")
		return httpapi.ErrValidation("name is required")
	}
	if req.Color != "" && !isValidColor(req.Color) {
		logValidation(c, opLabelCreate, msgInvalidColor)
		return httpapi.ErrValidation(msgInvalidColor)
	}
	l, err := h.labels.Create(c.Context(), req.Name, req.Color, req.IsFavourite)
	if err != nil {
		if errors.Is(err, repo.ErrConflict) {
			return httpapi.ErrConflict("label name already exists")
		}
		return httpapi.ErrInternal("create label").WithCause(err)
	}
	logMutation(c, opLabelCreate, slog.Int64("label_id", l.ID))
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
			return httpapi.ErrNotFound(msgLabelNotFound)
		}
		return httpapi.ErrInternal(msgGetLabel).WithCause(err)
	}
	return c.JSON(dto.LabelFromModel(*l))
}

func (h *LabelHandler) patch(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, opLabelPatch, slog.Int64("label_id", id))
	var req dto.PatchLabelRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, opLabelPatch, msgInvalidBody)
		return httpapi.ErrValidation(msgInvalidRequestBody)
	}
	if req.Color != nil && !isValidColor(*req.Color) {
		logValidation(c, opLabelPatch, msgInvalidColor)
		return httpapi.ErrValidation(msgInvalidColor)
	}
	l, err := h.labels.Update(c.Context(), id, repo.LabelUpdate{
		Name:        req.Name,
		Color:       req.Color,
		IsFavourite: req.IsFavourite,
		IsPrivate:   req.IsPrivate,
	})
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound(msgLabelNotFound)
		}
		if errors.Is(err, repo.ErrConflict) {
			return httpapi.ErrConflict("label name already exists")
		}
		return httpapi.ErrInternal("update label").WithCause(err)
	}
	logMutation(c, opLabelPatch, slog.Int64("label_id", l.ID))
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
			return httpapi.ErrNotFound(msgLabelNotFound)
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
			return httpapi.ErrNotFound(msgLabelNotFound)
		}
		return httpapi.ErrInternal(msgGetLabel).WithCause(err)
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
			return httpapi.ErrNotFound(msgLabelNotFound)
		}
		return httpapi.ErrInternal(msgGetLabel).WithCause(err)
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

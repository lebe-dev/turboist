package handlers

import (
	"errors"
	"log/slog"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
)

const (
	opSectionReorder    = "handler.Section.Reorder"
	opSectionPatch      = "handler.Section.Patch"
	opSectionCreateTask = "handler.Section.CreateTask"
	msgSectionNotFound  = "section not found"
	msgGetSection       = "get section"
)

// SectionHandler implements routes for /api/v1/sections/:id.
// Section creation (POST /projects/:id/sections) is in ProjectHandler (Task 10).
type SectionHandler struct {
	sections *repo.ProjectSectionRepo
	projects *repo.ProjectRepo
	tasks    *repo.TaskRepo
	taskSvc  *service.TaskService
	baseURL  string
}

// NewSectionHandler constructs a SectionHandler.
func NewSectionHandler(sections *repo.ProjectSectionRepo, projects *repo.ProjectRepo, tasks *repo.TaskRepo, taskSvc *service.TaskService, baseURL string) *SectionHandler {
	return &SectionHandler{sections: sections, projects: projects, tasks: tasks, taskSvc: taskSvc, baseURL: baseURL}
}

// Register wires section routes onto r.
func (h *SectionHandler) Register(r fiber.Router) {
	r.Get("/:id", httpapi.RequireScope(auth.ScopeSectionsRead), h.get)
	r.Patch("/:id", httpapi.RequireScope(auth.ScopeSectionsWrite), h.patch)
	r.Delete("/:id", httpapi.RequireScope(auth.ScopeSectionsWrite), h.delete)
	r.Get("/:id/tasks", httpapi.RequireScope(auth.ScopeTasksRead), h.listTasks)
	r.Post("/:id/tasks", httpapi.RequireScope(auth.ScopeTasksWrite), h.createTask)
	r.Post("/:id/reorder", httpapi.RequireScope(auth.ScopeSectionsWrite), h.reorder)
}

func (h *SectionHandler) reorder(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, opSectionReorder, slog.Int64("section_id", id))
	var req dto.ReorderSectionRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, opSectionReorder, msgInvalidBody)
		return httpapi.ErrValidation(msgInvalidRequestBody)
	}
	if req.Position < 0 {
		logValidation(c, opSectionReorder, "negative position", slog.Int("position", req.Position))
		return httpapi.ErrValidation("position must be non-negative")
	}
	s, err := h.sections.Reorder(c.Context(), id, req.Position)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound(msgSectionNotFound)
		}
		return httpapi.ErrInternal("reorder section").WithCause(err)
	}
	logMutation(c, opSectionReorder, slog.Int64("section_id", s.ID), slog.Int("position", req.Position))
	return c.JSON(dto.SectionFromModel(*s))
}

func (h *SectionHandler) get(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	s, err := h.sections.Get(c.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound(msgSectionNotFound)
		}
		return httpapi.ErrInternal(msgGetSection).WithCause(err)
	}
	return c.JSON(dto.SectionFromModel(*s))
}

func (h *SectionHandler) patch(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, opSectionPatch, slog.Int64("section_id", id))
	var req dto.PatchSectionRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, opSectionPatch, msgInvalidBody)
		return httpapi.ErrValidation(msgInvalidRequestBody)
	}
	s, err := h.sections.Update(c.Context(), id, repo.SectionUpdate{Title: req.Title})
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound(msgSectionNotFound)
		}
		return httpapi.ErrInternal("update section").WithCause(err)
	}
	logMutation(c, opSectionPatch, slog.Int64("section_id", s.ID))
	return c.JSON(dto.SectionFromModel(*s))
}

func (h *SectionHandler) delete(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.Section.Delete", slog.Int64("section_id", id))
	if err := h.sections.Delete(c.Context(), id); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound(msgSectionNotFound)
		}
		return httpapi.ErrInternal("delete section").WithCause(err)
	}
	logMutation(c, "handler.Section.Delete", slog.Int64("section_id", id))
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *SectionHandler) listTasks(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	if _, err := h.sections.Get(c.Context(), id); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound(msgSectionNotFound)
		}
		return httpapi.ErrInternal(msgGetSection).WithCause(err)
	}
	pp := dto.ParsePageParams(c.Query("limit"), c.Query("offset"))
	items, total, err := h.tasks.ListBySection(c.Context(), id, repo.TaskFilter{}, repo.Page{Limit: pp.Limit, Offset: pp.Offset})
	if err != nil {
		return httpapi.ErrInternal("list tasks by section").WithCause(err)
	}
	dtos := make([]dto.TaskDTO, len(items))
	for i, t := range items {
		dtos[i] = dto.TaskFromModel(t, h.baseURL)
	}
	return c.JSON(dto.NewPagedResponse(dtos, total, pp.Limit, pp.Offset))
}

func (h *SectionHandler) createTask(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, opSectionCreateTask, slog.Int64("section_id", id))
	sec, err := h.sections.Get(c.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound(msgSectionNotFound)
		}
		return httpapi.ErrInternal(msgGetSection).WithCause(err)
	}
	proj, err := h.projects.Get(c.Context(), sec.ProjectID)
	if err != nil {
		return httpapi.ErrInternal("get project for section").WithCause(err)
	}
	var req dto.CreateTaskRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, opSectionCreateTask, msgInvalidBody)
		return httpapi.ErrValidation(msgInvalidRequestBody)
	}
	if req.Title == "" {
		logValidation(c, opSectionCreateTask, "title required")
		return httpapi.ErrValidation("title is required")
	}
	placement := repo.Placement{
		ContextID: &proj.ContextID,
		ProjectID: &proj.ID,
		SectionID: &id,
	}
	return doCreateTask(c, h.taskSvc, placement, req, h.baseURL)
}

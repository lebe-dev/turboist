package handlers

import (
	"errors"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
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
	r.Get("/:id", h.get)
	r.Patch("/:id", h.patch)
	r.Delete("/:id", h.delete)
	r.Get("/:id/tasks", h.listTasks)
	r.Post("/:id/tasks", h.createTask)
	r.Post("/:id/reorder", h.reorder)
}

func (h *SectionHandler) reorder(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	var req dto.ReorderSectionRequest
	if err := c.Bind().JSON(&req); err != nil {
		return httpapi.ErrValidation("invalid request body")
	}
	if req.Position < 0 {
		return httpapi.ErrValidation("position must be non-negative")
	}
	s, err := h.sections.Reorder(c.Context(), id, req.Position)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("section not found")
		}
		return httpapi.ErrInternal("reorder section")
	}
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
			return httpapi.ErrNotFound("section not found")
		}
		return httpapi.ErrInternal("get section")
	}
	return c.JSON(dto.SectionFromModel(*s))
}

func (h *SectionHandler) patch(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	var req dto.PatchSectionRequest
	if err := c.Bind().JSON(&req); err != nil {
		return httpapi.ErrValidation("invalid request body")
	}
	s, err := h.sections.Update(c.Context(), id, repo.SectionUpdate{Title: req.Title})
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("section not found")
		}
		return httpapi.ErrInternal("update section")
	}
	return c.JSON(dto.SectionFromModel(*s))
}

func (h *SectionHandler) delete(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	if err := h.sections.Delete(c.Context(), id); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("section not found")
		}
		return httpapi.ErrInternal("delete section")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *SectionHandler) listTasks(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	if _, err := h.sections.Get(c.Context(), id); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("section not found")
		}
		return httpapi.ErrInternal("get section")
	}
	pp := dto.ParsePageParams(c.Query("limit"), c.Query("offset"))
	items, total, err := h.tasks.ListBySection(c.Context(), id, repo.TaskFilter{}, repo.Page{Limit: pp.Limit, Offset: pp.Offset})
	if err != nil {
		return httpapi.ErrInternal("list tasks by section")
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
	sec, err := h.sections.Get(c.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("section not found")
		}
		return httpapi.ErrInternal("get section")
	}
	proj, err := h.projects.Get(c.Context(), sec.ProjectID)
	if err != nil {
		return httpapi.ErrInternal("get project for section")
	}
	var req dto.CreateTaskRequest
	if err := c.Bind().JSON(&req); err != nil {
		return httpapi.ErrValidation("invalid request body")
	}
	if req.Title == "" {
		return httpapi.ErrValidation("title is required")
	}
	placement := repo.Placement{
		ContextID: &proj.ContextID,
		ProjectID: &proj.ID,
		SectionID: &id,
	}
	return doCreateTask(c, h.taskSvc, placement, req, h.baseURL)
}

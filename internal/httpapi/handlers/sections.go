package handlers

import (
	"errors"
	"log/slog"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

// SectionHandler implements routes for /api/v1/sections/:id.
// Section creation (POST /projects/:id/sections) is in ProjectHandler (Task 10).
type SectionHandler struct {
	sections *repo.ProjectSectionRepo
	projects *repo.ProjectRepo
	tasks    *repo.TaskRepo
	taskSvc  *service.TaskService
	baseURL  string

	// fedSections routes a patch/delete of a section in a FEDERATED project through
	// the federation Emitter so it emits the per-field HLC bump + signed outbox
	// event (US-3.2 AC1). nil when federation is off — the handler then uses the
	// plain repo path.
	fedSections *fedsvc.SectionMutator

	// fedGuard rejects a section mutation in a read-only federated project with 403
	// federation_read_only (Federation v1 F5.2, US-5.1 AC4). nil/unwired is a
	// no-op so the single-user path is untouched.
	fedGuard *FederationReadOnlyGuard
}

// NewSectionHandler constructs a SectionHandler.
func NewSectionHandler(sections *repo.ProjectSectionRepo, projects *repo.ProjectRepo, tasks *repo.TaskRepo, taskSvc *service.TaskService, baseURL string) *SectionHandler {
	return &SectionHandler{sections: sections, projects: projects, tasks: tasks, taskSvc: taskSvc, baseURL: baseURL}
}

// WithFederation wires the section federation mutator so a patch/delete of a
// section in a federated project emits through the Emitter (US-3.2 AC1). Returns
// the handler for chaining. A nil mutator leaves the handler on the repo path.
func (h *SectionHandler) WithFederation(m *fedsvc.SectionMutator) *SectionHandler {
	h.fedSections = m
	return h
}

// WithFederationGuard wires the read-only federated-project guard so every
// section mutation entry point rejects an edit of a section in a read-only
// federated project with 403 (Federation v1 F5.2, US-5.1 AC4). Returns the
// handler for chaining.
func (h *SectionHandler) WithFederationGuard(g *FederationReadOnlyGuard) *SectionHandler {
	h.fedGuard = g
	return h
}

// Register wires section routes onto r.
func (h *SectionHandler) Register(r fiber.Router) {
	r.Get("/:id", httpapi.RequireScope("sections:read"), h.get)
	r.Patch("/:id", httpapi.RequireScope("sections:write"), h.patch)
	r.Delete("/:id", httpapi.RequireScope("sections:write"), h.delete)
	r.Get("/:id/tasks", httpapi.RequireScope("tasks:read"), h.listTasks)
	r.Post("/:id/tasks", httpapi.RequireScope("tasks:write"), h.createTask)
	r.Post("/:id/reorder", httpapi.RequireScope("sections:write"), h.reorder)
}

func (h *SectionHandler) reorder(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.Section.Reorder", slog.Int64("section_id", id))
	if appErr := h.fedGuard.GuardSection(c, id); appErr != nil {
		return appErr
	}
	var req dto.ReorderSectionRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, "handler.Section.Reorder", "invalid body")
		return httpapi.ErrValidation("invalid request body")
	}
	if req.Position < 0 {
		logValidation(c, "handler.Section.Reorder", "negative position", slog.Int("position", req.Position))
		return httpapi.ErrValidation("position must be non-negative")
	}
	s, err := h.sections.Reorder(c.Context(), id, req.Position)
	if err != nil {
		if appErr := mutationErr(err, "section not found"); appErr != nil {
			return appErr
		}
		return httpapi.ErrInternal("reorder section").WithCause(err)
	}
	logMutation(c, "handler.Section.Reorder", slog.Int64("section_id", s.ID), slog.Int("position", req.Position))
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
		return httpapi.ErrInternal("get section").WithCause(err)
	}
	return c.JSON(dto.SectionFromModel(*s))
}

func (h *SectionHandler) patch(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.Section.Patch", slog.Int64("section_id", id))
	if appErr := h.fedGuard.GuardSection(c, id); appErr != nil {
		return appErr
	}
	var req dto.PatchSectionRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, "handler.Section.Patch", "invalid body")
		return httpapi.ErrValidation("invalid request body")
	}
	update := repo.SectionUpdate{Title: req.Title}
	// Federation-on: load the section so the mutator can emit a signed op=update
	// event for a section in a federated project (US-3.2 AC1). Federation-off keeps
	// the direct repo update.
	var s *model.ProjectSection
	if h.fedSections != nil {
		sec, err := h.sections.Get(c.Context(), id)
		if err != nil {
			if appErr := mutationErr(h.sections.NotFoundOrGone(c.Context(), id), "section not found"); appErr != nil {
				return appErr
			}
			return httpapi.ErrInternal("get section").WithCause(err)
		}
		if uerr := h.fedSections.Update(c.Context(), sec, update); uerr != nil {
			if appErr := mutationErr(uerr, "section not found"); appErr != nil {
				return appErr
			}
			return httpapi.ErrInternal("update section").WithCause(uerr)
		}
		s, err = h.sections.Get(c.Context(), id)
		if err != nil {
			return httpapi.ErrInternal("get section").WithCause(err)
		}
	} else {
		var err error
		s, err = h.sections.Update(c.Context(), id, update)
		if err != nil {
			if appErr := mutationErr(err, "section not found"); appErr != nil {
				return appErr
			}
			return httpapi.ErrInternal("update section").WithCause(err)
		}
	}
	logMutation(c, "handler.Section.Patch", slog.Int64("section_id", s.ID))
	return c.JSON(dto.SectionFromModel(*s))
}

func (h *SectionHandler) delete(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.Section.Delete", slog.Int64("section_id", id))
	if appErr := h.fedGuard.GuardSection(c, id); appErr != nil {
		return appErr
	}
	// Federation-on: load the section so the mutator can emit an op=delete tombstone
	// for a section in a federated project (US-3.2 AC1). Federation-off keeps the
	// direct repo delete.
	if h.fedSections != nil {
		sec, err := h.sections.Get(c.Context(), id)
		if err != nil {
			if appErr := mutationErr(h.sections.NotFoundOrGone(c.Context(), id), "section not found"); appErr != nil {
				return appErr
			}
			return httpapi.ErrInternal("get section").WithCause(err)
		}
		if derr := h.fedSections.Delete(c.Context(), sec); derr != nil {
			if appErr := mutationErr(derr, "section not found"); appErr != nil {
				return appErr
			}
			return httpapi.ErrInternal("delete section").WithCause(derr)
		}
		logMutation(c, "handler.Section.Delete", slog.Int64("section_id", id))
		return c.SendStatus(fiber.StatusNoContent)
	}
	if err := h.sections.Delete(c.Context(), id); err != nil {
		if appErr := mutationErr(err, "section not found"); appErr != nil {
			return appErr
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
			return httpapi.ErrNotFound("section not found")
		}
		return httpapi.ErrInternal("get section").WithCause(err)
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
	logEntry(c, "handler.Section.CreateTask", slog.Int64("section_id", id))
	sec, err := h.sections.Get(c.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("section not found")
		}
		return httpapi.ErrInternal("get section").WithCause(err)
	}
	if appErr := h.fedGuard.GuardProject(c, sec.ProjectID); appErr != nil {
		return appErr
	}
	proj, err := h.projects.Get(c.Context(), sec.ProjectID)
	if err != nil {
		return httpapi.ErrInternal("get project for section").WithCause(err)
	}
	var req dto.CreateTaskRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, "handler.Section.CreateTask", "invalid body")
		return httpapi.ErrValidation("invalid request body")
	}
	if req.Title == "" {
		logValidation(c, "handler.Section.CreateTask", "title required")
		return httpapi.ErrValidation("title is required")
	}
	placement := repo.Placement{
		ContextID: &proj.ContextID,
		ProjectID: &proj.ID,
		SectionID: &id,
	}
	return doCreateTask(c, h.taskSvc, placement, req, h.baseURL)
}

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

// ProjectHandler implements routes for projects, including creation via /contexts/:id/projects.
type ProjectHandler struct {
	projects *repo.ProjectRepo
	sections *repo.ProjectSectionRepo
	tasks    *repo.TaskRepo
	taskSvc  *service.TaskService
	labels   *repo.LabelRepo
	contexts *repo.ContextRepo
	pinSvc   *service.PinService
	baseURL  string
}

func NewProjectHandler(
	projects *repo.ProjectRepo,
	sections *repo.ProjectSectionRepo,
	tasks *repo.TaskRepo,
	taskSvc *service.TaskService,
	labels *repo.LabelRepo,
	contexts *repo.ContextRepo,
	pinSvc *service.PinService,
	baseURL string,
) *ProjectHandler {
	return &ProjectHandler{
		projects: projects,
		sections: sections,
		tasks:    tasks,
		taskSvc:  taskSvc,
		labels:   labels,
		contexts: contexts,
		pinSvc:   pinSvc,
		baseURL:  baseURL,
	}
}

// Register wires all project-related routes onto r (expected to be the /api/v1 group).
func (h *ProjectHandler) Register(r fiber.Router) {
	p := r.Group("/projects")
	p.Get("/", h.list)
	p.Get("/:id", h.get)
	p.Patch("/:id", h.patch)
	p.Delete("/:id", h.delete)
	p.Get("/:id/sections", h.listSections)
	p.Post("/:id/sections", h.createSection)
	p.Get("/:id/tasks", h.listTasks)
	p.Post("/:id/tasks", h.createTask)
	p.Post("/:id/complete", h.complete)
	p.Post("/:id/uncomplete", h.uncomplete)
	p.Post("/:id/cancel", h.cancel)
	p.Post("/:id/archive", h.archive)
	p.Post("/:id/unarchive", h.unarchive)
	p.Post("/:id/pin", h.pin)
	p.Post("/:id/unpin", h.unpin)

	r.Post("/contexts/:id/projects", h.createForContext)
}

func (h *ProjectHandler) list(c fiber.Ctx) error {
	pp := dto.ParsePageParams(c.Query("limit"), c.Query("offset"))
	filter := repo.ProjectListFilter{}
	if cid := c.Query("contextId"); cid != "" {
		n, err := strconv.ParseInt(cid, 10, 64)
		if err != nil || n <= 0 {
			return httpapi.ErrValidation("invalid contextId")
		}
		filter.ContextID = &n
	}
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

func (h *ProjectHandler) get(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	p, err := h.projects.Get(c.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("project not found")
		}
		return httpapi.ErrInternal("get project")
	}
	return c.JSON(dto.ProjectFromModel(*p))
}

func (h *ProjectHandler) createForContext(c fiber.Ctx) error {
	contextID, err := parseID(c)
	if err != nil {
		return err
	}
	if _, err := h.contexts.Get(c.Context(), contextID); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("context not found")
		}
		return httpapi.ErrInternal("get context")
	}
	var req dto.CreateProjectRequest
	if err := c.Bind().JSON(&req); err != nil {
		return httpapi.ErrValidation("invalid request body")
	}
	if req.Title == "" {
		return httpapi.ErrValidation("title is required")
	}
	if req.Color != "" && !isValidColor(req.Color) {
		return httpapi.ErrValidation("invalid color")
	}
	labelIDs, appErr := h.resolveLabels(c, req.Labels)
	if appErr != nil {
		return appErr
	}
	p, err := h.projects.Create(c.Context(), repo.CreateProject{
		ContextID:   contextID,
		Title:       req.Title,
		Description: req.Description,
		Color:       req.Color,
	})
	if err != nil {
		return httpapi.ErrInternal("create project")
	}
	if len(labelIDs) > 0 {
		if err := h.projects.SetLabels(c.Context(), p.ID, labelIDs); err != nil {
			return httpapi.ErrInternal("set project labels")
		}
		p, err = h.projects.Get(c.Context(), p.ID)
		if err != nil {
			return httpapi.ErrInternal("get project")
		}
	}
	return c.Status(fiber.StatusCreated).JSON(dto.ProjectFromModel(*p))
}

func (h *ProjectHandler) patch(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	var req dto.PatchProjectRequest
	if err := c.Bind().JSON(&req); err != nil {
		return httpapi.ErrValidation("invalid request body")
	}
	if req.Color != nil && !isValidColor(*req.Color) {
		return httpapi.ErrValidation("invalid color")
	}
	p, err := h.projects.Update(c.Context(), id, repo.ProjectUpdate{
		Title:       req.Title,
		Description: req.Description,
		Color:       req.Color,
	})
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("project not found")
		}
		return httpapi.ErrInternal("update project")
	}
	if req.Labels != nil {
		labelIDs, appErr := h.resolveLabels(c, *req.Labels)
		if appErr != nil {
			return appErr
		}
		if err := h.projects.SetLabels(c.Context(), id, labelIDs); err != nil {
			return httpapi.ErrInternal("set project labels")
		}
		p, err = h.projects.Get(c.Context(), id)
		if err != nil {
			return httpapi.ErrInternal("get project")
		}
	}
	return c.JSON(dto.ProjectFromModel(*p))
}

func (h *ProjectHandler) delete(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	if err := h.projects.Delete(c.Context(), id); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("project not found")
		}
		return httpapi.ErrInternal("delete project")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *ProjectHandler) listSections(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	if _, err := h.projects.Get(c.Context(), id); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("project not found")
		}
		return httpapi.ErrInternal("get project")
	}
	pp := dto.ParsePageParams(c.Query("limit"), c.Query("offset"))
	items, total, err := h.sections.ListByProject(c.Context(), id, repo.Page{Limit: pp.Limit, Offset: pp.Offset})
	if err != nil {
		return httpapi.ErrInternal("list sections")
	}
	dtos := make([]dto.SectionDTO, len(items))
	for i, s := range items {
		dtos[i] = dto.SectionFromModel(s)
	}
	return c.JSON(dto.NewPagedResponse(dtos, total, pp.Limit, pp.Offset))
}

func (h *ProjectHandler) createSection(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	if _, err := h.projects.Get(c.Context(), id); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("project not found")
		}
		return httpapi.ErrInternal("get project")
	}
	var req dto.CreateSectionRequest
	if err := c.Bind().JSON(&req); err != nil {
		return httpapi.ErrValidation("invalid request body")
	}
	if req.Title == "" {
		return httpapi.ErrValidation("title is required")
	}
	s, err := h.sections.Create(c.Context(), id, req.Title)
	if err != nil {
		return httpapi.ErrInternal("create section")
	}
	return c.Status(fiber.StatusCreated).JSON(dto.SectionFromModel(*s))
}

func (h *ProjectHandler) listTasks(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	if _, err := h.projects.Get(c.Context(), id); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("project not found")
		}
		return httpapi.ErrInternal("get project")
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
	if pr := c.Query("priority"); pr != "" {
		prio := model.Priority(pr)
		if !prio.IsValid() {
			return httpapi.ErrValidation("invalid priority")
		}
		filter.Priority = &prio
	}
	if lid := c.Query("labelId"); lid != "" {
		n, err := strconv.ParseInt(lid, 10, 64)
		if err != nil {
			return httpapi.ErrValidation("invalid labelId")
		}
		filter.LabelID = &n
	}
	items, total, err := h.tasks.ListByProject(c.Context(), id, filter, repo.Page{Limit: pp.Limit, Offset: pp.Offset})
	if err != nil {
		return httpapi.ErrInternal("list tasks")
	}
	dtos := make([]dto.TaskDTO, len(items))
	for i, t := range items {
		dtos[i] = dto.TaskFromModel(t, h.baseURL)
	}
	return c.JSON(dto.NewPagedResponse(dtos, total, pp.Limit, pp.Offset))
}

func (h *ProjectHandler) createTask(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	p, err := h.projects.Get(c.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("project not found")
		}
		return httpapi.ErrInternal("get project")
	}
	var req dto.CreateTaskRequest
	if err := c.Bind().JSON(&req); err != nil {
		return httpapi.ErrValidation("invalid request body")
	}
	if req.Title == "" {
		return httpapi.ErrValidation("title is required")
	}
	placement := repo.Placement{
		ContextID: &p.ContextID,
		ProjectID: &p.ID,
	}
	return doCreateTask(c, h.taskSvc, placement, req, h.baseURL)
}

func (h *ProjectHandler) complete(c fiber.Ctx) error {
	return h.setStatus(c, model.ProjectStatusCompleted)
}

func (h *ProjectHandler) uncomplete(c fiber.Ctx) error {
	return h.setStatus(c, model.ProjectStatusOpen)
}

func (h *ProjectHandler) cancel(c fiber.Ctx) error {
	return h.setStatus(c, model.ProjectStatusCancelled)
}

func (h *ProjectHandler) archive(c fiber.Ctx) error {
	return h.setStatus(c, model.ProjectStatusArchived)
}

func (h *ProjectHandler) unarchive(c fiber.Ctx) error {
	return h.setStatus(c, model.ProjectStatusOpen)
}

func (h *ProjectHandler) setStatus(c fiber.Ctx, status model.ProjectStatus) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	if err := h.projects.UpdateStatus(c.Context(), id, status); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("project not found")
		}
		return httpapi.ErrInternal("update project status")
	}
	p, err := h.projects.Get(c.Context(), id)
	if err != nil {
		return httpapi.ErrInternal("get project")
	}
	return c.JSON(dto.ProjectFromModel(*p))
}

func (h *ProjectHandler) pin(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	p, err := h.projects.Get(c.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("project not found")
		}
		return httpapi.ErrInternal("get project")
	}
	if p.Status != model.ProjectStatusOpen {
		return httpapi.ErrValidation("only open projects can be pinned")
	}
	if err := h.pinSvc.PinProject(c.Context(), id); err != nil {
		if errors.Is(err, service.ErrPinLimitExceeded) {
			return httpapi.ErrLimitExceeded("max pinned projects limit reached")
		}
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("project not found")
		}
		return httpapi.ErrInternal("pin project")
	}
	p, err = h.projects.Get(c.Context(), id)
	if err != nil {
		return httpapi.ErrInternal("get project")
	}
	return c.JSON(dto.ProjectFromModel(*p))
}

func (h *ProjectHandler) unpin(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	if err := h.pinSvc.UnpinProject(c.Context(), id); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("project not found")
		}
		return httpapi.ErrInternal("unpin project")
	}
	p, err := h.projects.Get(c.Context(), id)
	if err != nil {
		return httpapi.ErrInternal("get project")
	}
	return c.JSON(dto.ProjectFromModel(*p))
}

func (h *ProjectHandler) resolveLabels(c fiber.Ctx, names []string) ([]int64, *httpapi.AppError) {
	ids := make([]int64, 0, len(names))
	for _, name := range names {
		l, err := h.labels.GetByName(c.Context(), name)
		if err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				return nil, httpapi.ErrValidation("unknown label: " + name)
			}
			return nil, httpapi.ErrInternal("resolve label")
		}
		ids = append(ids, l.ID)
	}
	return ids, nil
}

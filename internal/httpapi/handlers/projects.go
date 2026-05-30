package handlers

import (
	"errors"
	"log/slog"
	"strconv"

	"github.com/gofiber/fiber/v3"
	"golang.org/x/sync/errgroup"

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
	p.Get("/", httpapi.RequireScope("projects:read"), h.list)
	p.Get("/:id", httpapi.RequireScope("projects:read"), h.get)
	p.Get("/:id/bundle", httpapi.RequireAllScopes("projects:read", "sections:read", "tasks:read"), h.bundle)
	p.Patch("/:id", httpapi.RequireScope("projects:write"), h.patch)
	p.Delete("/:id", httpapi.RequireScope("projects:write"), h.delete)
	p.Get("/:id/sections", httpapi.RequireScope("sections:read"), h.listSections)
	p.Post("/:id/sections", httpapi.RequireScope("sections:write"), h.createSection)
	p.Get("/:id/tasks", httpapi.RequireScope("tasks:read"), h.listTasks)
	p.Post("/:id/tasks", httpapi.RequireScope("tasks:write"), h.createTask)
	p.Post("/:id/complete", httpapi.RequireScope("projects:write"), h.complete)
	p.Post("/:id/uncomplete", httpapi.RequireScope("projects:write"), h.uncomplete)
	p.Post("/:id/cancel", httpapi.RequireScope("projects:write"), h.cancel)
	p.Post("/:id/archive", httpapi.RequireScope("projects:write"), h.archive)
	p.Post("/:id/unarchive", httpapi.RequireScope("projects:write"), h.unarchive)
	p.Post("/:id/pin", httpapi.RequireScope("projects:write"), h.pin)
	p.Post("/:id/unpin", httpapi.RequireScope("projects:write"), h.unpin)

	r.Post("/contexts/:id/projects", httpapi.RequireScope("projects:write"), h.createForContext)
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
		return httpapi.ErrInternal("list projects").WithCause(err)
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
		return httpapi.ErrInternal("get project").WithCause(err)
	}
	return c.JSON(dto.ProjectFromModel(*p))
}

// projectBundleResponse groups everything the project page needs — the project
// itself, its sections and all its tasks (subtasks included, flattened) — into
// a single response. Previously the page issued three parallel requests
// (project + sections + tasks); now it is one round-trip, mirroring the
// today/sidebar bundles.
type projectBundleResponse struct {
	Project  dto.ProjectDTO                    `json:"project"`
	Sections dto.PagedResponse[dto.SectionDTO] `json:"sections"`
	Tasks    dto.PagedResponse[dto.TaskDTO]    `json:"tasks"`
}

// projectBundleSectionLimit / projectBundleTaskLimit mirror the limits the
// frontend used when it called the separate list endpoints (sections?limit=200,
// tasks?limit=500). They cover any realistic single project.
const (
	projectBundleSectionLimit = 200
	projectBundleTaskLimit    = 500
)

func (h *ProjectHandler) bundle(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	p, err := h.projects.Get(c.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("project not found")
		}
		return httpapi.ErrInternal("get project").WithCause(err)
	}

	var (
		sectionsResp dto.PagedResponse[dto.SectionDTO]
		tasksResp    dto.PagedResponse[dto.TaskDTO]
	)

	g, gctx := errgroup.WithContext(c.Context())

	g.Go(func() error {
		items, total, err := h.sections.ListByProject(gctx, id, repo.Page{Limit: projectBundleSectionLimit})
		if err != nil {
			return err
		}
		dtos := make([]dto.SectionDTO, len(items))
		for i, s := range items {
			dtos[i] = dto.SectionFromModel(s)
		}
		sectionsResp = dto.NewPagedResponse(dtos, total, projectBundleSectionLimit, 0)
		return nil
	})

	g.Go(func() error {
		items, total, err := h.tasks.ListByProject(gctx, id, repo.TaskFilter{}, repo.Page{Limit: projectBundleTaskLimit})
		if err != nil {
			return err
		}
		tasksResp = dto.NewPagedResponse(tasksToDTO(items, h.baseURL), total, projectBundleTaskLimit, 0)
		return nil
	})

	if err := g.Wait(); err != nil {
		return httpapi.ErrInternal("load project bundle").WithCause(err)
	}

	return c.JSON(projectBundleResponse{
		Project:  dto.ProjectFromModel(*p),
		Sections: sectionsResp,
		Tasks:    tasksResp,
	})
}

func (h *ProjectHandler) createForContext(c fiber.Ctx) error {
	contextID, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.Project.Create", slog.Int64("context_id", contextID))
	if _, err := h.contexts.Get(c.Context(), contextID); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("context not found")
		}
		return httpapi.ErrInternal("get context").WithCause(err)
	}
	var req dto.CreateProjectRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, "handler.Project.Create", "invalid body")
		return httpapi.ErrValidation("invalid request body")
	}
	if req.Title == "" {
		logValidation(c, "handler.Project.Create", "title required")
		return httpapi.ErrValidation("title is required")
	}
	if req.Color != "" && !isValidColor(req.Color) {
		logValidation(c, "handler.Project.Create", "invalid color")
		return httpapi.ErrValidation("invalid color")
	}
	projectType := model.ProjectTypeGeneric
	if req.ProjectType != "" {
		pt := model.ProjectType(req.ProjectType)
		if !pt.IsValid() {
			logValidation(c, "handler.Project.Create", "invalid projectType")
			return httpapi.ErrValidation("invalid projectType")
		}
		projectType = pt
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
		Type:        projectType,
	})
	if err != nil {
		return httpapi.ErrInternal("create project").WithCause(err)
	}
	if len(labelIDs) > 0 {
		if err := h.projects.SetLabels(c.Context(), p.ID, labelIDs); err != nil {
			return httpapi.ErrInternal("set project labels").WithCause(err)
		}
		p, err = h.projects.Get(c.Context(), p.ID)
		if err != nil {
			return httpapi.ErrInternal("get project").WithCause(err)
		}
	}
	logMutation(c, "handler.Project.Create", slog.Int64("project_id", p.ID), slog.Int64("context_id", contextID))
	return c.Status(fiber.StatusCreated).JSON(dto.ProjectFromModel(*p))
}

func (h *ProjectHandler) patch(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.Project.Patch", slog.Int64("project_id", id))
	var req dto.PatchProjectRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, "handler.Project.Patch", "invalid body")
		return httpapi.ErrValidation("invalid request body")
	}
	if req.Color != nil && !isValidColor(*req.Color) {
		logValidation(c, "handler.Project.Patch", "invalid color")
		return httpapi.ErrValidation("invalid color")
	}
	var projectType *model.ProjectType
	if req.ProjectType != nil {
		pt := model.ProjectType(*req.ProjectType)
		if !pt.IsValid() {
			logValidation(c, "handler.Project.Patch", "invalid projectType")
			return httpapi.ErrValidation("invalid projectType")
		}
		projectType = &pt
	}
	p, err := h.projects.Update(c.Context(), id, repo.ProjectUpdate{
		Title:       req.Title,
		Description: req.Description,
		Color:       req.Color,
		IsPrivate:   req.IsPrivate,
		Type:        projectType,
	})
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("project not found")
		}
		return httpapi.ErrInternal("update project").WithCause(err)
	}
	if req.Labels != nil {
		labelIDs, appErr := h.resolveLabels(c, *req.Labels)
		if appErr != nil {
			return appErr
		}
		if err := h.projects.SetLabels(c.Context(), id, labelIDs); err != nil {
			return httpapi.ErrInternal("set project labels").WithCause(err)
		}
		p, err = h.projects.Get(c.Context(), id)
		if err != nil {
			return httpapi.ErrInternal("get project").WithCause(err)
		}
	}
	logMutation(c, "handler.Project.Patch", slog.Int64("project_id", p.ID))
	return c.JSON(dto.ProjectFromModel(*p))
}

func (h *ProjectHandler) delete(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.Project.Delete", slog.Int64("project_id", id))
	if err := h.projects.Delete(c.Context(), id); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("project not found")
		}
		return httpapi.ErrInternal("delete project").WithCause(err)
	}
	logMutation(c, "handler.Project.Delete", slog.Int64("project_id", id))
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
		return httpapi.ErrInternal("get project").WithCause(err)
	}
	pp := dto.ParsePageParams(c.Query("limit"), c.Query("offset"))
	items, total, err := h.sections.ListByProject(c.Context(), id, repo.Page{Limit: pp.Limit, Offset: pp.Offset})
	if err != nil {
		return httpapi.ErrInternal("list sections").WithCause(err)
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
	logEntry(c, "handler.Project.CreateSection", slog.Int64("project_id", id))
	if _, err := h.projects.Get(c.Context(), id); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("project not found")
		}
		return httpapi.ErrInternal("get project").WithCause(err)
	}
	var req dto.CreateSectionRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, "handler.Project.CreateSection", "invalid body")
		return httpapi.ErrValidation("invalid request body")
	}
	if req.Title == "" {
		logValidation(c, "handler.Project.CreateSection", "title required")
		return httpapi.ErrValidation("title is required")
	}
	s, err := h.sections.Create(c.Context(), id, req.Title)
	if err != nil {
		return httpapi.ErrInternal("create section").WithCause(err)
	}
	logMutation(c, "handler.Project.CreateSection", slog.Int64("section_id", s.ID), slog.Int64("project_id", id))
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
		return httpapi.ErrInternal("get project").WithCause(err)
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
		return httpapi.ErrInternal("list tasks").WithCause(err)
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
	logEntry(c, "handler.Project.CreateTask", slog.Int64("project_id", id))
	p, err := h.projects.Get(c.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("project not found")
		}
		return httpapi.ErrInternal("get project").WithCause(err)
	}
	var req dto.CreateTaskRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, "handler.Project.CreateTask", "invalid body")
		return httpapi.ErrValidation("invalid request body")
	}
	if req.Title == "" {
		logValidation(c, "handler.Project.CreateTask", "title required")
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
	logEntry(c, "handler.Project.SetStatus", slog.Int64("project_id", id), slog.String("status", string(status)))
	if err := h.projects.UpdateStatus(c.Context(), id, status); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("project not found")
		}
		return httpapi.ErrInternal("update project status").WithCause(err)
	}
	p, err := h.projects.Get(c.Context(), id)
	if err != nil {
		return httpapi.ErrInternal("get project").WithCause(err)
	}
	logMutation(c, "handler.Project.SetStatus", slog.Int64("project_id", id), slog.String("status", string(status)))
	return c.JSON(dto.ProjectFromModel(*p))
}

func (h *ProjectHandler) pin(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.Project.Pin", slog.Int64("project_id", id))
	p, err := h.projects.Get(c.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("project not found")
		}
		return httpapi.ErrInternal("get project").WithCause(err)
	}
	if p.Status != model.ProjectStatusOpen {
		logValidation(c, "handler.Project.Pin", "not open", slog.String("status", string(p.Status)))
		return httpapi.ErrValidation("only open projects can be pinned")
	}
	if err := h.pinSvc.PinProject(c.Context(), id); err != nil {
		if errors.Is(err, service.ErrPinLimitExceeded) {
			return httpapi.ErrLimitExceeded("max pinned projects limit reached")
		}
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("project not found")
		}
		return httpapi.ErrInternal("pin project").WithCause(err)
	}
	p, err = h.projects.Get(c.Context(), id)
	if err != nil {
		return httpapi.ErrInternal("get project").WithCause(err)
	}
	logMutation(c, "handler.Project.Pin", slog.Int64("project_id", id))
	return c.JSON(dto.ProjectFromModel(*p))
}

func (h *ProjectHandler) unpin(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.Project.Unpin", slog.Int64("project_id", id))
	if err := h.pinSvc.UnpinProject(c.Context(), id); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("project not found")
		}
		return httpapi.ErrInternal("unpin project").WithCause(err)
	}
	p, err := h.projects.Get(c.Context(), id)
	if err != nil {
		return httpapi.ErrInternal("get project").WithCause(err)
	}
	logMutation(c, "handler.Project.Unpin", slog.Int64("project_id", id))
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
			return nil, httpapi.ErrInternal("resolve label").WithCause(err)
		}
		ids = append(ids, l.ID)
	}
	return ids, nil
}

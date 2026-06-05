package handlers

import (
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
	"golang.org/x/sync/errgroup"

	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

// ProjectHandler implements routes for projects, including creation via /contexts/:id/projects.
type ProjectHandler struct {
	projects    *repo.ProjectRepo
	sections    *repo.ProjectSectionRepo
	tasks       *repo.TaskRepo
	taskSvc     *service.TaskService
	labels      *repo.LabelRepo
	contexts    *repo.ContextRepo
	pinSvc      *service.PinService
	fedProjects *repo.FederatedProjectRepo
	baseURL     string

	// fedProjectMut / fedSectionMut route a patch/delete of a FEDERATED project and
	// a create/patch/delete of a section in a federated project through the
	// federation Emitter so they emit the per-field HLC bump + signed outbox event
	// (US-3.2 AC1). nil when federation is off (no FEDERATION_KEY) — the handler
	// then falls back to the plain repo path, so the single-user path is untouched.
	fedProjectMut *fedsvc.ProjectMutator
	fedSectionMut *fedsvc.SectionMutator

	// ownerTimeout is the owner-death window a JOINED project's owner may go
	// without contact before the joiner flags it "owner offline" (Federation v1
	// F5.6a, US-6.5 AC1). 0 disables owner-offline derivation (fails safe to
	// "online"). Wired from config.FederationOwnerTimeout() via WithOwnerTimeout.
	ownerTimeout time.Duration
	// now is the clock used to derive owner-offline; injectable for tests. Defaults
	// to time.Now via the constructor.
	now func() time.Time
}

func NewProjectHandler(
	projects *repo.ProjectRepo,
	sections *repo.ProjectSectionRepo,
	tasks *repo.TaskRepo,
	taskSvc *service.TaskService,
	labels *repo.LabelRepo,
	contexts *repo.ContextRepo,
	pinSvc *service.PinService,
	fedProjects *repo.FederatedProjectRepo,
	baseURL string,
) *ProjectHandler {
	return &ProjectHandler{
		projects:    projects,
		sections:    sections,
		tasks:       tasks,
		taskSvc:     taskSvc,
		labels:      labels,
		contexts:    contexts,
		pinSvc:      pinSvc,
		fedProjects: fedProjects,
		baseURL:     baseURL,
		now:         time.Now,
	}
}

// WithOwnerTimeout wires the owner-death window used to derive the joined-project
// "owner offline" surface (Federation v1 F5.6a, US-6.5 AC1). 0 (or unset) leaves
// owner-offline derivation disabled (fails safe to "online"). Returns the handler
// for chaining.
func (h *ProjectHandler) WithOwnerTimeout(d time.Duration) *ProjectHandler {
	h.ownerTimeout = d
	return h
}

// WithClock overrides the clock used to derive owner-offline (default time.Now),
// for deterministic tests (Federation v1 F5.6a). A nil clock is ignored. Returns
// the handler for chaining.
func (h *ProjectHandler) WithClock(now func() time.Time) *ProjectHandler {
	if now != nil {
		h.now = now
	}
	return h
}

// WithFederation wires the project + section federation mutators so a patch/
// delete of a federated project and a create/patch/delete of a section in a
// federated project emit through the Emitter (US-3.2 AC1). Returns the handler
// for chaining. nil mutators leave the handler on the plain repo path.
func (h *ProjectHandler) WithFederation(projectMut *fedsvc.ProjectMutator, sectionMut *fedsvc.SectionMutator) *ProjectHandler {
	h.fedProjectMut = projectMut
	h.fedSectionMut = sectionMut
	return h
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

// projectDTO renders one project enriched with its federation surface
// (Federation v1 F2.4, US-2.4 AC1/AC2). It resolves the surface for a single
// project via the batch resolver so list and get agree on the shape. A
// non-federated project (no federated_projects row) keeps the federation fields
// nil.
func (h *ProjectHandler) projectDTO(c fiber.Ctx, p model.Project) dto.ProjectDTO {
	d := dto.ProjectFromModel(p)
	if h.fedProjects == nil || !p.IsFederated {
		return d
	}
	surfaces, err := h.fedProjects.FederationSurfaceByProjectIDs(c.Context(), []int64{p.ID})
	if err != nil {
		// Surface resolution is best-effort enrichment — a failure here must not
		// break the read path. The base DTO (isFederated=true, fields nil) still
		// renders; the authoritative guard runs on mutations, not reads. This is an
		// unexpected repo/SQL fault, NOT client validation, so log it at ERROR
		// (F2.4 #10 — was mislabeled as a WARN validation failure).
		ctx := c.Context()
		logging.FromContext(ctx).ErrorContext(ctx, "federation surface resolve failed",
			slog.String("op", "handler.Project.FederationSurface"),
			slog.Int64("project_id", p.ID),
			slog.String("err", err.Error()),
		)
		return d
	}
	if s, ok := surfaces[p.ID]; ok {
		d = d.WithFederationSurface(s.OriginInstanceURL, string(s.Permissions), s.IsOwner).
			WithReBootstrapMarker(s.RebootstrappedAt).
			WithFederationLost(s.Lost, string(s.LostReason)).
			WithFederationOwnerOffline(h.ownerOffline(s)).
			WithPeerInstances(h.peerInstances(c, []int64{p.ID})[p.ID])
	}
	return d
}

// peerInstances resolves the per-project named peer audience for the given
// federated project ids in ONE batched query (no N+1, Federation v1 F6.4, US-7.1
// AC3). It is best-effort enrichment: a resolution failure logs at ERROR and
// returns an empty map so the read path never breaks (the badge/hint simply do not
// render). selfInstanceURL is this instance's federation identity (baseURL), used
// to exclude the owner self-row.
func (h *ProjectHandler) peerInstances(c fiber.Ctx, ids []int64) map[int64][]dto.PeerInstanceDTO {
	out := make(map[int64][]dto.PeerInstanceDTO, len(ids))
	if h.fedProjects == nil || len(ids) == 0 {
		return out
	}
	byID, err := h.fedProjects.PeerInstancesByProjectIDs(c.Context(), ids, h.baseURL)
	if err != nil {
		ctx := c.Context()
		logging.FromContext(ctx).ErrorContext(ctx, "federation peer-instances resolve failed",
			slog.String("op", "handler.Project.PeerInstances"),
			slog.String("err", err.Error()),
		)
		return out
	}
	for id, peers := range byID {
		dtos := make([]dto.PeerInstanceDTO, len(peers))
		for i, p := range peers {
			dtos[i] = dto.PeerInstanceDTO{InstanceUrl: p.InstanceURL, DisplayName: p.DisplayName}
		}
		out[id] = dtos
	}
	return out
}

// ownerOffline derives whether a JOINED project's owner is unreachable past the
// configured owner-timeout window (Federation v1 F5.6a, US-6.5 AC1). It is a
// no-op (false) for the owner's own project (no owner-offline notion) and for an
// already-lost copy (the permanent lost/read-only marker takes precedence — a
// revoked/left copy is not merely "owner offline"). model.DeriveOwnerOffline
// fails safe to "online" when the timeout is non-positive.
func (h *ProjectHandler) ownerOffline(s repo.FederationSurface) bool {
	if s.IsOwner || s.Lost {
		return false
	}
	return model.DeriveOwnerOffline(s.OwnerLastContactAt, h.ownerTimeout, h.now())
}

// projectDTOs renders a slice of projects, resolving every project's federation
// surface in ONE query (no N+1, Federation v1 F2.4). Only federated projects are
// looked up; non-federated projects skip the join entirely.
func (h *ProjectHandler) projectDTOs(c fiber.Ctx, items []model.Project) []dto.ProjectDTO {
	dtos := make([]dto.ProjectDTO, len(items))
	for i, p := range items {
		dtos[i] = dto.ProjectFromModel(p)
	}
	if h.fedProjects == nil {
		return dtos
	}
	ids := make([]int64, 0, len(items))
	for _, p := range items {
		if p.IsFederated {
			ids = append(ids, p.ID)
		}
	}
	if len(ids) == 0 {
		return dtos
	}
	surfaces, err := h.fedProjects.FederationSurfaceByProjectIDs(c.Context(), ids)
	if err != nil {
		// Unexpected repo/SQL fault — log at ERROR, not as client validation
		// (F2.4 #10). The list still renders un-enriched; the guard runs on writes.
		ctx := c.Context()
		logging.FromContext(ctx).ErrorContext(ctx, "federation surface resolve failed (list)",
			slog.String("op", "handler.Project.FederationSurface"),
			slog.String("err", err.Error()),
		)
		return dtos
	}
	// Resolve every federated project's named peer audience in ONE batched query so
	// the new-task editor hint + the "visible to N peers" badge have a local source
	// without an extra round-trip (Federation v1 F6.4, US-7.1 AC3; no N+1).
	peers := h.peerInstances(c, ids)
	for i := range dtos {
		if s, ok := surfaces[dtos[i].ID]; ok {
			dtos[i] = dtos[i].WithFederationSurface(s.OriginInstanceURL, string(s.Permissions), s.IsOwner).
				WithReBootstrapMarker(s.RebootstrappedAt).
				WithFederationLost(s.Lost, string(s.LostReason)).
				WithFederationOwnerOffline(h.ownerOffline(s)).
				WithPeerInstances(peers[dtos[i].ID])
		}
	}
	return dtos
}

// guardReadOnly is the authoritative server-side enforcement seam for read-only
// federated projects (Federation v1 F2.4, US-2.4 AC4, §9.2). It rejects a local
// mutation with 403 federation_read_only when the project is a JOINED read-only
// peer copy (is_owner=0, permissions=read). The owner's own federated project
// (is_owner=1) and write/admin grants are never blocked, and non-federated
// projects are a no-op. UI disabling is insufficient — this is where the rule is
// actually enforced. It delegates to the shared FederationReadOnlyGuard so the
// project routes and the task/section routes enforce exactly the same rule
// (Federation v1 F5.2).
func (h *ProjectHandler) guardReadOnly(c fiber.Ctx, projectID int64) *httpapi.AppError {
	return NewFederationReadOnlyGuard(h.fedProjects, h.tasks, h.sections).GuardProject(c, projectID)
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
	dtos := h.projectDTOs(c, items)
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
	return c.JSON(h.projectDTO(c, *p))
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
		Project:  h.projectDTO(c, *p),
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
	createIn := repo.CreateProject{
		ContextID:   contextID,
		Title:       req.Title,
		Description: req.Description,
		Color:       req.Color,
		Type:        projectType,
	}
	// Federation-on routes the insert through the Emitter for uniformity; a freshly
	// created project is never born federated, so this is a plain insert (no event)
	// — federation is enabled later via the admin flow. Federation-off keeps the
	// direct repo create.
	var p *model.Project
	if h.fedProjectMut != nil {
		id, cerr := h.fedProjectMut.Create(c.Context(), createIn, model.NewClientID())
		if cerr != nil {
			return httpapi.ErrInternal("create project").WithCause(cerr)
		}
		p, err = h.projects.Get(c.Context(), id)
		if err != nil {
			return httpapi.ErrInternal("get project").WithCause(err)
		}
	} else {
		p, err = h.projects.Create(c.Context(), createIn)
		if err != nil {
			return httpapi.ErrInternal("create project").WithCause(err)
		}
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
	if appErr := h.guardReadOnly(c, id); appErr != nil {
		return appErr
	}
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
	if req.ContextID != nil {
		if *req.ContextID <= 0 {
			logValidation(c, "handler.Project.Patch", "invalid contextId")
			return httpapi.ErrValidation("invalid contextId")
		}
		if _, err := h.contexts.Get(c.Context(), *req.ContextID); err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				logValidation(c, "handler.Project.Patch", "context not found")
				return httpapi.ErrValidation("context not found")
			}
			return httpapi.ErrInternal("get context").WithCause(err)
		}
	}
	update := repo.ProjectUpdate{
		Title:       req.Title,
		Description: req.Description,
		Color:       req.Color,
		ContextID:   req.ContextID,
		IsPrivate:   req.IsPrivate,
		Type:        projectType,
	}
	// Federation-on: route through the Emitter so a PATCH of a federated project
	// emits a signed op=update event carrying the changed federated fields
	// (title/description/color). The mutator no-ops the sidecar for a non-federated
	// project (and when no federated field changed). Federation-off keeps the repo.
	var p *model.Project
	if h.fedProjectMut != nil {
		if uerr := h.fedProjectMut.Update(c.Context(), id, update); uerr != nil {
			if appErr := mutationErr(uerr, "project not found"); appErr != nil {
				return appErr
			}
			return httpapi.ErrInternal("update project").WithCause(uerr)
		}
		p, err = h.projects.Get(c.Context(), id)
		if err != nil {
			return httpapi.ErrInternal("get project").WithCause(err)
		}
	} else {
		p, err = h.projects.Update(c.Context(), id, update)
		if err != nil {
			if appErr := mutationErr(err, "project not found"); appErr != nil {
				return appErr
			}
			return httpapi.ErrInternal("update project").WithCause(err)
		}
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
	return c.JSON(h.projectDTO(c, *p))
}

func (h *ProjectHandler) delete(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.Project.Delete", slog.Int64("project_id", id))
	if appErr := h.guardReadOnly(c, id); appErr != nil {
		return appErr
	}
	// Federation-on: route through the Emitter so a delete of a federated project
	// emits an op=delete tombstone for the project entity (US-3.2 AC1). The mutator
	// no-ops the sidecar for a non-federated project. Federation-off keeps the repo.
	var delErr error
	if h.fedProjectMut != nil {
		delErr = h.fedProjectMut.Delete(c.Context(), id)
	} else {
		delErr = h.projects.Delete(c.Context(), id)
	}
	if delErr != nil {
		if appErr := mutationErr(delErr, "project not found"); appErr != nil {
			return appErr
		}
		return httpapi.ErrInternal("delete project").WithCause(delErr)
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
	if appErr := h.guardReadOnly(c, id); appErr != nil {
		return appErr
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
	// Federation-on: route through the Emitter so a section created in a federated
	// project emits a signed op=create event (US-3.1 AC1). Federation-off keeps the
	// direct repo create.
	var s *model.ProjectSection
	if h.fedSectionMut != nil {
		secID, cerr := h.fedSectionMut.Create(c.Context(), id, req.Title, model.NewClientID())
		if cerr != nil {
			return httpapi.ErrInternal("create section").WithCause(cerr)
		}
		s, err = h.sections.Get(c.Context(), secID)
		if err != nil {
			return httpapi.ErrInternal("get section").WithCause(err)
		}
	} else {
		s, err = h.sections.Create(c.Context(), id, req.Title)
		if err != nil {
			return httpapi.ErrInternal("create section").WithCause(err)
		}
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
	if appErr := h.guardReadOnly(c, id); appErr != nil {
		return appErr
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
	if appErr := h.guardReadOnly(c, id); appErr != nil {
		return appErr
	}
	// Federation-on: route through the Emitter so a status change on a federated
	// project emits a signed op=update event carrying the status field (US-3.2 AC1).
	// The mutator no-ops the sidecar for a non-federated project. Federation-off
	// keeps the direct repo path, so the single-user hot path is untouched.
	var statusErr error
	if h.fedProjectMut != nil {
		statusErr = h.fedProjectMut.UpdateStatus(c.Context(), id, status)
	} else {
		statusErr = h.projects.UpdateStatus(c.Context(), id, status)
	}
	if statusErr != nil {
		if errors.Is(statusErr, repo.ErrNotFound) {
			return httpapi.ErrNotFound("project not found")
		}
		return httpapi.ErrInternal("update project status").WithCause(statusErr)
	}
	p, err := h.projects.Get(c.Context(), id)
	if err != nil {
		return httpapi.ErrInternal("get project").WithCause(err)
	}
	logMutation(c, "handler.Project.SetStatus", slog.Int64("project_id", id), slog.String("status", string(status)))
	return c.JSON(h.projectDTO(c, *p))
}

func (h *ProjectHandler) pin(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.Project.Pin", slog.Int64("project_id", id))
	if appErr := h.guardReadOnly(c, id); appErr != nil {
		return appErr
	}
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
	return c.JSON(h.projectDTO(c, *p))
}

func (h *ProjectHandler) unpin(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.Project.Unpin", slog.Int64("project_id", id))
	if appErr := h.guardReadOnly(c, id); appErr != nil {
		return appErr
	}
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
	return c.JSON(h.projectDTO(c, *p))
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

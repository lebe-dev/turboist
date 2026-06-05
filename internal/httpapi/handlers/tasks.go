package handlers

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
	rrule "github.com/teambition/rrule-go"
)

var reTrailingCounter = regexp.MustCompile(`^(.*) \((\d+)\)$`)

func duplicateTitle(title string) string {
	if m := reTrailingCounter.FindStringSubmatch(title); m != nil {
		n, _ := strconv.Atoi(m[2])
		return m[1] + " (" + strconv.Itoa(n+1) + ")"
	}
	return title + " (2)"
}

// TaskHandler implements GET/PATCH/DELETE /tasks/:id and POST /tasks/:id/subtasks.
type TaskHandler struct {
	tasks    *repo.TaskRepo
	projects *repo.ProjectRepo
	taskSvc  *service.TaskService
	baseURL  string

	// fedTasks routes a delete of a FEDERATED task through the federation Emitter
	// so it emits the op=delete tombstone + child cascade to federation_outbox
	// (US-3.7 AC3). nil when federation is off (no FEDERATION_KEY) — the handler
	// then falls back to the plain repo delete, so the single-user path is untouched.
	fedTasks *fedsvc.TaskMutator

	// fedGuard rejects a local mutation against a read-only federated project with
	// 403 federation_read_only (Federation v1 F5.2, US-5.1 AC4). nil/unwired is a
	// no-op so the single-user path is untouched.
	fedGuard *FederationReadOnlyGuard

	// fedProjects resolves the task's project federation surface so the task detail
	// GET can carry federated + visibleToPeers for the "federated, visible to N
	// peers" header badge (Federation v1 F6.4, US-7.1 AC2). nil when federation is
	// off — the task DTO then carries federated=false / 0, untouched. Wired
	// additively by WithVisibility.
	fedProjects *repo.FederatedProjectRepo
}

// NewTaskHandler constructs a TaskHandler.
func NewTaskHandler(tasks *repo.TaskRepo, projects *repo.ProjectRepo, taskSvc *service.TaskService, baseURL string) *TaskHandler {
	return &TaskHandler{tasks: tasks, projects: projects, taskSvc: taskSvc, baseURL: baseURL}
}

// WithFederation wires the federation task mutator so a delete of a federated task
// emits through the Emitter (op=delete + child cascade, US-3.7 AC3). Returns the
// handler for chaining. A nil mutator leaves the handler on the plain repo path.
func (h *TaskHandler) WithFederation(m *fedsvc.TaskMutator) *TaskHandler {
	h.fedTasks = m
	return h
}

// WithFederationGuard wires the read-only federated-project guard so every task
// mutation entry point rejects an edit of a read-only federated task with 403
// (Federation v1 F5.2, US-5.1 AC4). Returns the handler for chaining.
func (h *TaskHandler) WithFederationGuard(g *FederationReadOnlyGuard) *TaskHandler {
	h.fedGuard = g
	return h
}

// WithVisibility wires the federated-projects repo so the task detail GET carries
// the federated flag + the visible-to-peers count for the "federated, visible to N
// peers" header badge (Federation v1 F6.4, US-7.1 AC2). Returns the handler for
// chaining. A nil repo leaves the task DTO with federated=false / 0.
func (h *TaskHandler) WithVisibility(fedProjects *repo.FederatedProjectRepo) *TaskHandler {
	h.fedProjects = fedProjects
	return h
}

// federationVisibility resolves whether a task's project is federated and, if so,
// how many non-revoked peer instances it is visible to (Federation v1 F6.4, US-7.1
// AC2). It is best-effort: a resolve failure (or no project / federation off)
// returns (false, 0) so the read path never breaks. One batched query is issued
// for the single project id.
func (h *TaskHandler) federationVisibility(c fiber.Ctx, t *model.Task) (bool, int) {
	if h.fedProjects == nil || t.ProjectID == nil {
		return false, 0
	}
	ids := []int64{*t.ProjectID}
	surfaces, err := h.fedProjects.FederationSurfaceByProjectIDs(c.Context(), ids)
	if err != nil {
		ctx := c.Context()
		logging.FromContext(ctx).ErrorContext(ctx, "federation visibility surface resolve failed",
			slog.String("op", "handler.Task.FederationVisibility"),
			slog.Int64("project_id", *t.ProjectID),
			slog.String("err", err.Error()),
		)
		return false, 0
	}
	if _, ok := surfaces[*t.ProjectID]; !ok {
		return false, 0
	}
	peersByID, err := h.fedProjects.PeerInstancesByProjectIDs(c.Context(), ids, h.baseURL)
	if err != nil {
		ctx := c.Context()
		logging.FromContext(ctx).ErrorContext(ctx, "federation visibility peers resolve failed",
			slog.String("op", "handler.Task.FederationVisibility"),
			slog.Int64("project_id", *t.ProjectID),
			slog.String("err", err.Error()),
		)
		return true, 0
	}
	return true, len(peersByID[*t.ProjectID])
}

// Register wires task routes onto r (the /api/v1 group).
func (h *TaskHandler) Register(r fiber.Router) {
	r.Get("/tasks/:id", httpapi.RequireScope("tasks:read"), h.get)
	r.Patch("/tasks/:id", httpapi.RequireScope("tasks:write"), h.patch)
	r.Delete("/tasks/:id", httpapi.RequireScope("tasks:write"), h.delete)
	r.Get("/tasks/:id/subtasks", httpapi.RequireScope("tasks:read"), h.listSubtasks)
	r.Post("/tasks/:id/subtasks", httpapi.RequireScope("tasks:write"), h.createSubtask)
	r.Post("/tasks/:id/duplicate", httpapi.RequireScope("tasks:write"), h.duplicate)
	r.Post("/tasks/:id/decompose", httpapi.RequireScope("tasks:write"), h.decompose)
}

func (h *TaskHandler) get(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	includeSubtasks := c.Query("subtasks") == "true"
	logEntry(c, "handler.Task.Get", slog.Int64("task_id", id), slog.Bool("subtasks", includeSubtasks))
	t, err := h.tasks.Get(c.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("task not found")
		}
		return httpapi.ErrInternal("get task").WithCause(err)
	}
	out := dto.TaskFromModel(*t, h.baseURL)
	// Federated-visibility enrichment for the task-header badge (Federation v1 F6.4,
	// US-7.1 AC2): federated + the visible-to-peers count, resolved from the task's
	// project. No-op (false / 0) for a non-federated task or a federation-off build.
	federated, visibleToPeers := h.federationVisibility(c, t)
	out = out.WithFederationVisibility(federated, visibleToPeers)
	if includeSubtasks {
		items, err := h.tasks.ListSubtasks(c.Context(), id)
		if err != nil {
			return httpapi.ErrInternal("list subtasks").WithCause(err)
		}
		dtos := make([]dto.TaskDTO, len(items))
		for i, st := range items {
			dtos[i] = dto.TaskFromModel(st, h.baseURL).WithFederationVisibility(federated, visibleToPeers)
		}
		page := dto.NewPagedResponse(dtos, len(dtos), len(dtos), 0)
		out.Subtasks = &page
	}
	return c.JSON(out)
}

func (h *TaskHandler) patch(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.Task.Patch", slog.Int64("task_id", id))
	t, err := h.tasks.Get(c.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			// A tombstoned task is invisible to Get; re-editing it is a 410, not
			// a 404 (US-3.7 AC2 foundation). A DB failure during disambiguation
			// is neither ErrGone nor ErrNotFound, so it must surface as a 500 —
			// never be masked as a 404 (Federation v1 F0.1 follow-up).
			resolveErr := h.tasks.NotFoundOrGone(c.Context(), id)
			if appErr := mutationErr(resolveErr, "task not found"); appErr != nil {
				return appErr
			}
			return httpapi.ErrInternal("resolve task tombstone").WithCause(resolveErr)
		}
		return httpapi.ErrInternal("get task").WithCause(err)
	}
	if t.ProjectID != nil {
		if appErr := h.fedGuard.GuardProject(c, *t.ProjectID); appErr != nil {
			return appErr
		}
	}
	var req dto.PatchTaskRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, "handler.Task.Patch", "invalid request body")
		return httpapi.ErrValidation("invalid request body")
	}

	u := repo.TaskUpdate{}
	if req.Title != nil {
		if strings.TrimSpace(*req.Title) == "" {
			logValidation(c, "handler.Task.Patch", "empty title")
			return httpapi.ErrValidation("title must not be empty")
		}
		u.Title = req.Title
	}
	if req.Description != nil {
		u.Description = req.Description
	}
	if req.Priority != nil {
		p := model.Priority(*req.Priority)
		if !p.IsValid() {
			logValidation(c, "handler.Task.Patch", "invalid priority")
			return httpapi.ErrValidation("invalid priority")
		}
		// Tasks in a Troiki-bound project have priority pinned by the project's
		// category — reject direct priority edits so a stale client or CLI can't
		// desync the two.
		if t.ProjectID != nil {
			proj, err := h.projects.Get(c.Context(), *t.ProjectID)
			if err != nil && !errors.Is(err, repo.ErrNotFound) {
				return httpapi.ErrInternal("get project").WithCause(err)
			}
			if proj != nil && proj.TroikiCategory != nil && p != service.PriorityForCategory(*proj.TroikiCategory) {
				return httpapi.ErrValidation("priority is managed by Troiki category")
			}
		}
		u.Priority = &p
	}
	if req.DueAt.IsNull() {
		u.DueAtClear = true
	} else if v, ok := req.DueAt.Value(); ok {
		ts, err := model.ParseUTC(v)
		if err != nil {
			return httpapi.ErrValidation("invalid dueAt format")
		}
		u.DueAt = &ts
	}
	if req.DueHasTime != nil {
		u.DueHasTime = req.DueHasTime
	}
	if req.DeadlineAt.IsNull() {
		u.DeadlineAtClear = true
	} else if v, ok := req.DeadlineAt.Value(); ok {
		ts, err := model.ParseUTC(v)
		if err != nil {
			return httpapi.ErrValidation("invalid deadlineAt format")
		}
		u.DeadlineAt = &ts
	}
	if req.DeadlineHasTime != nil {
		u.DeadlineHasTime = req.DeadlineHasTime
	}
	if req.DayPart != nil {
		dp := model.DayPart(*req.DayPart)
		if !dp.IsValid() {
			return httpapi.ErrValidation("invalid dayPart")
		}
		u.DayPart = &dp
	}
	if req.PlanState != nil {
		ps := model.PlanState(*req.PlanState)
		if !ps.IsValid() {
			return httpapi.ErrValidation("invalid planState")
		}
		u.PlanState = &ps
	}

	// Assigning a due date pulls a task out of the backlog: a scheduled task no
	// longer belongs to the unscheduled backlog. This is the inverse of planning
	// into backlog, which clears due (see service.PlanService.SetPlanState). An
	// explicit planState in the same request wins.
	if u.DueAt != nil && req.PlanState == nil && t.PlanState == model.PlanStateBacklog {
		none := model.PlanStateNone
		u.PlanState = &none
	}
	if req.RecurrenceRule.IsNull() {
		u.RecurrenceClear = true
	} else if v, ok := req.RecurrenceRule.Value(); ok {
		if _, err := rrule.StrToRRule(v); err != nil {
			return httpapi.ErrValidation("invalid recurrenceRule")
		}
		u.RecurrenceRule = &v
	}

	// Reject hasTime=true without a corresponding date — DB CHECK would
	// otherwise surface as a generic 500.
	if u.DueHasTime != nil && *u.DueHasTime {
		hasDue := u.DueAt != nil || (!u.DueAtClear && t.DueAt != nil)
		if !hasDue {
			return httpapi.ErrValidation("dueHasTime requires dueAt")
		}
	}
	if u.DeadlineHasTime != nil && *u.DeadlineHasTime {
		hasDeadline := u.DeadlineAt != nil || (!u.DeadlineAtClear && t.DeadlineAt != nil)
		if !hasDeadline {
			return httpapi.ErrValidation("deadlineHasTime requires deadlineAt")
		}
	}

	if req.IsPrivate != nil {
		u.IsPrivate = req.IsPrivate
	}

	u.IncPostponeCount = shouldIncPostpone(t, u, time.Now())

	// Federation-on: route through the Emitter so a PATCH of a federated task
	// emits a signed op=update event carrying the changed federated fields
	// (US-3.2 AC1). The mutator no-ops the federation sidecar for a non-federated
	// project (and when no federated field changed), so the single-user path is
	// untouched. Federation-off (nil mutator) keeps the plain repo update.
	if h.fedTasks != nil {
		if err := h.fedTasks.Update(c.Context(), t, u); err != nil {
			if appErr := mutationErr(err, "task not found"); appErr != nil {
				return appErr
			}
			return httpapi.ErrInternal("update task").WithCause(err)
		}
	} else if _, err := h.tasks.Update(c.Context(), id, u); err != nil {
		if appErr := mutationErr(err, "task not found"); appErr != nil {
			return appErr
		}
		return httpapi.ErrInternal("update task").WithCause(err)
	}
	updated, err := h.tasks.Get(c.Context(), id)
	if err != nil {
		return httpapi.ErrInternal("get task after patch").WithCause(err)
	}

	needsLabelUpdate := req.Title != nil || req.Labels != nil || len(req.RemovedAutoLabels) > 0
	if needsLabelUpdate {
		if err := h.taskSvc.PatchLabels(c.Context(), t, updated.Title, req.Labels, req.RemovedAutoLabels); err != nil {
			var ule *service.UnknownLabelError
			if errors.As(err, &ule) {
				return httpapi.ErrValidation("unknown label: " + ule.Name)
			}
			return httpapi.ErrInternal("apply labels").WithCause(err)
		}
		updated, err = h.tasks.Get(c.Context(), id)
		if err != nil {
			return httpapi.ErrInternal("get task after patch").WithCause(err)
		}
	}

	logMutation(c, "handler.Task.Patch", slog.Int64("task_id", updated.ID))
	return c.JSON(dto.TaskFromModel(*updated, h.baseURL))
}

func (h *TaskHandler) delete(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.Task.Delete", slog.Int64("task_id", id))

	// Reject a delete of a task in a read-only federated project (Federation v1
	// F5.2, US-5.1 AC4) before any work — a joined read peer may not delete the
	// owner's tasks; the tombstone arrives via the owner's relayed fan-out.
	if appErr := h.fedGuard.GuardTask(c, id); appErr != nil {
		return appErr
	}

	// Federation-on: route through the Emitter so a delete of a federated task
	// emits the op=delete tombstone + child cascade to federation_outbox (US-3.7
	// AC3). We must load the task first to know its project + cross-instance
	// client_id; the mutator no-ops the federation sidecar for a non-federated
	// project. Federation-off keeps the plain repo path (zero overhead).
	if h.fedTasks != nil {
		t, err := h.tasks.Get(c.Context(), id)
		if err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				// A tombstoned task is invisible to Get; re-deleting it is a 410, not
				// a 404 (US-3.7 AC2 foundation), mirroring patch's disambiguation.
				resolveErr := h.tasks.NotFoundOrGone(c.Context(), id)
				if appErr := mutationErr(resolveErr, "task not found"); appErr != nil {
					return appErr
				}
				return httpapi.ErrInternal("resolve task tombstone").WithCause(resolveErr)
			}
			return httpapi.ErrInternal("get task").WithCause(err)
		}
		if err := h.fedTasks.Delete(c.Context(), t); err != nil {
			if appErr := mutationErr(err, "task not found"); appErr != nil {
				return appErr
			}
			return httpapi.ErrInternal("delete task").WithCause(err)
		}
		logMutation(c, "handler.Task.Delete", slog.Int64("task_id", id))
		return c.SendStatus(fiber.StatusNoContent)
	}

	if err := h.tasks.Delete(c.Context(), id); err != nil {
		if appErr := mutationErr(err, "task not found"); appErr != nil {
			return appErr
		}
		return httpapi.ErrInternal("delete task").WithCause(err)
	}
	logMutation(c, "handler.Task.Delete", slog.Int64("task_id", id))
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *TaskHandler) listSubtasks(c fiber.Ctx) error {
	parentID, err := parseID(c)
	if err != nil {
		return err
	}
	if _, err := h.tasks.Get(c.Context(), parentID); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("task not found")
		}
		return httpapi.ErrInternal("get task").WithCause(err)
	}
	items, err := h.tasks.ListSubtasks(c.Context(), parentID)
	if err != nil {
		return httpapi.ErrInternal("list subtasks").WithCause(err)
	}
	dtos := make([]dto.TaskDTO, len(items))
	for i, t := range items {
		dtos[i] = dto.TaskFromModel(t, h.baseURL)
	}
	return c.JSON(dto.NewPagedResponse(dtos, len(dtos), len(dtos), 0))
}

func (h *TaskHandler) createSubtask(c fiber.Ctx) error {
	parentID, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.Task.CreateSubtask", slog.Int64("parent_id", parentID))
	parent, err := h.tasks.Get(c.Context(), parentID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("parent task not found")
		}
		return httpapi.ErrInternal("get parent task").WithCause(err)
	}
	if parent.InboxID != nil {
		logValidation(c, "handler.Task.CreateSubtask", "subtask in inbox")
		return httpapi.ErrForbiddenPlacement("subtasks cannot be placed in inbox")
	}
	if parent.ProjectID != nil {
		if appErr := h.fedGuard.GuardProject(c, *parent.ProjectID); appErr != nil {
			return appErr
		}
	}
	var req dto.CreateTaskRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, "handler.Task.CreateSubtask", "invalid body")
		return httpapi.ErrValidation("invalid request body")
	}
	if req.Title == "" {
		logValidation(c, "handler.Task.CreateSubtask", "title required")
		return httpapi.ErrValidation("title is required")
	}
	placement := repo.Placement{
		ContextID: parent.ContextID,
		ProjectID: parent.ProjectID,
		SectionID: parent.SectionID,
		ParentID:  &parentID,
	}
	in, appErr := buildTaskCreate(req, placement)
	if appErr != nil {
		return appErr
	}
	// Inherit parent's labels when caller omits the field. Explicit empty array
	// (req.Labels == []) decodes to non-nil empty slice and is treated as
	// "no labels", so users can still create unlabelled subtasks.
	labels := req.Labels
	if labels == nil && len(parent.Labels) > 0 {
		labels = make([]string, len(parent.Labels))
		for i, l := range parent.Labels {
			labels[i] = l.Name
		}
	}
	t, err := h.taskSvc.Create(c.Context(), in, labels, req.RemovedAutoLabels)
	if err != nil {
		return handleTaskCreateErr(c, err)
	}
	logMutation(c, "handler.Task.CreateSubtask", slog.Int64("task_id", t.ID), slog.Int64("parent_id", parentID))
	return c.Status(fiber.StatusCreated).JSON(dto.TaskFromModel(*t, h.baseURL))
}

func (h *TaskHandler) duplicate(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.Task.Duplicate", slog.Int64("task_id", id))
	src, err := h.tasks.Get(c.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("task not found")
		}
		return httpapi.ErrInternal("get task").WithCause(err)
	}
	if src.ProjectID != nil {
		if appErr := h.fedGuard.GuardProject(c, *src.ProjectID); appErr != nil {
			return appErr
		}
	}
	t, err := h.cloneTask(c.Context(), src, src.ParentID, duplicateTitle(src.Title))
	if err != nil {
		return handleTaskCreateErr(c, err)
	}
	out := dto.TaskFromModel(*t, h.baseURL)
	// Surface the cloned subtasks so the client can render them without a reload.
	subtasks, err := h.tasks.ListSubtasks(c.Context(), t.ID)
	if err != nil {
		return httpapi.ErrInternal("list subtasks").WithCause(err)
	}
	dtos := make([]dto.TaskDTO, len(subtasks))
	for i, st := range subtasks {
		dtos[i] = dto.TaskFromModel(st, h.baseURL)
	}
	page := dto.NewPagedResponse(dtos, len(dtos), len(dtos), 0)
	out.Subtasks = &page
	logMutation(c, "handler.Task.Duplicate", slog.Int64("task_id", t.ID), slog.Int64("source_id", id))
	return c.Status(fiber.StatusCreated).JSON(out)
}

// cloneTask deep-copies src as a new task placed under parentID (nil for a
// top-level task) with the given title, then recursively clones src's subtasks
// under the freshly created task. Only the top-level clone is renamed; subtasks
// keep their original titles. Each subtask is re-fetched via Get so its labels
// are hydrated (ListSubtasks does not hydrate labels).
func (h *TaskHandler) cloneTask(ctx context.Context, src *model.Task, parentID *int64, title string) (*model.Task, error) {
	in := repo.CreateTask{
		Placement: repo.Placement{
			InboxID:   src.InboxID,
			ContextID: src.ContextID,
			ProjectID: src.ProjectID,
			SectionID: src.SectionID,
			ParentID:  parentID,
		},
		Title:           title,
		Description:     src.Description,
		Priority:        src.Priority,
		DueAt:           src.DueAt,
		DueHasTime:      src.DueHasTime,
		DeadlineAt:      src.DeadlineAt,
		DeadlineHasTime: src.DeadlineHasTime,
		DayPart:         src.DayPart,
		PlanState:       src.PlanState,
		RecurrenceRule:  src.RecurrenceRule,
	}
	labelNames := make([]string, len(src.Labels))
	for i, l := range src.Labels {
		labelNames[i] = l.Name
	}
	t, err := h.taskSvc.Create(ctx, in, labelNames, nil)
	if err != nil {
		return nil, err
	}
	subtasks, err := h.tasks.ListSubtasks(ctx, src.ID)
	if err != nil {
		return nil, err
	}
	for i := range subtasks {
		sub, err := h.tasks.Get(ctx, subtasks[i].ID)
		if err != nil {
			return nil, err
		}
		if _, err := h.cloneTask(ctx, sub, &t.ID, sub.Title); err != nil {
			return nil, err
		}
	}
	return t, nil
}

func (h *TaskHandler) decompose(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.Task.Decompose", slog.Int64("task_id", id))
	var req dto.DecomposeTaskRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, "handler.Task.Decompose", "invalid body")
		return httpapi.ErrValidation("invalid request body")
	}
	titles := make([]string, 0, len(req.Titles))
	for _, raw := range req.Titles {
		s := strings.TrimSpace(raw)
		if s != "" {
			titles = append(titles, s)
		}
	}
	if len(titles) == 0 {
		logValidation(c, "handler.Task.Decompose", "empty titles")
		return httpapi.ErrValidation("titles must not be empty")
	}

	src, err := h.tasks.Get(c.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("task not found")
		}
		return httpapi.ErrInternal("get task").WithCause(err)
	}
	if src.ProjectID != nil {
		if appErr := h.fedGuard.GuardProject(c, *src.ProjectID); appErr != nil {
			return appErr
		}
	}
	subs, err := h.tasks.ListSubtasks(c.Context(), id)
	if err != nil {
		return httpapi.ErrInternal("list subtasks").WithCause(err)
	}
	if len(subs) > 0 {
		return httpapi.ErrConflict("task has subtasks")
	}

	labelNames := make([]string, len(src.Labels))
	for i, l := range src.Labels {
		labelNames[i] = l.Name
	}
	placement := repo.Placement{
		InboxID:   src.InboxID,
		ContextID: src.ContextID,
		ProjectID: src.ProjectID,
		SectionID: src.SectionID,
		ParentID:  src.ParentID,
	}

	created := make([]model.Task, 0, len(titles))
	createdIDs := make([]int64, 0, len(titles))
	rollback := func() {
		for _, cid := range createdIDs {
			if err := h.tasks.Delete(c.Context(), cid); err != nil {
				// Compensating delete failed: a partially-created decomposition is left
				// behind. We keep unwinding the rest (best-effort), but surface each
				// failure at WARN so an orphaned subtask is observable rather than
				// silently swallowed.
				logging.FromContext(c.Context()).WarnContext(c.Context(), "federation: decompose rollback compensating delete failed",
					slog.String("op", "handler.Task.Decompose"),
					slog.Int64("source_id", id),
					slog.Int64("task_id", cid),
					slog.String("err", err.Error()),
				)
			}
		}
	}
	for _, title := range titles {
		in := repo.CreateTask{
			Placement:       placement,
			Title:           title,
			Description:     src.Description,
			Priority:        src.Priority,
			DueAt:           src.DueAt,
			DueHasTime:      src.DueHasTime,
			DeadlineAt:      src.DeadlineAt,
			DeadlineHasTime: src.DeadlineHasTime,
			DayPart:         src.DayPart,
			PlanState:       src.PlanState,
			RecurrenceRule:  src.RecurrenceRule,
		}
		t, err := h.taskSvc.Create(c.Context(), in, labelNames, nil)
		if err != nil {
			rollback()
			return handleTaskCreateErr(c, err)
		}
		if src.IsPrivate {
			isPriv := true
			if updated, uerr := h.tasks.Update(c.Context(), t.ID, repo.TaskUpdate{IsPrivate: &isPriv}); uerr == nil {
				t = updated
			}
		}
		created = append(created, *t)
		createdIDs = append(createdIDs, t.ID)
	}

	if err := h.tasks.Delete(c.Context(), id); err != nil {
		rollback()
		return httpapi.ErrInternal("delete original task").WithCause(err)
	}

	out := dto.DecomposeTaskResponse{Created: make([]dto.TaskDTO, len(created))}
	for i, t := range created {
		out.Created[i] = dto.TaskFromModel(t, h.baseURL)
	}
	logMutation(c, "handler.Task.Decompose", slog.Int64("source_id", id), slog.Int("created", len(created)))
	return c.Status(fiber.StatusCreated).JSON(out)
}

// handleTaskCreateErr maps TaskService.Create errors to API errors.
func handleTaskCreateErr(c fiber.Ctx, err error) *httpapi.AppError {
	var ule *service.UnknownLabelError
	if errors.As(err, &ule) {
		logValidation(c, "handler.Task.Create", "unknown label", slog.String("label", ule.Name))
		return httpapi.ErrValidation("unknown label: " + ule.Name)
	}
	if errors.Is(err, repo.ErrInvalidPlacement) {
		logValidation(c, "handler.Task.Create", "invalid placement")
		return httpapi.ErrForbiddenPlacement("invalid task placement")
	}
	return httpapi.ErrInternal("create task").WithCause(err)
}

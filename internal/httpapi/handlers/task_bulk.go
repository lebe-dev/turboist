package handlers

import (
	"errors"
	"log/slog"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
)

// TaskBulkHandler handles bulk operations on tasks.
type TaskBulkHandler struct {
	completeSvc *service.CompleteService
	moveSvc     *service.MoveService
	groupSvc    *service.GroupService
	baseURL     string

	// fedGuard rejects a bulk operation that touches a task in a read-only
	// federated project with 403 federation_read_only (Federation v1 F5.2, US-5.1
	// AC4). A bulk op is rejected WHOLE — the guard runs before any task is
	// mutated, so there is no partial apply. nil/unwired is a no-op.
	fedGuard *FederationReadOnlyGuard
}

func NewTaskBulkHandler(completeSvc *service.CompleteService, moveSvc *service.MoveService, groupSvc *service.GroupService, baseURL string) *TaskBulkHandler {
	return &TaskBulkHandler{completeSvc: completeSvc, moveSvc: moveSvc, groupSvc: groupSvc, baseURL: baseURL}
}

// WithFederationGuard wires the read-only federated-project guard so every bulk
// entry point rejects a batch that touches a read-only federated task with 403
// (Federation v1 F5.2, US-5.1 AC4). Returns the handler for chaining.
func (h *TaskBulkHandler) WithFederationGuard(g *FederationReadOnlyGuard) *TaskBulkHandler {
	h.fedGuard = g
	return h
}

func (h *TaskBulkHandler) Register(r fiber.Router) {
	r.Post("/tasks/bulk/complete", httpapi.RequireScope("tasks:write"), h.bulkComplete)
	r.Post("/tasks/bulk/move", httpapi.RequireScope("tasks:write"), h.bulkMove)
	r.Post("/tasks/group", httpapi.RequireScope("tasks:write"), h.groupTasks)
}

// BulkIDsRequest is the body for bulk complete.
type BulkIDsRequest struct {
	IDs []int64 `json:"ids"`
}

// BulkMoveRequest is the body for bulk move.
type BulkMoveRequest struct {
	IDs       []int64 `json:"ids"`
	InboxID   *int64  `json:"inboxId"`
	ContextID *int64  `json:"contextId"`
	ProjectID *int64  `json:"projectId"`
	SectionID *int64  `json:"sectionId"`
	ParentID  *int64  `json:"parentId"`
}

type bulkErrDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type bulkFailedItem struct {
	ID    int64         `json:"id"`
	Error bulkErrDetail `json:"error"`
}

type bulkResponse struct {
	Succeeded []int64          `json:"succeeded"`
	Failed    []bulkFailedItem `json:"failed"`
}

func (h *TaskBulkHandler) bulkComplete(c fiber.Ctx) error {
	logEntry(c, "handler.Task.BulkComplete")
	var req BulkIDsRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, "handler.Task.BulkComplete", "invalid body")
		return httpapi.ErrValidation("invalid request body")
	}
	if len(req.IDs) > 100 {
		logValidation(c, "handler.Task.BulkComplete", "too many ids", slog.Int("count", len(req.IDs)))
		return httpapi.ErrValidation("too many ids")
	}
	if appErr := h.fedGuard.GuardTasks(c, req.IDs); appErr != nil {
		return appErr
	}

	resp := bulkResponse{
		Succeeded: make([]int64, 0),
		Failed:    make([]bulkFailedItem, 0),
	}
	for _, id := range req.IDs {
		_, err := h.completeSvc.Complete(c.Context(), id)
		if err != nil {
			resp.Failed = append(resp.Failed, bulkFailedItem{ID: id, Error: toErrDetail(err)})
		} else {
			resp.Succeeded = append(resp.Succeeded, id)
		}
	}
	logMutation(c, "handler.Task.BulkComplete", slog.Int("succeeded", len(resp.Succeeded)), slog.Int("failed", len(resp.Failed)))
	return c.JSON(resp)
}

func (h *TaskBulkHandler) bulkMove(c fiber.Ctx) error {
	logEntry(c, "handler.Task.BulkMove")
	var req BulkMoveRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, "handler.Task.BulkMove", "invalid body")
		return httpapi.ErrValidation("invalid request body")
	}
	if len(req.IDs) > 100 {
		logValidation(c, "handler.Task.BulkMove", "too many ids", slog.Int("count", len(req.IDs)))
		return httpapi.ErrValidation("too many ids")
	}

	target := repo.Placement{
		InboxID:   req.InboxID,
		ContextID: req.ContextID,
		ProjectID: req.ProjectID,
		SectionID: req.SectionID,
		ParentID:  req.ParentID,
	}
	if err := target.Validate(); err != nil {
		logValidation(c, "handler.Task.BulkMove", "invalid placement")
		return httpapi.ErrForbiddenPlacement("invalid task placement")
	}
	// Guard both legs (Federation v1 F5.2): no task may leave a read-only
	// federated project, and none may be moved INTO one.
	if appErr := h.fedGuard.GuardTasks(c, req.IDs); appErr != nil {
		return appErr
	}
	if req.ProjectID != nil {
		if appErr := h.fedGuard.GuardProject(c, *req.ProjectID); appErr != nil {
			return appErr
		}
	}

	resp := bulkResponse{
		Succeeded: make([]int64, 0),
		Failed:    make([]bulkFailedItem, 0),
	}
	for _, id := range req.IDs {
		_, err := h.moveSvc.Move(c.Context(), id, target)
		if err != nil {
			resp.Failed = append(resp.Failed, bulkFailedItem{ID: id, Error: toErrDetail(err)})
		} else {
			resp.Succeeded = append(resp.Succeeded, id)
		}
	}
	logMutation(c, "handler.Task.BulkMove", slog.Int("succeeded", len(resp.Succeeded)), slog.Int("failed", len(resp.Failed)))
	return c.JSON(resp)
}

// GroupTasksResponse is the body for POST /tasks/group.
type GroupTasksResponse struct {
	Parent    dto.TaskDTO      `json:"parent"`
	Succeeded []int64          `json:"succeeded"`
	Failed    []bulkFailedItem `json:"failed"`
}

func (h *TaskBulkHandler) groupTasks(c fiber.Ctx) error {
	logEntry(c, "handler.Task.Group")
	var req dto.GroupTasksRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, "handler.Task.Group", "invalid body")
		return httpapi.ErrValidation("invalid request body")
	}

	// Guard both legs (Federation v1 F5.2) BEFORE placement validation: no child
	// may be re-parented out of a read-only federated project, and the new parent
	// may not land in one. Running the read-only check first means a group that
	// touches a read-only federated task is rejected 403 rather than masked by a
	// placement error.
	if appErr := h.fedGuard.GuardTasks(c, req.ChildIDs); appErr != nil {
		return appErr
	}
	if req.ProjectID != nil {
		if appErr := h.fedGuard.GuardProject(c, *req.ProjectID); appErr != nil {
			return appErr
		}
	}

	placement := repo.Placement{
		ContextID: req.ContextID,
		ProjectID: req.ProjectID,
		SectionID: req.SectionID,
	}
	if err := placement.Validate(); err != nil {
		logValidation(c, "handler.Task.Group", "invalid placement")
		return httpapi.ErrForbiddenPlacement("invalid task placement")
	}

	create, appErr := buildTaskCreate(req.CreateTaskRequest, placement)
	if appErr != nil {
		logValidation(c, "handler.Task.Group", appErr.Message)
		return appErr
	}

	in := service.GroupInput{
		Parent:            create,
		ExplicitLabels:    req.Labels,
		RemovedAutoLabels: req.RemovedAutoLabels,
		ChildIDs:          req.ChildIDs,
	}
	res, err := h.groupSvc.Group(c.Context(), in)
	if err != nil {
		if errors.Is(err, service.ErrInvalidGroupRequest) {
			logValidation(c, "handler.Task.Group", err.Error())
			return httpapi.ErrValidation(err.Error())
		}
		if errors.Is(err, repo.ErrInvalidPlacement) {
			logValidation(c, "handler.Task.Group", "invalid placement")
			return httpapi.ErrForbiddenPlacement("invalid task placement")
		}
		var ule *service.UnknownLabelError
		if errors.As(err, &ule) {
			logValidation(c, "handler.Task.Group", "unknown label", slog.String("label", ule.Name))
			return httpapi.ErrValidation("unknown label: " + ule.Name)
		}
		return httpapi.ErrInternal("group tasks").WithCause(err)
	}

	resp := GroupTasksResponse{
		Parent:    dto.TaskFromModel(*res.Parent, h.baseURL),
		Succeeded: res.SucceededIDs,
		Failed:    make([]bulkFailedItem, 0, len(res.Failed)),
	}
	for _, f := range res.Failed {
		resp.Failed = append(resp.Failed, bulkFailedItem{ID: f.ID, Error: toErrDetail(f.Err)})
	}
	logMutation(c, "handler.Task.Group", slog.Int64("parent_id", res.Parent.ID), slog.Int("succeeded", len(resp.Succeeded)), slog.Int("failed", len(resp.Failed)))
	return c.Status(fiber.StatusCreated).JSON(resp)
}

// toErrDetail converts a service/repo error to a bulk error detail.
func toErrDetail(err error) bulkErrDetail {
	var appErr *httpapi.AppError
	if errors.As(err, &appErr) {
		return bulkErrDetail{Code: appErr.Code, Message: appErr.Message}
	}
	if errors.Is(err, repo.ErrNotFound) {
		return bulkErrDetail{Code: httpapi.CodeNotFound, Message: "task not found"}
	}
	if errors.Is(err, repo.ErrInvalidPlacement) || errors.Is(err, repo.ErrCycle) {
		return bulkErrDetail{Code: httpapi.CodeForbiddenPlacement, Message: "invalid task placement"}
	}
	return bulkErrDetail{Code: httpapi.CodeInternalError, Message: "internal error"}
}

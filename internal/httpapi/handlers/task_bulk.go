package handlers

import (
	"errors"
	"log/slog"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
)

const (
	opTaskBulkComplete      = "handler.Task.BulkComplete"
	opTaskBulkMove          = "handler.Task.BulkMove"
	opTaskBulkPriority      = "handler.Task.BulkPriority"
	opTaskGroup             = "handler.Task.Group"
	msgTooManyIDs           = "too many ids"
	msgInvalidPlacement     = "invalid placement"
	msgInvalidTaskPlacement = "invalid task placement"
	msgInvalidPriority      = "invalid priority"
)

// TaskBulkHandler handles bulk operations on tasks.
type TaskBulkHandler struct {
	completeSvc *service.CompleteService
	moveSvc     *service.MoveService
	groupSvc    *service.GroupService
	taskSvc     *service.TaskService
	baseURL     string
}

func NewTaskBulkHandler(completeSvc *service.CompleteService, moveSvc *service.MoveService, groupSvc *service.GroupService, taskSvc *service.TaskService, baseURL string) *TaskBulkHandler {
	return &TaskBulkHandler{completeSvc: completeSvc, moveSvc: moveSvc, groupSvc: groupSvc, taskSvc: taskSvc, baseURL: baseURL}
}

func (h *TaskBulkHandler) Register(r fiber.Router) {
	r.Post("/tasks/bulk/complete", httpapi.RequireScope(auth.ScopeTasksWrite), h.bulkComplete)
	r.Post("/tasks/bulk/move", httpapi.RequireScope(auth.ScopeTasksWrite), h.bulkMove)
	r.Post("/tasks/bulk/priority", httpapi.RequireScope(auth.ScopeTasksWrite), h.bulkPriority)
	r.Post("/tasks/group", httpapi.RequireScope(auth.ScopeTasksWrite), h.groupTasks)
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
	logEntry(c, opTaskBulkComplete)
	var req BulkIDsRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, opTaskBulkComplete, msgInvalidBody)
		return httpapi.ErrValidation(msgInvalidRequestBody)
	}
	if len(req.IDs) > 100 {
		logValidation(c, opTaskBulkComplete, msgTooManyIDs, slog.Int("count", len(req.IDs)))
		return httpapi.ErrValidation(msgTooManyIDs)
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
	logMutation(c, opTaskBulkComplete, slog.Int("succeeded", len(resp.Succeeded)), slog.Int("failed", len(resp.Failed)))
	return c.JSON(resp)
}

func (h *TaskBulkHandler) bulkMove(c fiber.Ctx) error {
	logEntry(c, opTaskBulkMove)
	var req BulkMoveRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, opTaskBulkMove, msgInvalidBody)
		return httpapi.ErrValidation(msgInvalidRequestBody)
	}
	if len(req.IDs) > 100 {
		logValidation(c, opTaskBulkMove, msgTooManyIDs, slog.Int("count", len(req.IDs)))
		return httpapi.ErrValidation(msgTooManyIDs)
	}

	target := repo.Placement{
		InboxID:   req.InboxID,
		ContextID: req.ContextID,
		ProjectID: req.ProjectID,
		SectionID: req.SectionID,
		ParentID:  req.ParentID,
	}
	if err := target.Validate(); err != nil {
		logValidation(c, opTaskBulkMove, msgInvalidPlacement)
		return httpapi.ErrForbiddenPlacement(msgInvalidTaskPlacement)
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
	logMutation(c, opTaskBulkMove, slog.Int("succeeded", len(resp.Succeeded)), slog.Int("failed", len(resp.Failed)))
	return c.JSON(resp)
}

// BulkPriorityRequest is the body for bulk set-priority.
type BulkPriorityRequest struct {
	IDs      []int64 `json:"ids"`
	Priority string  `json:"priority"`
}

func (h *TaskBulkHandler) bulkPriority(c fiber.Ctx) error {
	logEntry(c, opTaskBulkPriority)
	var req BulkPriorityRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, opTaskBulkPriority, msgInvalidBody)
		return httpapi.ErrValidation(msgInvalidRequestBody)
	}
	if len(req.IDs) > 100 {
		logValidation(c, opTaskBulkPriority, msgTooManyIDs, slog.Int("count", len(req.IDs)))
		return httpapi.ErrValidation(msgTooManyIDs)
	}
	p := model.Priority(req.Priority)
	if !p.IsValid() {
		logValidation(c, opTaskBulkPriority, msgInvalidPriority)
		return httpapi.ErrValidation(msgInvalidPriority)
	}

	resp := bulkResponse{
		Succeeded: make([]int64, 0),
		Failed:    make([]bulkFailedItem, 0),
	}
	for _, id := range req.IDs {
		_, err := h.taskSvc.SetPriority(c.Context(), id, p)
		if err != nil {
			resp.Failed = append(resp.Failed, bulkFailedItem{ID: id, Error: toErrDetail(err)})
		} else {
			resp.Succeeded = append(resp.Succeeded, id)
		}
	}
	logMutation(c, opTaskBulkPriority, slog.Int("succeeded", len(resp.Succeeded)), slog.Int("failed", len(resp.Failed)))
	return c.JSON(resp)
}

// GroupTasksResponse is the body for POST /tasks/group.
type GroupTasksResponse struct {
	Parent    dto.TaskDTO      `json:"parent"`
	Succeeded []int64          `json:"succeeded"`
	Failed    []bulkFailedItem `json:"failed"`
}

func (h *TaskBulkHandler) groupTasks(c fiber.Ctx) error {
	logEntry(c, opTaskGroup)
	var req dto.GroupTasksRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, opTaskGroup, msgInvalidBody)
		return httpapi.ErrValidation(msgInvalidRequestBody)
	}

	placement := repo.Placement{
		ContextID: req.ContextID,
		ProjectID: req.ProjectID,
		SectionID: req.SectionID,
	}
	if err := placement.Validate(); err != nil {
		logValidation(c, opTaskGroup, msgInvalidPlacement)
		return httpapi.ErrForbiddenPlacement(msgInvalidTaskPlacement)
	}

	create, appErr := buildTaskCreate(req.CreateTaskRequest, placement)
	if appErr != nil {
		logValidation(c, opTaskGroup, appErr.Message)
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
			logValidation(c, opTaskGroup, err.Error())
			return httpapi.ErrValidation(err.Error())
		}
		if errors.Is(err, repo.ErrInvalidPlacement) {
			logValidation(c, opTaskGroup, msgInvalidPlacement)
			return httpapi.ErrForbiddenPlacement(msgInvalidTaskPlacement)
		}
		var ule *service.UnknownLabelError
		if errors.As(err, &ule) {
			logValidation(c, opTaskGroup, "unknown label", slog.String("label", ule.Name))
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
	logMutation(c, opTaskGroup, slog.Int64("parent_id", res.Parent.ID), slog.Int("succeeded", len(resp.Succeeded)), slog.Int("failed", len(resp.Failed)))
	return c.Status(fiber.StatusCreated).JSON(resp)
}

// toErrDetail converts a service/repo error to a bulk error detail.
func toErrDetail(err error) bulkErrDetail {
	// Before the generic AppError branch: a blocked task must be reported per-item
	// as `task_blocked` rather than collapsing into "internal error", so the user
	// can see which of a bulk selection was held back and why.
	if blocked := taskBlockedErr(err); blocked != nil {
		return bulkErrDetail{Code: blocked.Code, Message: blocked.Message}
	}
	var appErr *httpapi.AppError
	if errors.As(err, &appErr) {
		return bulkErrDetail{Code: appErr.Code, Message: appErr.Message}
	}
	if errors.Is(err, repo.ErrNotFound) {
		return bulkErrDetail{Code: httpapi.CodeNotFound, Message: msgTaskNotFound}
	}
	if errors.Is(err, repo.ErrInvalidPlacement) || errors.Is(err, repo.ErrCycle) {
		return bulkErrDetail{Code: httpapi.CodeForbiddenPlacement, Message: msgInvalidTaskPlacement}
	}
	if errors.Is(err, service.ErrPriorityManagedByTroiki) {
		return bulkErrDetail{Code: httpapi.CodeValidationFailed, Message: "priority is managed by Troiki category"}
	}
	return bulkErrDetail{Code: httpapi.CodeInternalError, Message: "internal error"}
}

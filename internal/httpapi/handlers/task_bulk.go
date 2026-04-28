package handlers

import (
	"errors"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
)

// TaskBulkHandler handles bulk operations on tasks.
type TaskBulkHandler struct {
	completeSvc *service.CompleteService
	moveSvc     *service.MoveService
	baseURL     string
}

func NewTaskBulkHandler(completeSvc *service.CompleteService, moveSvc *service.MoveService, baseURL string) *TaskBulkHandler {
	return &TaskBulkHandler{completeSvc: completeSvc, moveSvc: moveSvc, baseURL: baseURL}
}

func (h *TaskBulkHandler) Register(r fiber.Router) {
	r.Post("/tasks/bulk/complete", h.bulkComplete)
	r.Post("/tasks/bulk/move", h.bulkMove)
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
	var req BulkIDsRequest
	if err := c.Bind().JSON(&req); err != nil {
		return httpapi.ErrValidation("invalid request body")
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
	return c.JSON(resp)
}

func (h *TaskBulkHandler) bulkMove(c fiber.Ctx) error {
	var req BulkMoveRequest
	if err := c.Bind().JSON(&req); err != nil {
		return httpapi.ErrValidation("invalid request body")
	}

	target := repo.Placement{
		InboxID:   req.InboxID,
		ContextID: req.ContextID,
		ProjectID: req.ProjectID,
		SectionID: req.SectionID,
		ParentID:  req.ParentID,
	}
	if err := target.Validate(); err != nil {
		return httpapi.ErrForbiddenPlacement("invalid task placement")
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
	return c.JSON(resp)
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

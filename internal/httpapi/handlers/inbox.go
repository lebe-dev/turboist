package handlers

import (
	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
)

const inboxID = int64(1)

// InboxHandler handles /api/v1/inbox endpoints.
type InboxHandler struct {
	tasks   *repo.TaskRepo
	taskSvc *service.TaskService
	cfg     *config.Config
	baseURL string
}

// NewInboxHandler constructs an InboxHandler.
func NewInboxHandler(tasks *repo.TaskRepo, taskSvc *service.TaskService, cfg *config.Config, baseURL string) *InboxHandler {
	return &InboxHandler{tasks: tasks, taskSvc: taskSvc, cfg: cfg, baseURL: baseURL}
}

// Register wires inbox routes onto r.
func (h *InboxHandler) Register(r fiber.Router) {
	r.Get("/", h.get)
	r.Post("/tasks", h.createTask)
}

type inboxResponse struct {
	Count                 int           `json:"count"`
	WarnThresholdExceeded bool          `json:"warnThresholdExceeded"`
	Tasks                 []dto.TaskDTO `json:"tasks"`
}

func (h *InboxHandler) get(c fiber.Ctx) error {
	tasks, total, err := h.tasks.ListInbox(c.Context(), repo.TaskFilter{}, repo.Page{Limit: 200})
	if err != nil {
		return httpapi.ErrInternal("list inbox")
	}
	dtos := make([]dto.TaskDTO, len(tasks))
	for i, t := range tasks {
		dtos[i] = dto.TaskFromModel(t, h.baseURL)
	}
	return c.JSON(inboxResponse{
		Count:                 total,
		WarnThresholdExceeded: total >= h.cfg.Inbox.WarnThreshold,
		Tasks:                 dtos,
	})
}

func (h *InboxHandler) createTask(c fiber.Ctx) error {
	var req dto.CreateTaskRequest
	if err := c.Bind().JSON(&req); err != nil {
		return httpapi.ErrValidation("invalid request body")
	}
	if req.Title == "" {
		return httpapi.ErrValidation("title is required")
	}
	id := inboxID
	return doCreateTask(c, h.taskSvc, repo.Placement{InboxID: &id}, req, h.baseURL)
}

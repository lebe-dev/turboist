package handlers

import (
	"errors"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
)

// TroikiHandler exposes the Troiki view and category-assignment endpoints.
type TroikiHandler struct {
	svc     *service.TroikiService
	baseURL string
}

func NewTroikiHandler(svc *service.TroikiService, baseURL string) *TroikiHandler {
	return &TroikiHandler{svc: svc, baseURL: baseURL}
}

// Register wires routes onto the authenticated /api/v1 group.
func (h *TroikiHandler) Register(r fiber.Router) {
	r.Get("/troiki", h.view)
	r.Post("/tasks/:id/troiki", h.setCategory)
}

type troikiSlotDTO struct {
	Capacity int           `json:"capacity"`
	Tasks    []dto.TaskDTO `json:"tasks"`
}

type troikiViewDTO struct {
	Important troikiSlotDTO `json:"important"`
	Medium    troikiSlotDTO `json:"medium"`
	Rest      troikiSlotDTO `json:"rest"`
}

func (h *TroikiHandler) toSlot(s service.TroikiSlot) troikiSlotDTO {
	tasks := make([]dto.TaskDTO, len(s.Tasks))
	for i, t := range s.Tasks {
		tasks[i] = dto.TaskFromModel(t, h.baseURL)
	}
	return troikiSlotDTO{Capacity: s.Capacity, Tasks: tasks}
}

func (h *TroikiHandler) view(c fiber.Ctx) error {
	v, err := h.svc.View(c.Context())
	if err != nil {
		return httpapi.ErrInternal("troiki view")
	}
	return c.JSON(troikiViewDTO{
		Important: h.toSlot(v.Important),
		Medium:    h.toSlot(v.Medium),
		Rest:      h.toSlot(v.Rest),
	})
}

// SetTroikiCategoryRequest is the body for POST /tasks/:id/troiki.
// `category` is one of "important", "medium", "rest", or null to clear.
type SetTroikiCategoryRequest struct {
	Category *string `json:"category"`
}

func (h *TroikiHandler) setCategory(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	var req SetTroikiCategoryRequest
	if err := c.Bind().JSON(&req); err != nil {
		return httpapi.ErrValidation("invalid request body")
	}
	var cat *model.TroikiCategory
	if req.Category != nil {
		v := model.TroikiCategory(*req.Category)
		if !v.IsValid() {
			return httpapi.ErrValidation("invalid troiki category")
		}
		cat = &v
	}
	t, err := h.svc.SetCategory(c.Context(), id, cat)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("task not found")
		}
		if errors.Is(err, service.ErrTroikiSlotFull) {
			return httpapi.ErrTroikiSlotFull("troiki slot is full")
		}
		if errors.Is(err, service.ErrTroikiNotRootTask) {
			return httpapi.ErrForbiddenPlacement("troiki category requires a root open task")
		}
		return httpapi.ErrInternal("set troiki category")
	}
	return c.JSON(dto.TaskFromModel(*t, h.baseURL))
}

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

// TroikiHandler exposes the Troiki view, start, and category-assignment endpoints.
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
	r.Post("/troiki/start", h.start)
	r.Post("/projects/:id/troiki", h.setProjectCategory)
}

type troikiProjectDTO struct {
	dto.ProjectDTO
	Tasks []dto.TaskDTO `json:"tasks"`
}

type troikiSlotDTO struct {
	Capacity int                `json:"capacity"`
	Projects []troikiProjectDTO `json:"projects"`
}

type troikiViewDTO struct {
	Important troikiSlotDTO `json:"important"`
	Medium    troikiSlotDTO `json:"medium"`
	Rest      troikiSlotDTO `json:"rest"`
	Started   bool          `json:"started"`
}

func (h *TroikiHandler) toSlot(s service.TroikiSlotProject) troikiSlotDTO {
	projects := make([]troikiProjectDTO, len(s.Projects))
	for i, p := range s.Projects {
		tasks := s.Tasks[p.ID]
		taskDTOs := make([]dto.TaskDTO, len(tasks))
		for j, t := range tasks {
			taskDTOs[j] = dto.TaskFromModel(t, h.baseURL)
		}
		projects[i] = troikiProjectDTO{
			ProjectDTO: dto.ProjectFromModel(p),
			Tasks:      taskDTOs,
		}
	}
	return troikiSlotDTO{Capacity: s.Capacity, Projects: projects}
}

func (h *TroikiHandler) renderView(v service.TroikiView) troikiViewDTO {
	return troikiViewDTO{
		Important: h.toSlot(v.Important),
		Medium:    h.toSlot(v.Medium),
		Rest:      h.toSlot(v.Rest),
		Started:   v.Started,
	}
}

func (h *TroikiHandler) view(c fiber.Ctx) error {
	v, err := h.svc.View(c.Context())
	if err != nil {
		return httpapi.ErrInternal("troiki view")
	}
	return c.JSON(h.renderView(v))
}

func (h *TroikiHandler) start(c fiber.Ctx) error {
	if err := h.svc.Start(c.Context()); err != nil {
		return httpapi.ErrInternal("troiki start")
	}
	v, err := h.svc.View(c.Context())
	if err != nil {
		return httpapi.ErrInternal("troiki view")
	}
	return c.JSON(h.renderView(v))
}

// SetTroikiCategoryRequest is the body for POST /projects/:id/troiki.
// `category` is one of "important", "medium", "rest", or null to clear.
type SetTroikiCategoryRequest struct {
	Category *string `json:"category"`
}

func (h *TroikiHandler) setProjectCategory(c fiber.Ctx) error {
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
	p, err := h.svc.SetCategory(c.Context(), id, cat)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound("project not found")
		}
		if errors.Is(err, service.ErrTroikiSlotFull) {
			return httpapi.ErrTroikiSlotFull("troiki slot is full")
		}
		if errors.Is(err, service.ErrTroikiInvalidProject) {
			return httpapi.ErrForbiddenPlacement("troiki category requires an open project")
		}
		return httpapi.ErrInternal("set troiki category")
	}
	return c.JSON(dto.ProjectFromModel(*p))
}

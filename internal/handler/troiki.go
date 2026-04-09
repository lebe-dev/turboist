package handler

import (
	"errors"

	"github.com/charmbracelet/log"
	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/troiki"
)

type TroikiHandler struct {
	service *troiki.Service
}

func NewTroikiHandler(service *troiki.Service) *TroikiHandler {
	return &TroikiHandler{service: service}
}

// State handles GET /api/troiki — returns the current troiki state.
func (h *TroikiHandler) State(c fiber.Ctx) error {
	state, err := h.service.ComputeState()
	if err != nil {
		log.Error("troiki compute state failed", "err", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(state)
}

type createTroikiTaskRequest struct {
	SectionClass string `json:"section_class"`
	Content      string `json:"content"`
	Description  string `json:"description"`
}

// Completed handles GET /api/troiki/completed — returns completed root tasks per section.
func (h *TroikiHandler) Completed(c fiber.Ctx) error {
	sections, err := h.service.FetchCompletedTasks(c.Context())
	if err != nil {
		log.Error("troiki fetch completed failed", "err", err)
		return todoistErrorResponse(c, err)
	}
	return c.JSON(fiber.Map{"sections": sections})
}

// CreateTask handles POST /api/troiki/tasks — creates a task in the specified troiki section.
func (h *TroikiHandler) CreateTask(c fiber.Ctx) error {
	var req createTroikiTaskRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.Content == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "content is required"})
	}

	class := troiki.SectionClass(req.SectionClass)
	if class != troiki.Important && class != troiki.Medium && class != troiki.Rest {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid section_class"})
	}

	id, err := h.service.AddTask(c.Context(), class, req.Content, req.Description)
	if err != nil {
		if errors.Is(err, troiki.ErrNoCapacity) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "no capacity available"})
		}
		log.Error("troiki add task failed", "err", err)
		return todoistErrorResponse(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"ok": true, "id": id})
}

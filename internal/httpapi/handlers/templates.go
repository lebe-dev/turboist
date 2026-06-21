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
	opTemplateCreate      = "handler.Template.Create"
	opTemplatePatch       = "handler.Template.Patch"
	opTemplateInstantiate = "handler.Template.Instantiate"
	msgTemplateNotFound   = "template not found"
)

// TemplateHandler implements CRUD for /api/v1/task-templates plus the
// instantiate action that materializes a template into a project.
type TemplateHandler struct {
	templates *repo.TemplateRepo
	svc       *service.TemplateService
	baseURL   string
}

// NewTemplateHandler constructs a TemplateHandler.
func NewTemplateHandler(templates *repo.TemplateRepo, svc *service.TemplateService, baseURL string) *TemplateHandler {
	return &TemplateHandler{templates: templates, svc: svc, baseURL: baseURL}
}

// Register wires template routes onto r.
func (h *TemplateHandler) Register(r fiber.Router) {
	r.Get("/", httpapi.RequireScope(auth.ScopeTemplatesRead), h.list)
	r.Post("/", httpapi.RequireScope(auth.ScopeTemplatesWrite), h.create)
	r.Get("/:id", httpapi.RequireScope(auth.ScopeTemplatesRead), h.get)
	r.Patch("/:id", httpapi.RequireScope(auth.ScopeTemplatesWrite), h.patch)
	r.Delete("/:id", httpapi.RequireScope(auth.ScopeTemplatesWrite), h.delete)
	r.Post("/:id/instantiate", httpapi.RequireScope(auth.ScopeTasksWrite), h.instantiate)
}

func (h *TemplateHandler) list(c fiber.Ctx) error {
	items, err := h.templates.List(c.Context())
	if err != nil {
		return httpapi.ErrInternal("list templates").WithCause(err)
	}
	dtos := make([]dto.TaskTemplateDTO, len(items))
	for i, t := range items {
		dtos[i] = dto.TaskTemplateFromModel(t)
	}
	return c.JSON(dto.NewPagedResponse(dtos, len(dtos), len(dtos), 0))
}

func (h *TemplateHandler) get(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	t, err := h.templates.Get(c.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound(msgTemplateNotFound)
		}
		return httpapi.ErrInternal("get template").WithCause(err)
	}
	return c.JSON(dto.TaskTemplateFromModel(*t))
}

func (h *TemplateHandler) create(c fiber.Ctx) error {
	logEntry(c, opTemplateCreate)
	var req dto.TaskTemplateRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, opTemplateCreate, msgInvalidBody)
		return httpapi.ErrValidation(msgInvalidRequestBody)
	}
	in, appErr := buildTemplateInput(req)
	if appErr != nil {
		logValidation(c, opTemplateCreate, appErr.Message)
		return appErr
	}
	t, err := h.templates.Create(c.Context(), in)
	if err != nil {
		return httpapi.ErrInternal("create template").WithCause(err)
	}
	logMutation(c, opTemplateCreate, slog.Int64("template_id", t.ID))
	return c.Status(fiber.StatusCreated).JSON(dto.TaskTemplateFromModel(*t))
}

func (h *TemplateHandler) patch(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, opTemplatePatch, slog.Int64("template_id", id))
	var req dto.TaskTemplateRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, opTemplatePatch, msgInvalidBody)
		return httpapi.ErrValidation(msgInvalidRequestBody)
	}
	in, appErr := buildTemplateInput(req)
	if appErr != nil {
		logValidation(c, opTemplatePatch, appErr.Message)
		return appErr
	}
	t, err := h.templates.Update(c.Context(), id, in)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound(msgTemplateNotFound)
		}
		return httpapi.ErrInternal("update template").WithCause(err)
	}
	logMutation(c, opTemplatePatch, slog.Int64("template_id", t.ID))
	return c.JSON(dto.TaskTemplateFromModel(*t))
}

func (h *TemplateHandler) delete(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, "handler.Template.Delete", slog.Int64("template_id", id))
	if err := h.templates.Delete(c.Context(), id); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return httpapi.ErrNotFound(msgTemplateNotFound)
		}
		return httpapi.ErrInternal("delete template").WithCause(err)
	}
	logMutation(c, "handler.Template.Delete", slog.Int64("template_id", id))
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *TemplateHandler) instantiate(c fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	logEntry(c, opTemplateInstantiate, slog.Int64("template_id", id))
	var req dto.InstantiateTemplateRequest
	if err := c.Bind().JSON(&req); err != nil {
		logValidation(c, opTemplateInstantiate, msgInvalidBody)
		return httpapi.ErrValidation(msgInvalidRequestBody)
	}
	if req.ProjectID <= 0 {
		logValidation(c, opTemplateInstantiate, "projectId required")
		return httpapi.ErrValidation("projectId is required")
	}
	res, err := h.svc.Instantiate(c.Context(), id, req.ProjectID)
	if err != nil {
		if errors.Is(err, service.ErrTemplateNotFound) {
			return httpapi.ErrNotFound(msgTemplateNotFound)
		}
		if errors.Is(err, service.ErrProjectNotFound) {
			return httpapi.ErrNotFound("project not found")
		}
		return httpapi.ErrInternal("instantiate template").WithCause(err)
	}
	subtasks := make([]dto.TaskDTO, len(res.Subtasks))
	for i, st := range res.Subtasks {
		subtasks[i] = dto.TaskFromModel(st, h.baseURL)
	}
	logMutation(c, opTemplateInstantiate,
		slog.Int64("template_id", id), slog.Int64("root_id", res.Root.ID), slog.Int("subtasks", len(subtasks)))
	return c.Status(fiber.StatusCreated).JSON(dto.InstantiateTemplateResponse{
		Root:     dto.TaskFromModel(*res.Root, h.baseURL),
		Subtasks: subtasks,
	})
}

// buildTemplateInput validates a template request and maps it to repo input.
func buildTemplateInput(req dto.TaskTemplateRequest) (repo.TemplateInput, *httpapi.AppError) {
	if req.Name == "" {
		return repo.TemplateInput{}, httpapi.ErrValidation("name is required")
	}
	priority, dayPart, appErr := parsePriorityDayPart(req.Priority, req.DayPart)
	if appErr != nil {
		return repo.TemplateInput{}, appErr
	}
	subtasks := make([]repo.TemplateSubtaskInput, 0, len(req.Subtasks))
	for _, st := range req.Subtasks {
		if st.Title == "" {
			return repo.TemplateInput{}, httpapi.ErrValidation("subtask title is required")
		}
		sp, sd, appErr := parsePriorityDayPart(st.Priority, st.DayPart)
		if appErr != nil {
			return repo.TemplateInput{}, appErr
		}
		subtasks = append(subtasks, repo.TemplateSubtaskInput{
			Title:       st.Title,
			Description: st.Description,
			Priority:    sp,
			DayPart:     sd,
			LabelIDs:    st.LabelIDs,
		})
	}
	return repo.TemplateInput{
		Name:        req.Name,
		Description: req.Description,
		Priority:    priority,
		DayPart:     dayPart,
		LabelIDs:    req.LabelIDs,
		Subtasks:    subtasks,
	}, nil
}

func parsePriorityDayPart(priorityStr, dayPartStr string) (model.Priority, model.DayPart, *httpapi.AppError) {
	priority := model.PriorityNone
	if priorityStr != "" {
		priority = model.Priority(priorityStr)
		if !priority.IsValid() {
			return "", "", httpapi.ErrValidation("invalid priority")
		}
	}
	dayPart := model.DayPartNone
	if dayPartStr != "" {
		dayPart = model.DayPart(dayPartStr)
		if !dayPart.IsValid() {
			return "", "", httpapi.ErrValidation("invalid dayPart")
		}
	}
	return priority, dayPart, nil
}

package handlers

import (
	"errors"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
	rrule "github.com/teambition/rrule-go"
)

func parseID(c fiber.Ctx) (int64, error) {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, httpapi.ErrValidation("invalid id")
	}
	return id, nil
}

var validNamedColors = map[string]struct{}{
	"red": {}, "orange": {}, "yellow": {}, "green": {},
	"teal": {}, "blue": {}, "purple": {}, "pink": {},
	"grey": {}, "brown": {},
}

func isValidColor(c string) bool {
	if _, ok := validNamedColors[c]; ok {
		return true
	}
	if len(c) == 7 && c[0] == '#' {
		for _, ch := range strings.ToLower(c[1:]) {
			if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
				return false
			}
		}
		return true
	}
	return false
}

func buildTaskCreate(req dto.CreateTaskRequest, placement repo.Placement) (repo.CreateTask, *httpapi.AppError) {
	priority := model.PriorityNone
	if req.Priority != "" {
		priority = model.Priority(req.Priority)
		if !priority.IsValid() {
			return repo.CreateTask{}, httpapi.ErrValidation("invalid priority")
		}
	}
	dayPart := model.DayPartNone
	if req.DayPart != "" {
		dayPart = model.DayPart(req.DayPart)
		if !dayPart.IsValid() {
			return repo.CreateTask{}, httpapi.ErrValidation("invalid dayPart")
		}
	}
	planState := model.PlanStateNone
	if req.PlanState != "" {
		planState = model.PlanState(req.PlanState)
		if !planState.IsValid() {
			return repo.CreateTask{}, httpapi.ErrValidation("invalid planState")
		}
	}
	in := repo.CreateTask{
		Placement:   placement,
		Title:       req.Title,
		Description: req.Description,
		Priority:    priority,
		DayPart:     dayPart,
		PlanState:   planState,
	}
	if req.DueAt != nil {
		t, err := model.ParseUTC(*req.DueAt)
		if err != nil {
			return repo.CreateTask{}, httpapi.ErrValidation("invalid dueAt format")
		}
		in.DueAt = &t
		in.DueHasTime = req.DueHasTime
	}
	if req.DeadlineAt != nil {
		t, err := model.ParseUTC(*req.DeadlineAt)
		if err != nil {
			return repo.CreateTask{}, httpapi.ErrValidation("invalid deadlineAt format")
		}
		in.DeadlineAt = &t
		in.DeadlineHasTime = req.DeadlineHasTime
	}
	if req.RecurrenceRule != nil {
		if _, err := rrule.StrToRRule(*req.RecurrenceRule); err != nil {
			return repo.CreateTask{}, httpapi.ErrValidation("invalid recurrenceRule")
		}
	}
	in.RecurrenceRule = req.RecurrenceRule
	return in, nil
}

// doCreateTask is the shared task-creation flow used by container handlers.
func doCreateTask(c fiber.Ctx, svc *service.TaskService, placement repo.Placement, req dto.CreateTaskRequest, baseURL string) error {
	in, appErr := buildTaskCreate(req, placement)
	if appErr != nil {
		return appErr
	}
	t, err := svc.Create(c.Context(), in, req.Labels, req.RemovedAutoLabels)
	if err != nil {
		var ule *service.UnknownLabelError
		if errors.As(err, &ule) {
			return httpapi.ErrValidation("unknown label: " + ule.Name)
		}
		if errors.Is(err, repo.ErrInvalidPlacement) {
			return httpapi.ErrForbiddenPlacement("invalid task placement")
		}
		return httpapi.ErrInternal("create task")
	}
	return c.Status(fiber.StatusCreated).JSON(dto.TaskFromModel(*t, baseURL))
}

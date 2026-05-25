package handlers

import (
	"errors"
	"log/slog"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
	rrule "github.com/teambition/rrule-go"
)

// logEntry emits a DEBUG record marking the start of a handler invocation.
// op should be "handler.<Domain>.<Action>" — used both as message and `op` attr.
func logEntry(c fiber.Ctx, op string, attrs ...any) {
	ctx := c.Context()
	args := append([]any{slog.String("op", op)}, attrs...)
	logging.FromContext(ctx).DebugContext(ctx, op, args...)
}

// logValidation emits a WARN record before returning a validation error.
func logValidation(c fiber.Ctx, op, reason string, attrs ...any) {
	ctx := c.Context()
	args := append([]any{slog.String("op", op), slog.String("reason", reason)}, attrs...)
	logging.FromContext(ctx).WarnContext(ctx, op+": validation failed", args...)
}

// logMutation emits an INFO record after a successful mutating handler.
func logMutation(c fiber.Ctx, op string, attrs ...any) {
	ctx := c.Context()
	args := append([]any{slog.String("op", op)}, attrs...)
	logging.FromContext(ctx).InfoContext(ctx, op+": ok", args...)
}

func parseID(c fiber.Ctx) (int64, error) {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		logValidation(c, "handler.parseID", "invalid id", slog.String("raw", c.Params("id")))
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
	in.ClientID = req.ClientID
	return in, nil
}

// doCreateTask is the shared task-creation flow used by container handlers.
func doCreateTask(c fiber.Ctx, svc *service.TaskService, placement repo.Placement, req dto.CreateTaskRequest, baseURL string) error {
	in, appErr := buildTaskCreate(req, placement)
	if appErr != nil {
		logValidation(c, "handler.Task.Create", appErr.Message)
		return appErr
	}
	t, err := svc.Create(c.Context(), in, req.Labels, req.RemovedAutoLabels)
	if err != nil {
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
	logMutation(c, "handler.Task.Create", slog.Int64("task_id", t.ID))
	return c.Status(fiber.StatusCreated).JSON(dto.TaskFromModel(*t, baseURL))
}

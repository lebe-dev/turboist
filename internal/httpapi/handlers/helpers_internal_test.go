package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// runWithIDParam mounts a tiny route that calls parseID and surfaces the
// parsed value (or AppError) back to the test through closures.
func runWithIDParam(t *testing.T, idPathSegment string) (int64, *httpapi.AppError) {
	t.Helper()
	var gotID int64
	var gotErr *httpapi.AppError

	app := fiber.New()
	app.Get("/:id", func(c fiber.Ctx) error {
		id, err := parseID(c)
		if err != nil {
			var appErr *httpapi.AppError
			if errors.As(err, &appErr) {
				gotErr = appErr
			}
			return c.SendStatus(fiber.StatusOK)
		}
		gotID = id
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/"+idPathSegment, nil)
	resp, testErr := app.Test(req)
	if testErr != nil {
		t.Fatalf("app.Test: %v", testErr)
	}
	_ = resp.Body.Close()
	return gotID, gotErr
}

func TestParseID_Valid(t *testing.T) {
	id, err := runWithIDParam(t, "42")
	if err != nil {
		t.Fatalf("got AppError %+v, want nil", err)
	}
	if id != 42 {
		t.Errorf("got id %d, want 42", id)
	}
}

func TestParseID_Invalid(t *testing.T) {
	cases := []string{"abc", "-1", "0", "1.5", "%20"}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			id, err := runWithIDParam(t, in)
			if err == nil {
				t.Fatalf("got id=%d nil err, want validation_failed", id)
			}
			if err.Code != httpapi.CodeValidationFailed {
				t.Errorf("got code %q, want %q", err.Code, httpapi.CodeValidationFailed)
			}
		})
	}
}

func TestIsValidColor_Named(t *testing.T) {
	for name := range validNamedColors {
		if !isValidColor(name) {
			t.Errorf("named color %q: got false, want true", name)
		}
	}
}

func TestIsValidColor_Hex(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"#aabbcc", true},
		{"#ABCDEF", true},
		{"#012345", true},
		{"#abc", false},      // wrong length
		{"#aabbcz", false},   // non-hex char
		{"aabbcc", false},    // missing leading #
		{"", false},          // empty
		{"#aabbccdd", false}, // too long
		{"red ", false},      // trailing space, not in named map
		{"not-a-color", false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := isValidColor(tc.in)
			if got != tc.want {
				t.Errorf("isValidColor(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestBuildTaskCreate_Defaults(t *testing.T) {
	placement := repo.Placement{InboxID: ptr(int64(2))}
	in, appErr := buildTaskCreate(dto.CreateTaskRequest{Title: "x"}, placement)
	if appErr != nil {
		t.Fatalf("got AppError %+v, want nil", appErr)
	}
	if in.Title != "x" {
		t.Errorf("Title: got %q, want %q", in.Title, "x")
	}
	if in.Priority != model.PriorityNone {
		t.Errorf("Priority: got %q, want %q", in.Priority, model.PriorityNone)
	}
	if in.DayPart != model.DayPartNone {
		t.Errorf("DayPart: got %q, want %q", in.DayPart, model.DayPartNone)
	}
	if in.PlanState != model.PlanStateNone {
		t.Errorf("PlanState: got %q, want %q", in.PlanState, model.PlanStateNone)
	}
	if in.DueAt != nil || in.DeadlineAt != nil {
		t.Errorf("times: got Due=%v Deadline=%v, want nil/nil", in.DueAt, in.DeadlineAt)
	}
	if in.RecurrenceRule != nil {
		t.Errorf("RecurrenceRule: got %v, want nil", in.RecurrenceRule)
	}
	if in.InboxID == nil || *in.InboxID != 2 {
		t.Errorf("Placement.InboxID: got %v, want 2", in.InboxID)
	}
}

func TestBuildTaskCreate_AllFieldsValid(t *testing.T) {
	placement := repo.Placement{InboxID: ptr(int64(2))}
	due := "2026-04-27T14:30:45.000Z"
	deadline := "2026-04-28T09:00:00.000Z"
	rrule := "FREQ=DAILY"
	req := dto.CreateTaskRequest{
		Title:           "task",
		Description:     "desc",
		Priority:        string(model.PriorityHigh),
		DayPart:         string(model.DayPartMorning),
		PlanState:       string(model.PlanStateWeek),
		DueAt:           &due,
		DueHasTime:      true,
		DeadlineAt:      &deadline,
		DeadlineHasTime: true,
		RecurrenceRule:  &rrule,
	}
	in, appErr := buildTaskCreate(req, placement)
	if appErr != nil {
		t.Fatalf("got AppError %+v, want nil", appErr)
	}
	if in.Priority != model.PriorityHigh {
		t.Errorf("Priority: got %q, want %q", in.Priority, model.PriorityHigh)
	}
	if in.DayPart != model.DayPartMorning {
		t.Errorf("DayPart: got %q, want %q", in.DayPart, model.DayPartMorning)
	}
	if in.PlanState != model.PlanStateWeek {
		t.Errorf("PlanState: got %q, want %q", in.PlanState, model.PlanStateWeek)
	}
	if in.DueAt == nil || !in.DueHasTime {
		t.Errorf("DueAt/DueHasTime: got %v/%v", in.DueAt, in.DueHasTime)
	}
	if in.DeadlineAt == nil || !in.DeadlineHasTime {
		t.Errorf("DeadlineAt/DeadlineHasTime: got %v/%v", in.DeadlineAt, in.DeadlineHasTime)
	}
	if in.RecurrenceRule == nil || *in.RecurrenceRule != rrule {
		t.Errorf("RecurrenceRule: got %v, want %q", in.RecurrenceRule, rrule)
	}
}

func TestBuildTaskCreate_InvalidEnum(t *testing.T) {
	placement := repo.Placement{InboxID: ptr(int64(2))}
	cases := []struct {
		name    string
		req     dto.CreateTaskRequest
		wantMsg string
	}{
		{
			name:    "priority",
			req:     dto.CreateTaskRequest{Title: "x", Priority: "ultra"},
			wantMsg: "invalid priority",
		},
		{
			name:    "dayPart",
			req:     dto.CreateTaskRequest{Title: "x", DayPart: "midnight"},
			wantMsg: "invalid dayPart",
		},
		{
			name:    "planState",
			req:     dto.CreateTaskRequest{Title: "x", PlanState: "later"},
			wantMsg: "invalid planState",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, appErr := buildTaskCreate(tc.req, placement)
			if appErr == nil {
				t.Fatal("got nil AppError, want validation_failed")
			}
			if appErr.Code != httpapi.CodeValidationFailed {
				t.Errorf("Code: got %q, want %q", appErr.Code, httpapi.CodeValidationFailed)
			}
			if appErr.Message != tc.wantMsg {
				t.Errorf("Message: got %q, want %q", appErr.Message, tc.wantMsg)
			}
		})
	}
}

func TestBuildTaskCreate_InvalidTimeFormat(t *testing.T) {
	placement := repo.Placement{InboxID: ptr(int64(2))}
	bad := "2026-04-27T14:30:45Z" // missing .000Z

	t.Run("dueAt", func(t *testing.T) {
		_, appErr := buildTaskCreate(dto.CreateTaskRequest{Title: "x", DueAt: &bad}, placement)
		if appErr == nil || appErr.Message != "invalid dueAt format" {
			t.Fatalf("got %+v, want validation_failed/invalid dueAt format", appErr)
		}
	})

	t.Run("deadlineAt", func(t *testing.T) {
		_, appErr := buildTaskCreate(dto.CreateTaskRequest{Title: "x", DeadlineAt: &bad}, placement)
		if appErr == nil || appErr.Message != "invalid deadlineAt format" {
			t.Fatalf("got %+v, want validation_failed/invalid deadlineAt format", appErr)
		}
	})
}

func TestBuildTaskCreate_InvalidRRule(t *testing.T) {
	placement := repo.Placement{InboxID: ptr(int64(2))}
	bad := "not a real rrule"
	_, appErr := buildTaskCreate(dto.CreateTaskRequest{Title: "x", RecurrenceRule: &bad}, placement)
	if appErr == nil {
		t.Fatal("got nil AppError, want validation_failed")
	}
	if appErr.Code != httpapi.CodeValidationFailed {
		t.Errorf("Code: got %q, want %q", appErr.Code, httpapi.CodeValidationFailed)
	}
	if appErr.Message != "invalid recurrenceRule" {
		t.Errorf("Message: got %q, want %q", appErr.Message, "invalid recurrenceRule")
	}
}

package httpapi

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/service"
)

func TestResolveAppError_StatusAndCode(t *testing.T) {
	locked := &service.TodayLockedError{Until: time.Date(2026, 11, 11, 21, 0, 0, 0, time.UTC)}
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"app_error_404", ErrNotFound("nope"), 404, CodeNotFound},
		{"app_error_500", ErrInternal("boom"), 500, CodeInternalError},
		{"fiber_error", fiber.NewError(418, "teapot"), 418, CodeInternalError},
		{"plain_error_defaults_500", errors.New("x"), 500, CodeInternalError},
		{"wrapped_app_error", fmt.Errorf("ctx: %w", ErrConflict("dup")), 409, CodeConflict},
		{"joined_app_error", errors.Join(errors.New("ctx"), ErrConflict("dup")), 409, CodeConflict},
		{"typed_nil_app_error", (*AppError)(nil), 500, CodeInternalError},
		{
			"today_locked_behind_internal_error",
			ErrInternal("complete task").WithCause(fmt.Errorf("group: %w", locked)),
			409, CodeTodayLocked,
		},
		{"today_locked_bare", locked, 409, CodeTodayLocked},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			appErr := resolveAppError(tc.err)
			if appErr.HTTPStatus != tc.wantStatus {
				t.Errorf("status: got %d, want %d", appErr.HTTPStatus, tc.wantStatus)
			}
			if appErr.Code != tc.wantCode {
				t.Errorf("code: got %q, want %q", appErr.Code, tc.wantCode)
			}
		})
	}
}

// A refusal wrapped by a handler as an internal error is answered as the lock's
// 409 with its details, so the client is never told a business rule broke the
// server.
func TestResolveAppError_TodayLockedCarriesDetails(t *testing.T) {
	projectID := int64(7)
	locked := &service.TodayLockedError{
		Until:     time.Date(2026, 11, 11, 21, 0, 0, 0, time.UTC),
		ProjectID: &projectID,
	}
	appErr := resolveAppError(ErrInternal("create task").WithCause(locked))
	details, ok := appErr.Details.(map[string]any)
	if !ok {
		t.Fatalf("details: got %T, want map", appErr.Details)
	}
	if details["lockedUntil"] != "2026-11-11T21:00:00.000Z" {
		t.Errorf("lockedUntil: got %v", details["lockedUntil"])
	}
	if id, ok := details["projectId"].(*int64); !ok || id == nil || *id != projectID {
		t.Errorf("projectId: got %v, want %d", details["projectId"], projectID)
	}
}

func TestReportableError_UnwrapsAppErrorCause(t *testing.T) {
	cause := errors.New("db exploded")
	appErr := ErrInternal("internal server error").WithCause(cause)
	if got := reportableError(appErr); got != cause {
		t.Errorf("reportableError: got %v, want underlying cause %v", got, cause)
	}
}

func TestReportableError_KeepsAppErrorWithoutCause(t *testing.T) {
	appErr := ErrNotFound("nope")
	if got := reportableError(appErr); got != error(appErr) {
		t.Errorf("reportableError: got %v, want the AppError itself", got)
	}
}

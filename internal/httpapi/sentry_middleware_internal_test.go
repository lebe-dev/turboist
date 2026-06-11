package httpapi

import (
	"errors"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestStatusFromError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"app_error_404", ErrNotFound("nope"), 404},
		{"app_error_500", ErrInternal("boom"), 500},
		{"fiber_error", fiber.NewError(418, "teapot"), 418},
		{"plain_error_defaults_500", errors.New("x"), 500},
		{"wrapped_app_error", errors.Join(errors.New("ctx"), ErrConflict("dup")), 409},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := statusFromError(tc.err); got != tc.want {
				t.Errorf("statusFromError: got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestReportableError_UnwrapsAppErrorCause(t *testing.T) {
	cause := errors.New("db exploded")
	err := ErrInternal("internal server error").WithCause(cause)
	if got := reportableError(err); got != cause {
		t.Errorf("reportableError: got %v, want underlying cause %v", got, cause)
	}
}

func TestReportableError_KeepsAppErrorWithoutCause(t *testing.T) {
	err := ErrNotFound("nope")
	if got := reportableError(err); got != error(err) {
		t.Errorf("reportableError: got %v, want the AppError itself", got)
	}
}

func TestReportableError_KeepsPlainError(t *testing.T) {
	err := errors.New("raw")
	if got := reportableError(err); got != err {
		t.Errorf("reportableError: got %v, want %v", got, err)
	}
}

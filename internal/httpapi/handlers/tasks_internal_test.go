package handlers

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
)

// captureHandleTaskCreateErr runs handleTaskCreateErr inside a Fiber handler
// so a real fiber.Ctx (with active request) is available.
func captureHandleTaskCreateErr(t *testing.T, err error) *httpapi.AppError {
	t.Helper()
	var got *httpapi.AppError
	app := fiber.New()
	app.Get("/", func(c fiber.Ctx) error {
		got = handleTaskCreateErr(c, err)
		return nil
	})
	req := httptest.NewRequest("GET", "/", nil)
	if _, err := app.Test(req); err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	return got
}

func TestHandleTaskCreateErr_UnknownLabel(t *testing.T) {
	ule := &service.UnknownLabelError{Name: "missing"}
	got := captureHandleTaskCreateErr(t, ule)
	if got == nil {
		t.Fatalf("expected AppError, got nil")
	}
	if got.HTTPStatus != 422 && got.HTTPStatus != 400 {
		t.Errorf("status: got %d, want 4xx validation", got.HTTPStatus)
	}
	if got.Code != "validation_failed" && got.Code != "validation_error" {
		t.Logf("code: %s (informational)", got.Code)
	}
}

func TestHandleTaskCreateErr_InvalidPlacement(t *testing.T) {
	got := captureHandleTaskCreateErr(t, repo.ErrInvalidPlacement)
	if got == nil {
		t.Fatalf("expected AppError, got nil")
	}
	if got.HTTPStatus != 403 && got.HTTPStatus != 422 && got.HTTPStatus != 400 {
		t.Errorf("status: got %d, want 4xx", got.HTTPStatus)
	}
}

func TestHandleTaskCreateErr_GenericInternal(t *testing.T) {
	got := captureHandleTaskCreateErr(t, errors.New("kaboom"))
	if got == nil {
		t.Fatalf("expected AppError, got nil")
	}
	if got.HTTPStatus != 500 {
		t.Errorf("status: got %d, want 500", got.HTTPStatus)
	}
	if got.Unwrap() == nil {
		t.Errorf("expected unwrappable cause")
	}
}

func TestHandleTaskCreateErr_WrappedUnknownLabel(t *testing.T) {
	inner := &service.UnknownLabelError{Name: "wrapped"}
	wrapped := errors.Join(errors.New("outer"), inner)
	got := captureHandleTaskCreateErr(t, wrapped)
	if got == nil {
		t.Fatalf("expected AppError, got nil")
	}
	if got.HTTPStatus == 500 {
		t.Errorf("wrapped UnknownLabelError must still be detected via errors.As; got 500")
	}
}

package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
)

// TestBackupHandler_RestoreRejectsOversizedBody shrinks maxBackupUploadBytes
// so the handler's size guard can be exercised without allocating dozens of
// megabytes. Lives in the internal package because the cap is unexported.
func TestBackupHandler_RestoreRejectsOversizedBody(t *testing.T) {
	original := maxBackupUploadBytes
	maxBackupUploadBytes = 64
	t.Cleanup(func() { maxBackupUploadBytes = original })

	app := fiber.New(fiber.Config{ErrorHandler: testErrorHandler})
	h := &BackupHandler{}
	app.Post("/restore", h.restore)

	body := bytes.Repeat([]byte("x"), 128)
	req := httptest.NewRequest(http.MethodPost, "/restore", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

// testErrorHandler converts *httpapi.AppError to its declared HTTP status so
// the focused internal tests do not need to spin up the full app.
func testErrorHandler(c fiber.Ctx, err error) error {
	if appErr, ok := err.(*httpapi.AppError); ok {
		return c.Status(appErr.HTTPStatus).JSON(fiber.Map{"error": appErr.Message})
	}
	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
}

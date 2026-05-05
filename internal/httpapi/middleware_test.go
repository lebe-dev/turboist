package httpapi_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
)

func TestRequestIDMiddleware_Generates(t *testing.T) {
	app := fiber.New()
	app.Use(httpapi.RequestIDMiddleware())
	app.Get("/", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp := doRequest(t, app, req)
	defer func() { _ = resp.Body.Close() }()

	got := resp.Header.Get("X-Request-ID")
	if got == "" {
		t.Fatal("X-Request-ID empty, want generated UUID")
	}
	// UUID v4 string is 36 chars; we don't pin format strictly, but reject
	// obviously wrong lengths.
	if len(got) < 16 {
		t.Errorf("X-Request-ID = %q (len %d), want >= 16 chars", got, len(got))
	}
}

func TestRequestIDMiddleware_Propagates(t *testing.T) {
	app := fiber.New()
	app.Use(httpapi.RequestIDMiddleware())
	app.Get("/", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	const incoming = "trace-abc-123"
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", incoming)
	resp := doRequest(t, app, req)
	defer func() { _ = resp.Body.Close() }()

	got := resp.Header.Get("X-Request-ID")
	if got != incoming {
		t.Errorf("X-Request-ID = %q, want %q", got, incoming)
	}
}

// captureHandler is a minimal slog.Handler that stores every record emitted
// at or above the configured level so tests can inspect it.
type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(_ string) slog.Handler      { return h }

func TestAccessLogMiddleware_Logs(t *testing.T) {
	cap := &captureHandler{}
	logger := slog.New(cap)

	app := fiber.New()
	app.Use(httpapi.RequestIDMiddleware())
	app.Use(httpapi.AccessLogMiddleware(logger))
	app.Get("/ping", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusTeapot) })

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	resp := doRequest(t, app, req)
	defer func() { _ = resp.Body.Close() }()

	cap.mu.Lock()
	defer cap.mu.Unlock()
	if len(cap.records) != 1 {
		t.Fatalf("got %d records, want 1", len(cap.records))
	}
	rec := cap.records[0]
	if rec.Level != slog.LevelInfo {
		t.Errorf("level: got %v, want %v", rec.Level, slog.LevelInfo)
	}

	want := map[string]bool{
		"method":     false,
		"path":       false,
		"status":     false,
		"duration":   false,
		"request_id": false,
	}
	rec.Attrs(func(a slog.Attr) bool {
		if _, ok := want[a.Key]; ok {
			want[a.Key] = true
		}
		switch a.Key {
		case "method":
			if a.Value.String() != http.MethodGet {
				t.Errorf("method: got %q, want %q", a.Value.String(), http.MethodGet)
			}
		case "path":
			if a.Value.String() != "/ping" {
				t.Errorf("path: got %q, want %q", a.Value.String(), "/ping")
			}
		case "status":
			if a.Value.Int64() != int64(fiber.StatusTeapot) {
				t.Errorf("status: got %d, want %d", a.Value.Int64(), fiber.StatusTeapot)
			}
		case "request_id":
			if a.Value.String() == "" {
				t.Error("request_id is empty, want generated id")
			}
		}
		return true
	})
	for k, ok := range want {
		if !ok {
			t.Errorf("attr %q missing from record", k)
		}
	}
}

func TestGetClaims_NoMiddleware(t *testing.T) {
	app := fiber.New()
	var nilClaims bool
	app.Get("/", func(c fiber.Ctx) error {
		nilClaims = httpapi.GetClaims(c) == nil
		return c.SendStatus(fiber.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp := doRequest(t, app, req)
	defer func() { _ = resp.Body.Close() }()
	if !nilClaims {
		t.Error("GetClaims without AuthMiddleware: got non-nil, want nil")
	}
}

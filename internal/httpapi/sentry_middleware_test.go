package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	sentry "github.com/getsentry/sentry-go"
	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
)

// captureTransport is a synchronous Sentry transport that records every event
// in memory so tests can assert what the middleware reported.
type captureTransport struct {
	mu     sync.Mutex
	events []*sentry.Event
}

func (t *captureTransport) Configure(sentry.ClientOptions)        {}
func (t *captureTransport) SendEvent(e *sentry.Event)             { t.mu.Lock(); t.events = append(t.events, e); t.mu.Unlock() }
func (t *captureTransport) Flush(time.Duration) bool              { return true }
func (t *captureTransport) FlushWithContext(context.Context) bool { return true }
func (t *captureTransport) Close()                                {}

func (t *captureTransport) all() []*sentry.Event {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]*sentry.Event(nil), t.events...)
}

// bindSentry installs a capturing transport on the global hub for the duration
// of the test, then unbinds the client so other tests stay isolated.
func bindSentry(t *testing.T) *captureTransport {
	t.Helper()
	tr := &captureTransport{}
	if err := sentry.Init(sentry.ClientOptions{
		Dsn:       "https://test@localhost/1",
		Transport: tr,
	}); err != nil {
		t.Fatalf("sentry.Init: %v", err)
	}
	t.Cleanup(func() { sentry.CurrentHub().BindClient(nil) })
	return tr
}

func sentryTestApp() *fiber.App {
	app := httpapi.NewApp(httpapi.Deps{SentryEnabled: true})
	app.Get("/ok", func(c fiber.Ctx) error { return c.SendString("ok") })
	app.Get("/notfound", func(c fiber.Ctx) error { return httpapi.ErrNotFound("missing thing") })
	app.Get("/boom", func(c fiber.Ctx) error {
		return httpapi.ErrInternal("internal server error").WithCause(errBoom)
	})
	app.Get("/panic", func(c fiber.Ctx) error { panic("kaboom") })
	return app
}

var errBoom = &stringErr{"boom cause"}

type stringErr struct{ s string }

func (e *stringErr) Error() string { return e.s }

func get(t *testing.T, app *fiber.App, path string) int {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil))
	if err != nil {
		t.Fatalf("app.Test %s: %v", path, err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

func TestSentryMiddleware_IgnoresSuccess(t *testing.T) {
	tr := bindSentry(t)
	app := sentryTestApp()
	if status := get(t, app, "/ok"); status != http.StatusOK {
		t.Fatalf("status: got %d, want 200", status)
	}
	if n := len(tr.all()); n != 0 {
		t.Errorf("captured events: got %d, want 0", n)
	}
}

func TestSentryMiddleware_Captures4xx(t *testing.T) {
	tr := bindSentry(t)
	app := sentryTestApp()
	if status := get(t, app, "/notfound"); status != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404", status)
	}
	events := tr.all()
	if len(events) != 1 {
		t.Fatalf("captured events: got %d, want 1", len(events))
	}
	if tag := events[0].Tags["http.status"]; tag != "404" {
		t.Errorf("http.status tag: got %q, want 404", tag)
	}
}

func TestSentryMiddleware_Captures5xxWithUnderlyingCause(t *testing.T) {
	tr := bindSentry(t)
	app := sentryTestApp()
	if status := get(t, app, "/boom"); status != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want 500", status)
	}
	events := tr.all()
	if len(events) != 1 {
		t.Fatalf("captured events: got %d, want 1", len(events))
	}
	if !eventMentions(events[0], "boom cause") {
		t.Errorf("event should carry the underlying cause, got %+v", events[0].Exception)
	}
}

func TestSentryMiddleware_CapturesPanic(t *testing.T) {
	tr := bindSentry(t)
	app := sentryTestApp()
	if status := get(t, app, "/panic"); status != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want 500 (panic converted to clean 500)", status)
	}
	events := tr.all()
	if len(events) != 1 {
		t.Fatalf("captured events: got %d, want 1", len(events))
	}
	if !eventMentions(events[0], "kaboom") {
		t.Errorf("panic event should mention the panic value, got %+v", events[0].Exception)
	}
}

func TestAPIConfig_ServesFrontendSentryConfig(t *testing.T) {
	deps := httpapi.Deps{SentryFrontendDSN: "https://front@localhost/2", SentryEnvironment: "production"}
	app := httpapi.NewApp(deps)
	httpapi.RegisterRoutes(app, deps)

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/config", nil))
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	var body struct {
		Sentry struct {
			DSN         string `json:"dsn"`
			Environment string `json:"environment"`
		} `json:"sentry"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Sentry.DSN != "https://front@localhost/2" {
		t.Errorf("dsn: got %q", body.Sentry.DSN)
	}
	if body.Sentry.Environment != "production" {
		t.Errorf("environment: got %q", body.Sentry.Environment)
	}
}

func eventMentions(e *sentry.Event, substr string) bool {
	if strings.Contains(e.Message, substr) {
		return true
	}
	for _, ex := range e.Exception {
		if strings.Contains(ex.Value, substr) || strings.Contains(ex.Type, substr) {
			return true
		}
	}
	return false
}

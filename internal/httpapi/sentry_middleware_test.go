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
	"github.com/lebe-dev/turboist/internal/service"
)

// captureTransport is a synchronous Sentry transport that records every event
// in memory so tests can assert what the middleware reported.
type captureTransport struct {
	mu     sync.Mutex
	events []*sentry.Event
}

func (t *captureTransport) Configure(sentry.ClientOptions) {}
func (t *captureTransport) SendEvent(e *sentry.Event) {
	t.mu.Lock()
	t.events = append(t.events, e)
	t.mu.Unlock()
}
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
	app.Get("/badrequest", func(c fiber.Ctx) error { return httpapi.ErrValidation("malformed thing") })
	app.Get("/unauthorized", func(c fiber.Ctx) error { return httpapi.ErrAuthInvalid("nope") })
	app.Get("/notfound", func(c fiber.Ctx) error { return httpapi.ErrNotFound("missing thing") })
	app.Get("/boom", func(c fiber.Ctx) error {
		return httpapi.ErrInternal("internal server error").WithCause(errBoom)
	})
	app.Get("/tasks/:id", func(c fiber.Ctx) error {
		return httpapi.ErrInternal("load task").WithCause(errBoom)
	})
	app.Post("/tasks/:id/complete", func(c fiber.Ctx) error {
		return httpapi.ErrInternal("complete task").WithCause(&service.TodayLockedError{
			Until: time.Date(2026, 11, 23, 21, 0, 0, 0, time.UTC),
		})
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

func TestSentryMiddleware_Captures400(t *testing.T) {
	tr := bindSentry(t)
	app := sentryTestApp()
	if status := get(t, app, "/badrequest"); status != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400", status)
	}
	events := tr.all()
	if len(events) != 1 {
		t.Fatalf("captured events: got %d, want 1", len(events))
	}
	if tag := events[0].Tags["http.status"]; tag != "400" {
		t.Errorf("http.status tag: got %q, want 400", tag)
	}
	if tag := events[0].Tags["error.code"]; tag != httpapi.CodeValidationFailed {
		t.Errorf("error.code tag: got %q, want %q", tag, httpapi.CodeValidationFailed)
	}
}

func TestSentryMiddleware_DropsExpected4xx(t *testing.T) {
	tr := bindSentry(t)
	app := sentryTestApp()
	for _, path := range []string{"/unauthorized", "/notfound"} {
		if status := get(t, app, path); status < 400 || status >= 500 {
			t.Fatalf("%s status: got %d, want a 4xx", path, status)
		}
	}
	if n := len(tr.all()); n != 0 {
		t.Errorf("captured events: got %d, want 0 (401/404 are expected client errors)", n)
	}
}

// A business refusal wrapped by a handler as an internal error is answered as a
// 409 and must not reach Sentry as a server failure.
func TestSentryMiddleware_DropsWrappedBusinessRefusal(t *testing.T) {
	tr := bindSentry(t)
	app := sentryTestApp()
	resp, err := app.Test(httptest.NewRequest(http.MethodPost, "/tasks/42/complete", nil))
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status: got %d, want 409", resp.StatusCode)
	}
	if n := len(tr.all()); n != 0 {
		t.Errorf("captured events: got %d, want 0 (a locked Today is not a server failure)", n)
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

// Every event names the route pattern rather than the concrete path, and groups
// by what failed — a stacktrace captured in middleware is identical for every
// request and would otherwise file unrelated failures under one issue.
func TestSentryMiddleware_EventIsReadableAndGrouped(t *testing.T) {
	tr := bindSentry(t)
	app := sentryTestApp()
	if status := get(t, app, "/tasks/42"); status != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want 500", status)
	}
	events := tr.all()
	if len(events) != 1 {
		t.Fatalf("captured events: got %d, want 1", len(events))
	}
	event := events[0]
	if event.Message == "" {
		t.Error("event has no message; the issue title would fall back to \"Unknown error\"")
	}
	if event.Transaction != "/tasks/:id" {
		t.Errorf("transaction: got %q, want the route pattern /tasks/:id", event.Transaction)
	}
	if got := event.Tags["http.route"]; got != "/tasks/:id" {
		t.Errorf("http.route tag: got %q, want the route pattern /tasks/:id", got)
	}
	if got := event.Tags["error.type"]; got != "*httpapi_test.stringErr" {
		t.Errorf("error.type tag: got %q, want the cause's type", got)
	}
	httpCtx, ok := event.Contexts["http"]
	if !ok {
		t.Fatalf("contexts: got %v, want an http context", event.Contexts)
	}
	if httpCtx["path"] != "/tasks/42" {
		t.Errorf("http context path: got %v, want /tasks/42", httpCtx["path"])
	}
	if len(event.Fingerprint) != 4 || event.Fingerprint[0] != "error" ||
		event.Fingerprint[1] != "/tasks/:id" || event.Fingerprint[2] != httpapi.CodeInternalError {
		t.Errorf("fingerprint: got %v, want [error /tasks/:id internal_error <type>]", event.Fingerprint)
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
	if got := events[0].Tags["error.kind"]; got != "panic" {
		t.Errorf("error.kind tag: got %q, want panic", got)
	}
	if got := events[0].Fingerprint; len(got) == 0 || got[0] != "panic" {
		t.Errorf("fingerprint: got %v, want it to lead with panic", got)
	}
	// The event is captured from the deferred recover, so the stack it carries
	// still runs through the panicking handler — that is the frame worth seeing.
	outermost := events[0].Exception[len(events[0].Exception)-1]
	if outermost.Stacktrace == nil || len(outermost.Stacktrace.Frames) == 0 {
		t.Error("panic event has no stacktrace")
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

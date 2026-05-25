package httpapi_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/repo"
)

// idemEnv wires the minimum required for IdempotencyMiddleware: real SQLite,
// a JWT issuer with a seeded user, and a /api/v1 group that mirrors the
// production middleware order.
type idemEnv struct {
	app   *fiber.App
	token string
	calls *atomic.Int32
}

func setupIdempotencyEnv(t *testing.T) *idemEnv {
	t.Helper()
	dir := t.TempDir()
	sqlDB, err := db.Open(filepath.Join(dir, "idem.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.RunMigrations(context.Background(), sqlDB); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	users := repo.NewUserRepo(sqlDB)
	if _, err := users.Create(context.Background(), "admin", "h"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	issuer := auth.NewJWTIssuer(testSecret)
	tok, _, err := issuer.Issue(1, 1)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	deps := httpapi.Deps{
		Log:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		JWTIssuer:       issuer,
		APITokenRepo:    repo.NewAPITokenRepo(sqlDB),
		APITokenSalt:    []byte("test-api-token-salt-32-bytes-pad!"),
		IdempotencyRepo: repo.NewIdempotencyRepo(sqlDB),
	}
	app := httpapi.NewApp(deps)
	api := httpapi.RegisterRoutes(app, deps)

	calls := &atomic.Int32{}
	api.Post("/echo", func(c fiber.Ctx) error {
		calls.Add(1)
		var body map[string]any
		_ = c.Bind().JSON(&body)
		body["count"] = calls.Load()
		return c.Status(201).JSON(body)
	})
	api.Post("/fail", func(c fiber.Ctx) error {
		calls.Add(1)
		return httpapi.ErrValidation("nope")
	})

	return &idemEnv{app: app, token: tok, calls: calls}
}

func (e *idemEnv) post(t *testing.T, path, key string, body string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.token)
	if key != "" {
		req.Header.Set(httpapi.IdempotencyHeader, key)
	}
	resp, err := e.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	return resp
}

func TestIdempotencyMiddleware_ReplaysCachedResponse(t *testing.T) {
	env := setupIdempotencyEnv(t)
	first := env.post(t, "/api/v1/echo", "key-1", `{"name":"a"}`)
	defer func() { _ = first.Body.Close() }()
	b1, _ := io.ReadAll(first.Body)

	second := env.post(t, "/api/v1/echo", "key-1", `{"name":"b"}`)
	defer func() { _ = second.Body.Close() }()
	b2, _ := io.ReadAll(second.Body)

	if first.StatusCode != 201 || second.StatusCode != 201 {
		t.Fatalf("status: got %d/%d, want 201/201", first.StatusCode, second.StatusCode)
	}
	if string(b1) != string(b2) {
		t.Errorf("body mismatch on replay:\nfirst:  %s\nsecond: %s", b1, b2)
	}
	if env.calls.Load() != 1 {
		t.Errorf("handler calls: got %d, want 1 (second request must replay cache)", env.calls.Load())
	}
	if got := second.Header.Get(httpapi.IdempotencyReplayHeader); got != "true" {
		t.Errorf("%s header: got %q, want true", httpapi.IdempotencyReplayHeader, got)
	}
	if first.Header.Get(httpapi.IdempotencyReplayHeader) == "true" {
		t.Error("first response wrongly marked as replay")
	}
}

func TestIdempotencyMiddleware_DistinctKeysRunIndependently(t *testing.T) {
	env := setupIdempotencyEnv(t)
	r1 := env.post(t, "/api/v1/echo", "key-1", `{"v":1}`)
	defer func() { _ = r1.Body.Close() }()
	_, _ = io.ReadAll(r1.Body)
	r2 := env.post(t, "/api/v1/echo", "key-2", `{"v":2}`)
	defer func() { _ = r2.Body.Close() }()
	body2, _ := io.ReadAll(r2.Body)

	if env.calls.Load() != 2 {
		t.Errorf("handler calls: got %d, want 2 (each key runs once)", env.calls.Load())
	}
	var parsed map[string]any
	if err := json.Unmarshal(body2, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if v, _ := parsed["v"].(float64); v != 2 {
		t.Errorf("body[v]: got %v, want 2", parsed["v"])
	}
}

func TestIdempotencyMiddleware_NoKeyHeaderBypasses(t *testing.T) {
	env := setupIdempotencyEnv(t)
	r1 := env.post(t, "/api/v1/echo", "", `{"v":1}`)
	defer func() { _ = r1.Body.Close() }()
	_, _ = io.ReadAll(r1.Body)
	r2 := env.post(t, "/api/v1/echo", "", `{"v":2}`)
	defer func() { _ = r2.Body.Close() }()
	_, _ = io.ReadAll(r2.Body)

	if env.calls.Load() != 2 {
		t.Errorf("handler calls: got %d, want 2 (no key means no dedup)", env.calls.Load())
	}
}

func TestIdempotencyMiddleware_ErrorResponsesAreNotCached(t *testing.T) {
	env := setupIdempotencyEnv(t)
	r1 := env.post(t, "/api/v1/fail", "key-fail", `{}`)
	defer func() { _ = r1.Body.Close() }()
	_, _ = io.ReadAll(r1.Body)
	r2 := env.post(t, "/api/v1/fail", "key-fail", `{}`)
	defer func() { _ = r2.Body.Close() }()
	_, _ = io.ReadAll(r2.Body)

	if r1.StatusCode != 400 || r2.StatusCode != 400 {
		t.Fatalf("status: got %d/%d, want 400/400", r1.StatusCode, r2.StatusCode)
	}
	if env.calls.Load() != 2 {
		t.Errorf("handler calls: got %d, want 2 (error responses must not be cached)", env.calls.Load())
	}
}

func TestIdempotencyMiddleware_NonMutatingMethodsSkip(t *testing.T) {
	// GET routes never store cache entries even if Idempotency-Key is sent,
	// because retried reads do not need dedup and would otherwise pin a stale
	// snapshot indefinitely.
	env := setupIdempotencyEnv(t)
	api := env.app
	api.Get("/api/v1/ping", func(c fiber.Ctx) error {
		env.calls.Add(1)
		return c.SendString("pong")
	})

	mkReq := func() *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil)
		req.Header.Set("Authorization", "Bearer "+env.token)
		req.Header.Set(httpapi.IdempotencyHeader, "k")
		return req
	}
	r1, _ := env.app.Test(mkReq())
	defer func() { _ = r1.Body.Close() }()
	_, _ = io.ReadAll(r1.Body)
	r2, _ := env.app.Test(mkReq())
	defer func() { _ = r2.Body.Close() }()
	_, _ = io.ReadAll(r2.Body)

	if env.calls.Load() != 2 {
		t.Errorf("handler calls: got %d, want 2 (GET must not dedup)", env.calls.Load())
	}
}

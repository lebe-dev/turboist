package httpapi_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/repo"
)

func TestRequestIDMiddleware_Generates(t *testing.T) {
	app := fiber.New()
	app.Use(httpapi.RequestIDMiddleware(slog.New(slog.NewTextHandler(io.Discard, nil))))
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
	app.Use(httpapi.RequestIDMiddleware(slog.New(slog.NewTextHandler(io.Discard, nil))))
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
	mu      *sync.Mutex
	records *[]slog.Record
	attrs   []slog.Attr
}

func newCaptureHandler() *captureHandler {
	return &captureHandler{mu: &sync.Mutex{}, records: &[]slog.Record{}}
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	rec := r.Clone()
	if len(h.attrs) > 0 {
		rec.AddAttrs(h.attrs...)
	}
	*h.records = append(*h.records, rec)
	return nil
}

func (h *captureHandler) WithAttrs(a []slog.Attr) slog.Handler {
	merged := make([]slog.Attr, 0, len(h.attrs)+len(a))
	merged = append(merged, h.attrs...)
	merged = append(merged, a...)
	return &captureHandler{mu: h.mu, records: h.records, attrs: merged}
}
func (h *captureHandler) WithGroup(_ string) slog.Handler { return h }

func TestAccessLogMiddleware_Logs(t *testing.T) {
	cap := newCaptureHandler()
	logger := slog.New(cap)

	app := fiber.New()
	app.Use(httpapi.RequestIDMiddleware(slog.New(slog.NewTextHandler(io.Discard, nil))))
	app.Use(httpapi.AccessLogMiddleware(logger))
	app.Get("/ping", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusTeapot) })

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	resp := doRequest(t, app, req)
	defer func() { _ = resp.Body.Close() }()

	cap.mu.Lock()
	defer cap.mu.Unlock()
	if len(*cap.records) != 1 {
		t.Fatalf("got %d records, want 1", len(*cap.records))
	}
	rec := (*cap.records)[0]
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

func TestRequestIDMiddleware_AttachesLoggerToContext(t *testing.T) {
	cap := newCaptureHandler()
	logger := slog.New(cap)

	app := fiber.New()
	app.Use(httpapi.RequestIDMiddleware(logger))
	app.Get("/", func(c fiber.Ctx) error {
		// Emit a record via the request-scoped logger and verify
		// that the request_id attribute is automatically attached.
		logging.FromContext(c.Context()).InfoContext(c.Context(), "hello from handler")
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "trace-xyz")
	resp := doRequest(t, app, req)
	defer func() { _ = resp.Body.Close() }()

	cap.mu.Lock()
	defer cap.mu.Unlock()
	if len(*cap.records) != 1 {
		t.Fatalf("got %d records, want 1", len(*cap.records))
	}
	var foundRID string
	(*cap.records)[0].Attrs(func(a slog.Attr) bool {
		if a.Key == "request_id" {
			foundRID = a.Value.String()
		}
		return true
	})
	if foundRID != "trace-xyz" {
		t.Errorf("request_id: got %q, want %q", foundRID, "trace-xyz")
	}
}

func TestAccessLogMiddleware_LevelByStatus(t *testing.T) {
	cases := []struct {
		name   string
		status int
		want   slog.Level
	}{
		{"ok", fiber.StatusOK, slog.LevelDebug},
		{"client_error", fiber.StatusBadRequest, slog.LevelInfo},
		{"server_error", fiber.StatusInternalServerError, slog.LevelWarn},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cap := newCaptureHandler()
			logger := slog.New(cap)

			app := fiber.New()
			app.Use(httpapi.RequestIDMiddleware(logger))
			app.Use(httpapi.AccessLogMiddleware(logger))
			st := tc.status
			app.Get("/x", func(c fiber.Ctx) error { return c.SendStatus(st) })

			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			resp := doRequest(t, app, req)
			defer func() { _ = resp.Body.Close() }()

			cap.mu.Lock()
			defer cap.mu.Unlock()
			var got slog.Level
			var found bool
			for _, r := range *cap.records {
				if r.Message == "request" {
					got = r.Level
					found = true
				}
			}
			if !found {
				t.Fatal("no access log record emitted")
			}
			if got != tc.want {
				t.Errorf("level: got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAuthMiddleware_LogsWarnOnMissingHeader(t *testing.T) {
	cap := newCaptureHandler()
	logger := slog.New(cap)

	issuer := auth.NewJWTIssuer([]byte("test-secret-thats-long-enough-for-hs256-1234567890"))

	app := fiber.New(fiber.Config{ErrorHandler: func(c fiber.Ctx, err error) error {
		var ae *httpapi.AppError
		if errors.As(err, &ae) {
			return c.Status(ae.HTTPStatus).SendString(ae.Code)
		}
		return c.Status(500).SendString(err.Error())
	}})
	app.Use(httpapi.RequestIDMiddleware(logger))
	app.Use(httpapi.AuthMiddleware(issuer))
	app.Get("/p", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/p", nil)
	resp := doRequest(t, app, req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 401 {
		t.Fatalf("status: got %d, want 401", resp.StatusCode)
	}

	cap.mu.Lock()
	defer cap.mu.Unlock()
	var sawWarn bool
	for _, r := range *cap.records {
		if r.Level == slog.LevelWarn && r.Message == "auth: missing authorization header" {
			sawWarn = true
		}
	}
	if !sawWarn {
		t.Error("no WARN record for missing authorization header")
	}
}

func TestAuthMiddleware_MasksInvalidToken(t *testing.T) {
	cap := newCaptureHandler()
	logger := slog.New(cap)

	issuer := auth.NewJWTIssuer([]byte("test-secret-thats-long-enough-for-hs256-1234567890"))

	app := fiber.New(fiber.Config{ErrorHandler: func(c fiber.Ctx, err error) error {
		return c.SendStatus(401)
	}})
	app.Use(httpapi.RequestIDMiddleware(logger))
	app.Use(httpapi.AuthMiddleware(issuer))
	app.Get("/p", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	const bogus = "ABCDEFGHIJKLMNOP.tail"
	req := httptest.NewRequest(http.MethodGet, "/p", nil)
	req.Header.Set("Authorization", "Bearer "+bogus)
	resp := doRequest(t, app, req)
	defer func() { _ = resp.Body.Close() }()

	cap.mu.Lock()
	defer cap.mu.Unlock()
	var foundPrefix string
	for _, r := range *cap.records {
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == "token_prefix" {
				foundPrefix = a.Value.String()
			}
			return true
		})
	}
	if foundPrefix == "" {
		t.Fatal("no token_prefix attribute logged")
	}
	if foundPrefix == bogus {
		t.Errorf("token_prefix not masked: %q", foundPrefix)
	}
	// Should contain the first 6 chars and not the full token.
	if foundPrefix[:6] != bogus[:6] {
		t.Errorf("token_prefix: got %q, want prefix %q...", foundPrefix, bogus[:6])
	}
	if strings.Contains(foundPrefix, "tail") {
		t.Errorf("token_prefix leaks full token: %q", foundPrefix)
	}
}

func TestErrorHandler_LogsClientErrorAtDebug(t *testing.T) {
	cap := newCaptureHandler()
	logger := slog.New(cap)

	deps := httpapi.Deps{Log: logger, JWTIssuer: auth.NewJWTIssuer(testSecret)}
	app := httpapi.NewApp(deps)
	app.Get("/bad", func(c fiber.Ctx) error {
		return httpapi.ErrValidation("bad input")
	})

	req := httptest.NewRequest(http.MethodGet, "/bad", nil)
	resp := doRequest(t, app, req)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 400 {
		t.Fatalf("status: got %d, want 400", resp.StatusCode)
	}

	cap.mu.Lock()
	defer cap.mu.Unlock()
	var sawDebug bool
	for _, r := range *cap.records {
		if r.Level == slog.LevelDebug && r.Message == "client error" {
			sawDebug = true
		}
	}
	if !sawDebug {
		t.Error("expected DEBUG client error log for 4xx response")
	}
}

func TestErrorHandler_SkipsAuthErrors(t *testing.T) {
	cap := newCaptureHandler()
	logger := slog.New(cap)

	deps := httpapi.Deps{Log: logger, JWTIssuer: auth.NewJWTIssuer(testSecret)}
	app := httpapi.NewApp(deps)
	app.Get("/auth", func(c fiber.Ctx) error {
		return httpapi.ErrAuthInvalid("nope")
	})

	req := httptest.NewRequest(http.MethodGet, "/auth", nil)
	resp := doRequest(t, app, req)
	defer func() { _ = resp.Body.Close() }()

	cap.mu.Lock()
	defer cap.mu.Unlock()
	for _, r := range *cap.records {
		if r.Message == "client error" {
			t.Error("error handler should not emit 'client error' DEBUG for auth_* codes (covered by auth middleware)")
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

func TestGetAuthMethod_Empty(t *testing.T) {
	app := fiber.New()
	var got string
	app.Get("/", func(c fiber.Ctx) error {
		got = httpapi.GetAuthMethod(c)
		return c.SendStatus(fiber.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp := doRequest(t, app, req)
	defer func() { _ = resp.Body.Close() }()
	if got != "" {
		t.Errorf("GetAuthMethod without auth: got %q, want \"\"", got)
	}
}

func TestGetAuthMethod_AfterJWTAuth(t *testing.T) {
	issuer := auth.NewJWTIssuer(testSecret)
	tok, _, err := issuer.Issue(42, 1)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	var got string
	app := fiber.New()
	app.Use(httpapi.RequestIDMiddleware(slog.New(slog.NewTextHandler(io.Discard, nil))))
	app.Use(httpapi.AuthMiddleware(issuer))
	app.Get("/", func(c fiber.Ctx) error {
		got = httpapi.GetAuthMethod(c)
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp := doRequest(t, app, req)
	defer func() { _ = resp.Body.Close() }()
	if got != httpapi.AuthMethodJWT {
		t.Errorf("GetAuthMethod after JWT auth: got %q, want %q", got, httpapi.AuthMethodJWT)
	}
}

func TestRequireJWTAuth_AllowsJWT(t *testing.T) {
	issuer := auth.NewJWTIssuer(testSecret)
	tok, _, err := issuer.Issue(7, 1)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	app := fiber.New(fiber.Config{ErrorHandler: func(c fiber.Ctx, err error) error {
		var ae *httpapi.AppError
		if errors.As(err, &ae) {
			return c.Status(ae.HTTPStatus).SendString(ae.Code)
		}
		return c.Status(500).SendString(err.Error())
	}})
	app.Use(httpapi.RequestIDMiddleware(slog.New(slog.NewTextHandler(io.Discard, nil))))
	app.Use(httpapi.AuthMiddleware(issuer))
	app.Use(httpapi.RequireJWTAuth())
	app.Get("/p", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/p", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp := doRequest(t, app, req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want 200", resp.StatusCode)
	}
}

func TestRequireJWTAuth_RejectsAPIToken(t *testing.T) {
	dir := t.TempDir()
	sqlDB, err := db.Open(filepath.Join(dir, "mw.db"))
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
	apiTokens := repo.NewAPITokenRepo(sqlDB)
	salt := []byte("test-api-token-salt-32-bytes-pad!")

	rawToken, err := auth.GenerateAPIToken()
	if err != nil {
		t.Fatalf("gen token: %v", err)
	}
	hash := auth.HashAPIToken(rawToken, salt)
	if _, err := apiTokens.Create(context.Background(), 1, "test", hash); err != nil {
		t.Fatalf("create api token: %v", err)
	}

	issuer := auth.NewJWTIssuer(testSecret)
	app := fiber.New(fiber.Config{ErrorHandler: func(c fiber.Ctx, err error) error {
		var ae *httpapi.AppError
		if errors.As(err, &ae) {
			return c.Status(ae.HTTPStatus).SendString(ae.Code)
		}
		return c.Status(500).SendString(err.Error())
	}})
	app.Use(httpapi.RequestIDMiddleware(slog.New(slog.NewTextHandler(io.Discard, nil))))
	app.Use(httpapi.APIAuthMiddleware(issuer, apiTokens, salt))
	app.Use(httpapi.RequireJWTAuth())
	app.Get("/p", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/p", nil)
	req.Header.Set("Authorization", "Bearer "+rawToken)
	resp := doRequest(t, app, req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", resp.StatusCode)
	}
}

func TestAPIAuthMiddleware_AcceptsAPIToken(t *testing.T) {
	dir := t.TempDir()
	sqlDB, err := db.Open(filepath.Join(dir, "mw.db"))
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
	apiTokens := repo.NewAPITokenRepo(sqlDB)
	salt := []byte("test-api-token-salt-32-bytes-pad!")

	rawToken, err := auth.GenerateAPIToken()
	if err != nil {
		t.Fatalf("gen token: %v", err)
	}
	hash := auth.HashAPIToken(rawToken, salt)
	if _, err := apiTokens.Create(context.Background(), 1, "test", hash); err != nil {
		t.Fatalf("create api token: %v", err)
	}

	issuer := auth.NewJWTIssuer(testSecret)
	app := fiber.New()
	app.Use(httpapi.RequestIDMiddleware(slog.New(slog.NewTextHandler(io.Discard, nil))))
	app.Use(httpapi.APIAuthMiddleware(issuer, apiTokens, salt))
	var gotMethod string
	var gotUserID int64
	app.Get("/p", func(c fiber.Ctx) error {
		gotMethod = httpapi.GetAuthMethod(c)
		gotUserID = httpapi.GetUserID(c)
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/p", nil)
	req.Header.Set("Authorization", "Bearer "+rawToken)
	resp := doRequest(t, app, req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	if gotMethod != httpapi.AuthMethodAPIToken {
		t.Errorf("auth method: got %q, want %q", gotMethod, httpapi.AuthMethodAPIToken)
	}
	if gotUserID != 1 {
		t.Errorf("user id: got %d, want 1", gotUserID)
	}
}

func TestAPIAuthMiddleware_RejectsExpiredJWT(t *testing.T) {
	cap := newCaptureHandler()
	logger := slog.New(cap)

	issuer := auth.NewJWTIssuer(testSecret)
	issuer.SetClock(func() time.Time { return time.Now().Add(-2 * time.Hour) })
	tok, _, err := issuer.Issue(1, 1)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	issuer.SetClock(time.Now)

	app := fiber.New(fiber.Config{ErrorHandler: func(c fiber.Ctx, err error) error {
		var ae *httpapi.AppError
		if errors.As(err, &ae) {
			return c.Status(ae.HTTPStatus).SendString(ae.Code)
		}
		return c.Status(500).SendString(err.Error())
	}})
	app.Use(httpapi.RequestIDMiddleware(logger))
	app.Use(httpapi.APIAuthMiddleware(issuer, nil, nil))
	app.Get("/p", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/p", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp := doRequest(t, app, req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", resp.StatusCode)
	}

	cap.mu.Lock()
	defer cap.mu.Unlock()
	var sawWarn bool
	for _, r := range *cap.records {
		if r.Level == slog.LevelWarn && r.Message == "auth: token expired" {
			sawWarn = true
		}
	}
	if !sawWarn {
		t.Error("no WARN record for expired token")
	}
}

func TestAPIAuthMiddleware_RejectsBadHeader(t *testing.T) {
	cap := newCaptureHandler()
	logger := slog.New(cap)

	issuer := auth.NewJWTIssuer(testSecret)
	app := fiber.New(fiber.Config{ErrorHandler: func(c fiber.Ctx, err error) error {
		var ae *httpapi.AppError
		if errors.As(err, &ae) {
			return c.Status(ae.HTTPStatus).SendString(ae.Code)
		}
		return c.Status(500).SendString(err.Error())
	}})
	app.Use(httpapi.RequestIDMiddleware(logger))
	app.Use(httpapi.APIAuthMiddleware(issuer, nil, nil))
	app.Get("/p", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/p", nil)
	req.Header.Set("Authorization", "Basic abc")
	resp := doRequest(t, app, req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", resp.StatusCode)
	}

	cap.mu.Lock()
	defer cap.mu.Unlock()
	var sawWarn bool
	for _, r := range *cap.records {
		if r.Level == slog.LevelWarn && r.Message == "auth: bad authorization header format" {
			sawWarn = true
		}
	}
	if !sawWarn {
		t.Error("no WARN record for bad authorization header format")
	}
}

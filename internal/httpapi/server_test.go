package httpapi_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/httpapi"
)

var testSecret = []byte("test-secret-key-32-bytes-padding!")

func newTestApp(t *testing.T) (*fiber.App, *auth.JWTIssuer) {
	t.Helper()
	issuer := auth.NewJWTIssuer(testSecret)
	deps := httpapi.Deps{JWTIssuer: issuer}
	app := httpapi.NewApp(deps)
	httpapi.RegisterRoutes(app, deps)
	return app, issuer
}

func doRequest(t *testing.T, app *fiber.App, req *http.Request) *http.Response {
	t.Helper()
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	return resp
}

func readBody(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return b
}

type errEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func parseError(t *testing.T, b []byte) errEnvelope {
	t.Helper()
	var e errEnvelope
	if err := json.Unmarshal(b, &e); err != nil {
		t.Fatalf("parse error envelope: %v — body: %s", err, b)
	}
	return e
}

// TestErrorHandler_AppError verifies each AppError maps to the correct HTTP
// status and error code in the JSON envelope.
func TestErrorHandler_AppError(t *testing.T) {
	cases := []struct {
		name       string
		err        *httpapi.AppError
		wantStatus int
		wantCode   string
	}{
		{"validation_failed", httpapi.ErrValidation("bad"), 400, httpapi.CodeValidationFailed},
		{"auth_invalid", httpapi.ErrAuthInvalid("bad token"), 401, httpapi.CodeAuthInvalid},
		{"auth_expired", httpapi.ErrAuthExpired(), 401, httpapi.CodeAuthExpired},
		{"auth_rate_limited", httpapi.ErrAuthRateLimited(), 429, httpapi.CodeAuthRateLimited},
		{"forbidden", httpapi.ErrForbidden("no"), 403, httpapi.CodeForbidden},
		{"not_found", httpapi.ErrNotFound("missing"), 404, httpapi.CodeNotFound},
		{"conflict", httpapi.ErrConflict("dup"), 409, httpapi.CodeConflict},
		{"setup_already_done", httpapi.ErrSetupAlreadyDone(), 410, httpapi.CodeSetupAlreadyDone},
		{"limit_exceeded", httpapi.ErrLimitExceeded("too many"), 422, httpapi.CodeLimitExceeded},
		{"forbidden_placement", httpapi.ErrForbiddenPlacement("bad placement"), 422, httpapi.CodeForbiddenPlacement},
		{"recurrence_invalid", httpapi.ErrRecurrenceInvalid("bad rrule"), 422, httpapi.CodeRecurrenceInvalid},
		{"internal_error", httpapi.ErrInternal("boom"), 500, httpapi.CodeInternalError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := httpapi.NewApp(httpapi.Deps{})
			app.Get("/err", func(c fiber.Ctx) error {
				return tc.err
			})

			req := httptest.NewRequest(http.MethodGet, "/err", nil)
			resp := doRequest(t, app, req)
			body := readBody(t, resp)

			if resp.StatusCode != tc.wantStatus {
				t.Errorf("status = %d; want %d (body: %s)", resp.StatusCode, tc.wantStatus, body)
			}
			e := parseError(t, body)
			if e.Error.Code != tc.wantCode {
				t.Errorf("code = %q; want %q", e.Error.Code, tc.wantCode)
			}
			if e.Error.Message == "" {
				t.Error("message must not be empty")
			}
		})
	}
}

// TestErrorHandler_FiberNotFound checks the 404 envelope for unmatched routes.
func TestErrorHandler_FiberNotFound(t *testing.T) {
	app := httpapi.NewApp(httpapi.Deps{})

	req := httptest.NewRequest(http.MethodGet, "/no-such-route", nil)
	resp := doRequest(t, app, req)
	body := readBody(t, resp)

	if resp.StatusCode != 404 {
		t.Errorf("status = %d; want 404", resp.StatusCode)
	}
	e := parseError(t, body)
	if e.Error.Code != httpapi.CodeNotFound {
		t.Errorf("code = %q; want %q", e.Error.Code, httpapi.CodeNotFound)
	}
}

// TestHealthz verifies the unauthenticated health endpoint returns 200.
func TestHealthz(t *testing.T) {
	app, _ := newTestApp(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	resp := doRequest(t, app, req)
	if resp.StatusCode != 200 {
		t.Errorf("status = %d; want 200", resp.StatusCode)
	}
}

// TestAuthMiddleware_ValidBearer passes a valid token and expects 200.
func TestAuthMiddleware_ValidBearer(t *testing.T) {
	issuer := auth.NewJWTIssuer(testSecret)
	app := httpapi.NewApp(httpapi.Deps{JWTIssuer: issuer})
	app.Get("/protected", httpapi.AuthMiddleware(issuer), func(c fiber.Ctx) error {
		if httpapi.GetClaims(c) == nil {
			return httpapi.ErrInternal("claims nil")
		}
		return c.SendStatus(200)
	})

	token, _, err := issuer.Issue(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp := doRequest(t, app, req)
	if resp.StatusCode != 200 {
		t.Errorf("status = %d; want 200", resp.StatusCode)
	}
}

// TestAuthMiddleware_MissingHeader returns 401 auth_invalid.
func TestAuthMiddleware_MissingHeader(t *testing.T) {
	issuer := auth.NewJWTIssuer(testSecret)
	app := httpapi.NewApp(httpapi.Deps{JWTIssuer: issuer})
	app.Get("/protected", httpapi.AuthMiddleware(issuer), func(c fiber.Ctx) error {
		return c.SendStatus(200)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	resp := doRequest(t, app, req)
	body := readBody(t, resp)

	if resp.StatusCode != 401 {
		t.Errorf("status = %d; want 401", resp.StatusCode)
	}
	e := parseError(t, body)
	if e.Error.Code != httpapi.CodeAuthInvalid {
		t.Errorf("code = %q; want %q", e.Error.Code, httpapi.CodeAuthInvalid)
	}
}

// TestAuthMiddleware_InvalidToken returns 401 auth_invalid for a garbage token.
func TestAuthMiddleware_InvalidToken(t *testing.T) {
	issuer := auth.NewJWTIssuer(testSecret)
	app := httpapi.NewApp(httpapi.Deps{JWTIssuer: issuer})
	app.Get("/protected", httpapi.AuthMiddleware(issuer), func(c fiber.Ctx) error {
		return c.SendStatus(200)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer not.a.valid.jwt")
	resp := doRequest(t, app, req)
	body := readBody(t, resp)

	if resp.StatusCode != 401 {
		t.Errorf("status = %d; want 401", resp.StatusCode)
	}
	e := parseError(t, body)
	if e.Error.Code != httpapi.CodeAuthInvalid {
		t.Errorf("code = %q; want %q", e.Error.Code, httpapi.CodeAuthInvalid)
	}
}

// TestAuthMiddleware_ExpiredToken returns 401 auth_expired.
func TestAuthMiddleware_ExpiredToken(t *testing.T) {
	secret := testSecret
	issuer := auth.NewJWTIssuer(secret)

	// Issue a token with a clock set 1 hour in the past so it's already expired.
	pastIssuer := auth.NewJWTIssuer(secret)
	pastIssuer.SetClock(func() time.Time {
		return time.Now().Add(-1 * time.Hour)
	})
	token, _, err := pastIssuer.Issue(1, 1)
	if err != nil {
		t.Fatal(err)
	}

	app := httpapi.NewApp(httpapi.Deps{JWTIssuer: issuer})
	app.Get("/protected", httpapi.AuthMiddleware(issuer), func(c fiber.Ctx) error {
		return c.SendStatus(200)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp := doRequest(t, app, req)
	body := readBody(t, resp)

	if resp.StatusCode != 401 {
		t.Errorf("status = %d; want 401", resp.StatusCode)
	}
	e := parseError(t, body)
	if e.Error.Code != httpapi.CodeAuthExpired {
		t.Errorf("code = %q; want %q", e.Error.Code, httpapi.CodeAuthExpired)
	}
}

// TestAuthMiddleware_BadScheme returns 401 for non-Bearer schemes.
func TestAuthMiddleware_BadScheme(t *testing.T) {
	issuer := auth.NewJWTIssuer(testSecret)
	app := httpapi.NewApp(httpapi.Deps{JWTIssuer: issuer})
	app.Get("/protected", httpapi.AuthMiddleware(issuer), func(c fiber.Ctx) error {
		return c.SendStatus(200)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	resp := doRequest(t, app, req)

	if resp.StatusCode != 401 {
		t.Errorf("status = %d; want 401", resp.StatusCode)
	}
}

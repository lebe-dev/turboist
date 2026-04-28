package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/handlers"
	"github.com/lebe-dev/turboist/internal/repo"
	"golang.org/x/time/rate"
)

// testEnv holds all wired dependencies for auth handler tests.
type testEnv struct {
	app      *fiber.App
	users    *repo.UserRepo
	sessions *repo.SessionRepo
	jwt      *auth.JWTIssuer
	limiter  *auth.IPLimiter
}

func setupAuthTest(t *testing.T) *testEnv {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.RunMigrations(context.Background(), d); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	users := repo.NewUserRepo(d)
	sessions := repo.NewSessionRepo(d)
	issuer := auth.NewJWTIssuer([]byte("test-secret-key-32-bytes-padding!"))
	// Generous limiter for most tests; individual tests override when checking rate-limit.
	limiter := auth.NewIPLimiter(rate.Every(time.Millisecond), 1000, 10*time.Minute)
	t.Cleanup(limiter.Stop)

	handler := handlers.NewAuthHandler(users, sessions, issuer, limiter)

	deps := httpapi.Deps{JWTIssuer: issuer}
	app := httpapi.NewApp(deps)
	handler.RegisterAuth(app.Group("/auth"), issuer)

	return &testEnv{app: app, users: users, sessions: sessions, jwt: issuer, limiter: limiter}
}

func doReq(t *testing.T, app *fiber.App, req *http.Request) (*http.Response, []byte) {
	t.Helper()
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	b, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp, b
}

func jsonBody(v any) *bytes.Buffer {
	b, _ := json.Marshal(v)
	return bytes.NewBuffer(b)
}

type errResp struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func parseErr(t *testing.T, b []byte) errResp {
	t.Helper()
	var e errResp
	if err := json.Unmarshal(b, &e); err != nil {
		t.Fatalf("parse error envelope: %v — body: %s", err, b)
	}
	return e
}

type authResp struct {
	Access  string `json:"access"`
	Refresh string `json:"refresh"`
	User    struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
	} `json:"user"`
}

func doSetup(t *testing.T, env *testEnv, clientKind string) authResp {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/auth/setup", jsonBody(map[string]string{
		"username":   "admin",
		"password":   "secret123",
		"clientKind": clientKind,
	}))
	req.Header.Set("Content-Type", "application/json")
	resp, body := doReq(t, env.app, req)
	if resp.StatusCode != 200 {
		t.Fatalf("setup status %d, body: %s", resp.StatusCode, body)
	}
	var ar authResp
	if err := json.Unmarshal(body, &ar); err != nil {
		t.Fatalf("parse auth resp: %v", err)
	}
	return ar
}

func bearerHeader(token string) string {
	return "Bearer " + token
}

// --- Tests ---

func TestSetupRequired_NoUser(t *testing.T) {
	env := setupAuthTest(t)
	req := httptest.NewRequest(http.MethodGet, "/auth/setup-required", nil)
	resp, body := doReq(t, env.app, req)
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
	var result struct {
		Required bool `json:"required"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !result.Required {
		t.Error("required: got false, want true")
	}
}

func TestSetupRequired_WithUser(t *testing.T) {
	env := setupAuthTest(t)
	doSetup(t, env, "cli")

	req := httptest.NewRequest(http.MethodGet, "/auth/setup-required", nil)
	resp, body := doReq(t, env.app, req)
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
	var result struct {
		Required bool `json:"required"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Required {
		t.Error("required: got true, want false")
	}
}

func TestSetup_Success(t *testing.T) {
	env := setupAuthTest(t)
	ar := doSetup(t, env, "cli")
	if ar.Access == "" {
		t.Error("access token is empty")
	}
	if ar.Refresh == "" {
		t.Error("refresh token is empty")
	}
	if ar.User.Username != "admin" {
		t.Errorf("username: got %q, want %q", ar.User.Username, "admin")
	}
}

func TestSetup_WebSetsCookie(t *testing.T) {
	env := setupAuthTest(t)
	req := httptest.NewRequest(http.MethodPost, "/auth/setup", jsonBody(map[string]string{
		"username":   "admin",
		"password":   "secret123",
		"clientKind": "web",
	}))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := doReq(t, env.app, req)
	if resp.StatusCode != 200 {
		t.Fatalf("setup web status: got %d, want 200", resp.StatusCode)
	}
	cookieHeader := resp.Header.Get("Set-Cookie")
	if !strings.Contains(cookieHeader, "refresh=") {
		t.Errorf("Set-Cookie header missing refresh cookie: %q", cookieHeader)
	}
	lower := strings.ToLower(cookieHeader)
	if !strings.Contains(lower, "httponly") {
		t.Error("refresh cookie missing HttpOnly")
	}
	if !strings.Contains(lower, "path=/auth/refresh") {
		t.Errorf("refresh cookie missing Path=/auth/refresh; got: %q", cookieHeader)
	}
}

func TestSetup_NonWebNoCookie(t *testing.T) {
	env := setupAuthTest(t)
	req := httptest.NewRequest(http.MethodPost, "/auth/setup", jsonBody(map[string]string{
		"username":   "admin",
		"password":   "secret123",
		"clientKind": "ios",
	}))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := doReq(t, env.app, req)
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
	if resp.Header.Get("Set-Cookie") != "" {
		t.Error("non-web setup should not set cookie")
	}
}

func TestSetup_AlreadyDone(t *testing.T) {
	env := setupAuthTest(t)
	doSetup(t, env, "cli")

	req := httptest.NewRequest(http.MethodPost, "/auth/setup", jsonBody(map[string]string{
		"username":   "admin2",
		"password":   "secret123",
		"clientKind": "cli",
	}))
	req.Header.Set("Content-Type", "application/json")
	resp, body := doReq(t, env.app, req)
	if resp.StatusCode != 410 {
		t.Fatalf("got %d, want 410; body: %s", resp.StatusCode, body)
	}
	e := parseErr(t, body)
	if e.Error.Code != httpapi.CodeSetupAlreadyDone {
		t.Errorf("code: got %q, want %q", e.Error.Code, httpapi.CodeSetupAlreadyDone)
	}
}

func TestSetup_RateLimit(t *testing.T) {
	env := setupAuthTest(t)
	// Replace the limiter with a very tight one.
	tightLimiter := auth.NewIPLimiter(rate.Every(time.Hour), 1, time.Minute)
	t.Cleanup(tightLimiter.Stop)

	tightHandler := handlers.NewAuthHandler(env.users, env.sessions, env.jwt, tightLimiter)
	app := httpapi.NewApp(httpapi.Deps{JWTIssuer: env.jwt})
	tightHandler.RegisterAuth(app.Group("/auth"), env.jwt)

	body := jsonBody(map[string]string{"username": "a", "password": "b", "clientKind": "cli"})

	// First request consumes the burst.
	req1 := httptest.NewRequest(http.MethodPost, "/auth/setup", bytes.NewBuffer(body.Bytes()))
	req1.Header.Set("Content-Type", "application/json")
	resp1, _ := doReq(t, app, req1)
	// Could be 410 (if first request succeeds at setup check) or validation error — not 429.
	if resp1.StatusCode == 429 {
		t.Fatal("first request should not be rate-limited")
	}

	// Second request should be rate-limited.
	req2 := httptest.NewRequest(http.MethodPost, "/auth/setup", bytes.NewBuffer(body.Bytes()))
	req2.Header.Set("Content-Type", "application/json")
	resp2, b2 := doReq(t, app, req2)
	if resp2.StatusCode != 429 {
		t.Fatalf("second request: got %d, want 429; body: %s", resp2.StatusCode, b2)
	}
	e := parseErr(t, b2)
	if e.Error.Code != httpapi.CodeAuthRateLimited {
		t.Errorf("code: got %q, want %q", e.Error.Code, httpapi.CodeAuthRateLimited)
	}
}

func TestSetup_ValidationErrors(t *testing.T) {
	cases := []struct {
		name string
		body map[string]string
	}{
		{"missing username", map[string]string{"password": "pw", "clientKind": "cli"}},
		{"missing password", map[string]string{"username": "u", "clientKind": "cli"}},
		{"invalid clientKind", map[string]string{"username": "u", "password": "pw", "clientKind": "fax"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := setupAuthTest(t)
			req := httptest.NewRequest(http.MethodPost, "/auth/setup", jsonBody(tc.body))
			req.Header.Set("Content-Type", "application/json")
			resp, body := doReq(t, env.app, req)
			if resp.StatusCode != 400 {
				t.Fatalf("got %d, want 400; body: %s", resp.StatusCode, body)
			}
			e := parseErr(t, body)
			if e.Error.Code != httpapi.CodeValidationFailed {
				t.Errorf("code: got %q, want %q", e.Error.Code, httpapi.CodeValidationFailed)
			}
		})
	}
}

func TestLogin_Success(t *testing.T) {
	env := setupAuthTest(t)
	doSetup(t, env, "cli")

	req := httptest.NewRequest(http.MethodPost, "/auth/login", jsonBody(map[string]string{
		"username":   "admin",
		"password":   "secret123",
		"clientKind": "cli",
	}))
	req.Header.Set("Content-Type", "application/json")
	resp, body := doReq(t, env.app, req)
	if resp.StatusCode != 200 {
		t.Fatalf("login status: got %d; body: %s", resp.StatusCode, body)
	}
	var ar authResp
	if err := json.Unmarshal(body, &ar); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ar.Access == "" || ar.Refresh == "" {
		t.Error("missing tokens in login response")
	}
	if ar.User.Username != "admin" {
		t.Errorf("username: got %q, want %q", ar.User.Username, "admin")
	}
}

func TestLogin_InvalidPassword(t *testing.T) {
	env := setupAuthTest(t)
	doSetup(t, env, "cli")

	req := httptest.NewRequest(http.MethodPost, "/auth/login", jsonBody(map[string]string{
		"username":   "admin",
		"password":   "wrongpassword",
		"clientKind": "cli",
	}))
	req.Header.Set("Content-Type", "application/json")
	resp, body := doReq(t, env.app, req)
	if resp.StatusCode != 401 {
		t.Fatalf("got %d, want 401; body: %s", resp.StatusCode, body)
	}
	e := parseErr(t, body)
	if e.Error.Code != httpapi.CodeAuthInvalid {
		t.Errorf("code: got %q, want %q", e.Error.Code, httpapi.CodeAuthInvalid)
	}
}

func TestLogin_RateLimit(t *testing.T) {
	env := setupAuthTest(t)
	doSetup(t, env, "cli")

	tightLimiter := auth.NewIPLimiter(rate.Every(time.Hour), 1, time.Minute)
	t.Cleanup(tightLimiter.Stop)

	tightHandler := handlers.NewAuthHandler(env.users, env.sessions, env.jwt, tightLimiter)
	app := httpapi.NewApp(httpapi.Deps{JWTIssuer: env.jwt})
	tightHandler.RegisterAuth(app.Group("/auth"), env.jwt)

	body := jsonBody(map[string]string{"username": "admin", "password": "secret123", "clientKind": "cli"})

	req1 := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBuffer(body.Bytes()))
	req1.Header.Set("Content-Type", "application/json")
	doReq(t, app, req1) // consumes burst

	req2 := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBuffer(body.Bytes()))
	req2.Header.Set("Content-Type", "application/json")
	resp2, b2 := doReq(t, app, req2)
	if resp2.StatusCode != 429 {
		t.Fatalf("got %d, want 429; body: %s", resp2.StatusCode, b2)
	}
}

func TestLogin_WebSetsCookie(t *testing.T) {
	env := setupAuthTest(t)
	doSetup(t, env, "cli")

	req := httptest.NewRequest(http.MethodPost, "/auth/login", jsonBody(map[string]string{
		"username":   "admin",
		"password":   "secret123",
		"clientKind": "web",
	}))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := doReq(t, env.app, req)
	if resp.StatusCode != 200 {
		t.Fatalf("login status: got %d", resp.StatusCode)
	}
	cookieHeader := resp.Header.Get("Set-Cookie")
	if !strings.Contains(cookieHeader, "refresh=") {
		t.Errorf("missing refresh cookie: %q", cookieHeader)
	}
}

func TestLogin_NonWebNoCookie(t *testing.T) {
	env := setupAuthTest(t)
	doSetup(t, env, "cli")

	req := httptest.NewRequest(http.MethodPost, "/auth/login", jsonBody(map[string]string{
		"username":   "admin",
		"password":   "secret123",
		"clientKind": "ios",
	}))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := doReq(t, env.app, req)
	if resp.StatusCode != 200 {
		t.Fatalf("login status: got %d", resp.StatusCode)
	}
	if resp.Header.Get("Set-Cookie") != "" {
		t.Error("ios login should not set cookie")
	}
}

func TestRefresh_FromBody(t *testing.T) {
	env := setupAuthTest(t)
	ar := doSetup(t, env, "cli")

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", jsonBody(map[string]string{
		"refresh": ar.Refresh,
	}))
	req.Header.Set("Content-Type", "application/json")
	resp, body := doReq(t, env.app, req)
	if resp.StatusCode != 200 {
		t.Fatalf("refresh status: got %d; body: %s", resp.StatusCode, body)
	}
	var rr struct {
		Access  string `json:"access"`
		Refresh string `json:"refresh"`
	}
	if err := json.Unmarshal(body, &rr); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if rr.Access == "" || rr.Refresh == "" {
		t.Error("missing tokens in refresh response")
	}
	if rr.Refresh == ar.Refresh {
		t.Error("refresh token must be rotated")
	}
}

func TestRefresh_FromCookie(t *testing.T) {
	env := setupAuthTest(t)
	// Setup as web so cookie is set.
	req0 := httptest.NewRequest(http.MethodPost, "/auth/setup", jsonBody(map[string]string{
		"username":   "admin",
		"password":   "secret123",
		"clientKind": "web",
	}))
	req0.Header.Set("Content-Type", "application/json")
	resp0, _ := doReq(t, env.app, req0)
	if resp0.StatusCode != 200 {
		t.Fatal("setup failed")
	}

	// Find the refresh cookie.
	var refreshCookieVal string
	for _, c := range resp0.Cookies() {
		if c.Name == "refresh" {
			refreshCookieVal = c.Value
			break
		}
	}
	if refreshCookieVal == "" {
		t.Fatal("no refresh cookie in setup response")
	}

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: "refresh", Value: refreshCookieVal})
	resp, body := doReq(t, env.app, req)
	if resp.StatusCode != 200 {
		t.Fatalf("refresh via cookie status: got %d; body: %s", resp.StatusCode, body)
	}
}

func TestRefresh_InvalidToken(t *testing.T) {
	env := setupAuthTest(t)
	doSetup(t, env, "cli")

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", jsonBody(map[string]string{
		"refresh": "not-a-real-token",
	}))
	req.Header.Set("Content-Type", "application/json")
	resp, body := doReq(t, env.app, req)
	if resp.StatusCode != 401 {
		t.Fatalf("got %d, want 401; body: %s", resp.StatusCode, body)
	}
	e := parseErr(t, body)
	if e.Error.Code != httpapi.CodeAuthInvalid {
		t.Errorf("code: got %q, want %q", e.Error.Code, httpapi.CodeAuthInvalid)
	}
}

func TestRefresh_TheftDetection(t *testing.T) {
	env := setupAuthTest(t)
	ar := doSetup(t, env, "cli")
	oldRefresh := ar.Refresh

	// Rotate once: new tokens issued.
	req1 := httptest.NewRequest(http.MethodPost, "/auth/refresh", jsonBody(map[string]string{
		"refresh": oldRefresh,
	}))
	req1.Header.Set("Content-Type", "application/json")
	resp1, _ := doReq(t, env.app, req1)
	if resp1.StatusCode != 200 {
		t.Fatal("first refresh failed")
	}

	// Attempt to reuse the old refresh token → theft detected.
	req2 := httptest.NewRequest(http.MethodPost, "/auth/refresh", jsonBody(map[string]string{
		"refresh": oldRefresh,
	}))
	req2.Header.Set("Content-Type", "application/json")
	resp2, body2 := doReq(t, env.app, req2)
	if resp2.StatusCode != 401 {
		t.Fatalf("reuse old token: got %d, want 401; body: %s", resp2.StatusCode, body2)
	}
}

func TestMe_ReturnsUser(t *testing.T) {
	env := setupAuthTest(t)
	ar := doSetup(t, env, "cli")

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.Header.Set("Authorization", bearerHeader(ar.Access))
	resp, body := doReq(t, env.app, req)
	if resp.StatusCode != 200 {
		t.Fatalf("me status: got %d; body: %s", resp.StatusCode, body)
	}
	var result struct {
		User struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
		} `json:"user"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.User.Username != "admin" {
		t.Errorf("username: got %q, want admin", result.User.Username)
	}
}

func TestMe_RequiresAuth(t *testing.T) {
	env := setupAuthTest(t)
	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	resp, body := doReq(t, env.app, req)
	if resp.StatusCode != 401 {
		t.Fatalf("got %d, want 401; body: %s", resp.StatusCode, body)
	}
}

func TestLogout_RevokesSession(t *testing.T) {
	env := setupAuthTest(t)
	ar := doSetup(t, env, "cli")

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.Header.Set("Authorization", bearerHeader(ar.Access))
	resp, body := doReq(t, env.app, req)
	if resp.StatusCode != 204 {
		t.Fatalf("logout status: got %d; body: %s", resp.StatusCode, body)
	}

	// Refresh should now fail since session is revoked.
	req2 := httptest.NewRequest(http.MethodPost, "/auth/refresh", jsonBody(map[string]string{
		"refresh": ar.Refresh,
	}))
	req2.Header.Set("Content-Type", "application/json")
	resp2, _ := doReq(t, env.app, req2)
	if resp2.StatusCode != 401 {
		t.Errorf("refresh after logout: got %d, want 401", resp2.StatusCode)
	}
}

func TestLogoutAll_RevokesAllSessions(t *testing.T) {
	env := setupAuthTest(t)
	ar := doSetup(t, env, "cli")

	// Login a second session.
	req2 := httptest.NewRequest(http.MethodPost, "/auth/login", jsonBody(map[string]string{
		"username":   "admin",
		"password":   "secret123",
		"clientKind": "cli",
	}))
	req2.Header.Set("Content-Type", "application/json")
	resp2, body2 := doReq(t, env.app, req2)
	if resp2.StatusCode != 200 {
		t.Fatalf("second login: got %d; body: %s", resp2.StatusCode, body2)
	}
	var ar2 authResp
	_ = json.Unmarshal(body2, &ar2)

	// Logout-all with first session's access token.
	req := httptest.NewRequest(http.MethodPost, "/auth/logout-all", nil)
	req.Header.Set("Authorization", bearerHeader(ar.Access))
	resp, _ := doReq(t, env.app, req)
	if resp.StatusCode != 204 {
		t.Fatalf("logout-all status: got %d", resp.StatusCode)
	}

	// Both refresh tokens should now be invalid.
	for i, token := range []string{ar.Refresh, ar2.Refresh} {
		req := httptest.NewRequest(http.MethodPost, "/auth/refresh", jsonBody(map[string]string{
			"refresh": token,
		}))
		req.Header.Set("Content-Type", "application/json")
		r, _ := doReq(t, env.app, req)
		if r.StatusCode != 401 {
			t.Errorf("session %d: got %d, want 401", i+1, r.StatusCode)
		}
	}
}

func TestEndToEnd_SetupLoginRefreshMeLogout(t *testing.T) {
	env := setupAuthTest(t)

	// 1. Setup.
	ar := doSetup(t, env, "cli")

	// 2. Refresh.
	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", jsonBody(map[string]string{
		"refresh": ar.Refresh,
	}))
	req.Header.Set("Content-Type", "application/json")
	resp, body := doReq(t, env.app, req)
	if resp.StatusCode != 200 {
		t.Fatalf("refresh status: got %d; body: %s", resp.StatusCode, body)
	}
	var rr struct {
		Access  string `json:"access"`
		Refresh string `json:"refresh"`
	}
	_ = json.Unmarshal(body, &rr)

	// 3. Me with new access token.
	reqMe := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	reqMe.Header.Set("Authorization", bearerHeader(rr.Access))
	respMe, _ := doReq(t, env.app, reqMe)
	if respMe.StatusCode != 200 {
		t.Fatalf("me: got %d", respMe.StatusCode)
	}

	// 4. Logout.
	reqLogout := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	reqLogout.Header.Set("Authorization", bearerHeader(rr.Access))
	respLogout, _ := doReq(t, env.app, reqLogout)
	if respLogout.StatusCode != 204 {
		t.Fatalf("logout: got %d", respLogout.StatusCode)
	}
}

func TestSessionLimit_EnforcedOnLogin(t *testing.T) {
	env := setupAuthTest(t)
	doSetup(t, env, "cli") // first session via setup

	// Create 4 more sessions via login (total 5).
	for range 4 {
		req := httptest.NewRequest(http.MethodPost, "/auth/login", jsonBody(map[string]string{
			"username":   "admin",
			"password":   "secret123",
			"clientKind": "cli",
		}))
		req.Header.Set("Content-Type", "application/json")
		resp, body := doReq(t, env.app, req)
		if resp.StatusCode != 200 {
			t.Fatalf("extra login: got %d; body: %s", resp.StatusCode, body)
		}
	}

	// 6th login: new session is created, oldest must be purged.
	req := httptest.NewRequest(http.MethodPost, "/auth/login", jsonBody(map[string]string{
		"username":   "admin",
		"password":   "secret123",
		"clientKind": "cli",
	}))
	req.Header.Set("Content-Type", "application/json")
	resp, body := doReq(t, env.app, req)
	if resp.StatusCode != 200 {
		t.Fatalf("6th login: got %d; body: %s", resp.StatusCode, body)
	}

	// Verify active session count does not exceed 5.
	sessions, err := env.sessions.ListActiveForUser(context.Background(), 1)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) > 5 {
		t.Errorf("session count: got %d, want ≤ 5", len(sessions))
	}
}

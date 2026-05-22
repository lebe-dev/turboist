package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/pquerna/otp/totp"
	"golang.org/x/time/rate"

	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/handlers"
	"github.com/lebe-dev/turboist/internal/repo"
	totpsvc "github.com/lebe-dev/turboist/internal/service/totp"
)

type otpLoginEnv struct {
	app           *fiber.App
	users         *repo.UserRepo
	sessions      *repo.SessionRepo
	jwt           *auth.JWTIssuer
	limiter       *auth.IPLimiter
	svc           *totpsvc.Service
	access        string // post-setup access (used to enroll)
	secret        string
	recoveryCodes []string
}

// setupOTPLoginTest wires an AuthHandler with TOTP enabled, completes /auth/setup,
// enrolls the user in TOTP (via /auth/totp/*), and returns env carrying the
// TOTP secret and recovery codes for use by login-OTP tests. The user is left
// with TOTP enabled so subsequent /auth/login calls hit the awaiting-OTP branch.
func setupOTPLoginTest(t *testing.T) *otpLoginEnv {
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
	recovery := repo.NewTOTPRecoveryRepo(d)
	issuer := auth.NewJWTIssuer([]byte("test-secret-key-32-bytes-padding!"))
	limiter := auth.NewIPLimiter(rate.Every(time.Millisecond), 1000, 10*time.Minute)
	t.Cleanup(limiter.Stop)

	cheap := auth.Argon2Params{Memory: 8 * 1024, Time: 1, Threads: 1}
	svc := totpsvc.NewService(crypto.NewTokenCipher(totpCipherKey), users, recovery, cheap)

	authHandler := handlers.NewAuthHandler(users, sessions, issuer, limiter, cheap).WithTOTP(svc)
	t.Cleanup(authHandler.Stop)
	totpHandler := handlers.NewTOTPHandler(users, svc, limiter)

	app := httpapi.NewApp(httpapi.Deps{JWTIssuer: issuer})
	authGroup := app.Group("/auth")
	authHandler.RegisterAuth(authGroup, issuer)
	totpHandler.RegisterTOTP(authGroup.Group("", httpapi.AuthMiddleware(issuer)))

	ar := doSetupOn(t, app, "cli")
	env := &otpLoginEnv{
		app: app, users: users, sessions: sessions, jwt: issuer, limiter: limiter,
		svc: svc, access: ar.Access,
	}

	// Enroll TOTP.
	setupResp, body := postJSONAuthOnApp(t, app, ar.Access, "/auth/totp/setup", nil)
	if setupResp.StatusCode != 200 {
		t.Fatalf("totp setup: %d %s", setupResp.StatusCode, body)
	}
	var s totpSetupResp
	if err := json.Unmarshal(body, &s); err != nil {
		t.Fatalf("parse totp setup: %v", err)
	}
	env.secret = s.Secret
	// Confirm with the previous step's code so tests can still verify with a
	// fresh code from the current step (replay protection burns the step used
	// to confirm).
	code, err := totp.GenerateCode(s.Secret, time.Now().Add(-30*time.Second))
	if err != nil {
		t.Fatalf("gen code: %v", err)
	}
	confResp, cbody := postJSONAuthOnApp(t, app, ar.Access, "/auth/totp/confirm", map[string]string{"code": code})
	if confResp.StatusCode != 200 {
		t.Fatalf("totp confirm: %d %s", confResp.StatusCode, cbody)
	}
	var cr struct {
		RecoveryCodes []string `json:"recoveryCodes"`
	}
	if err := json.Unmarshal(cbody, &cr); err != nil {
		t.Fatalf("parse confirm: %v", err)
	}
	env.recoveryCodes = cr.RecoveryCodes
	return env
}

func postJSONAuthOnApp(t *testing.T, app *fiber.App, access, path string, body any) (*http.Response, []byte) {
	t.Helper()
	var req *http.Request
	if body == nil {
		req = httptest.NewRequest(http.MethodPost, path, nil)
	} else {
		req = httptest.NewRequest(http.MethodPost, path, jsonBody(body))
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", bearerHeader(access))
	return doReq(t, app, req)
}

type otpChallengeResp struct {
	OTPRequired bool   `json:"otpRequired"`
	Ticket      string `json:"ticket"`
}

func TestLogin_TOTPEnabled_ReturnsTicket(t *testing.T) {
	env := setupOTPLoginTest(t)

	req := httptest.NewRequest(http.MethodPost, "/auth/login", jsonBody(map[string]string{
		"username":   "admin",
		"password":   "secret123",
		"clientKind": "cli",
	}))
	req.Header.Set("Content-Type", "application/json")
	resp, body := doReq(t, env.app, req)
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var ch otpChallengeResp
	if err := json.Unmarshal(body, &ch); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !ch.OTPRequired {
		t.Error("otpRequired: got false, want true")
	}
	if ch.Ticket == "" {
		t.Error("ticket empty")
	}
	// Should NOT contain the regular access/refresh fields filled in.
	var ar authResp
	_ = json.Unmarshal(body, &ar)
	if ar.Access != "" || ar.Refresh != "" {
		t.Errorf("login with TOTP should not return tokens directly: %s", body)
	}
}

func TestLoginOTP_Success(t *testing.T) {
	env := setupOTPLoginTest(t)
	ticket := beginOTPLogin(t, env)

	code, err := totp.GenerateCode(env.secret, time.Now())
	if err != nil {
		t.Fatalf("gen code: %v", err)
	}
	resp, body := postJSON(t, env.app, "/auth/login/otp", map[string]string{
		"ticket": ticket,
		"code":   code,
	})
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var ar authResp
	if err := json.Unmarshal(body, &ar); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ar.Access == "" || ar.Refresh == "" {
		t.Error("missing tokens in OTP login response")
	}
	if ar.User.Username != "admin" {
		t.Errorf("username: got %q, want admin", ar.User.Username)
	}
}

func TestLoginOTP_WrongCode(t *testing.T) {
	env := setupOTPLoginTest(t)
	ticket := beginOTPLogin(t, env)

	resp, body := postJSON(t, env.app, "/auth/login/otp", map[string]string{
		"ticket": ticket,
		"code":   "000000",
	})
	if resp.StatusCode != 401 {
		t.Fatalf("got %d, want 401; body: %s", resp.StatusCode, body)
	}
	e := parseErr(t, body)
	if e.Error.Code != httpapi.CodeTOTPInvalidCode {
		t.Errorf("code: got %q, want %q", e.Error.Code, httpapi.CodeTOTPInvalidCode)
	}
}

func TestLoginOTP_ExpiredTicket(t *testing.T) {
	env := setupOTPLoginTest(t)
	// Issue a ticket with a clock far in the past so it's expired by the time we present it.
	env.jwt.SetClock(func() time.Time { return time.Now().Add(-1 * time.Hour) })
	ticket, _, err := env.jwt.IssueOTPTicket(1, "cli")
	if err != nil {
		t.Fatalf("issue ticket: %v", err)
	}
	env.jwt.SetClock(time.Now)

	code, _ := totp.GenerateCode(env.secret, time.Now())
	resp, body := postJSON(t, env.app, "/auth/login/otp", map[string]string{
		"ticket": ticket,
		"code":   code,
	})
	if resp.StatusCode != 401 {
		t.Fatalf("got %d, want 401; body: %s", resp.StatusCode, body)
	}
	e := parseErr(t, body)
	if e.Error.Code != httpapi.CodeAuthInvalid {
		t.Errorf("code: got %q, want %q", e.Error.Code, httpapi.CodeAuthInvalid)
	}
}

func TestLoginOTP_InvalidTicket(t *testing.T) {
	env := setupOTPLoginTest(t)
	resp, body := postJSON(t, env.app, "/auth/login/otp", map[string]string{
		"ticket": "not-a-jwt",
		"code":   "123456",
	})
	if resp.StatusCode != 401 {
		t.Fatalf("got %d, want 401; body: %s", resp.StatusCode, body)
	}
	e := parseErr(t, body)
	if e.Error.Code != httpapi.CodeAuthInvalid {
		t.Errorf("code: got %q, want %q", e.Error.Code, httpapi.CodeAuthInvalid)
	}
}

func TestLoginOTP_RejectsAccessTokenAsTicket(t *testing.T) {
	env := setupOTPLoginTest(t)
	// env.access is a regular access JWT — must not be accepted as a ticket.
	resp, body := postJSON(t, env.app, "/auth/login/otp", map[string]string{
		"ticket": env.access,
		"code":   "123456",
	})
	if resp.StatusCode != 401 {
		t.Fatalf("got %d, want 401; body: %s", resp.StatusCode, body)
	}
}

func TestLoginOTP_RecoveryCode(t *testing.T) {
	env := setupOTPLoginTest(t)
	if len(env.recoveryCodes) == 0 {
		t.Fatal("no recovery codes generated")
	}
	ticket := beginOTPLogin(t, env)

	resp, body := postJSON(t, env.app, "/auth/login/otp", map[string]string{
		"ticket": ticket,
		"code":   env.recoveryCodes[0],
	})
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var ar authResp
	if err := json.Unmarshal(body, &ar); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ar.Access == "" || ar.Refresh == "" {
		t.Error("missing tokens after recovery-code login")
	}

	// Recovery code must be single-use: a second attempt with the same code fails.
	ticket2 := beginOTPLogin(t, env)
	resp2, body2 := postJSON(t, env.app, "/auth/login/otp", map[string]string{
		"ticket": ticket2,
		"code":   env.recoveryCodes[0],
	})
	if resp2.StatusCode != 401 {
		t.Errorf("reused recovery code: got %d, want 401; body: %s", resp2.StatusCode, body2)
	}
}

func TestLoginOTP_RateLimit(t *testing.T) {
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
	recovery := repo.NewTOTPRecoveryRepo(d)
	issuer := auth.NewJWTIssuer([]byte("test-secret-key-32-bytes-padding!"))
	tight := auth.NewIPLimiter(rate.Every(time.Hour), 1, time.Minute)
	t.Cleanup(tight.Stop)
	cheap := auth.Argon2Params{Memory: 8 * 1024, Time: 1, Threads: 1}
	svc := totpsvc.NewService(crypto.NewTokenCipher(totpCipherKey), users, recovery, cheap)
	authHandler := handlers.NewAuthHandler(users, sessions, issuer, tight, cheap).WithTOTP(svc)
	t.Cleanup(authHandler.Stop)
	app := httpapi.NewApp(httpapi.Deps{JWTIssuer: issuer})
	authHandler.RegisterAuth(app.Group("/auth"), issuer)

	// Burst already consumed during setup/login is unpredictable; just hit the endpoint twice and check the second is 429.
	body := map[string]string{"ticket": "t", "code": "c"}
	postJSON(t, app, "/auth/login/otp", body)
	resp, b := postJSON(t, app, "/auth/login/otp", body)
	if resp.StatusCode != 429 {
		t.Fatalf("got %d, want 429; body: %s", resp.StatusCode, b)
	}
}

// --- helpers ---

func beginOTPLogin(t *testing.T, env *otpLoginEnv) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/auth/login", jsonBody(map[string]string{
		"username":   "admin",
		"password":   "secret123",
		"clientKind": "cli",
	}))
	req.Header.Set("Content-Type", "application/json")
	resp, body := doReq(t, env.app, req)
	if resp.StatusCode != 200 {
		t.Fatalf("login: %d %s", resp.StatusCode, body)
	}
	var ch otpChallengeResp
	if err := json.Unmarshal(body, &ch); err != nil {
		t.Fatalf("parse challenge: %v", err)
	}
	if !ch.OTPRequired || strings.TrimSpace(ch.Ticket) == "" {
		t.Fatalf("expected otp challenge, got %s", body)
	}
	return ch.Ticket
}

func postJSON(t *testing.T, app *fiber.App, path string, body any) (*http.Response, []byte) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, jsonBody(body))
	req.Header.Set("Content-Type", "application/json")
	return doReq(t, app, req)
}

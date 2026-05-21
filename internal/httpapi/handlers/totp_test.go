package handlers_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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

const totpCipherKey = "test-totp-cipher-key-32-bytes-padding!"

type totpEnv struct {
	app      *fiber.App
	users    *repo.UserRepo
	recovery *repo.TOTPRecoveryRepo
	svc      *totpsvc.Service
	jwt      *auth.JWTIssuer
	access   string
}

func setupTOTPTest(t *testing.T) *totpEnv {
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

	authHandler := handlers.NewAuthHandler(users, sessions, issuer, limiter, cheap)
	t.Cleanup(authHandler.Stop)
	totpHandler := handlers.NewTOTPHandler(users, svc, limiter)

	app := httpapi.NewApp(httpapi.Deps{JWTIssuer: issuer})
	authGroup := app.Group("/auth")
	authHandler.RegisterAuth(authGroup, issuer)
	totpHandler.RegisterTOTP(authGroup.Group("", httpapi.AuthMiddleware(issuer)))

	ar := doSetupOn(t, app, "cli")
	return &totpEnv{
		app:      app,
		users:    users,
		recovery: recovery,
		svc:      svc,
		jwt:      issuer,
		access:   ar.Access,
	}
}

func doSetupOn(t *testing.T, app *fiber.App, clientKind string) authResp {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/auth/setup", jsonBody(map[string]string{
		"username":   "admin",
		"password":   "secret123",
		"clientKind": clientKind,
	}))
	req.Header.Set("Content-Type", "application/json")
	resp, body := doReq(t, app, req)
	if resp.StatusCode != 200 {
		t.Fatalf("setup status %d, body: %s", resp.StatusCode, body)
	}
	var ar authResp
	if err := json.Unmarshal(body, &ar); err != nil {
		t.Fatalf("parse: %v", err)
	}
	return ar
}

type totpSetupResp struct {
	Secret      string `json:"secret"`
	OtpauthURL  string `json:"otpauthUrl"`
	QRPngBase64 string `json:"qrPngBase64"`
}

func postJSONAuth(t *testing.T, env *totpEnv, path string, body any) (*http.Response, []byte) {
	t.Helper()
	var rdr *bytes.Buffer
	if body == nil {
		rdr = bytes.NewBuffer(nil)
	} else {
		rdr = jsonBody(body)
	}
	req := httptest.NewRequest(http.MethodPost, path, rdr)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", bearerHeader(env.access))
	return doReq(t, env.app, req)
}

func TestTOTP_Setup_Success(t *testing.T) {
	env := setupTOTPTest(t)
	resp, body := postJSONAuth(t, env, "/auth/totp/setup", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("setup status: got %d, body: %s", resp.StatusCode, body)
	}
	var s totpSetupResp
	if err := json.Unmarshal(body, &s); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if s.Secret == "" {
		t.Error("secret empty")
	}
	if s.OtpauthURL == "" {
		t.Error("otpauth url empty")
	}
	png, err := base64.StdEncoding.DecodeString(s.QRPngBase64)
	if err != nil {
		t.Fatalf("decode qr png: %v", err)
	}
	if len(png) < 8 || string(png[:8]) != "\x89PNG\r\n\x1a\n" {
		t.Error("qr png signature missing")
	}
}

func TestTOTP_Setup_RequiresAuth(t *testing.T) {
	env := setupTOTPTest(t)
	req := httptest.NewRequest(http.MethodPost, "/auth/totp/setup", nil)
	resp, body := doReq(t, env.app, req)
	if resp.StatusCode != 401 {
		t.Fatalf("got %d, want 401; body: %s", resp.StatusCode, body)
	}
}

func TestTOTP_Setup_AlreadyEnabled(t *testing.T) {
	env := setupTOTPTest(t)
	// Enable TOTP via service for the seeded user (id 1).
	postJSONAuth(t, env, "/auth/totp/setup", nil)
	if err := env.users.EnableTOTP(context.Background(), 1); err != nil {
		t.Fatalf("enable: %v", err)
	}

	resp, body := postJSONAuth(t, env, "/auth/totp/setup", nil)
	if resp.StatusCode != 409 {
		t.Fatalf("got %d, want 409; body: %s", resp.StatusCode, body)
	}
	e := parseErr(t, body)
	if e.Error.Code != httpapi.CodeTOTPAlreadyEnabled {
		t.Errorf("code: got %q, want %q", e.Error.Code, httpapi.CodeTOTPAlreadyEnabled)
	}
}

func TestTOTP_Confirm_Success(t *testing.T) {
	env := setupTOTPTest(t)
	resp, body := postJSONAuth(t, env, "/auth/totp/setup", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("setup status: got %d, body: %s", resp.StatusCode, body)
	}
	var s totpSetupResp
	if err := json.Unmarshal(body, &s); err != nil {
		t.Fatalf("parse: %v", err)
	}
	code, err := totp.GenerateCode(s.Secret, time.Now())
	if err != nil {
		t.Fatalf("gen code: %v", err)
	}
	resp2, body2 := postJSONAuth(t, env, "/auth/totp/confirm", map[string]string{"code": code})
	if resp2.StatusCode != 200 {
		t.Fatalf("confirm status: got %d, body: %s", resp2.StatusCode, body2)
	}
	var cr struct {
		RecoveryCodes []string `json:"recoveryCodes"`
	}
	if err := json.Unmarshal(body2, &cr); err != nil {
		t.Fatalf("parse confirm: %v", err)
	}
	if len(cr.RecoveryCodes) != totpsvc.RecoveryCodeCount {
		t.Errorf("recovery codes: got %d, want %d", len(cr.RecoveryCodes), totpsvc.RecoveryCodeCount)
	}
	st, err := env.users.GetTOTPState(context.Background(), 1)
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	if !st.Enabled {
		t.Error("totp not enabled after confirm")
	}
}

func TestTOTP_Confirm_InvalidCode(t *testing.T) {
	env := setupTOTPTest(t)
	postJSONAuth(t, env, "/auth/totp/setup", nil)
	resp, body := postJSONAuth(t, env, "/auth/totp/confirm", map[string]string{"code": "000000"})
	if resp.StatusCode != 401 {
		t.Fatalf("got %d, want 401; body: %s", resp.StatusCode, body)
	}
	e := parseErr(t, body)
	if e.Error.Code != httpapi.CodeTOTPInvalidCode {
		t.Errorf("code: got %q, want %q", e.Error.Code, httpapi.CodeTOTPInvalidCode)
	}
}

func TestTOTP_Confirm_MissingCode(t *testing.T) {
	env := setupTOTPTest(t)
	postJSONAuth(t, env, "/auth/totp/setup", nil)
	resp, body := postJSONAuth(t, env, "/auth/totp/confirm", map[string]string{"code": ""})
	if resp.StatusCode != 400 {
		t.Fatalf("got %d, want 400; body: %s", resp.StatusCode, body)
	}
	e := parseErr(t, body)
	if e.Error.Code != httpapi.CodeValidationFailed {
		t.Errorf("code: got %q, want %q", e.Error.Code, httpapi.CodeValidationFailed)
	}
}

func TestTOTP_Confirm_NoPendingSetup(t *testing.T) {
	env := setupTOTPTest(t)
	// No setup performed.
	resp, body := postJSONAuth(t, env, "/auth/totp/confirm", map[string]string{"code": "123456"})
	if resp.StatusCode != 409 {
		t.Fatalf("got %d, want 409; body: %s", resp.StatusCode, body)
	}
	e := parseErr(t, body)
	if e.Error.Code != httpapi.CodeTOTPNotEnabled {
		t.Errorf("code: got %q, want %q", e.Error.Code, httpapi.CodeTOTPNotEnabled)
	}
}

func TestTOTP_Disable_WithTOTPCode(t *testing.T) {
	env := setupTOTPTest(t)
	resp, body := postJSONAuth(t, env, "/auth/totp/setup", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("setup: %d %s", resp.StatusCode, body)
	}
	var s totpSetupResp
	_ = json.Unmarshal(body, &s)
	code, _ := totp.GenerateCode(s.Secret, time.Now())
	postJSONAuth(t, env, "/auth/totp/confirm", map[string]string{"code": code})

	// Wait until a fresh code window so we don't replay (skew=1 still treats same code as valid).
	disableCode, _ := totp.GenerateCode(s.Secret, time.Now())
	resp2, body2 := postJSONAuth(t, env, "/auth/totp/disable", map[string]string{"code": disableCode})
	if resp2.StatusCode != 204 {
		t.Fatalf("disable status: got %d, body: %s", resp2.StatusCode, body2)
	}
	st, err := env.users.GetTOTPState(context.Background(), 1)
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	if st.Enabled {
		t.Error("totp still enabled after disable")
	}
}

func TestTOTP_Disable_WithRecoveryCode(t *testing.T) {
	env := setupTOTPTest(t)
	resp, body := postJSONAuth(t, env, "/auth/totp/setup", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("setup: %d %s", resp.StatusCode, body)
	}
	var s totpSetupResp
	_ = json.Unmarshal(body, &s)
	code, _ := totp.GenerateCode(s.Secret, time.Now())
	_, cbody := postJSONAuth(t, env, "/auth/totp/confirm", map[string]string{"code": code})
	var cr struct {
		RecoveryCodes []string `json:"recoveryCodes"`
	}
	if err := json.Unmarshal(cbody, &cr); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(cr.RecoveryCodes) == 0 {
		t.Fatal("no recovery codes")
	}

	resp2, body2 := postJSONAuth(t, env, "/auth/totp/disable", map[string]string{"code": cr.RecoveryCodes[0]})
	if resp2.StatusCode != 204 {
		t.Fatalf("disable status: got %d, body: %s", resp2.StatusCode, body2)
	}
}

func TestTOTP_Disable_NotEnabled(t *testing.T) {
	env := setupTOTPTest(t)
	resp, body := postJSONAuth(t, env, "/auth/totp/disable", map[string]string{"code": "123456"})
	if resp.StatusCode != 409 {
		t.Fatalf("got %d, want 409; body: %s", resp.StatusCode, body)
	}
	e := parseErr(t, body)
	if e.Error.Code != httpapi.CodeTOTPNotEnabled {
		t.Errorf("code: got %q, want %q", e.Error.Code, httpapi.CodeTOTPNotEnabled)
	}
}

func TestTOTP_Disable_InvalidCode(t *testing.T) {
	env := setupTOTPTest(t)
	resp, body := postJSONAuth(t, env, "/auth/totp/setup", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("setup: %d %s", resp.StatusCode, body)
	}
	var s totpSetupResp
	_ = json.Unmarshal(body, &s)
	code, _ := totp.GenerateCode(s.Secret, time.Now())
	postJSONAuth(t, env, "/auth/totp/confirm", map[string]string{"code": code})

	resp2, body2 := postJSONAuth(t, env, "/auth/totp/disable", map[string]string{"code": "000000"})
	if resp2.StatusCode != 401 {
		t.Fatalf("got %d, want 401; body: %s", resp2.StatusCode, body2)
	}
	e := parseErr(t, body2)
	if e.Error.Code != httpapi.CodeTOTPInvalidCode {
		t.Errorf("code: got %q, want %q", e.Error.Code, httpapi.CodeTOTPInvalidCode)
	}
}

func TestTOTP_Confirm_RateLimit(t *testing.T) {
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
	generous := auth.NewIPLimiter(rate.Every(time.Millisecond), 1000, 10*time.Minute)
	t.Cleanup(generous.Stop)
	tight := auth.NewIPLimiter(rate.Every(time.Hour), 1, time.Minute)
	t.Cleanup(tight.Stop)
	cheap := auth.Argon2Params{Memory: 8 * 1024, Time: 1, Threads: 1}
	svc := totpsvc.NewService(crypto.NewTokenCipher(totpCipherKey), users, recovery, cheap)

	authHandler := handlers.NewAuthHandler(users, sessions, issuer, generous, cheap)
	t.Cleanup(authHandler.Stop)
	totpHandler := handlers.NewTOTPHandler(users, svc, tight)

	app := httpapi.NewApp(httpapi.Deps{JWTIssuer: issuer})
	authGroup := app.Group("/auth")
	authHandler.RegisterAuth(authGroup, issuer)
	totpHandler.RegisterTOTP(authGroup.Group("", httpapi.AuthMiddleware(issuer)))

	ar := doSetupOn(t, app, "cli")
	env := &totpEnv{app: app, users: users, recovery: recovery, svc: svc, jwt: issuer, access: ar.Access}

	postJSONAuth(t, env, "/auth/totp/confirm", map[string]string{"code": "123456"}) // consume burst
	resp, body := postJSONAuth(t, env, "/auth/totp/confirm", map[string]string{"code": "123456"})
	if resp.StatusCode != 429 {
		t.Fatalf("got %d, want 429; body: %s", resp.StatusCode, body)
	}
	e := parseErr(t, body)
	if e.Error.Code != httpapi.CodeAuthRateLimited {
		t.Errorf("code: got %q, want %q", e.Error.Code, httpapi.CodeAuthRateLimited)
	}
}

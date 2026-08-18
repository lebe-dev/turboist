package handlers_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/handlers"
	"github.com/lebe-dev/turboist/internal/repo"
	passkeysvc "github.com/lebe-dev/turboist/internal/service/passkey"
	"golang.org/x/time/rate"
)

// setupPasskeyAuthTest wires the /auth group with passkey login enabled.
func setupPasskeyAuthTest(t *testing.T) (*fiber.App, *repo.WebAuthnRepo) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.RunMigrations(context.Background(), d); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	users := repo.NewUserRepo(d)
	if _, err := users.Create(context.Background(), "admin", "h"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	sessions := repo.NewSessionRepo(d)
	creds := repo.NewWebAuthnRepo(d)
	issuer := auth.NewJWTIssuer([]byte("test-secret-key-32-bytes-padding!"))
	limiter := auth.NewIPLimiter(rate.Every(time.Millisecond), 1000, 10*time.Minute)
	t.Cleanup(limiter.Stop)

	svc, err := passkeysvc.NewService(passkeysvc.Config{
		RPID:          "localhost",
		RPDisplayName: "Turboist",
		Origins:       []string{"http://localhost"},
	}, creds, users)
	if err != nil {
		t.Fatalf("passkey service: %v", err)
	}

	handler := handlers.NewAuthHandler(users, sessions, issuer, limiter, auth.DefaultArgon2Params()).
		WithPasskeys(svc)
	t.Cleanup(handler.Stop)

	app := httpapi.NewApp(httpapi.Deps{JWTIssuer: issuer})
	handler.RegisterAuth(app.Group("/auth"), issuer)
	return app, creds
}

func TestPasskeyLogin_Begin(t *testing.T) {
	app, _ := setupPasskeyAuthTest(t)

	resp, body := postJSON(t, app, "/auth/passkey/login/begin", map[string]any{"clientKind": "web"})
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var got struct {
		CeremonyID string `json:"ceremonyId"`
		Options    struct {
			PublicKey struct {
				Challenge        string `json:"challenge"`
				RPID             string `json:"rpId"`
				AllowCredentials []any  `json:"allowCredentials"`
			} `json:"publicKey"`
		} `json:"options"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode: %v; body: %s", err, body)
	}
	if got.CeremonyID == "" {
		t.Error("ceremonyId: got empty string, want an opaque id")
	}
	if got.Options.PublicKey.Challenge == "" {
		t.Error("challenge: got empty string, want a generated challenge")
	}
	if got.Options.PublicKey.RPID != "localhost" {
		t.Errorf("rpId: got %q, want localhost", got.Options.PublicKey.RPID)
	}
	// Discoverable login: the authenticator picks the credential, so the server
	// must not narrow the list (that is what makes the flow usernameless).
	if len(got.Options.PublicKey.AllowCredentials) != 0 {
		t.Errorf("allowCredentials: got %d entries, want 0", len(got.Options.PublicKey.AllowCredentials))
	}
}

func TestPasskeyLogin_Begin_ValidatesClientKind(t *testing.T) {
	app, _ := setupPasskeyAuthTest(t)

	resp, body := postJSON(t, app, "/auth/passkey/login/begin", map[string]any{"clientKind": "toaster"})
	if resp.StatusCode != 400 {
		t.Fatalf("status: got %d, want 400; body: %s", resp.StatusCode, body)
	}
	if code := parseErr(t, body).Error.Code; code != "validation_failed" {
		t.Errorf("code: got %q, want validation_failed", code)
	}
}

func TestPasskeyLogin_Finish_UnknownCeremony(t *testing.T) {
	app, _ := setupPasskeyAuthTest(t)

	resp, body := postJSON(t, app, "/auth/passkey/login/finish", map[string]any{
		"ceremonyId": "nope",
		"clientKind": "web",
		"credential": map[string]any{"id": "x"},
	})
	if resp.StatusCode != 400 {
		t.Fatalf("status: got %d, want 400; body: %s", resp.StatusCode, body)
	}
	if code := parseErr(t, body).Error.Code; code != "passkey_ceremony_invalid" {
		t.Errorf("code: got %q, want passkey_ceremony_invalid", code)
	}
}

// A garbage assertion must read as a failed login, not as a server fault, and
// must not disclose whether any credential exists.
func TestPasskeyLogin_Finish_InvalidAssertion(t *testing.T) {
	app, _ := setupPasskeyAuthTest(t)

	resp, body := postJSON(t, app, "/auth/passkey/login/begin", map[string]any{"clientKind": "web"})
	if resp.StatusCode != 200 {
		t.Fatalf("begin status: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var begin struct {
		CeremonyID string `json:"ceremonyId"`
	}
	if err := json.Unmarshal(body, &begin); err != nil {
		t.Fatalf("decode: %v; body: %s", err, body)
	}

	resp, body = postJSON(t, app, "/auth/passkey/login/finish", map[string]any{
		"ceremonyId": begin.CeremonyID,
		"clientKind": "web",
		"credential": map[string]any{"id": "x"},
	})
	if resp.StatusCode != 401 {
		t.Fatalf("status: got %d, want 401; body: %s", resp.StatusCode, body)
	}
	if code := parseErr(t, body).Error.Code; code != "auth_invalid" {
		t.Errorf("code: got %q, want auth_invalid", code)
	}
}

func TestPasskeyLogin_Finish_ValidatesBody(t *testing.T) {
	app, _ := setupPasskeyAuthTest(t)

	resp, body := postJSON(t, app, "/auth/passkey/login/finish", map[string]any{"clientKind": "web"})
	if resp.StatusCode != 400 {
		t.Fatalf("status: got %d, want 400; body: %s", resp.StatusCode, body)
	}
	if code := parseErr(t, body).Error.Code; code != "validation_failed" {
		t.Errorf("code: got %q, want validation_failed", code)
	}
}

// Without a passkey service the routes must not exist at all, so an install
// that could not configure WebAuthn simply has no passkey surface.
func TestPasskeyLogin_RoutesAbsentWhenDisabled(t *testing.T) {
	e := setupAuthTest(t)

	resp, _ := postJSON(t, e.app, "/auth/passkey/login/begin", map[string]any{"clientKind": "web"})
	if resp.StatusCode != 404 {
		t.Errorf("status: got %d, want 404", resp.StatusCode)
	}
}

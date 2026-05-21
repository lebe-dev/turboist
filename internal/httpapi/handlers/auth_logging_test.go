package handlers_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/handlers"
	"github.com/lebe-dev/turboist/internal/repo"
	"golang.org/x/time/rate"
)

// captureHandler stores every slog.Record so tests can inspect them.
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

func (h *captureHandler) snapshot() []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]slog.Record, len(*h.records))
	copy(out, *h.records)
	return out
}

// setupAuthTestWithLog mirrors setupAuthTest but injects a capture logger
// through the deps so handlers' logging.FromContext picks it up.
func setupAuthTestWithLog(t *testing.T, limiter *auth.IPLimiter) (*testEnv, *captureHandler) {
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
	if limiter == nil {
		limiter = auth.NewIPLimiter(rate.Every(time.Millisecond), 1000, 10*time.Minute)
		t.Cleanup(limiter.Stop)
	}

	cap := newCaptureHandler()
	logger := slog.New(cap)

	handler := handlers.NewAuthHandler(users, sessions, issuer, limiter, auth.DefaultArgon2Params())
	t.Cleanup(handler.Stop)

	deps := httpapi.Deps{Log: logger, JWTIssuer: issuer}
	app := httpapi.NewApp(deps)
	handler.RegisterAuth(app.Group("/auth"), issuer)

	return &testEnv{app: app, users: users, sessions: sessions, jwt: issuer, limiter: limiter}, cap
}

func findRecord(records []slog.Record, level slog.Level, msg string) (slog.Record, bool) {
	for _, r := range records {
		if r.Level == level && r.Message == msg {
			return r, true
		}
	}
	return slog.Record{}, false
}

func TestLogin_WrongPassword_LogsWarn(t *testing.T) {
	env, cap := setupAuthTestWithLog(t, nil)
	doSetup(t, env, "cli")

	req := httptest.NewRequest(http.MethodPost, "/auth/login", jsonBody(map[string]string{
		"username":   "admin",
		"password":   "WRONG",
		"clientKind": "cli",
	}))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := doReq(t, env.app, req)
	if resp.StatusCode != 401 {
		t.Fatalf("status: got %d, want 401", resp.StatusCode)
	}

	rec, ok := findRecord(cap.snapshot(), slog.LevelWarn, "auth: login wrong password")
	if !ok {
		t.Fatal("no WARN 'auth: login wrong password' record")
	}
	var uid int64
	rec.Attrs(func(a slog.Attr) bool {
		if a.Key == "user_id" {
			uid = a.Value.Int64()
		}
		return true
	})
	if uid == 0 {
		t.Error("user_id missing from wrong-password log")
	}
}

func TestLogin_UnknownUser_LogsWarn(t *testing.T) {
	env, cap := setupAuthTestWithLog(t, nil)
	doSetup(t, env, "cli")

	req := httptest.NewRequest(http.MethodPost, "/auth/login", jsonBody(map[string]string{
		"username":   "ghost",
		"password":   "irrelevant",
		"clientKind": "cli",
	}))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := doReq(t, env.app, req)
	if resp.StatusCode != 401 {
		t.Fatalf("status: got %d, want 401", resp.StatusCode)
	}

	if _, ok := findRecord(cap.snapshot(), slog.LevelWarn, "auth: login unknown user"); !ok {
		t.Error("no WARN 'auth: login unknown user' record")
	}
}

func TestLogin_Success_LogsInfo(t *testing.T) {
	env, cap := setupAuthTestWithLog(t, nil)
	doSetup(t, env, "cli")

	req := httptest.NewRequest(http.MethodPost, "/auth/login", jsonBody(map[string]string{
		"username":   "admin",
		"password":   "secret123",
		"clientKind": "cli",
	}))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := doReq(t, env.app, req)
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}

	if _, ok := findRecord(cap.snapshot(), slog.LevelInfo, "auth: login ok"); !ok {
		t.Error("no INFO 'auth: login ok' record")
	}
}

func TestRefresh_InvalidToken_LogsWarn(t *testing.T) {
	env, cap := setupAuthTestWithLog(t, nil)
	doSetup(t, env, "cli")

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", jsonBody(map[string]string{
		"refresh": "not-a-real-token",
	}))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := doReq(t, env.app, req)
	if resp.StatusCode != 401 {
		t.Fatalf("status: got %d, want 401", resp.StatusCode)
	}

	if _, ok := findRecord(cap.snapshot(), slog.LevelWarn, "auth: refresh token unknown"); !ok {
		t.Error("no WARN 'auth: refresh token unknown' record")
	}
}

func TestRefresh_TokenReuse_LogsWarn(t *testing.T) {
	env, cap := setupAuthTestWithLog(t, nil)
	ar := doSetup(t, env, "cli")
	old := ar.Refresh

	// First refresh: rotates.
	req1 := httptest.NewRequest(http.MethodPost, "/auth/refresh", jsonBody(map[string]string{"refresh": old}))
	req1.Header.Set("Content-Type", "application/json")
	resp1, _ := doReq(t, env.app, req1)
	if resp1.StatusCode != 200 {
		t.Fatalf("first refresh: got %d", resp1.StatusCode)
	}

	// Reuse the old token → theft detected.
	req2 := httptest.NewRequest(http.MethodPost, "/auth/refresh", jsonBody(map[string]string{"refresh": old}))
	req2.Header.Set("Content-Type", "application/json")
	resp2, _ := doReq(t, env.app, req2)
	if resp2.StatusCode != 401 {
		t.Fatalf("reuse: got %d", resp2.StatusCode)
	}

	if _, ok := findRecord(cap.snapshot(), slog.LevelWarn, "auth: refresh token reuse"); !ok {
		t.Error("no WARN 'auth: refresh token reuse' record")
	}
}

func TestRefresh_Success_LogsInfo(t *testing.T) {
	env, cap := setupAuthTestWithLog(t, nil)
	ar := doSetup(t, env, "cli")

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", jsonBody(map[string]string{"refresh": ar.Refresh}))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := doReq(t, env.app, req)
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d", resp.StatusCode)
	}

	if _, ok := findRecord(cap.snapshot(), slog.LevelInfo, "auth: refresh ok"); !ok {
		t.Error("no INFO 'auth: refresh ok' record")
	}
}

func TestLogin_RateLimited_LogsWarn(t *testing.T) {
	tightLimiter := auth.NewIPLimiter(rate.Every(time.Hour), 1, time.Minute)
	t.Cleanup(tightLimiter.Stop)

	env, cap := setupAuthTestWithLog(t, tightLimiter)
	doSetup(t, env, "cli")

	body := jsonBody(map[string]string{"username": "admin", "password": "secret123", "clientKind": "cli"})
	// First login uses up the burst (and limiter already consumed by setup).
	req1 := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBuffer(body.Bytes()))
	req1.Header.Set("Content-Type", "application/json")
	doReq(t, env.app, req1)

	req2 := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBuffer(body.Bytes()))
	req2.Header.Set("Content-Type", "application/json")
	resp2, _ := doReq(t, env.app, req2)
	if resp2.StatusCode != 429 {
		t.Fatalf("status: got %d, want 429", resp2.StatusCode)
	}

	if _, ok := findRecord(cap.snapshot(), slog.LevelWarn, "auth: login rate limited"); !ok {
		t.Error("no WARN 'auth: login rate limited' record")
	}
}

func TestLogout_Success_LogsInfo(t *testing.T) {
	env, cap := setupAuthTestWithLog(t, nil)
	ar := doSetup(t, env, "cli")

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.Header.Set("Authorization", bearerHeader(ar.Access))
	resp, _ := doReq(t, env.app, req)
	if resp.StatusCode != 204 {
		t.Fatalf("status: got %d, want 204", resp.StatusCode)
	}

	if _, ok := findRecord(cap.snapshot(), slog.LevelInfo, "auth: logout ok"); !ok {
		t.Error("no INFO 'auth: logout ok' record")
	}
}

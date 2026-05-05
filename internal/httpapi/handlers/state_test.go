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

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/handlers"
	"github.com/lebe-dev/turboist/internal/repo"
)

// stateEnv is a minimal app wired only with what StateHandler needs.
type stateEnv struct {
	app   *fiber.App
	jwt   *auth.JWTIssuer
	users *repo.UserRepo
}

func setupStateEnv(t *testing.T) *stateEnv {
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
	if _, err := users.Create(context.Background(), "admin", "h"); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	issuer := auth.NewJWTIssuer([]byte("test-secret-key-32-bytes-padding!"))
	deps := httpapi.Deps{JWTIssuer: issuer}
	app := httpapi.NewApp(deps)
	api := httpapi.RegisterRoutes(app, deps)
	handlers.NewStateHandler(users).Register(api)

	return &stateEnv{app: app, jwt: issuer, users: users}
}

func (e *stateEnv) authedReq(t *testing.T, method, url string, body io.Reader) *http.Request {
	t.Helper()
	tok, _, err := e.jwt.Issue(1, 1)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	req := httptest.NewRequest(method, url, body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	return req
}

func doStateReq(t *testing.T, e *stateEnv, req *http.Request) (*http.Response, []byte) {
	t.Helper()
	resp, err := e.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	b, err := io.ReadAll(resp.Body)
	if cerr := resp.Body.Close(); cerr != nil && err == nil {
		err = cerr
	}
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp, b
}

func TestStateHandler_Get_Empty(t *testing.T) {
	e := setupStateEnv(t)
	resp, body := doStateReq(t, e, e.authedReq(t, http.MethodGet, "/api/v1/state", nil))

	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if string(body) != "{}" {
		t.Errorf("body = %q, want %q", body, "{}")
	}
}

func TestStateHandler_Patch_StoresAndMerges(t *testing.T) {
	e := setupStateEnv(t)

	resp, body := doStateReq(t, e, e.authedReq(t, http.MethodPatch, "/api/v1/state",
		bytes.NewBufferString(`{"a":1}`)))
	if resp.StatusCode != 200 {
		t.Fatalf("first patch status = %d body=%s", resp.StatusCode, body)
	}
	got := map[string]any{}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if v, ok := got["a"].(float64); !ok || v != 1 {
		t.Errorf("after first patch: got %+v, want a=1", got)
	}

	resp, body = doStateReq(t, e, e.authedReq(t, http.MethodPatch, "/api/v1/state",
		bytes.NewBufferString(`{"b":2}`)))
	if resp.StatusCode != 200 {
		t.Fatalf("second patch status = %d body=%s", resp.StatusCode, body)
	}
	got = map[string]any{}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if v, ok := got["a"].(float64); !ok || v != 1 {
		t.Errorf("after second patch: a missing/wrong: %+v", got)
	}
	if v, ok := got["b"].(float64); !ok || v != 2 {
		t.Errorf("after second patch: b missing/wrong: %+v", got)
	}

	resp, body = doStateReq(t, e, e.authedReq(t, http.MethodGet, "/api/v1/state", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("get status = %d", resp.StatusCode)
	}
	got = map[string]any{}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 2 || got["a"].(float64) != 1 || got["b"].(float64) != 2 {
		t.Errorf("get returned %+v, want {a:1, b:2}", got)
	}
}

func TestStateHandler_Patch_NullRemovesKey(t *testing.T) {
	e := setupStateEnv(t)

	resp, _ := doStateReq(t, e, e.authedReq(t, http.MethodPatch, "/api/v1/state",
		bytes.NewBufferString(`{"a":1,"b":2}`)))
	if resp.StatusCode != 200 {
		t.Fatalf("seed status = %d", resp.StatusCode)
	}

	resp, body := doStateReq(t, e, e.authedReq(t, http.MethodPatch, "/api/v1/state",
		bytes.NewBufferString(`{"a":null}`)))
	if resp.StatusCode != 200 {
		t.Fatalf("delete-key patch status = %d body=%s", resp.StatusCode, body)
	}
	got := map[string]any{}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := got["a"]; ok {
		t.Errorf("key 'a' still present after null patch: %+v", got)
	}
	if v, ok := got["b"].(float64); !ok || v != 2 {
		t.Errorf("key 'b' lost: %+v", got)
	}

	_, body = doStateReq(t, e, e.authedReq(t, http.MethodGet, "/api/v1/state", nil))
	got = map[string]any{}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := got["a"]; ok {
		t.Errorf("get after delete: 'a' should be gone: %+v", got)
	}
}

func TestStateHandler_Patch_InvalidJSON(t *testing.T) {
	e := setupStateEnv(t)
	resp, body := doStateReq(t, e, e.authedReq(t, http.MethodPatch, "/api/v1/state",
		bytes.NewBufferString(`not json`)))
	if resp.StatusCode != 400 {
		t.Errorf("status = %d, want 400 (body=%s)", resp.StatusCode, body)
	}
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if env.Error.Code != httpapi.CodeValidationFailed {
		t.Errorf("code = %q, want %q", env.Error.Code, httpapi.CodeValidationFailed)
	}
}

func TestStateHandler_Patch_TooLarge(t *testing.T) {
	e := setupStateEnv(t)
	// > 64 KiB body
	big := strings.Repeat("a", 65*1024)
	payload := `{"k":"` + big + `"}`
	resp, body := doStateReq(t, e, e.authedReq(t, http.MethodPatch, "/api/v1/state",
		bytes.NewBufferString(payload)))
	if resp.StatusCode != 400 {
		t.Errorf("status = %d, want 400 (body trimmed=%.80s)", resp.StatusCode, body)
	}
}

func TestStateHandler_Unauthenticated(t *testing.T) {
	e := setupStateEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/state", nil)
	resp, _ := doStateReq(t, e, req)
	if resp.StatusCode != 401 {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

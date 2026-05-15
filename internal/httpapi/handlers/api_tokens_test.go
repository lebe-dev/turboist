package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lebe-dev/turboist/internal/auth"
)

func TestAPITokensHandler_CreateReturnsTokenOnce(t *testing.T) {
	env := setupAPIEnv(t)
	resp, err := env.app.Test(env.authedReq(t, http.MethodPost, "/api/v1/api-tokens/", map[string]any{"name": "n8n"}))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusCreated)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	tok, _ := body["token"].(string)
	if tok == "" {
		t.Fatalf("token must be present in create response")
	}
	if _, ok := body["tokenHash"]; ok {
		t.Errorf("tokenHash must NOT leak in response")
	}
	if name, _ := body["name"].(string); name != "n8n" {
		t.Errorf("name: got %q, want %q", name, "n8n")
	}

	// list must not include the plaintext token
	listResp, err := env.app.Test(env.authedReq(t, http.MethodGet, "/api/v1/api-tokens/", nil))
	if err != nil {
		t.Fatal(err)
	}
	var list []map[string]any
	if err := json.NewDecoder(listResp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("list length: got %d, want 1", len(list))
	}
	if _, ok := list[0]["token"]; ok {
		t.Errorf("list must not expose token")
	}
	if _, ok := list[0]["tokenHash"]; ok {
		t.Errorf("list must not expose tokenHash")
	}
}

func TestAPITokensHandler_CreateValidation(t *testing.T) {
	env := setupAPIEnv(t)

	resp, err := env.app.Test(env.authedReq(t, http.MethodPost, "/api/v1/api-tokens/", map[string]any{"name": ""}))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("empty name: got %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	resp2, err := env.app.Test(env.authedReq(t, http.MethodPost, "/api/v1/api-tokens/", map[string]any{"name": strings.Repeat("a", 65)}))
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != http.StatusBadRequest {
		t.Errorf("long name: got %d, want %d", resp2.StatusCode, http.StatusBadRequest)
	}
}

func TestAPITokensHandler_Delete(t *testing.T) {
	env := setupAPIEnv(t)

	resp, err := env.app.Test(env.authedReq(t, http.MethodPost, "/api/v1/api-tokens/", map[string]any{"name": "n8n"}))
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	id, _ := body["id"].(float64)
	if id == 0 {
		t.Fatalf("missing id in create response")
	}

	delResp, err := env.app.Test(env.authedReq(t, http.MethodDelete, "/api/v1/api-tokens/"+itoa(int64(id)), nil))
	if err != nil {
		t.Fatal(err)
	}
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status: got %d, want %d", delResp.StatusCode, http.StatusNoContent)
	}

	// second delete -> 404
	delResp2, err := env.app.Test(env.authedReq(t, http.MethodDelete, "/api/v1/api-tokens/"+itoa(int64(id)), nil))
	if err != nil {
		t.Fatal(err)
	}
	if delResp2.StatusCode != http.StatusNotFound {
		t.Fatalf("second delete status: got %d, want %d", delResp2.StatusCode, http.StatusNotFound)
	}
}

func TestAPITokensHandler_APITokenForbiddenOnTokenRoutes(t *testing.T) {
	env := setupAPIEnv(t)

	// create a token via JWT
	resp, err := env.app.Test(env.authedReq(t, http.MethodPost, "/api/v1/api-tokens/", map[string]any{"name": "n8n"}))
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	plain, _ := body["token"].(string)
	if plain == "" {
		t.Fatal("missing token")
	}

	// list using the API token -> must be rejected
	req := httptest.NewRequest(http.MethodGet, "/api/v1/api-tokens/", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	r, err := env.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("api-token access to /api-tokens: got %d, want %d", r.StatusCode, http.StatusUnauthorized)
	}
}

func TestAPITokensHandler_APITokenAccessesOtherRoutes(t *testing.T) {
	env := setupAPIEnv(t)

	// store a token directly via repo with a known plaintext
	plain, err := auth.GenerateAPIToken()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	hash := auth.HashAPIToken(plain, env.apiTokenSalt)
	if _, err := env.apiTokens.Create(context.Background(), 1, "n8n", hash); err != nil {
		t.Fatalf("repo create: %v", err)
	}

	// request /api/v1/settings with the api token -> 200
	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	r, err := env.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if r.StatusCode != http.StatusOK {
		t.Fatalf("api-token access to /settings: got %d, want 200", r.StatusCode)
	}
}

func TestAPITokensHandler_InvalidTokenRejected(t *testing.T) {
	env := setupAPIEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	req.Header.Set("Authorization", "Bearer garbage-not-a-token")
	r, err := env.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("garbage token: got %d, want 401", r.StatusCode)
	}
}

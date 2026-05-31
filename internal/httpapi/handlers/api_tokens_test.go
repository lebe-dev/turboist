package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/lebe-dev/turboist/internal/auth"
)

func TestAPITokensHandler_CreateReturnsTokenOnce(t *testing.T) {
	env := setupAPIEnv(t)
	wantScopes := []string{"tasks:read", "tasks:write", "projects:read"}
	resp, err := env.app.Test(env.authedReq(t, http.MethodPost, "/api/v1/api-tokens/", map[string]any{
		"name":   "n8n",
		"scopes": wantScopes,
	}))
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
	if gotScopes := scopesFromJSON(body["scopes"]); !reflect.DeepEqual(gotScopes, wantScopes) {
		t.Errorf("scopes: got %v, want %v", gotScopes, wantScopes)
	}

	// list must not include the plaintext token and must include scopes
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
	if gotScopes := scopesFromJSON(list[0]["scopes"]); !reflect.DeepEqual(gotScopes, wantScopes) {
		t.Errorf("list scopes: got %v, want %v", gotScopes, wantScopes)
	}
}

func TestAPITokensHandler_CreateWithWildcard(t *testing.T) {
	env := setupAPIEnv(t)
	resp, err := env.app.Test(env.authedReq(t, http.MethodPost, "/api/v1/api-tokens/", map[string]any{
		"name":   "full",
		"scopes": []string{"*"},
	}))
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
	if got := scopesFromJSON(body["scopes"]); !reflect.DeepEqual(got, []string{"*"}) {
		t.Errorf("scopes: got %v, want [*]", got)
	}
}

func TestAPITokensHandler_CreateValidation(t *testing.T) {
	env := setupAPIEnv(t)

	resp, err := env.app.Test(env.authedReq(t, http.MethodPost, "/api/v1/api-tokens/", map[string]any{"name": "", "scopes": []string{"*"}}))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("empty name: got %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	resp2, err := env.app.Test(env.authedReq(t, http.MethodPost, "/api/v1/api-tokens/", map[string]any{"name": strings.Repeat("a", 65), "scopes": []string{"*"}}))
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != http.StatusBadRequest {
		t.Errorf("long name: got %d, want %d", resp2.StatusCode, http.StatusBadRequest)
	}
}

func TestAPITokensHandler_CreateScopesValidation(t *testing.T) {
	env := setupAPIEnv(t)

	cases := []struct {
		name    string
		payload map[string]any
	}{
		{"missing scopes", map[string]any{"name": "n"}},
		{"empty scopes", map[string]any{"name": "n", "scopes": []string{}}},
		{"invalid scope", map[string]any{"name": "n", "scopes": []string{"foo:bar"}}},
		{"write without read", map[string]any{"name": "n", "scopes": []string{"tasks:write"}}},
		{"duplicate scope", map[string]any{"name": "n", "scopes": []string{"tasks:read", "tasks:read"}}},
		{"wildcard with other", map[string]any{"name": "n", "scopes": []string{"*", "tasks:read"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := env.app.Test(env.authedReq(t, http.MethodPost, "/api/v1/api-tokens/", tc.payload))
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != http.StatusUnprocessableEntity {
				t.Errorf("status: got %d, want 422", resp.StatusCode)
			}
		})
	}
}

func TestAPITokensHandler_Delete(t *testing.T) {
	env := setupAPIEnv(t)

	resp, err := env.app.Test(env.authedReq(t, http.MethodPost, "/api/v1/api-tokens/", map[string]any{
		"name":   "n8n",
		"scopes": []string{"*"},
	}))
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
	resp, err := env.app.Test(env.authedReq(t, http.MethodPost, "/api/v1/api-tokens/", map[string]any{
		"name":   "n8n",
		"scopes": []string{"*"},
	}))
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
	if _, err := env.apiTokens.Create(context.Background(), 1, "n8n", hash, []string{"*"}); err != nil {
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

// scopesFromJSON extracts a []string from a JSON-decoded "scopes" field which
// arrives as []any (encoding/json default for arrays).
func scopesFromJSON(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, x := range arr {
		s, _ := x.(string)
		out = append(out, s)
	}
	return out
}

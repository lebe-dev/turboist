package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/httpapi/handlers"
)

func assertOpenAPIDocument(t *testing.T, status int, body []byte) {
	t.Helper()
	if status != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body: %s", status, body)
	}
	var doc struct {
		OpenAPI string         `json:"openapi"`
		Paths   map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if doc.OpenAPI != "3.1.0" {
		t.Errorf("openapi: got %q, want 3.1.0", doc.OpenAPI)
	}
	if _, ok := doc.Paths["/api/v1/tasks/{id}"]; !ok {
		t.Errorf("paths: missing /api/v1/tasks/{id}")
	}
}

func TestOpenAPIEndpoint_RejectsAnonymous(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, httptest.NewRequest(http.MethodGet, handlers.OpenAPIPath, nil))
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want 401; body: %s", resp.StatusCode, body)
	}
}

func TestOpenAPIEndpoint_RejectsInvalidToken(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, tokenReq(http.MethodGet, handlers.OpenAPIPath, "tbst_not-a-real-token", nil))
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want 401; body: %s", resp.StatusCode, body)
	}
}

func TestOpenAPIEndpoint_ServesDocumentToSession(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, handlers.OpenAPIPath, nil))
	assertOpenAPIDocument(t, resp.StatusCode, body)
}

// Any valid API token may read the document, whatever its scopes: it only
// describes the API, it exposes no user data.
func TestOpenAPIEndpoint_ServesDocumentToAPIToken(t *testing.T) {
	e := setupAPIEnv(t)
	tok := issueAPIToken(t, e, "openapi-reader", []string{auth.ScopeTasksRead})
	resp, body := doReq(t, e.app, tokenReq(http.MethodGet, handlers.OpenAPIPath, tok, nil))
	assertOpenAPIDocument(t, resp.StatusCode, body)
}

package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/repo"
)

// issueAPIToken stores an API token with the given scopes via the repo and
// returns the plaintext token to be used as a Bearer credential.
func issueAPIToken(t *testing.T, e *apiEnv, name string, scopes []string) string {
	t.Helper()
	plain, err := auth.GenerateAPIToken()
	if err != nil {
		t.Fatalf("generate api token: %v", err)
	}
	hash := auth.HashAPIToken(plain, e.apiTokenSalt)
	if _, err := e.apiTokens.Create(context.Background(), 1, name, hash, scopes); err != nil {
		t.Fatalf("create api token: %v", err)
	}
	return plain
}

// tokenReq builds a request authenticated with the given API-token plaintext.
func tokenReq(method, url, token string, body any) *http.Request {
	var r *http.Request
	if body != nil {
		b, _ := json.Marshal(body)
		r = httptest.NewRequest(method, url, bytes.NewReader(b))
	} else {
		r = httptest.NewRequest(method, url, nil)
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+token)
	return r
}

func runRequest(t *testing.T, app *fiber.App, req *http.Request) (*http.Response, []byte) {
	t.Helper()
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp, body
}

// seedTask seeds a single inbox task and returns its id.
func seedTaskForScope(t *testing.T, e *apiEnv) int64 {
	t.Helper()
	inboxID := int64(1)
	tk, err := e.tasks.Create(context.Background(), repo.CreateTask{
		Placement: repo.Placement{InboxID: &inboxID},
		Title:     "scope-fixture",
	})
	if err != nil {
		t.Fatalf("seed task: %v", err)
	}
	return tk.ID
}

// seedContext seeds a single context and returns its id.
func seedContextForScope(t *testing.T, e *apiEnv) int64 {
	t.Helper()
	c, err := e.ctxs.Create(context.Background(), "ScopeCtx", "blue", false)
	if err != nil {
		t.Fatalf("seed context: %v", err)
	}
	return c.ID
}

// seedProject seeds a context + project and returns the project id.
func seedProjectForScope(t *testing.T, e *apiEnv) int64 {
	t.Helper()
	ctxID := seedContextForScope(t, e)
	p, err := e.projects.Create(context.Background(), repo.CreateProject{
		ContextID: ctxID,
		Title:     "ScopeProj",
		Color:     "red",
	})
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}
	return p.ID
}

func TestScope_ReadToken_AllowsRead(t *testing.T) {
	e := setupAPIEnv(t)
	taskID := seedTaskForScope(t, e)
	tok := issueAPIToken(t, e, "read-only", []string{"tasks:read"})

	resp, body := runRequest(t, e.app, tokenReq(http.MethodGet, "/api/v1/tasks/"+itoa(taskID), tok, nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /tasks/:id: got %d, want 200; body: %s", resp.StatusCode, body)
	}
}

func TestScope_ReadToken_BlocksWrite(t *testing.T) {
	e := setupAPIEnv(t)
	tok := issueAPIToken(t, e, "read-only", []string{"tasks:read"})

	resp, body := runRequest(t, e.app, tokenReq(http.MethodPost, "/api/v1/inbox/tasks", tok, map[string]any{
		"title": "new task",
	}))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("POST /inbox/tasks: got %d, want 403; body: %s", resp.StatusCode, body)
	}
}

func TestScope_WriteToken_AllowsWrite(t *testing.T) {
	e := setupAPIEnv(t)
	tok := issueAPIToken(t, e, "rw", []string{"tasks:read", "tasks:write"})

	resp, body := runRequest(t, e.app, tokenReq(http.MethodPost, "/api/v1/inbox/tasks", tok, map[string]any{
		"title": "new task",
	}))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /inbox/tasks: got %d, want 201; body: %s", resp.StatusCode, body)
	}
}

func TestScope_WildcardToken_AllowsAny(t *testing.T) {
	e := setupAPIEnv(t)
	taskID := seedTaskForScope(t, e)
	tok := issueAPIToken(t, e, "wildcard", []string{"*"})

	// Wildcard satisfies tasks:read (GET) ...
	resp, body := runRequest(t, e.app, tokenReq(http.MethodGet, "/api/v1/tasks/"+itoa(taskID), tok, nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /tasks/:id: got %d, want 200; body: %s", resp.StatusCode, body)
	}

	// ... as well as tasks:write (POST), contexts:read (GET), settings:read.
	for _, c := range []struct {
		method string
		url    string
		body   any
		want   int
	}{
		{http.MethodPost, "/api/v1/inbox/tasks", map[string]any{"title": "wild"}, http.StatusCreated},
		{http.MethodGet, "/api/v1/contexts/", nil, http.StatusOK},
		{http.MethodGet, "/api/v1/settings", nil, http.StatusOK},
	} {
		resp, body := runRequest(t, e.app, tokenReq(c.method, c.url, tok, c.body))
		if resp.StatusCode != c.want {
			t.Errorf("%s %s: got %d, want %d; body: %s", c.method, c.url, resp.StatusCode, c.want, body)
		}
	}
}

func TestScope_JWT_BypassesScopeCheck(t *testing.T) {
	e := setupAPIEnv(t)
	taskID := seedTaskForScope(t, e)

	// JWT never goes through RequireScope — every protected endpoint is
	// reachable regardless of the scope a co-existing API token might hold.
	for _, c := range []struct {
		method string
		url    string
		body   any
		want   int
	}{
		{http.MethodGet, "/api/v1/tasks/" + itoa(taskID), nil, http.StatusOK},
		{http.MethodPost, "/api/v1/inbox/tasks", map[string]any{"title": "via jwt"}, http.StatusCreated},
		{http.MethodGet, "/api/v1/settings", nil, http.StatusOK},
		{http.MethodGet, "/api/v1/contexts/", nil, http.StatusOK},
	} {
		resp, body := runRequest(t, e.app, e.authedReq(t, c.method, c.url, c.body))
		if resp.StatusCode != c.want {
			t.Errorf("%s %s (jwt): got %d, want %d; body: %s", c.method, c.url, resp.StatusCode, c.want, body)
		}
	}
}

func TestScope_ForbiddenResponse_DoesNotLeakScopeName(t *testing.T) {
	e := setupAPIEnv(t)
	tok := issueAPIToken(t, e, "narrow", []string{"tasks:read"})

	resp, body := runRequest(t, e.app, tokenReq(http.MethodPost, "/api/v1/inbox/tasks", tok, map[string]any{
		"title": "blocked",
	}))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status: got %d, want 403; body: %s", resp.StatusCode, body)
	}

	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("parse error envelope: %v; body: %s", err, body)
	}
	if envelope.Error.Code != "forbidden" {
		t.Errorf("code: got %q, want %q", envelope.Error.Code, "forbidden")
	}
	if envelope.Error.Message != "insufficient scope" {
		t.Errorf("message: got %q, want %q", envelope.Error.Message, "insufficient scope")
	}
	// The scope name itself must never appear in the response body — that would
	// hand attackers the full scope catalogue via 403-probing.
	for _, leak := range []string{"tasks:write", "tasks:read", "projects:write"} {
		if strings.Contains(string(body), leak) {
			t.Errorf("response leaks scope name %q: %s", leak, body)
		}
	}
}

func TestScope_APIToken_BlockedFromSSE(t *testing.T) {
	e := setupAPIEnv(t)
	tok := issueAPIToken(t, e, "broad", []string{"*"})

	// /events itself is mounted public (ticket-auth in handler) and rejects
	// requests without a ticket. The path that matters here is /events/ticket:
	// it's guarded by RequireJWTAuth so API tokens cannot mint a stream ticket.
	resp, body := runRequest(t, e.app, tokenReq(http.MethodPost, "/api/v1/events/ticket", tok, nil))
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("POST /events/ticket: got %d, want 401; body: %s", resp.StatusCode, body)
	}

	// And the stream endpoint without a ticket → 401 too.
	resp2, body2 := runRequest(t, e.app, tokenReq(http.MethodGet, "/api/v1/events", tok, nil))
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("GET /events: got %d, want 401; body: %s", resp2.StatusCode, body2)
	}
}

func TestScope_APIToken_BlockedFromBackup(t *testing.T) {
	e := setupAPIEnv(t)
	tok := issueAPIToken(t, e, "broad", []string{"*"})

	resp, body := runRequest(t, e.app, tokenReq(http.MethodGet, "/api/v1/backup", tok, nil))
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("GET /backup: got %d, want 401; body: %s", resp.StatusCode, body)
	}
}

// Cross-resource: POST /projects/:id/tasks requires only tasks:write, not
// projects:* — the project id is just a placement target.
func TestScope_CrossResource_CreateTaskInProject(t *testing.T) {
	e := setupAPIEnv(t)
	projID := seedProjectForScope(t, e)
	tok := issueAPIToken(t, e, "tasks-rw-only", []string{"tasks:read", "tasks:write"})

	resp, body := runRequest(t, e.app, tokenReq(http.MethodPost, "/api/v1/projects/"+itoa(projID)+"/tasks", tok, map[string]any{
		"title": "in proj",
	}))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /projects/:id/tasks: got %d, want 201; body: %s", resp.StatusCode, body)
	}
}

// Cross-resource: GET /contexts/:id/tasks lists tasks, so it requires
// tasks:read — not contexts:read.
func TestScope_CrossResource_ListTasksInContext(t *testing.T) {
	e := setupAPIEnv(t)
	ctxID := seedContextForScope(t, e)
	tok := issueAPIToken(t, e, "tasks-read-only", []string{"tasks:read"})

	resp, body := runRequest(t, e.app, tokenReq(http.MethodGet, "/api/v1/contexts/"+itoa(ctxID)+"/tasks", tok, nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /contexts/:id/tasks: got %d, want 200; body: %s", resp.StatusCode, body)
	}
}

func TestScope_Bundle_RequiresAllDomains(t *testing.T) {
	e := setupAPIEnv(t)
	projID := seedProjectForScope(t, e)
	url := "/api/v1/projects/" + itoa(projID) + "/bundle"

	// Only tasks:read — missing projects:read + sections:read → forbidden, even
	// though the bundle returns task data the token could read on its own.
	partial := issueAPIToken(t, e, "tasks-only", []string{"tasks:read"})
	resp, body := runRequest(t, e.app, tokenReq(http.MethodGet, url, partial, nil))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("partial scope: got %d, want 403; body: %s", resp.StatusCode, body)
	}

	// All three read scopes → allowed.
	full := issueAPIToken(t, e, "bundle-reader", []string{"projects:read", "sections:read", "tasks:read"})
	resp, body = runRequest(t, e.app, tokenReq(http.MethodGet, url, full, nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("full scope: got %d, want 200; body: %s", resp.StatusCode, body)
	}
}

// /api/v1/config is the workspace bootstrap aggregate: it returns tasks,
// projects, labels, contexts, templates and the troiki board. A lone
// settings:read token used to be enough to read all of it.
func TestScope_Config_RequiresEveryEmbeddedDomain(t *testing.T) {
	e := setupAPIEnv(t)
	const url = "/api/v1/config"

	partial := issueAPIToken(t, e, "settings-only", []string{"settings:read"})
	resp, body := runRequest(t, e.app, tokenReq(http.MethodGet, url, partial, nil))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("settings:read alone: got %d, want 403; body: %s", resp.StatusCode, body)
	}

	full := issueAPIToken(t, e, "config-reader", []string{
		"settings:read", "tasks:read", "projects:read",
		"labels:read", "contexts:read", "troiki:read", "templates:read",
	})
	resp, body = runRequest(t, e.app, tokenReq(http.MethodGet, url, full, nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("full scope set: got %d, want 200; body: %s", resp.StatusCode, body)
	}

	wildcard := issueAPIToken(t, e, "config-wildcard", []string{"*"})
	resp, body = runRequest(t, e.app, tokenReq(http.MethodGet, url, wildcard, nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("wildcard token: got %d, want 200; body: %s", resp.StatusCode, body)
	}
}

// The label usage report is guarded by labels:read even though it aggregates task
// data: a tasks-only token must not be able to enumerate the label set through it.
func TestScope_LabelStats_RequiresLabelsRead(t *testing.T) {
	e := setupAPIEnv(t)
	const url = "/api/v1/labels/stats"

	wrong := issueAPIToken(t, e, "tasks-only", []string{"tasks:read"})
	resp, body := runRequest(t, e.app, tokenReq(http.MethodGet, url, wrong, nil))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("tasks:read only: got %d, want 403; body: %s", resp.StatusCode, body)
	}

	right := issueAPIToken(t, e, "labels-reader", []string{"labels:read"})
	resp, body = runRequest(t, e.app, tokenReq(http.MethodGet, url, right, nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("labels:read: got %d, want 200; body: %s", resp.StatusCode, body)
	}

	// A write-only token is not a read token.
	writeOnly := issueAPIToken(t, e, "labels-writer", []string{"labels:write"})
	resp, body = runRequest(t, e.app, tokenReq(http.MethodGet, url, writeOnly, nil))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("labels:write only: got %d, want 403; body: %s", resp.StatusCode, body)
	}
}

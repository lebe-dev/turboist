package handlers

import (
	"bytes"
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/openapi"
	"github.com/lebe-dev/turboist/internal/service/passkey"
)

var updateOpenAPI = flag.Bool("update", false, "rewrite the committed OpenAPI document")

// openAPIFile is the committed document, relative to this package.
var openAPIFile = filepath.Join("..", "..", "..", "api", "openapi.yaml")

// mountedApp and mountedRoutes mount every handler exactly as the server does — with the
// optional features switched on — and lists the routes that end up registered.
func mountedRoutes(t *testing.T) []string {
	t.Helper()
	app := mountedApp()
	seen := map[string]bool{}
	for _, r := range app.GetRoutes(true) {
		if r.Method == http.MethodHead {
			continue
		}
		path := r.Path
		if len(path) > 1 {
			path = strings.TrimSuffix(path, "/")
		}
		seen[r.Method+" "+path] = true
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func mountedApp() *fiber.App {
	app := fiber.New()
	Mount(app, httpapi.Deps{}, Handlers{
		Auth:            &AuthHandler{passkeys: &passkey.Service{}},
		TOTP:            &TOTPHandler{},
		Events:          &EventsHandler{},
		Calendar:        &CalendarHandler{},
		Contexts:        &ContextHandler{},
		Labels:          &LabelHandler{},
		Templates:       &TemplateHandler{},
		Sections:        &SectionHandler{},
		Projects:        &ProjectHandler{},
		Inbox:           &InboxHandler{},
		TaskBulk:        &TaskBulkHandler{},
		TaskViews:       &TaskViewHandler{},
		TaskActions:     &TaskActionHandler{},
		Troiki:          &TroikiHandler{},
		TaskRelations:   &TaskRelationHandler{},
		Tasks:           &TaskHandler{},
		Search:          &SearchHandler{},
		Meta:            &MetaHandler{},
		State:           &StateHandler{},
		Settings:        &SettingsHandler{},
		Harpoon:         &HarpoonHandler{},
		AppSettings:     &AppSettingsHandler{},
		InboxProcessing: &InboxProcessingHandler{},
		Sync:            &SyncHandler{},
		APITokens:       &APITokensHandler{},
		Sessions:        &SessionsHandler{},
		Passkeys:        &PasskeysHandler{},
		Backup:          &BackupHandler{},
	})
	return app
}

func describedRoutes() []string {
	ops := Operations()
	out := make([]string, 0, len(ops))
	for _, op := range ops {
		out = append(out, op.Method+" "+op.Path)
	}
	sort.Strings(out)
	return out
}

func TestOperations_DescribeEveryMountedRoute(t *testing.T) {
	described := map[string]int{}
	for _, k := range describedRoutes() {
		described[k]++
	}
	mounted := map[string]bool{}
	for _, k := range mountedRoutes(t) {
		mounted[k] = true
		if described[k] == 0 {
			t.Errorf("route %s is mounted but has no entry in Operations()", k)
		}
	}
	for k, n := range described {
		if !mounted[k] {
			t.Errorf("Operations() describes %s, but no such route is mounted", k)
		}
		if n > 1 {
			t.Errorf("Operations() describes %s %d times", k, n)
		}
	}
}

func TestOperations_HaveSummaryAndTag(t *testing.T) {
	for _, op := range Operations() {
		if op.Summary == "" || op.Tag == "" {
			t.Errorf("%s %s: got summary %q, tag %q; want both set", op.Method, op.Path, op.Summary, op.Tag)
		}
	}
}

// TestOpenAPIDocument_IsUpToDate fails when the committed document no longer
// matches what the code describes. Regenerate it with `just openapi`.
func TestOpenAPIDocument_IsUpToDate(t *testing.T) {
	got, err := openapi.YAML(OpenAPIDocument(), Operations())
	if err != nil {
		t.Fatalf("build document: %v", err)
	}
	if *updateOpenAPI {
		if err := os.MkdirAll(filepath.Dir(openAPIFile), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(openAPIFile, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(openAPIFile)
	if err != nil {
		t.Fatalf("read %s: %v (run `just openapi`)", openAPIFile, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s is out of date with the API; run `just openapi` and commit the result", openAPIFile)
	}
}

// The document is public: it is mounted ahead of the /api/v1 group, whose
// middleware (here built on nil repositories) must never run for it.
func TestOpenAPIEndpoint_ServesDocumentWithoutAuth(t *testing.T) {
	app := mountedApp()
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, OpenAPIPath, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	var doc struct {
		OpenAPI string         `json:"openapi"`
		Paths   map[string]any `json:"paths"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if doc.OpenAPI != "3.1.0" {
		t.Errorf("openapi: got %q, want 3.1.0", doc.OpenAPI)
	}
	if _, ok := doc.Paths["/api/v1/tasks/{id}"]; !ok {
		t.Errorf("paths: missing /api/v1/tasks/{id}")
	}
}

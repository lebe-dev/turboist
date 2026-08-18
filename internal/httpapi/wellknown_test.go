package httpapi_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
)

func newWellKnownApp(t *testing.T, dir string) *fiber.App {
	t.Helper()
	app := httpapi.NewApp(httpapi.Deps{})
	httpapi.RegisterWellKnown(app, dir, slog.Default())
	return app
}

func TestRegisterWellKnown_ServesFileAsJSON(t *testing.T) {
	dir := t.TempDir()
	// The Apple association file has no extension, which is exactly why the
	// handler sets the content type itself.
	if err := os.WriteFile(filepath.Join(dir, "apple-app-site-association"),
		[]byte(`{"webcredentials":{"apps":["TEAM.app"]}}`), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	app := newWellKnownApp(t, dir)

	resp := doRequest(t, app, httptest.NewRequest(http.MethodGet, "/.well-known/apple-app-site-association", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != fiber.MIMEApplicationJSON {
		t.Errorf("content type: got %q, want %q", ct, fiber.MIMEApplicationJSON)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	_ = resp.Body.Close()
	if string(body) != `{"webcredentials":{"apps":["TEAM.app"]}}` {
		t.Errorf("body: got %s, want the file contents verbatim", body)
	}
}

func TestRegisterWellKnown_MissingFile(t *testing.T) {
	app := newWellKnownApp(t, t.TempDir())

	resp := doRequest(t, app, httptest.NewRequest(http.MethodGet, "/.well-known/assetlinks.json", nil))
	if resp.StatusCode != 404 {
		t.Errorf("status: got %d, want 404", resp.StatusCode)
	}
}

// A file name is never a path: anything that could climb out of the configured
// directory must be refused rather than read.
func TestRegisterWellKnown_RejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Dir(dir)
	if err := os.WriteFile(filepath.Join(parent, "secret.json"), []byte("nope"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	app := newWellKnownApp(t, dir)

	resp := doRequest(t, app, httptest.NewRequest(http.MethodGet, "/.well-known/..%2fsecret.json", nil))
	if resp.StatusCode != 404 {
		t.Errorf("status: got %d, want 404", resp.StatusCode)
	}
}

func TestRegisterWellKnown_UnsetLeavesRouteUnmounted(t *testing.T) {
	app := newWellKnownApp(t, "")

	resp := doRequest(t, app, httptest.NewRequest(http.MethodGet, "/.well-known/assetlinks.json", nil))
	if resp.StatusCode != 404 {
		t.Errorf("status: got %d, want 404", resp.StatusCode)
	}
}

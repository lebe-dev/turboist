package httpapi_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/gofiber/fiber/v3"

	"github.com/lebe-dev/turboist/internal/httpapi"
)

const spaIndexHTML = "<!doctype html><html><body><div id=\"app\">turboist-spa</div></body></html>"

// setupSPAApp builds a Fiber app with the federation discovery + signed routes
// wired the same way main.go wires them, then mounts the SPA fallback from a
// synthetic in-memory build dir. It returns the app for app.Test requests.
func setupSPAApp(t *testing.T) *fiber.App {
	t.Helper()

	app := httpapi.NewApp(httpapi.Deps{})

	// Stand-ins for the real federation JSON routes. They must keep returning
	// their JSON envelope (NOT the SPA index.html) — F2.1 pins /federation/join
	// as the only browser-facing federation route.
	app.Get("/federation/.well-known/instance", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"instance_url": "https://me.example"})
	})
	app.Post("/federation/handshake", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"ok": true})
	})

	buildFS := fstest.MapFS{
		"frontend/build/index.html": {Data: []byte(spaIndexHTML)},
	}
	if err := httpapi.RegisterSPA(app, buildFS, "frontend/build"); err != nil {
		t.Fatalf("register spa: %v", err)
	}
	return app
}

func doGET(t *testing.T, app *fiber.App, path string) (*http.Response, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, http.NoBody)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("test %s: %v", path, err)
	}
	body, _ := io.ReadAll(resp.Body)
	return resp, string(body)
}

// TestSPA_FederationJoinServesIndex asserts the browser-facing /federation/join
// route falls through to the SPA shell (index.html), so the SvelteKit join page
// renders client-side (Federation v1 F2.1 — "/join serves SPA").
func TestSPA_FederationJoinServesIndex(t *testing.T) {
	app := setupSPAApp(t)

	resp, body := doGET(t, app, "/federation/join")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/federation/join status: got %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get(fiber.HeaderContentType); ct != fiber.MIMETextHTMLCharsetUTF8 {
		t.Errorf("/federation/join content-type: got %q, want html", ct)
	}
	if body != spaIndexHTML {
		t.Errorf("/federation/join body: got %q, want the SPA index", body)
	}
}

// TestSPA_FederationJoinSubpathServesIndex asserts deeper client-side routes
// under /federation/join/* also serve the SPA shell (SvelteKit may add nested
// segments) while still NOT colliding with signed API routes.
func TestSPA_FederationJoinSubpathServesIndex(t *testing.T) {
	app := setupSPAApp(t)

	resp, body := doGET(t, app, "/federation/join/accept")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/federation/join/accept status: got %d, want 200", resp.StatusCode)
	}
	if body != spaIndexHTML {
		t.Errorf("/federation/join/accept body: got %q, want the SPA index", body)
	}
}

// TestSPA_FederationAPIPathsDoNotServeIndex asserts the signed/discovery
// federation routes return their JSON envelope and are NOT swallowed by the SPA
// fallback (Federation v1 F2.1 — "API paths don't" serve SPA).
func TestSPA_FederationAPIPathsDoNotServeIndex(t *testing.T) {
	app := setupSPAApp(t)

	for _, path := range []string{"/federation/.well-known/instance"} {
		resp, body := doGET(t, app, path)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s status: got %d, want 200", path, resp.StatusCode)
		}
		if body == spaIndexHTML {
			t.Errorf("%s served the SPA index but must return JSON", path)
		}
		if ct := resp.Header.Get(fiber.HeaderContentType); ct == fiber.MIMETextHTMLCharsetUTF8 {
			t.Errorf("%s content-type is html; want json", path)
		}
	}
}

// TestSPA_UnknownFederationAPIPathReturnsJSON404 asserts a federation route that
// is not /federation/join (and is unmounted) returns the router's JSON 404
// envelope rather than the SPA shell — proving the carve-out in
// isFederationAPIPath treats everything-but-join as a JSON API surface.
func TestSPA_UnknownFederationAPIPathReturnsJSON404(t *testing.T) {
	app := setupSPAApp(t)

	resp, body := doGET(t, app, "/federation/events")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("/federation/events status: got %d, want 404", resp.StatusCode)
	}
	if body == spaIndexHTML {
		t.Error("/federation/events served the SPA index but must return a JSON 404")
	}
}

// TestSPA_APIPrefixesPassThrough is a regression guard that the pre-existing
// /api/ and /auth/ carve-outs still pass through to the router (so the SPA does
// not regress the JSON API surface while F2.1 adds the federation carve-out).
func TestSPA_APIPrefixesPassThrough(t *testing.T) {
	app := setupSPAApp(t)

	for _, path := range []string{"/api/v1/config", "/auth/login"} {
		resp, body := doGET(t, app, path)
		if body == spaIndexHTML {
			t.Errorf("%s served the SPA index but must pass through to the router", path)
		}
		if resp.StatusCode == http.StatusOK {
			t.Errorf("%s unexpectedly 200 from SPA; want a router response", path)
		}
	}
}

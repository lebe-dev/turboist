package httpapi_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/gofiber/fiber/v3"

	"github.com/lebe-dev/turboist/internal/federation/nonce"
	"github.com/lebe-dev/turboist/internal/federation/peerkeys"
	"github.com/lebe-dev/turboist/internal/httpapi"
)

const spaIndexHTML = "<!doctype html><html><body><div id=\"app\">turboist-spa</div></body></html>"

// setupSPAApp builds a Fiber app with the federation discovery + signed routes
// wired the same way main.go wires them, then mounts the SPA fallback from a
// synthetic in-memory build dir. It returns the app for app.Test requests.
func setupSPAApp(t *testing.T) *fiber.App {
	t.Helper()

	app := httpapi.NewApp(httpapi.Deps{})

	app.Get("/federation/.well-known/instance", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"instance_url": "https://me.example"})
	})

	// Wire the signed federation routes the way main.go does: a /federation group
	// carrying HTTPSignatureMiddleware on the PREFIX. This is load-bearing for the
	// /federation/join carve-out — the prefix-scoped middleware must not fire on
	// the browser-facing join navigation (it would otherwise shadow the SPA shell
	// with federation_signature_invalid before the fallback runs).
	noFetch := func(context.Context, string) (*peerkeys.Instance, error) { return nil, context.Canceled }
	signed := app.Group("/federation", httpapi.HTTPSignatureMiddleware(httpapi.FederationSignatureDeps{
		Nonces:   nonce.NewCache(),
		PeerKeys: peerkeys.NewCache(noFetch),
	}))
	signed.Post("/handshake", func(c fiber.Ctx) error {
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

// TestSPA_SignedFederationPathDoesNotServeIndex asserts a federation route that
// is not /federation/join is treated as a signed server-to-server API surface,
// NOT the SPA shell. Under the production wiring the /federation-prefixed
// HTTPSignatureMiddleware fires first and rejects the unsigned request with the
// federation_signature_invalid JSON envelope (401) — proving the join carve-out
// in isFederationAPIPath does NOT leak to its siblings.
func TestSPA_SignedFederationPathDoesNotServeIndex(t *testing.T) {
	app := setupSPAApp(t)

	resp, body := doGET(t, app, "/federation/events")
	if body == spaIndexHTML {
		t.Error("/federation/events served the SPA index but must stay a signed JSON route")
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("/federation/events status: got %d, want 401 (missing signature)", resp.StatusCode)
	}
	if ct := resp.Header.Get(fiber.HeaderContentType); ct == fiber.MIMETextHTMLCharsetUTF8 {
		t.Errorf("/federation/events content-type is html; want json")
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

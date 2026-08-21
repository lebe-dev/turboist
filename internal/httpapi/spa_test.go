package httpapi

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/gofiber/fiber/v3"
)

func TestCacheControlFor_ImmutableBundle(t *testing.T) {
	for _, p := range []string{
		"/_app/immutable/entry/app.D58mlDSu.js",
		"/_app/immutable/assets/0.Yvav3x71.css",
	} {
		if got, want := cacheControlFor(p), cacheImmutable; got != want {
			t.Errorf("%s: got %q, want %q", p, got, want)
		}
	}
}

func TestCacheControlFor_RevalidatedEntryPoints(t *testing.T) {
	// The SPA shell, the deploy stamp and the SW script must never be served
	// from the browser's HTTP cache without a revalidation round-trip.
	for _, p := range []string{"/", "/index.html", "/service-worker.js", "/_app/version.json"} {
		if got, want := cacheControlFor(p), cacheRevalidate; got != want {
			t.Errorf("%s: got %q, want %q", p, got, want)
		}
	}
}

func TestCacheControlFor_UnhashedAssets(t *testing.T) {
	for _, p := range []string{"/robots.txt", "/manifest.webmanifest", "/icons/icon-192.png"} {
		if got, want := cacheControlFor(p), cacheShort; got != want {
			t.Errorf("%s: got %q, want %q", p, got, want)
		}
	}
}

func spaTestApp(t *testing.T) *fiber.App {
	t.Helper()
	build := fstest.MapFS{
		"frontend/build/index.html":                         {Data: []byte("<html></html>")},
		"frontend/build/service-worker.js":                  {Data: []byte("//sw")},
		"frontend/build/robots.txt":                         {Data: []byte("User-agent: *")},
		"frontend/build/_app/version.json":                  {Data: []byte(`{"version":"1"}`)},
		"frontend/build/_app/immutable/entry/app.abc123.js": {Data: []byte("//app")},
	}
	app := fiber.New()
	app.Get("/healthz", func(c fiber.Ctx) error { return c.SendString("ok") })
	if err := RegisterSPA(app, fs.FS(build), "frontend/build"); err != nil {
		t.Fatal(err)
	}
	return app
}

func spaGet(t *testing.T, app *fiber.App, path string) *http.Response {
	t.Helper()
	res, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	return res
}

func TestRegisterSPA_CacheControlHeaders(t *testing.T) {
	app := spaTestApp(t)

	cases := []struct {
		path string
		want string
	}{
		{"/", cacheRevalidate},
		{"/index.html", cacheRevalidate},
		{"/service-worker.js", cacheRevalidate},
		{"/_app/version.json", cacheRevalidate},
		{"/_app/immutable/entry/app.abc123.js", cacheImmutable},
		{"/robots.txt", cacheShort},
		// Client-side route -> index.html fallback, must revalidate as well.
		{"/today", cacheRevalidate},
	}
	for _, tc := range cases {
		res := spaGet(t, app, tc.path)
		if res.StatusCode != http.StatusOK {
			t.Errorf("%s: status got %d, want 200", tc.path, res.StatusCode)
			continue
		}
		if got := res.Header.Get(fiber.HeaderCacheControl); got != tc.want {
			t.Errorf("%s: Cache-Control got %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestRegisterSPA_PassesThroughReservedPrefixes(t *testing.T) {
	app := spaTestApp(t)
	res := spaGet(t, app, "/healthz")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status got %d, want 200", res.StatusCode)
	}
	if got := res.Header.Get(fiber.HeaderCacheControl); got != "" {
		t.Errorf("Cache-Control got %q, want empty", got)
	}
}

// The zero-ModTime trap: embed.FS (and fstest.MapFS) report a zero ModTime for
// every file, so a browser that cached the entry document during a previous
// deploy revalidates with `If-Modified-Since: Mon, 01 Jan 0001 00:00:00 GMT` —
// which matches forever. The static handler must not answer that with a 304, or
// the tab keeps booting the previous deploy's index.html.
const staleIfModifiedSince = "Mon, 01 Jan 0001 00:00:00 GMT"

func spaGetIfModifiedSince(t *testing.T, app *fiber.App, path string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(fiber.HeaderIfModifiedSince, staleIfModifiedSince)
	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	return res
}

func TestRegisterSPA_StaleValidatorStillServesFreshFile(t *testing.T) {
	app := spaTestApp(t)

	for _, path := range []string{"/", "/index.html", "/service-worker.js", "/_app/version.json", "/robots.txt"} {
		res := spaGetIfModifiedSince(t, app, path)
		if res.StatusCode != http.StatusOK {
			t.Errorf("%s: status got %d, want 200", path, res.StatusCode)
			continue
		}
		body, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		if len(body) == 0 {
			t.Errorf("%s: empty body", path)
		}
	}
}

func TestRegisterSPA_NoLastModifiedOnMutableFiles(t *testing.T) {
	app := spaTestApp(t)

	for _, path := range []string{"/", "/index.html", "/service-worker.js", "/_app/version.json", "/robots.txt", "/today"} {
		res := spaGet(t, app, path)
		if got := res.Header.Get(fiber.HeaderLastModified); got != "" {
			t.Errorf("%s: Last-Modified got %q, want empty", path, got)
		}
	}
}

func TestRegisterSPA_ImmutableBundleKeepsConditionalRevalidation(t *testing.T) {
	// Content-hashed URLs change with their content, so a 304 there is always
	// correct — and saves the round-trip's body.
	app := spaTestApp(t)
	res := spaGetIfModifiedSince(t, app, "/_app/immutable/entry/app.abc123.js")
	if res.StatusCode != http.StatusNotModified {
		t.Errorf("status got %d, want 304", res.StatusCode)
	}
}

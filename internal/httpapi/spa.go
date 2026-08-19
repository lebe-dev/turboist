package httpapi

import (
	"io/fs"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/static"
)

// immutablePrefix is SvelteKit's content-hashed bundle directory: every file
// under it is uniquely named by its content hash, so it can be cached forever.
const immutablePrefix = "/_app/immutable/"

const (
	cacheImmutable = "public, max-age=31536000, immutable"
	// The SPA entry document, the deploy stamp SvelteKit polls and the service
	// worker script MUST be revalidated on every request. Serving a stale
	// index.html boots the browser into the previous deploy's (now missing)
	// chunk hashes while `/_app/version.json` already reports the new version:
	// `updated.current` latches true, the "new version available" toast fires
	// every poll and `beforeNavigate` turns every navigation into a full page
	// reload — which re-reads the same cached HTML and never converges.
	cacheRevalidate = "no-cache"
	// Unhashed static assets (PWA icons, robots.txt, the manifest): stale for a
	// while is harmless, and they are fetched rarely.
	cacheShort = "public, max-age=3600"
)

// cacheControlFor picks the Cache-Control value for a static file served out of
// the embedded SvelteKit build, keyed on the request path.
func cacheControlFor(path string) string {
	if strings.HasPrefix(path, immutablePrefix) {
		return cacheImmutable
	}
	switch path {
	case "/", "/index.html", "/service-worker.js", "/_app/version.json":
		return cacheRevalidate
	}
	return cacheShort
}

// RegisterSPA mounts the embedded SvelteKit build at "/" with index.html
// fallback for client-side routes. Must be called after all API/auth routes
// so they are matched first; this handler only fires for unmatched paths.
//
// Requests under /api/, /auth/, /healthz, /version are passed through so the
// router returns its normal JSON 404 envelope instead of index.html.
func RegisterSPA(app *fiber.App, embeddedFS fs.FS, buildDir string) error {
	sub, err := fs.Sub(embeddedFS, buildDir)
	if err != nil {
		return err
	}

	indexBytes, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		return err
	}

	serveIndex := func(c fiber.Ctx) error {
		c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
		c.Set(fiber.HeaderCacheControl, cacheRevalidate)
		return c.Status(fiber.StatusOK).Send(indexBytes)
	}

	app.Use(static.New("", static.Config{
		FS:         sub,
		IndexNames: []string{"index.html"},
		// MaxAge would paint one blanket `public, max-age=N` over every file,
		// including index.html; Cache-Control is set per path in ModifyResponse.
		MaxAge: 0,
		ModifyResponse: func(c fiber.Ctx) error {
			c.Set(fiber.HeaderCacheControl, cacheControlFor(c.Path()))
			return nil
		},
		Next: func(c fiber.Ctx) bool {
			p := c.Path()
			return strings.HasPrefix(p, "/api/") ||
				strings.HasPrefix(p, "/auth/") ||
				p == "/healthz" ||
				p == "/version"
		},
		NotFoundHandler: serveIndex,
	}))

	return nil
}

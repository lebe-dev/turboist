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

// isImmutable reports whether a request path addresses a content-hashed bundle
// file, i.e. one whose URL changes whenever its bytes change.
func isImmutable(path string) bool {
	return strings.HasPrefix(path, immutablePrefix)
}

// cacheControlFor picks the Cache-Control value for a static file served out of
// the embedded SvelteKit build, keyed on the request path.
func cacheControlFor(path string) string {
	if isImmutable(path) {
		return cacheImmutable
	}
	switch path {
	case "/", "/index.html", "/service-worker.js", "/_app/version.json":
		return cacheRevalidate
	}
	return cacheShort
}

// isReservedPath reports whether a path belongs to the API surface rather than
// to the embedded SPA, so the router answers with its JSON 404 envelope instead
// of index.html.
func isReservedPath(path string) bool {
	return strings.HasPrefix(path, "/api/") ||
		strings.HasPrefix(path, "/auth/") ||
		path == "/healthz" ||
		path == "/version"
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

	// Every file in the embedded build carries the same zero ModTime
	// (embed.FS has no timestamps), which the static handler still advertises as
	// `Last-Modified: Mon, 01 Jan 0001 00:00:00 GMT`. A browser that stored the
	// entry document during an earlier deploy therefore revalidates with an
	// `If-Modified-Since` that matches the *new* deploy just as well, gets a
	// `304 Not Modified` and keeps booting the previous bundle — while
	// `/_app/version.json`, which SvelteKit polls with `cache-control: no-cache`,
	// already reports the new version. That is what made the "new version
	// available" toast come back on every launch with "Reload" unable to clear it
	// (a plain reload revalidates, so it hit the same 304).
	//
	// Mutable files therefore carry no validator at all: the request header is
	// dropped before the static handler can act on it — which is also what lets
	// already-poisoned browsers recover without a hard reload — and
	// `Last-Modified` is stripped from the response so nothing new is stored.
	// Content-hashed files under /_app/immutable/ keep theirs: their URL changes
	// with their content, so a 304 there is always correct.
	app.Use(func(c fiber.Ctx) error {
		if !isReservedPath(c.Path()) && !isImmutable(c.Path()) {
			c.Request().Header.Del(fiber.HeaderIfModifiedSince)
		}
		return c.Next()
	})

	app.Use(static.New("", static.Config{
		FS:         sub,
		IndexNames: []string{"index.html"},
		// MaxAge would paint one blanket `public, max-age=N` over every file,
		// including index.html; Cache-Control is set per path in ModifyResponse.
		MaxAge: 0,
		ModifyResponse: func(c fiber.Ctx) error {
			c.Set(fiber.HeaderCacheControl, cacheControlFor(c.Path()))
			if !isImmutable(c.Path()) {
				c.Response().Header.Del(fiber.HeaderLastModified)
			}
			return nil
		},
		Next: func(c fiber.Ctx) bool {
			return isReservedPath(c.Path())
		},
		NotFoundHandler: serveIndex,
	}))

	return nil
}

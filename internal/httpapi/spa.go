package httpapi

import (
	"io/fs"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/static"
)

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
		c.Set(fiber.HeaderCacheControl, "no-cache")
		return c.Status(fiber.StatusOK).Send(indexBytes)
	}

	app.Use(static.New("", static.Config{
		FS:         sub,
		IndexNames: []string{"index.html"},
		MaxAge:     3600,
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

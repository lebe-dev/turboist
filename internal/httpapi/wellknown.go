package httpapi

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/logging"
)

// RegisterWellKnown serves files from dir under /.well-known/.
//
// It exists for the native passkey flow: iOS reads
// `/.well-known/apple-app-site-association` and Android reads
// `/.well-known/assetlinks.json` from the Relying Party's domain to confirm the
// app may use the domain's credentials. Both files are deployment-specific
// (they carry the Apple team id and the APK signing fingerprints), so they are
// read from disk rather than embedded, and the route is only mounted when
// WELL_KNOWN_PATH points at a directory. A reverse proxy serving the same paths
// works just as well — this is the batteries-included option.
//
// Must be registered before RegisterSPA, whose catch-all would otherwise answer
// with index.html.
//
// Both the mount and every miss are logged: the platforms report a failed
// association check as a generic "RP ID cannot be validated" on the device, with
// nothing to say whether the file was missing, unreadable, or never mounted —
// so the server has to be the one that says it.
func RegisterWellKnown(app *fiber.App, dir string, log *slog.Logger) {
	if dir == "" {
		return
	}
	if log == nil {
		log = slog.Default()
	}
	logMountState(dir, log)

	app.Get("/.well-known/:file", func(c fiber.Ctx) error {
		ctx := c.Context()
		name := c.Params("file")
		// Path traversal guard: only a plain file name may be requested.
		if name == "" || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
			return ErrNotFound("not found")
		}
		path := filepath.Join(dir, name)
		body, err := os.ReadFile(path)
		if err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "well-known file unavailable",
				slog.String("op", "httpapi.WellKnown"),
				slog.String("path", path),
				slog.String("err", err.Error()),
			)
			return ErrNotFound("not found")
		}
		// apple-app-site-association must be served as JSON despite having no
		// extension, so the content type is set explicitly for both files.
		c.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
		return c.Send(body)
	})
}

// logMountState reports at startup what the configured directory actually holds,
// which turns "the association file 404s" into a one-line answer instead of a
// round-trip through the device.
func logMountState(dir string, log *slog.Logger) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Warn("well-known directory unreadable — association files will 404",
			"path", dir, "err", err.Error())
		return
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		log.Warn("well-known directory is empty — association files will 404", "path", dir)
		return
	}
	log.Info("well-known files served", "path", dir, "files", names)
}

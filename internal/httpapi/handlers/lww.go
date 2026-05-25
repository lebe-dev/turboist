package handlers

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/model"
)

// readBaseUpdatedAt extracts the client's expected updated_at for a PATCH
// request. It first checks the bodyBase field (typically `baseUpdatedAt`) and
// falls back to the `If-Unmodified-Since` header. Returns nil when the client
// did not opt into LWW conflict detection.
func readBaseUpdatedAt(c fiber.Ctx, bodyBase *string) (*time.Time, *httpapi.AppError) {
	raw := ""
	if bodyBase != nil && *bodyBase != "" {
		raw = *bodyBase
	} else if h := c.Get("If-Unmodified-Since"); h != "" {
		raw = h
	}
	if raw == "" {
		return nil, nil
	}
	ts, err := model.ParseUTC(raw)
	if err != nil {
		return nil, httpapi.ErrValidation("invalid baseUpdatedAt")
	}
	return &ts, nil
}

// isStalePatch reports whether the server's version is newer than the client's
// base. When true, the PATCH must be silently ignored (LWW) and the caller is
// expected to return the current server state instead.
func isStalePatch(base *time.Time, serverUpdatedAt time.Time) bool {
	return base != nil && serverUpdatedAt.After(*base)
}

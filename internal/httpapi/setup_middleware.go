package httpapi

import (
	"sync/atomic"

	"github.com/gofiber/fiber/v3"

	"github.com/lebe-dev/turboist/internal/repo"
)

// SetupCheckMiddleware short-circuits every request with 503 setup_required
// when the single-user instance has no admin user yet. It is wired on the
// /api/v1 group before auth runs, so unauthenticated callers can probe
// /api/v1/config to discover the setup state without a separate endpoint.
//
// /auth/setup is mounted outside this group and stays reachable so the
// frontend can finish the setup flow even while the rest of the API is
// blocked.
//
// Once a user exists the flag is latched in memory — subsequent requests skip
// the DB round-trip. The flag never flips back to false at runtime (the only
// way to clear users is an offline DB rewrite), so caching is safe.
func SetupCheckMiddleware(users *repo.UserRepo) fiber.Handler {
	// When users is nil (some narrow middleware tests that do not need setup
	// gating), the middleware becomes a no-op so callers do not have to wire
	// a UserRepo just to mount the API group.
	if users == nil {
		return func(c fiber.Ctx) error { return c.Next() }
	}
	var done atomic.Bool
	return func(c fiber.Ctx) error {
		if done.Load() {
			return c.Next()
		}
		exists, err := users.Exists(c.Context())
		if err != nil {
			return ErrInternal("check setup").WithCause(err)
		}
		if !exists {
			return ErrSetupRequired()
		}
		done.Store(true)
		return c.Next()
	}
}

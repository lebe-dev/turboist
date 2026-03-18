package auth

import (
	"strings"

	"github.com/charmbracelet/log"
	"github.com/gofiber/fiber/v3"
)

const cookieName = "turboist_token"

var skipPaths = []string{
	"/api/auth/login",
	"/api/health",
	"/api/ws",
}

func NewMiddleware(store *SessionStore) fiber.Handler {
	return func(c fiber.Ctx) error {
		path := c.Path()

		// Only protect API routes; let frontend static files through
		if !strings.HasPrefix(path, "/api/") {
			return c.Next()
		}

		for _, skip := range skipPaths {
			if strings.HasPrefix(path, skip) {
				return c.Next()
			}
		}

		token := c.Cookies(cookieName)
		if token == "" || !store.ValidateSession(token) {
			log.Debug("unauthorized request", "path", path, "ip", c.IP())
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
		}

		return c.Next()
	}
}

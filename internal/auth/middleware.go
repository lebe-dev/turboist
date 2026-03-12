package auth

import (
	"strings"

	"github.com/gofiber/fiber/v3"
)

const cookieName = "turboist_token"

var skipPaths = []string{
	"/api/auth/login",
	"/api/health",
}

func NewMiddleware(store *SessionStore) fiber.Handler {
	return func(c fiber.Ctx) error {
		path := c.Path()
		for _, skip := range skipPaths {
			if strings.HasPrefix(path, skip) {
				return c.Next()
			}
		}

		token := c.Cookies(cookieName)
		if token == "" || !store.ValidateSession(token) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
		}

		return c.Next()
	}
}

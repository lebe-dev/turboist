package httpapi

import (
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/lebe-dev/turboist/internal/auth"
)

const (
	localsClaimsKey    = "auth_claims"
	localsRequestIDKey = "request_id"
)

// RequestIDMiddleware propagates or generates an X-Request-ID header.
func RequestIDMiddleware() fiber.Handler {
	return func(c fiber.Ctx) error {
		id := c.Get("X-Request-ID")
		if id == "" {
			id = uuid.New().String()
		}
		c.Set("X-Request-ID", id)
		c.Locals(localsRequestIDKey, id)
		return c.Next()
	}
}

// AccessLogMiddleware logs each request with method, path, status, and duration.
func AccessLogMiddleware(log *slog.Logger) fiber.Handler {
	return func(c fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		rid, _ := c.Locals(localsRequestIDKey).(string)
		log.Info("request",
			slog.String("method", c.Method()),
			slog.String("path", c.Path()),
			slog.Int("status", c.Response().StatusCode()),
			slog.Duration("duration", time.Since(start)),
			slog.String("request_id", rid),
		)
		return err
	}
}

// AuthMiddleware validates the Bearer token and stores claims in Locals.
func AuthMiddleware(issuer *auth.JWTIssuer) fiber.Handler {
	return func(c fiber.Ctx) error {
		header := c.Get("Authorization")
		if header == "" {
			return ErrAuthInvalid("missing authorization header")
		}
		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			return ErrAuthInvalid("invalid authorization header format")
		}
		claims, err := issuer.Verify(parts[1])
		if err != nil {
			if errors.Is(err, auth.ErrTokenExpired) {
				return ErrAuthExpired()
			}
			return ErrAuthInvalid("invalid token")
		}
		c.Locals(localsClaimsKey, claims)
		return c.Next()
	}
}

// GetClaims retrieves the auth claims from the request context.
// Returns nil if AuthMiddleware was not applied or token was invalid.
func GetClaims(c fiber.Ctx) *auth.Claims {
	v, _ := c.Locals(localsClaimsKey).(*auth.Claims)
	return v
}

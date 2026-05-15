package httpapi

import (
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/repo"
)

const (
	localsClaimsKey     = "auth_claims"
	localsRequestIDKey  = "request_id"
	localsAuthMethodKey = "auth_method"
	localsUserIDKey     = "auth_user_id"

	AuthMethodJWT      = "jwt"
	AuthMethodAPIToken = "api_token"
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

// AuthMiddleware validates the Bearer JWT token and stores claims in Locals.
// JWT-only — does not accept API tokens. Used by /auth/* protected routes.
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
		c.Locals(localsAuthMethodKey, AuthMethodJWT)
		c.Locals(localsUserIDKey, claims.UserID)
		return c.Next()
	}
}

// APIAuthMiddleware validates the Bearer token, accepting either a JWT access
// token or a long-lived API token (HMAC-hashed lookup). On success it stores
// the resolved user id and auth method in Locals so downstream handlers can
// read them via GetUserID and require a specific method via RequireJWTAuth.
func APIAuthMiddleware(issuer *auth.JWTIssuer, apiTokens *repo.APITokenRepo, salt []byte) fiber.Handler {
	return func(c fiber.Ctx) error {
		header := c.Get("Authorization")
		if header == "" {
			return ErrAuthInvalid("missing authorization header")
		}
		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			return ErrAuthInvalid("invalid authorization header format")
		}
		token := parts[1]

		claims, jwtErr := issuer.Verify(token)
		if jwtErr == nil {
			c.Locals(localsClaimsKey, claims)
			c.Locals(localsAuthMethodKey, AuthMethodJWT)
			c.Locals(localsUserIDKey, claims.UserID)
			return c.Next()
		}
		if errors.Is(jwtErr, auth.ErrTokenExpired) {
			return ErrAuthExpired()
		}

		hash := auth.HashAPIToken(token, salt)
		apiToken, err := apiTokens.GetByTokenHash(c.Context(), hash)
		if err != nil {
			return ErrAuthInvalid("invalid token")
		}
		c.Locals(localsAuthMethodKey, AuthMethodAPIToken)
		c.Locals(localsUserIDKey, apiToken.UserID)
		return c.Next()
	}
}

// RequireJWTAuth rejects requests authenticated via API token. Use it on
// subgroups (e.g. /api/v1/api-tokens) that must require a user session.
func RequireJWTAuth() fiber.Handler {
	return func(c fiber.Ctx) error {
		method, _ := c.Locals(localsAuthMethodKey).(string)
		if method != AuthMethodJWT {
			return ErrAuthInvalid("session required")
		}
		return c.Next()
	}
}

// GetClaims retrieves the JWT auth claims from the request context.
// Returns nil for API-token-authenticated requests or when no auth middleware ran.
func GetClaims(c fiber.Ctx) *auth.Claims {
	v, _ := c.Locals(localsClaimsKey).(*auth.Claims)
	return v
}

// GetUserID returns the authenticated user id regardless of auth method.
// Returns 0 if no auth middleware ran.
func GetUserID(c fiber.Ctx) int64 {
	if v, ok := c.Locals(localsUserIDKey).(int64); ok {
		return v
	}
	return 0
}

// GetAuthMethod returns AuthMethodJWT, AuthMethodAPIToken, or "" if no auth ran.
func GetAuthMethod(c fiber.Ctx) string {
	v, _ := c.Locals(localsAuthMethodKey).(string)
	return v
}

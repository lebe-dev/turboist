package httpapi

import (
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/repo"
)

const (
	localsClaimsKey      = "auth_claims"
	localsRequestIDKey   = "request_id"
	localsAuthMethodKey  = "auth_method"
	localsUserIDKey      = "auth_user_id"
	localsTokenScopesKey = "token_scopes"

	AuthMethodJWT      = "jwt"
	AuthMethodAPIToken = "api_token"
)

// RequestIDMiddleware propagates or generates an X-Request-ID header and
// attaches a request-scoped logger (carrying request_id) to c.Context() so
// downstream layers can retrieve it via logging.FromContext.
func RequestIDMiddleware(log *slog.Logger) fiber.Handler {
	return func(c fiber.Ctx) error {
		id := c.Get("X-Request-ID")
		if id == "" {
			id = uuid.New().String()
		}
		c.Set("X-Request-ID", id)
		c.Locals(localsRequestIDKey, id)

		base := log
		if base == nil {
			base = slog.Default()
		}
		reqLog := base.With(slog.String("request_id", id))
		c.SetContext(logging.WithLogger(c.Context(), reqLog))
		return c.Next()
	}
}

// AccessLogMiddleware logs each request with method, path, status, and duration.
// Level depends on status: DEBUG for <400, INFO for 4xx, WARN for 5xx.
func AccessLogMiddleware(log *slog.Logger) fiber.Handler {
	return func(c fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		status := c.Response().StatusCode()
		rid, _ := c.Locals(localsRequestIDKey).(string)
		userID := GetUserID(c)

		attrs := []any{
			slog.String("method", c.Method()),
			slog.String("path", c.Path()),
			slog.Int("status", status),
			slog.Duration("duration", time.Since(start)),
			slog.String("request_id", rid),
<<<<<<< HEAD
		}
		// Reading Response.Body() on a streaming response (e.g. SSE) drains
		// the stream synchronously and blocks until it ends — for long-lived
		// streams that is effectively forever. Skip the byte count in that
		// case so the middleware can return and fasthttp can start streaming.
		if !c.Response().IsBodyStream() {
			attrs = append(attrs, slog.Int("bytes_out", len(c.Response().Body())))
=======
			slog.Int("bytes_out", len(c.Response().Body())),
>>>>>>> 049dc85 (v1.6.0 (#32))
		}
		if userID != 0 {
			attrs = append(attrs, slog.Int64("user_id", userID))
		}

		level := slog.LevelDebug
		switch {
		case status >= 500:
			level = slog.LevelWarn
		case status >= 400:
			level = slog.LevelInfo
		}
		log.Log(c.Context(), level, "request", attrs...)
		return err
	}
}

// maskToken returns a short prefix of a bearer token suitable for logging.
// Never logs the full token. Returns "" for empty input. For tokens that are
// already short enough to be guessable verbatim, returns only the ellipsis so
// the caller never accidentally records the entire value.
func maskToken(t string) string {
	if t == "" {
		return ""
	}
	const n = 6
	if len(t) <= n {
		return "…"
	}
	return t[:n] + "…"
}

// enrichLogger replaces the request-scoped logger on c.Context() with one that
// carries the given attrs in addition to whatever the previous logger had.
func enrichLogger(c fiber.Ctx, attrs ...any) {
	log := logging.FromContext(c.Context()).With(attrs...)
	c.SetContext(logging.WithLogger(c.Context(), log))
}

// AuthMiddleware validates the Bearer JWT token and stores claims in Locals.
// JWT-only — does not accept API tokens. Used by /auth/* protected routes.
func AuthMiddleware(issuer *auth.JWTIssuer) fiber.Handler {
	return func(c fiber.Ctx) error {
		ctx := c.Context()
		header := c.Get("Authorization")
		if header == "" {
			logging.FromContext(ctx).WarnContext(ctx, "auth: missing authorization header", slog.String("op", "httpapi.AuthMiddleware"))
			return ErrAuthInvalid("missing authorization header")
		}
		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			logging.FromContext(ctx).WarnContext(ctx, "auth: bad authorization header format", slog.String("op", "httpapi.AuthMiddleware"))
			return ErrAuthInvalid("invalid authorization header format")
		}
		claims, err := issuer.Verify(parts[1])
		if err != nil {
			masked := maskToken(parts[1])
			if errors.Is(err, auth.ErrTokenExpired) {
				logging.FromContext(ctx).WarnContext(ctx, "auth: token expired",
					slog.String("op", "httpapi.AuthMiddleware"),
					slog.String("token_prefix", masked),
				)
				return ErrAuthExpired()
			}
			logging.FromContext(ctx).WarnContext(ctx, "auth: invalid token",
				slog.String("op", "httpapi.AuthMiddleware"),
				slog.String("token_prefix", masked),
			)
			return ErrAuthInvalid("invalid token")
		}
		c.Locals(localsClaimsKey, claims)
		c.Locals(localsAuthMethodKey, AuthMethodJWT)
		c.Locals(localsUserIDKey, claims.UserID)
		enrichLogger(c,
			slog.Int64("user_id", claims.UserID),
			slog.String("auth_method", AuthMethodJWT),
		)
		return c.Next()
	}
}

// APIAuthMiddleware validates the Bearer token, accepting either a JWT access
// token or a long-lived API token (HMAC-hashed lookup). On success it stores
// the resolved user id and auth method in Locals so downstream handlers can
// read them via GetUserID and require a specific method via RequireJWTAuth.
func APIAuthMiddleware(issuer *auth.JWTIssuer, apiTokens *repo.APITokenRepo, salt []byte) fiber.Handler {
	return func(c fiber.Ctx) error {
		ctx := c.Context()
		header := c.Get("Authorization")
		if header == "" {
			logging.FromContext(ctx).WarnContext(ctx, "auth: missing authorization header", slog.String("op", "httpapi.APIAuthMiddleware"))
			return ErrAuthInvalid("missing authorization header")
		}
		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			logging.FromContext(ctx).WarnContext(ctx, "auth: bad authorization header format", slog.String("op", "httpapi.APIAuthMiddleware"))
			return ErrAuthInvalid("invalid authorization header format")
		}
		token := parts[1]
		masked := maskToken(token)

		claims, jwtErr := issuer.Verify(token)
		if jwtErr == nil {
			c.Locals(localsClaimsKey, claims)
			c.Locals(localsAuthMethodKey, AuthMethodJWT)
			c.Locals(localsUserIDKey, claims.UserID)
			enrichLogger(c,
				slog.Int64("user_id", claims.UserID),
				slog.String("auth_method", AuthMethodJWT),
			)
			return c.Next()
		}
		if errors.Is(jwtErr, auth.ErrTokenExpired) {
			logging.FromContext(c.Context()).WarnContext(c.Context(), "auth: token expired",
				slog.String("op", "httpapi.APIAuthMiddleware"),
				slog.String("token_prefix", masked),
			)
			return ErrAuthExpired()
		}

		hash := auth.HashAPIToken(token, salt)
		apiToken, err := apiTokens.GetByTokenHash(c.Context(), hash)
		if err != nil {
			if !errors.Is(err, repo.ErrNotFound) {
				return ErrInternal("lookup api token").WithCause(err)
			}
			logging.FromContext(c.Context()).WarnContext(c.Context(), "auth: invalid token",
				slog.String("op", "httpapi.APIAuthMiddleware"),
				slog.String("token_prefix", masked),
			)
			return ErrAuthInvalid("invalid token")
		}
		c.Locals(localsAuthMethodKey, AuthMethodAPIToken)
		c.Locals(localsUserIDKey, apiToken.UserID)
<<<<<<< HEAD
		c.Locals(localsTokenScopesKey, apiToken.Scopes)
=======
>>>>>>> 049dc85 (v1.6.0 (#32))
		enrichLogger(c,
			slog.Int64("user_id", apiToken.UserID),
			slog.String("auth_method", AuthMethodAPIToken),
		)
		return c.Next()
	}
}

// RequireScope enforces a granular permission on the authenticated API token.
// JWT-authenticated requests always pass (full access). For API tokens the
// granted set is read from Locals and matched against the required scope via
// auth.HasScope (wildcard "*" satisfies any). On rejection the response stays
// generic ("insufficient scope") to avoid leaking the scope catalogue via
// enumeration; the actual required/granted pair is logged at WARN server-side.
func RequireScope(scope string) fiber.Handler {
	return func(c fiber.Ctx) error {
		if GetAuthMethod(c) == AuthMethodJWT {
			return c.Next()
		}
		scopes, _ := c.Locals(localsTokenScopesKey).([]string)
		if auth.HasScope(scopes, scope) {
			return c.Next()
		}
		ctx := c.Context()
		logging.FromContext(ctx).WarnContext(ctx, "scope check failed",
			slog.String("op", "httpapi.RequireScope"),
			slog.String("required", scope),
			slog.Any("granted", scopes),
		)
		return ErrForbidden("insufficient scope")
	}
}

// GetTokenScopes returns the scopes attached to the current API-token request,
// or nil for JWT-authenticated requests / when no auth middleware ran.
func GetTokenScopes(c fiber.Ctx) []string {
	scopes, _ := c.Locals(localsTokenScopesKey).([]string)
	return scopes
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

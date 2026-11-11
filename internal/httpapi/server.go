package httpapi

import (
	"errors"
	"log/slog"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
	"github.com/lebe-dev/turboist/internal/service/events"
)

// Deps holds all dependencies injected into the HTTP layer.
type Deps struct {
	Log          *slog.Logger
	JWTIssuer    *auth.JWTIssuer
	UserRepo     *repo.UserRepo
	SessionRepo  *repo.SessionRepo
	APITokenRepo *repo.APITokenRepo
	APITokenSalt []byte
	IPLimiter    *auth.IPLimiter
	ContextRepo  *repo.ContextRepo
	LabelRepo    *repo.LabelRepo
	SectionRepo  *repo.ProjectSectionRepo
	ProjectRepo  *repo.ProjectRepo
	TaskRepo     *repo.TaskRepo
	PinService   *service.PinService
	BackupSvc    *service.BackupService
	Cfg          *config.Config
	BaseURL      string
	Version      string
	// EventsHub is optional. When set, the API group emits coarse change
	// notifications (see PublishMiddleware) so SSE subscribers can invalidate
	// stale views without polling.
	EventsHub *events.Hub

	// PasskeyRepo is optional. When set, GET /api/config reports whether any
	// passkey is registered so the login page can hide a button that would open
	// an empty platform picker.
	PasskeyRepo *repo.WebAuthnRepo

	// IdempotencyRepo is optional. When set, the API group installs
	// IdempotencyMiddleware so mutating requests carrying an Idempotency-Key
	// can be safely retried (replayed) without re-executing the handler.
	IdempotencyRepo *repo.IdempotencyRepo

	// SentryEnabled gates the SentryMiddleware (400 + 5xx + panic reporting); set
	// when the backend DSN is configured. SentryFrontendDSN and SentryEnvironment
	// are served to the browser via GET /api/config so the SPA can initialise
	// Sentry at runtime without baking the DSN into the static bundle.
	SentryEnabled     bool
	SentryFrontendDSN string
	SentryEnvironment string
}

// ErrorEnvelope is the body of every failed request.
type ErrorEnvelope struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail carries a machine-readable code and a human-readable message.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// HealthResponse is the body of GET /healthz.
type HealthResponse struct {
	Status string `json:"status"`
}

// VersionResponse is the body of GET /version.
type VersionResponse struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"buildTime"`
}

// PublicConfigResponse is the body of GET /api/config: what the SPA needs
// before anybody has signed in.
type PublicConfigResponse struct {
	Sentry   PublicSentryConfig   `json:"sentry"`
	Passkeys PublicPasskeysConfig `json:"passkeys"`
}

// PublicSentryConfig tells the SPA how to start Sentry; a blank DSN leaves it off.
type PublicSentryConfig struct {
	DSN         string `json:"dsn"`
	Environment string `json:"environment"`
}

// PublicPasskeysConfig tells the login page whether to offer passkey sign-in.
type PublicPasskeysConfig struct {
	Enabled   bool `json:"enabled"`
	Available bool `json:"available"`
}

// NewApp creates a Fiber app with the custom error handler and standard middleware.
// BodyLimit is bumped above Fiber's 4 MiB default so the backup restore endpoint
// can accept payloads up to the cap enforced by handlers.BackupHandler. Smaller
// caps are still enforced per-handler at the application layer.
func NewApp(deps Deps) *fiber.App {
	app := fiber.New(fiber.Config{
		ErrorHandler: makeErrorHandler(deps.Log),
		BodyLimit:    64 * 1024 * 1024,
	})
	app.Use(recover.New())
	app.Use(RequestIDMiddleware(deps.Log))
	if deps.SentryEnabled {
		app.Use(SentryMiddleware())
	}
	if deps.Log != nil {
		app.Use(AccessLogMiddleware(deps.Log))
	}
	return app
}

// RegisterRoutes wires the public endpoints and returns the authenticated API
// group so callers (main.go, tests) can attach resource handlers without
// creating a circular import between httpapi and httpapi/handlers.
func RegisterRoutes(app *fiber.App, deps Deps) fiber.Router {
	app.Get("/healthz", func(c fiber.Ctx) error {
		return c.JSON(HealthResponse{Status: "ok"})
	})
	app.Get("/version", func(c fiber.Ctx) error {
		v := deps.Version
		if v == "" {
			v = "dev"
		}
		return c.JSON(VersionResponse{Version: v})
	})
	// Public runtime config for the SPA — must stay unauthenticated so the
	// browser can initialise Sentry before login. A blank dsn means the
	// frontend leaves Sentry disabled.
	app.Get("/api/config", func(c fiber.Ctx) error {
		passkeysAvailable := false
		if deps.PasskeyRepo != nil {
			n, err := deps.PasskeyRepo.CountAll(c.Context())
			if err != nil {
				return ErrInternal("count passkeys").WithCause(err)
			}
			passkeysAvailable = n > 0
		}
		return c.JSON(PublicConfigResponse{
			Sentry: PublicSentryConfig{
				DSN:         deps.SentryFrontendDSN,
				Environment: deps.SentryEnvironment,
			},
			Passkeys: PublicPasskeysConfig{
				Enabled:   deps.PasskeyRepo != nil,
				Available: passkeysAvailable,
			},
		})
	})
	return app.Group(
		"/api/v1",
		SetupCheckMiddleware(deps.UserRepo),
		APIAuthMiddleware(deps.JWTIssuer, deps.APITokenRepo, deps.APITokenSalt),
		// Idempotency must sit after auth (it needs the resolved user id) and
		// before Publish so a replay short-circuits before any SSE is emitted.
		IdempotencyMiddleware(deps.IdempotencyRepo),
		PublishMiddleware(deps.EventsHub),
	)
}

func makeErrorHandler(log *slog.Logger) fiber.ErrorHandler {
	return func(c fiber.Ctx, err error) error {
		var appErr *AppError
		switch e := err.(type) {
		case nil:
			return nil
		case *AppError:
			if e != nil {
				appErr = e
			}
		case *fiber.Error:
			if e != nil {
				code := CodeInternalError
				if e.Code == 404 {
					code = CodeNotFound
				}
				appErr = &AppError{HTTPStatus: e.Code, Code: code, Message: e.Message}
			}
		default:
			appErr = &AppError{HTTPStatus: 500, Code: CodeInternalError, Message: "unexpected server error", Internal: err}
		}
		if appErr == nil {
			appErr = &AppError{HTTPStatus: 500, Code: CodeInternalError, Message: "unexpected server error"}
		}
		// The Today focus lock is asked deep inside services that many handlers
		// call and wrap as an internal error. A refusal is never a server fault,
		// so whichever path it came up through, it is answered as the lock's 409.
		var locked *service.TodayLockedError
		if appErr.HTTPStatus >= 500 && appErr.Internal != nil && errors.As(appErr.Internal, &locked) {
			appErr = ErrTodayLocked(model.FormatUTC(locked.Until), locked.ProjectID)
		}
		ctx := c.Context()
		reqLog := logging.FromContext(ctx)
		if log != nil && appErr.HTTPStatus >= 500 && appErr.Internal != nil {
			reqLog.ErrorContext(ctx, "server error",
				slog.String("code", appErr.Code),
				slog.String("message", appErr.Message),
				slog.String("cause", appErr.Internal.Error()),
				slog.String("path", c.Path()),
				slog.String("method", c.Method()),
			)
		} else if log != nil && appErr.HTTPStatus >= 400 && appErr.HTTPStatus < 500 && !strings.HasPrefix(appErr.Code, "auth_") {
			attrs := []any{
				slog.String("code", appErr.Code),
				slog.String("message", appErr.Message),
				slog.Int("status", appErr.HTTPStatus),
				slog.String("path", c.Path()),
				slog.String("method", c.Method()),
			}
			if appErr.Internal != nil {
				attrs = append(attrs, slog.String("cause", appErr.Internal.Error()))
			}
			reqLog.DebugContext(ctx, "client error", attrs...)
		}
		return c.Status(appErr.HTTPStatus).JSON(ErrorEnvelope{
			Error: ErrorDetail{
				Code:    appErr.Code,
				Message: appErr.Message,
				Details: appErr.Details,
			},
		})
	}
}

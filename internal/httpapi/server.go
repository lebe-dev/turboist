package httpapi

import (
	"log/slog"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
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
}

type errorEnvelope struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
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
		return c.JSON(fiber.Map{"status": "ok"})
	})
	app.Get("/version", func(c fiber.Ctx) error {
		v := deps.Version
		if v == "" {
			v = "dev"
		}
		return c.JSON(fiber.Map{"version": v, "commit": "", "buildTime": ""})
	})
	return app.Group("/api/v1", APIAuthMiddleware(deps.JWTIssuer, deps.APITokenRepo, deps.APITokenSalt))
}

func makeErrorHandler(log *slog.Logger) fiber.ErrorHandler {
	return func(c fiber.Ctx, err error) error {
		var appErr *AppError
		switch e := err.(type) {
		case *AppError:
			appErr = e
		case *fiber.Error:
			code := CodeInternalError
			if e.Code == 404 {
				code = CodeNotFound
			}
			appErr = &AppError{HTTPStatus: e.Code, Code: code, Message: e.Message}
		default:
			if log != nil {
				log.Error("unhandled error", slog.String("error", err.Error()))
			}
			appErr = &AppError{HTTPStatus: 500, Code: CodeInternalError, Message: "unexpected server error"}
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
		return c.Status(appErr.HTTPStatus).JSON(errorEnvelope{
			Error: errorDetail{
				Code:    appErr.Code,
				Message: appErr.Message,
				Details: appErr.Details,
			},
		})
	}
}

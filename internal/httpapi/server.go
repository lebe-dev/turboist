package httpapi

import (
	"log/slog"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
)

// Deps holds all dependencies injected into the HTTP layer.
type Deps struct {
	Log         *slog.Logger
	JWTIssuer   *auth.JWTIssuer
	UserRepo    *repo.UserRepo
	SessionRepo *repo.SessionRepo
	IPLimiter   *auth.IPLimiter
	ContextRepo *repo.ContextRepo
	LabelRepo   *repo.LabelRepo
	SectionRepo *repo.ProjectSectionRepo
	ProjectRepo *repo.ProjectRepo
	TaskRepo    *repo.TaskRepo
	PinService  *service.PinService
	Cfg         *config.Config
	BaseURL     string
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
func NewApp(deps Deps) *fiber.App {
	app := fiber.New(fiber.Config{
		ErrorHandler: makeErrorHandler(deps.Log),
	})
	app.Use(recover.New())
	app.Use(RequestIDMiddleware())
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
		return c.JSON(fiber.Map{"version": "dev", "commit": "", "buildTime": ""})
	})
	return app.Group("/api/v1", AuthMiddleware(deps.JWTIssuer))
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
		return c.Status(appErr.HTTPStatus).JSON(errorEnvelope{
			Error: errorDetail{
				Code:    appErr.Code,
				Message: appErr.Message,
				Details: appErr.Details,
			},
		})
	}
}

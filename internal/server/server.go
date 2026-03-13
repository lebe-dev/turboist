package server

import (
	"io/fs"

	turboist "github.com/lebe-dev/turboist"

	"github.com/charmbracelet/log"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/proxy"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/static"
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/handler"
	"github.com/lebe-dev/turboist/internal/todoist"
)

func New(cfg *config.Config, cache *todoist.Cache) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName: "turboist",
	})

	app.Use(recover.New())
	app.Use(logger.New())

	store := auth.NewSessionStore()
	authHandler := handler.NewAuthHandler(store, cfg.Env.AdminPassword, cfg.Env.Dev)
	app.Use(auth.NewMiddleware(store))

	healthHandler := handler.NewHealthHandler(cache)
	app.Get("/api/health", healthHandler.Health)

	tasksHandler := handler.NewTasksHandler(cache, &cfg.App)
	app.Get("/api/tasks", tasksHandler.Tasks)
	app.Get("/api/tasks/weekly", tasksHandler.Weekly)
	app.Get("/api/tasks/next-week", tasksHandler.NextWeek)
	app.Get("/api/tasks/today", tasksHandler.Today)
	app.Get("/api/tasks/tomorrow", tasksHandler.Tomorrow)
	app.Post("/api/tasks", tasksHandler.Create)
	app.Patch("/api/tasks/:id", tasksHandler.Update)
	app.Post("/api/tasks/:id/complete", tasksHandler.Complete)

	projectsHandler := handler.NewProjectsHandler(cache)
	app.Get("/api/projects", projectsHandler.Projects)

	labelsHandler := handler.NewLabelsHandler(cache)
	app.Get("/api/labels", labelsHandler.Labels)

	contextsHandler := handler.NewContextsHandler(&cfg.App)
	app.Get("/api/contexts", contextsHandler.Contexts)

	configHandler := handler.NewConfigHandler(cache, &cfg.App)
	app.Get("/api/config", configHandler.Config)

	api := app.Group("/api/auth")
	api.Post("/login", authHandler.Login)
	api.Post("/logout", authHandler.Logout)
	api.Get("/me", authHandler.Me)

	if cfg.Env.Dev {
		log.Info("dev mode: proxying frontend to localhost:5173")
		app.Use(func(c fiber.Ctx) error {
			if len(c.Path()) >= 4 && c.Path()[:4] == "/api" {
				return c.Next()
			}
			return proxy.Forward("http://localhost:5173" + c.OriginalURL())(c)
		})
	} else {
		subFS, err := fs.Sub(turboist.StaticFS, "frontend/build")
		if err != nil {
			log.Fatal("failed to create sub FS", "err", err)
		}
		app.Use("/", static.New("", static.Config{
			FS:         subFS,
			IndexNames: []string{"index.html"},
			NotFoundHandler: func(c fiber.Ctx) error {
				return c.Next()
			},
		}))
	}

	return app
}

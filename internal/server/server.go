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
	"github.com/lebe-dev/turboist/internal/storage"
	"github.com/lebe-dev/turboist/internal/todoist"
	"github.com/lebe-dev/turboist/internal/ws"
)

// AutoRemovePauser exposes the circuit breaker state from the auto-remove scheduler.
// Pass nil when auto-remove is not configured.
type AutoRemovePauser interface {
	Paused() bool
}

func New(cfg *config.Config, cache *todoist.Cache, store *storage.Store, hub *ws.Hub, autoRemove AutoRemovePauser) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName: "turboist",
	})

	app.Use(recover.New())
	app.Use(logger.New())

	sessionStore := auth.NewSessionStore()
	authHandler := handler.NewAuthHandler(sessionStore, cfg.Env.AdminPassword, cfg.Env.Dev)
	app.Use(auth.NewMiddleware(sessionStore))

	healthHandler := handler.NewHealthHandler(cache)
	app.Get("/api/health", healthHandler.Health)

	// WebSocket endpoint
	app.Get("/api/ws", func(c fiber.Ctx) error {
		if !ws.IsWebSocketUpgrade(c) {
			return fiber.ErrUpgradeRequired
		}
		token := c.Cookies("turboist_token")
		if token == "" || !sessionStore.ValidateSession(token) {
			return c.SendStatus(401)
		}
		return hub.HandleWS(c)
	})

	tasksHandler := handler.NewTasksHandler(cache, &cfg.App, store)
	app.Get("/api/tasks", tasksHandler.Tasks)
	app.Get("/api/tasks/inbox", tasksHandler.Inbox)
	app.Get("/api/tasks/weekly", tasksHandler.Weekly)
	app.Get("/api/tasks/next-week", tasksHandler.NextWeek)
	app.Get("/api/tasks/today", tasksHandler.Today)
	app.Get("/api/tasks/completed", tasksHandler.Completed)
	app.Get("/api/tasks/tomorrow", tasksHandler.Tomorrow)
	app.Get("/api/tasks/backlog", tasksHandler.Backlog)
	app.Post("/api/tasks/reset-weekly", tasksHandler.ResetWeekly)
	app.Post("/api/tasks/batch-update-labels", tasksHandler.BatchUpdateLabels)
	app.Post("/api/tasks", tasksHandler.Create)
	app.Get("/api/tasks/:id", tasksHandler.GetByID)
	app.Patch("/api/tasks/:id", tasksHandler.Update)
	app.Post("/api/tasks/:id/complete", tasksHandler.Complete)
	app.Post("/api/tasks/:id/duplicate", tasksHandler.Duplicate)
	app.Post("/api/tasks/:id/decompose", tasksHandler.Decompose)
	app.Post("/api/tasks/:id/move", tasksHandler.Move)
	app.Delete("/api/tasks/:id", tasksHandler.Delete)
	app.Get("/api/tasks/:id/completed-subtasks", tasksHandler.CompletedSubtasks)

	configHandler := handler.NewConfigHandler(cache, &cfg.App, store, autoRemove)
	app.Get("/api/config", configHandler.Config)

	stateHandler := handler.NewStateHandler(store, &cfg.App)
	app.Patch("/api/state", stateHandler.Update)

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
				// SPA fallback: serve index.html for client-side routing
				content, err := fs.ReadFile(subFS, "index.html")
				if err != nil {
					return c.Next()
				}
				c.Set("Content-Type", "text/html; charset=utf-8")
				return c.Send(content)
			},
		}))
	}

	return app
}

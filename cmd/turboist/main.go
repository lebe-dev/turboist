package main

import (
	"github.com/charmbracelet/log"
	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/config"
)

const Version = "0.1.0"

func main() {
	log.Info("starting turboist", "version", Version)

	cfg, err := config.Load()
	if err != nil {
		log.Fatal("failed to load config", "err", err)
	}

	app := fiber.New(fiber.Config{
		AppName: "turboist " + Version,
	})

	app.Get("/api/health", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	log.Info("listening", "bind", cfg.Env.Bind)
	if err := app.Listen(cfg.Env.Bind); err != nil {
		log.Fatal("server error", "err", err)
	}
}

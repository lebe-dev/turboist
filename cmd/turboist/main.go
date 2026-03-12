package main

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/charmbracelet/log"
	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/scheduler"
	"github.com/lebe-dev/turboist/internal/server"
	"github.com/lebe-dev/turboist/internal/todoist"
)

const Version = "0.1.0"

func main() {
	log.Info("starting turboist", "version", Version)

	cfg, err := config.Load()
	if err != nil {
		log.Fatal("failed to load config", "err", err)
	}

	client := todoist.NewClient(cfg.Env.TodoistAPIKey)

	log.Info("warming cache...")
	cache := todoist.NewCache(client)
	log.Info("cache warmed")

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cache.StartPolling(ctx, cfg.App.PollInterval)
	log.Info("cache polling started", "interval", cfg.App.PollInterval)

	sched := scheduler.New(cfg.App.PollInterval)
	if len(cfg.App.AutoExpire) > 0 {
		ae := scheduler.NewAutoExpire(cache, cfg.App.AutoExpire)
		sched.Register("auto-expire", ae.Job)
		log.Info("auto-expire rules registered", "count", len(cfg.App.AutoExpire))
	}
	if cfg.App.Weekly.MaxTasks > 0 {
		wl := scheduler.NewWeeklyLimit(cache, cfg.App.Weekly)
		sched.Register("weekly-limit", wl.Job)
		log.Info("weekly-limit check registered", "max_tasks", cfg.App.Weekly.MaxTasks)
	}
	sched.Start(ctx)
	log.Info("scheduler started", "interval", cfg.App.PollInterval)

	app := server.New(cfg, cache)

	go func() {
		<-ctx.Done()
		log.Info("shutting down server")
		_ = app.Shutdown()
	}()

	log.Info("listening", "bind", cfg.Env.Bind)
	if err := app.Listen(cfg.Env.Bind); err != nil {
		log.Fatal("server error", "err", err)
	}
}

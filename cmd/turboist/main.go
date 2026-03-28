package main

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/charmbracelet/log"
	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/scheduler"
	"github.com/lebe-dev/turboist/internal/server"
	"github.com/lebe-dev/turboist/internal/storage"
	"github.com/lebe-dev/turboist/internal/todoist"
	"github.com/lebe-dev/turboist/internal/ws"
)

const Version = "0.11.0"

func main() {
	log.Info("starting turboist", "version", Version)

	cfg, err := config.Load()
	if err != nil {
		log.Fatal("failed to load config", "err", err)
	}

	if level, err := log.ParseLevel(cfg.Env.LogLevel); err != nil {
		log.Warn("invalid LOG_LEVEL, defaulting to info", "value", cfg.Env.LogLevel)
	} else {
		log.SetLevel(level)
	}

	log.Info("config loaded",
		"bind", cfg.Env.Bind,
		"base_url", cfg.Env.BaseURL,
		"dev", cfg.Env.Dev,
		"poll_interval", cfg.App.PollInterval,
		"contexts", len(cfg.App.Contexts),
		"weekly_label", cfg.App.Weekly.Label,
		"weekly_max_tasks", cfg.App.Weekly.MaxTasks,
		"auto_expire_rules", len(cfg.App.AutoExpire),
	)

	client := todoist.NewClient(cfg.Env.TodoistAPIKey)

	log.Info("warming cache...")
	cache := todoist.NewCache(client)
	log.Info("cache warmed")

	hub := ws.NewHub(cache, &cfg.App)
	cache.SetOnRefresh(hub.Broadcast)
	log.Info("websocket hub initialized")

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cache.StartPolling(ctx, cfg.App.PollInterval)
	log.Info("cache polling started", "interval", cfg.App.PollInterval)

	store, err := storage.New("./data/turboist.db")
	if err != nil {
		log.Fatal("failed to init storage", "err", err)
	}
	defer func() { _ = store.Close() }()
	log.Info("storage initialized")

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
	if len(cfg.App.LabelProjectMap) > 0 {
		lp := scheduler.NewLabelProjectSync(cache, cfg.App.LabelProjectMap)
		sched.Register("label-project", lp.Job)
		log.Info("label-project sync registered", "mappings", len(cfg.App.LabelProjectMap))
	}
	sched.Start(ctx)
	log.Info("scheduler started", "interval", cfg.App.PollInterval)

	app := server.New(cfg, cache, store, hub)

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

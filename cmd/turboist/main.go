package main

import (
	"context"
	"os/signal"
	"slices"
	"syscall"
	"time"

	"github.com/charmbracelet/log"
	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/scheduler"
	"github.com/lebe-dev/turboist/internal/server"
	"github.com/lebe-dev/turboist/internal/storage"
	"github.com/lebe-dev/turboist/internal/todoist"
	"github.com/lebe-dev/turboist/internal/troiki"
	"github.com/lebe-dev/turboist/internal/ws"
)

const Version = "0.20.1"

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
		"auto_remove_rules", len(cfg.App.AutoRemove.Rules),
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

	// Sync label blocks from config to DB
	if cfg.App.Constraints.Enabled && len(cfg.App.Constraints.LabelBlocks) > 0 {
		now := time.Now()
		configuredLabels := make([]string, 0, len(cfg.App.Constraints.LabelBlocks))
		for _, lb := range cfg.App.Constraints.LabelBlocks {
			configuredLabels = append(configuredLabels, lb.Label)
			if err := store.UpsertLabelBlock(lb.Label, now); err != nil {
				log.Fatal("upsert label block failed", "label", lb.Label, "err", err)
			}
		}
		if err := store.DeleteUnconfiguredLabelBlocks(configuredLabels); err != nil {
			log.Fatal("delete unconfigured label blocks failed", "err", err)
		}
		// Delete blocks whose duration has elapsed
		blocks, err := store.GetLabelBlocks()
		if err != nil {
			log.Fatal("get label blocks failed", "err", err)
		}
		durationByLabel := make(map[string]time.Duration, len(cfg.App.Constraints.LabelBlocks))
		for _, lb := range cfg.App.Constraints.LabelBlocks {
			durationByLabel[lb.Label] = lb.Duration
		}
		for _, b := range blocks {
			if dur, ok := durationByLabel[b.Label]; ok {
				if now.After(b.StartedAt.Add(dur)) {
					if err := store.DeleteLabelBlock(b.Label); err != nil {
						log.Fatal("delete expired label block failed", "label", b.Label, "err", err)
					}
					log.Info("expired label block removed", "label", b.Label)
				}
			}
		}
		log.Info("label blocks synced", "count", len(cfg.App.Constraints.LabelBlocks))
	}

	var troikiService *troiki.Service
	if cfg.App.TroikiSystem.Enabled {
		troikiService = troiki.NewService(cache, cfg.App.TroikiSystem, store)
		if err := troikiService.Init(ctx); err != nil {
			log.Fatal("troiki init failed", "err", err)
		}
		hub.SetTroikiService(troikiService)
		log.Info("troiki system enabled", "project", cfg.App.TroikiSystem.ProjectName)
	}

	cache.SetTaskEnricher(func(tasks []*todoist.Task) {
		counts, err := store.GetPostponeCounts()
		if err != nil {
			log.Error("load postpone counts failed", "err", err)
		} else {
			for _, t := range tasks {
				if c, ok := counts[t.ID]; ok {
					t.PostponeCount = c
				}
			}
		}

		if cfg.App.AutoRemove.Enabled && len(cfg.App.AutoRemove.Rules) > 0 {
			firstSeenMap, err := store.GetAutoRemoveFirstSeen()
			if err != nil {
				log.Error("load auto_remove first_seen failed", "err", err)
			} else {
				for _, t := range tasks {
					for _, rule := range cfg.App.AutoRemove.Rules {
						if slices.Contains(t.Labels, rule.Label) {
							key := t.ID + ":" + rule.Label
							if seen, ok := firstSeenMap[key]; ok {
								expiresAt := seen.Add(rule.TTL).Format(time.RFC3339)
								t.ExpiresAt = &expiresAt
								break // first matching rule wins
							}
						}
					}
				}
			}
		}
	})
	if err := cache.Refresh(ctx); err != nil {
		log.Warn("initial enrichment refresh failed", "err", err)
	}

	sched := scheduler.New(cfg.App.PollInterval)
	var autoRemove *scheduler.AutoRemove
	if cfg.App.AutoRemove.Enabled && len(cfg.App.AutoRemove.Rules) > 0 {
		autoRemove = scheduler.NewAutoRemove(cache, cfg.App.AutoRemove, store)
		sched.Register("auto-remove", autoRemove.Job)
		log.Info("auto-remove rules registered", "count", len(cfg.App.AutoRemove.Rules))
	}
	if cfg.App.Weekly.MaxTasks > 0 {
		wl := scheduler.NewWeeklyLimit(cache, cfg.App.Weekly)
		sched.Register("weekly-limit", wl.Job)
		log.Info("weekly-limit check registered", "max_tasks", cfg.App.Weekly.MaxTasks)
	}
	if cfg.App.LabelProjectMap.Enabled && len(cfg.App.LabelProjectMap.Mappings) > 0 {
		lp := scheduler.NewLabelProjectSync(cache, cfg.App.LabelProjectMap.Mappings)
		if troikiService != nil {
			lp.ExcludeProjects(troikiService.ProjectID())
		}
		sched.Register("label-project", lp.Job)
		log.Info("label-project sync registered", "mappings", len(cfg.App.LabelProjectMap.Mappings))
	}
	sched.Start(ctx)
	log.Info("scheduler started", "interval", cfg.App.PollInterval)

	var autoRemovePauser server.AutoRemovePauser
	if autoRemove != nil {
		autoRemovePauser = autoRemove
	}
	app := server.New(cfg, cache, store, hub, autoRemovePauser, troikiService)

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

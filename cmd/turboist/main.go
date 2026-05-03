package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	turboist "github.com/lebe-dev/turboist"
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/handlers"
	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
	"golang.org/x/time/rate"
)

const Version = "1.0.0"

func main() {
	configPath := flag.String("config", "config.yml", "path to config.yml")
	flag.Parse()

	_ = godotenv.Load()

	env, err := config.LoadEnv()
	if err != nil {
		_, _ = os.Stderr.WriteString("env error: " + err.Error() + "\n")
		os.Exit(1)
	}

	log := logging.New(env.LogLevel)

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Error("config error", "err", err)
		os.Exit(1)
	}

	log.Info("starting turboist",
		"version", Version,
		"bind", env.Bind,
		"baseUrl", env.BaseURL,
		"timezone", cfg.Timezone,
	)

	sqlDB, err := db.Open(env.DataPath)
	if err != nil {
		log.Error("open db", "err", err)
		os.Exit(1)
	}
	defer func() { _ = sqlDB.Close() }()

	if err := db.RunMigrations(context.Background(), sqlDB); err != nil {
		log.Error("run migrations", "err", err)
		os.Exit(1)
	}

	// repos
	plabels := repo.NewProjectLabelsRepo(sqlDB)
	tlabels := repo.NewTaskLabelsRepo(sqlDB)
	userRepo := repo.NewUserRepo(sqlDB)
	sessionRepo := repo.NewSessionRepo(sqlDB)
	ctxRepo := repo.NewContextRepo(sqlDB)
	labelRepo := repo.NewLabelRepo(sqlDB)
	sectionRepo := repo.NewProjectSectionRepo(sqlDB)
	projectRepo := repo.NewProjectRepo(sqlDB, plabels)
	taskRepo := repo.NewTaskRepo(sqlDB, tlabels)
	searchRepo := repo.NewSearchRepo(taskRepo, projectRepo)

	// auth
	jwtIssuer := auth.NewJWTIssuer([]byte(env.JWTSecret))
	// 10 requests per minute per IP for auth endpoints
	ipLimiter := auth.NewIPLimiter(rate.Every(6*time.Second), 10, 10*time.Minute)

	// services
	pinSvc := service.NewPinService(taskRepo, projectRepo, cfg.MaxPinned)
	autoLabelsSvc := service.NewAutoLabelsService(labelRepo, cfg)
	taskSvc := service.NewTaskService(taskRepo, projectRepo, tlabels, autoLabelsSvc)
	completeSvc := service.NewCompleteServiceWithLoc(taskRepo, projectRepo, userRepo, cfg.Location)
	moveSvc := service.NewMoveService(taskRepo, projectRepo)
	planSvc := service.NewPlanService(taskRepo, ctxRepo, cfg.Weekly.Limit, cfg.Backlog.Limit)
	troikiSvc := service.NewTroikiService(taskRepo, projectRepo, userRepo)

	// session cleanup
	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
	defer cleanupCancel()
	auth.StartSessionCleanup(cleanupCtx, sessionRepo, log)

	// HTTP app
	deps := httpapi.Deps{
		Log:         log,
		JWTIssuer:   jwtIssuer,
		UserRepo:    userRepo,
		SessionRepo: sessionRepo,
		IPLimiter:   ipLimiter,
		ContextRepo: ctxRepo,
		LabelRepo:   labelRepo,
		SectionRepo: sectionRepo,
		ProjectRepo: projectRepo,
		TaskRepo:    taskRepo,
		PinService:  pinSvc,
		Cfg:         cfg,
		BaseURL:     env.BaseURL,
		Version:     Version,
	}
	app := httpapi.NewApp(deps)
	api := httpapi.RegisterRoutes(app, deps)

	authHandler := handlers.NewAuthHandler(userRepo, sessionRepo, jwtIssuer, ipLimiter, env.Argon2Params)
	authHandler.RegisterAuth(app.Group("/auth"), jwtIssuer)
	handlers.NewContextHandler(ctxRepo, projectRepo, taskRepo, taskSvc, env.BaseURL).Register(api.Group("/contexts"))
	handlers.NewLabelHandler(labelRepo, projectRepo, taskRepo, env.BaseURL).Register(api.Group("/labels"))
	handlers.NewSectionHandler(sectionRepo, projectRepo, taskRepo, taskSvc, env.BaseURL).Register(api.Group("/sections"))
	handlers.NewProjectHandler(projectRepo, sectionRepo, taskRepo, taskSvc, labelRepo, ctxRepo, pinSvc, env.BaseURL).Register(api)
	handlers.NewInboxHandler(taskRepo, taskSvc, cfg, env.BaseURL).Register(api.Group("/inbox"))
	handlers.NewTaskBulkHandler(completeSvc, moveSvc, env.BaseURL).Register(api)
	handlers.NewTaskViewHandler(taskRepo, cfg, env.BaseURL).Register(api)
	handlers.NewTaskActionHandler(taskRepo, completeSvc, planSvc, pinSvc, moveSvc, env.BaseURL).Register(api)
	handlers.NewTroikiHandler(troikiSvc, env.BaseURL).Register(api)
	handlers.NewTaskHandler(taskRepo, projectRepo, taskSvc, env.BaseURL).Register(api)
	handlers.NewSearchHandler(searchRepo, env.BaseURL).Register(api)
	handlers.NewMetaHandler(cfg).Register(api)
	handlers.NewStateHandler(userRepo).Register(api)

	// embedded SvelteKit SPA (must be registered after API/auth routes)
	if err := httpapi.RegisterSPA(app, turboist.StaticFS, "frontend/build"); err != nil {
		log.Error("register SPA", "err", err)
		os.Exit(1)
	}

	// graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		log.Info("shutting down")
		cleanupCancel()
		authHandler.Stop()
		if err := app.ShutdownWithTimeout(5 * time.Second); err != nil {
			log.Error("shutdown error", "err", err)
		}
	}()

	log.Info("listening", "bind", env.Bind)
	if err := app.Listen(env.Bind); err != nil {
		log.Error("server error", "err", err)
		os.Exit(1)
	}
}

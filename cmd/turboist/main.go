package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	turboist "github.com/lebe-dev/turboist"
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/handlers"
	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/obs"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
	calendarsvc "github.com/lebe-dev/turboist/internal/service/calendar"
	"github.com/lebe-dev/turboist/internal/service/events"
	"github.com/lebe-dev/turboist/internal/service/inboxproc"
	"github.com/lebe-dev/turboist/internal/service/linktitle"
	passkeysvc "github.com/lebe-dev/turboist/internal/service/passkey"
	totpsvc "github.com/lebe-dev/turboist/internal/service/totp"
	"golang.org/x/time/rate"
)

var Version = "dev"

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
	slog.SetDefault(log)

	sentryFlush, err := obs.Init(env.SentryDSN, env.SentryEnvironment, Version)
	if err != nil {
		_, _ = os.Stderr.WriteString("sentry init error: " + err.Error() + "\n")
		os.Exit(1)
	}
	defer sentryFlush()
	if env.SentryDSN != "" {
		log.Info("sentry error reporting enabled", "environment", env.SentryEnvironment)
	} else {
		log.Info("sentry error reporting disabled (SENTRY_DSN not set)")
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		obs.CaptureError(err, map[string]string{"op": "config.Load"})
		sentryFlush()
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
		obs.CaptureError(err, map[string]string{"op": "db.Open"})
		sentryFlush()
		log.Error("open db", "err", err)
		os.Exit(1)
	}
	defer logging.LogClose(context.Background(), "main.sqlDB", sqlDB)

	if err := db.RunMigrations(context.Background(), sqlDB); err != nil {
		obs.CaptureError(err, map[string]string{"op": "db.RunMigrations"})
		sentryFlush()
		log.Error("run migrations", "err", err)
		os.Exit(1)
	}

	// repos
	plabels := repo.NewProjectLabelsRepo(sqlDB)
	tlabels := repo.NewTaskLabelsRepo(sqlDB)
	trelations := repo.NewTaskRelationsRepo(sqlDB)
	userRepo := repo.NewUserRepo(sqlDB)
	sessionRepo := repo.NewSessionRepo(sqlDB)
	appSettingsRepo := repo.NewAppSettingsRepo(sqlDB)
	apiTokenRepo := repo.NewAPITokenRepo(sqlDB)
	calendarRepo := repo.NewCalendarRepo(sqlDB)
	totpRecoveryRepo := repo.NewTOTPRecoveryRepo(sqlDB)
	ctxRepo := repo.NewContextRepo(sqlDB)
	labelRepo := repo.NewLabelRepo(sqlDB)
	sectionRepo := repo.NewProjectSectionRepo(sqlDB)
	projectRepo := repo.NewProjectRepo(sqlDB, plabels)
	taskRepo := repo.NewTaskRepo(sqlDB, tlabels, trelations)
	templateRepo := repo.NewTemplateRepo(sqlDB)
	searchRepo := repo.NewSearchRepo(taskRepo, projectRepo)
	idempotencyRepo := repo.NewIdempotencyRepo(sqlDB)
	webauthnRepo := repo.NewWebAuthnRepo(sqlDB)
	changeLogRepo := repo.NewChangeLogRepo(sqlDB)
	inboxProcRepo := repo.NewInboxProcessingRepo(sqlDB, tlabels)
	linkTitleQueue := repo.NewLinkTitleQueueRepo(sqlDB)

	// auth
	jwtIssuer := auth.NewJWTIssuer([]byte(env.JWTSecret))
	// 10 requests per minute per IP for auth endpoints
	ipLimiter := auth.NewIPLimiter(rate.Every(6*time.Second), 10, 10*time.Minute)

	// services
	pinSvc := service.NewPinService(taskRepo, projectRepo, userRepo)
	autoLabelsSvc := service.NewAutoLabelsService(labelRepo, appSettingsRepo)
	taskSvc := service.NewTaskService(taskRepo, projectRepo, tlabels, autoLabelsSvc)
	completeSvc := service.NewCompleteServiceWithLoc(taskRepo, projectRepo, userRepo, trelations, cfg.Location)
	relationSvc := service.NewRelationService(taskRepo, trelations)
	moveSvc := service.NewMoveService(taskRepo, projectRepo)
	groupSvc := service.NewGroupService(taskSvc, moveSvc, taskRepo, tlabels)
	planSvc := service.NewPlanService(taskRepo, ctxRepo, cfg.Weekly.Limit, cfg.Backlog.Limit)
	troikiSvc := service.NewTroikiService(taskRepo, projectRepo, userRepo)
	harpoonSvc := service.NewHarpoonService(userRepo, taskRepo, projectRepo)
	templateSvc := service.NewTemplateService(templateRepo, projectRepo, taskSvc)
	backupSvc := service.NewBackupService(sqlDB)

	var totpSvc *totpsvc.Service
	if env.TOTPSecretKey != "" {
		totpSvc = totpsvc.NewService(
			crypto.NewTokenCipher(env.TOTPSecretKey),
			userRepo,
			totpRecoveryRepo,
			env.Argon2Params,
		)
		log.Info("totp 2FA enabled")
	} else {
		log.Info("totp 2FA disabled (TOTP_SECRET_KEY not set)")
	}

	// Passkeys (WebAuthn). The Relying Party is derived from BASE_URL, so a
	// standard install needs no extra config; a rejected config disables the
	// feature rather than the server, since the password login still works.
	passkeySvc, err := passkeysvc.NewService(passkeysvc.Config{
		RPID:          env.WebAuthnRPID,
		RPDisplayName: "Turboist",
		Origins:       env.WebAuthnOrigins,
	}, webauthnRepo, userRepo)
	if err != nil {
		log.Warn("passkeys disabled", "err", err)
		passkeySvc = nil
	} else {
		log.Info("passkeys enabled", "rp_id", env.WebAuthnRPID, "origins", env.WebAuthnOrigins)
	}

	// session, idempotency-key and change-log cleanup (share the shutdown-scoped context)
	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
	defer cleanupCancel()
	auth.StartSessionCleanup(cleanupCtx, sessionRepo, log)
	startIdempotencyCleanup(cleanupCtx, idempotencyRepo, log)
	startChangeLogPrune(cleanupCtx, changeLogRepo, log)
	startInboxProcessingPrune(cleanupCtx, inboxProcRepo, log)

	// events hub (SSE pub/sub) — owned by main so that Deps and the events
	// handler share the same instance.
	eventsHub := events.NewHub(log)
	eventsTickets := events.NewTicketStore()

	// LLM Inbox processing. The processor exists even when disabled so the
	// journal, revert and prompt editor keep working; only the loop needs a key.
	ipEnv := env.InboxProcessing
	var inboxClassifier inboxproc.Classifier
	if ipEnv.Enabled {
		inboxClassifier = inboxproc.NewOpenAIClient(inboxproc.OpenAIClientConfig{
			APIURL:  ipEnv.APIURL,
			APIKey:  ipEnv.APIKey,
			Model:   ipEnv.Model,
			Referer: env.BaseURL,
			Timeout: ipEnv.Timeout,
		})
	}
	inboxProcessor := inboxproc.NewProcessor(inboxproc.Config{
		Enabled:    ipEnv.Enabled,
		Interval:   ipEnv.Interval,
		APIURL:     ipEnv.APIURL,
		APIKey:     ipEnv.APIKey,
		Model:      ipEnv.Model,
		BatchLimit: ipEnv.BatchLimit,
		Timeout:    ipEnv.Timeout,
	}, inboxClassifier, inboxproc.Deps{
		Tasks:       taskRepo,
		TaskLabels:  tlabels,
		State:       inboxProcRepo,
		Contexts:    ctxRepo,
		Projects:    projectRepo,
		Labels:      labelRepo,
		AppSettings: appSettingsRepo,
		Users:       userRepo,
		Move:        moveSvc,
		Hub:         eventsHub,
		Location:    cfg.Location,
		Log:         log,
	})
	if ipEnv.Enabled {
		go inboxProcessor.Run(cleanupCtx)
		log.Info("inbox processing enabled", "model", ipEnv.Model, "interval", ipEnv.Interval.String(),
			"batch_limit", ipEnv.BatchLimit)
	} else {
		log.Info("inbox processing disabled (INBOX_PROCESSING_ENABLED not set)")
	}

	// Link titles: a task created with a bare https:// link as its title gets
	// the page's title. Always wired; the settings toggle decides whether it acts.
	var linkRenderer linktitle.PageFetcher
	if r := linktitle.NewRenderer(env.LinkTitles.RendererURL, env.LinkTitles.RendererTimeout); r != nil {
		linkRenderer = r
	}
	linkTitles := linktitle.NewService(linktitle.Deps{
		Queue:       linkTitleQueue,
		Tasks:       taskRepo,
		AppSettings: appSettingsRepo,
		Users:       userRepo,
		Hub:         eventsHub,
		Log:         log,
	}, linktitle.ChainFetcher{Direct: linktitle.NewDirectFetcher(), Renderer: linkRenderer})
	taskSvc.WithCreatedHook(linkTitles)
	go linkTitles.Run(cleanupCtx)
	log.Info("link titles worker started", "renderer", linkRenderer != nil)

	// HTTP app
	deps := httpapi.Deps{
		Log:             log,
		JWTIssuer:       jwtIssuer,
		UserRepo:        userRepo,
		SessionRepo:     sessionRepo,
		APITokenRepo:    apiTokenRepo,
		APITokenSalt:    []byte(env.APITokenSalt),
		IPLimiter:       ipLimiter,
		ContextRepo:     ctxRepo,
		LabelRepo:       labelRepo,
		SectionRepo:     sectionRepo,
		ProjectRepo:     projectRepo,
		TaskRepo:        taskRepo,
		PinService:      pinSvc,
		BackupSvc:       backupSvc,
		Cfg:             cfg,
		BaseURL:         env.BaseURL,
		Version:         Version,
		EventsHub:       eventsHub,
		IdempotencyRepo: idempotencyRepo,

		SentryEnabled:     env.SentryDSN != "",
		SentryFrontendDSN: env.SentryFrontendDSN,
		SentryEnvironment: env.SentryEnvironment,
	}
	if passkeySvc != nil {
		// Only advertise passkeys on GET /api/config once the ceremony service
		// actually came up — otherwise the login page would offer a button no
		// endpoint answers.
		deps.PasskeyRepo = webauthnRepo
	}
	app := httpapi.NewApp(deps)
	calendarSvc := calendarsvc.NewService(
		calendarRepo,
		userRepo,
		env.BaseURL,
		env.JWTSecret,
		env.CalendarCacheTTL,
		log,
	)
	authHandler := handlers.NewAuthHandler(userRepo, sessionRepo, jwtIssuer, ipLimiter, env.Argon2Params)
	if totpSvc != nil {
		authHandler.WithTOTP(totpSvc)
	}
	if passkeySvc != nil {
		authHandler.WithPasskeys(passkeySvc)
	}
	var totpHandler *handlers.TOTPHandler
	if totpSvc != nil {
		totpHandler = handlers.NewTOTPHandler(userRepo, totpSvc, ipLimiter)
	}
	var passkeysHandler *handlers.PasskeysHandler
	if passkeySvc != nil {
		passkeysHandler = handlers.NewPasskeysHandler(passkeySvc)
	}
	handlers.Mount(app, deps, handlers.Handlers{
		Auth:          authHandler,
		TOTP:          totpHandler,
		Events:        handlers.NewEventsHandler(eventsHub, eventsTickets),
		Calendar:      handlers.NewCalendarHandler(calendarSvc, calendarRepo, userRepo, env.BaseURL, log),
		Contexts:      handlers.NewContextHandler(ctxRepo, projectRepo, taskRepo, taskSvc, env.BaseURL),
		Labels:        handlers.NewLabelHandler(labelRepo, projectRepo, taskRepo, cfg, env.BaseURL),
		Templates:     handlers.NewTemplateHandler(templateRepo, templateSvc, env.BaseURL),
		Sections:      handlers.NewSectionHandler(sectionRepo, projectRepo, taskRepo, taskSvc, env.BaseURL),
		Projects:      handlers.NewProjectHandler(projectRepo, sectionRepo, taskRepo, taskSvc, labelRepo, ctxRepo, pinSvc, env.BaseURL).WithLocation(cfg.Location),
		Inbox:         handlers.NewInboxHandler(taskRepo, taskSvc, cfg, env.BaseURL),
		TaskBulk:      handlers.NewTaskBulkHandler(completeSvc, moveSvc, groupSvc, taskSvc, env.BaseURL),
		TaskViews:     handlers.NewTaskViewHandler(taskRepo, userRepo, troikiSvc, cfg, env.BaseURL),
		TaskActions:   handlers.NewTaskActionHandler(taskRepo, completeSvc, planSvc, pinSvc, moveSvc, env.BaseURL),
		Troiki:        handlers.NewTroikiHandler(troikiSvc, env.BaseURL),
		TaskRelations: handlers.NewTaskRelationHandler(relationSvc, env.BaseURL),
		Tasks:         handlers.NewTaskHandler(taskRepo, projectRepo, taskSvc, env.BaseURL),
		Search:        handlers.NewSearchHandler(searchRepo, env.BaseURL),
		Meta: handlers.NewMetaHandler(cfg, totpSvc != nil, inboxProcessor.Enabled(), ctxRepo, projectRepo, labelRepo,
			taskRepo, userRepo, appSettingsRepo, templateRepo, troikiSvc, harpoonSvc, env.BaseURL),
		State:           handlers.NewStateHandler(userRepo),
		Settings:        handlers.NewSettingsHandler(userRepo),
		Harpoon:         handlers.NewHarpoonHandler(harpoonSvc),
		AppSettings:     handlers.NewAppSettingsHandler(appSettingsRepo, labelRepo, projectRepo),
		InboxProcessing: handlers.NewInboxProcessingHandler(inboxProcessor, inboxProcRepo, appSettingsRepo, env.BaseURL),
		Sync:            handlers.NewSyncHandler(changeLogRepo, env.BaseURL),
		APITokens:       handlers.NewAPITokensHandler(apiTokenRepo, []byte(env.APITokenSalt)),
		Sessions:        handlers.NewSessionsHandler(sessionRepo),
		Passkeys:        passkeysHandler,
		Backup:          handlers.NewBackupHandler(backupSvc),
	})

	// Deployment-specific association files for the native passkey flow; a
	// no-op unless WELL_KNOWN_PATH is set. Must precede the SPA catch-all.
	httpapi.RegisterWellKnown(app, env.WellKnownPath, log)

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
		eventsHub.Close()
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

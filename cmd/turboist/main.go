package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	turboist "github.com/lebe-dev/turboist"
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/db"
	fedaudit "github.com/lebe-dev/turboist/internal/federation/audit"
	"github.com/lebe-dev/turboist/internal/federation/client"
	fedgc "github.com/lebe-dev/turboist/internal/federation/gc"
	"github.com/lebe-dev/turboist/internal/federation/hlc"
	"github.com/lebe-dev/turboist/internal/federation/inbox"
	fedmetrics "github.com/lebe-dev/turboist/internal/federation/metrics"
	"github.com/lebe-dev/turboist/internal/federation/nonce"
	"github.com/lebe-dev/turboist/internal/federation/outbox"
	"github.com/lebe-dev/turboist/internal/federation/peerkeys"
	"github.com/lebe-dev/turboist/internal/federation/ratelimit"
	"github.com/lebe-dev/turboist/internal/federation/recovery"
	"github.com/lebe-dev/turboist/internal/federation/store"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/handlers"
	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
	calendarsvc "github.com/lebe-dev/turboist/internal/service/calendar"
	"github.com/lebe-dev/turboist/internal/service/events"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
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
	defer logging.LogClose(context.Background(), "main.sqlDB", sqlDB)

	if err := db.RunMigrations(context.Background(), sqlDB); err != nil {
		log.Error("run migrations", "err", err)
		os.Exit(1)
	}

	// repos
	plabels := repo.NewProjectLabelsRepo(sqlDB)
	tlabels := repo.NewTaskLabelsRepo(sqlDB)
	userRepo := repo.NewUserRepo(sqlDB)
	sessionRepo := repo.NewSessionRepo(sqlDB)
	appSettingsRepo := repo.NewAppSettingsRepo(sqlDB)
	apiTokenRepo := repo.NewAPITokenRepo(sqlDB)
	calendarRepo := repo.NewCalendarRepo(sqlDB)
	totpRecoveryRepo := repo.NewTOTPRecoveryRepo(sqlDB)
	federationKeysRepo := repo.NewFederationKeysRepo(sqlDB)
	federatedProjectRepo := repo.NewFederatedProjectRepo(sqlDB)
	federationInviteRepo := repo.NewFederationInviteRepo(sqlDB)
	federatedInstanceRepo := repo.NewFederatedInstanceRepo(sqlDB)
	federationSnapshotRepo := repo.NewFederationSnapshotRepo(sqlDB)
	federationIncidentRepo := repo.NewFederationSecurityIncidentRepo(sqlDB)
	federationAuditRepo := repo.NewFederationAuditLogRepo(sqlDB)
	ctxRepo := repo.NewContextRepo(sqlDB)
	labelRepo := repo.NewLabelRepo(sqlDB)
	sectionRepo := repo.NewProjectSectionRepo(sqlDB)
	projectRepo := repo.NewProjectRepo(sqlDB, plabels)
	taskRepo := repo.NewTaskRepo(sqlDB, tlabels)
	commentRepo := repo.NewCommentRepo(sqlDB)
	checklistRepo := repo.NewChecklistItemRepo(sqlDB)
	searchRepo := repo.NewSearchRepo(taskRepo, projectRepo)

	// auth
	jwtIssuer := auth.NewJWTIssuer([]byte(env.JWTSecret))
	// 10 requests per minute per IP for auth endpoints
	ipLimiter := auth.NewIPLimiter(rate.Every(6*time.Second), 10, 10*time.Minute)

	// services
	pinSvc := service.NewPinService(taskRepo, projectRepo, cfg.MaxPinned)
	autoLabelsSvc := service.NewAutoLabelsService(labelRepo, appSettingsRepo)
	taskSvc := service.NewTaskService(taskRepo, projectRepo, tlabels, autoLabelsSvc)
	completeSvc := service.NewCompleteServiceWithLoc(taskRepo, projectRepo, userRepo, cfg.Location)
	moveSvc := service.NewMoveService(taskRepo, projectRepo)
	groupSvc := service.NewGroupService(taskSvc, moveSvc, taskRepo, tlabels)
	planSvc := service.NewPlanService(taskRepo, ctxRepo, cfg.Weekly.Limit, cfg.Backlog.Limit)
	troikiSvc := service.NewTroikiService(taskRepo, projectRepo, userRepo)
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

	// session cleanup
	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
	defer cleanupCancel()
	auth.StartSessionCleanup(cleanupCtx, sessionRepo, log)

	// events hub (SSE pub/sub) — owned by main so that Deps and the events
	// handler share the same instance.
	eventsHub := events.NewHub(log)
	eventsTickets := events.NewTicketStore()

	// HTTP app
	deps := httpapi.Deps{
		Log:          log,
		JWTIssuer:    jwtIssuer,
		UserRepo:     userRepo,
		SessionRepo:  sessionRepo,
		APITokenRepo: apiTokenRepo,
		APITokenSalt: []byte(env.APITokenSalt),
		IPLimiter:    ipLimiter,
		ContextRepo:  ctxRepo,
		LabelRepo:    labelRepo,
		SectionRepo:  sectionRepo,
		ProjectRepo:  projectRepo,
		TaskRepo:     taskRepo,
		PinService:   pinSvc,
		BackupSvc:    backupSvc,
		Cfg:          cfg,
		BaseURL:      env.BaseURL,
		Version:      Version,
		EventsHub:    eventsHub,
	}
	app := httpapi.NewApp(deps)
	calendarSvc := calendarsvc.NewService(
		calendarRepo,
		userRepo,
		env.BaseURL,
		env.JWTSecret,
		log,
	)
	calendarHandler := handlers.NewCalendarHandler(
		calendarSvc,
		calendarRepo,
		userRepo,
		env.BaseURL,
		log,
	)
	calendarHandler.RegisterPublic(app)

	eventsHandler := handlers.NewEventsHandler(eventsHub, eventsTickets)
	eventsHandler.RegisterPublic(app)

	// Federation trust plane (Federation v1 F0.3 / F1.1). The public .well-known
	// discovery endpoint is only mounted when FEDERATION_KEY is configured —
	// without it the instance cannot encrypt its private seed at rest, so
	// federation is disabled (mirrors the TOTP_SECRET_KEY gate). When enabled, the
	// owner-facing federation service backs the JWT admin routes registered below.
	var federationSvc *fedsvc.Service
	var federationHandler *handlers.FederationHandler
	var federationWorker *outbox.Worker
	var federationPublisher *fedsvc.Publisher
	var federationRecovery *recovery.Loop
	var federationQueue *inbox.Queue
	var federationRateLimiter *ratelimit.PeerLimiter
	var federationHandshakeLimiter *ratelimit.PeerLimiter
	var federationTaskMutator *fedsvc.TaskMutator
	var federationProjectMutator *fedsvc.ProjectMutator
	var federationSectionMutator *fedsvc.SectionMutator
	var federationAuditWriter *fedaudit.Writer
	var federationRetentionSvc *fedsvc.RetentionService
	var federationMetrics *fedmetrics.Collectors
	if env.FederationKey != "" {
		federationCipher := crypto.NewTokenCipher(env.FederationKey)
		federationHandler = handlers.NewFederationHandler(federationKeysRepo, federationCipher, env.BaseURL)
		federationHandler.RegisterPublic(app)
		// Audit-log async writer (Federation v1 F6.3, US-7.4): a single buffered
		// goroutine drains security-relevant federation events into
		// federation_audit_log so logging never blocks a rejection. Started on the
		// cleanup context and drained on shutdown. Never persists secrets/signatures.
		federationAuditWriter = fedaudit.NewWriter(federationAuditRepo, log)
		federationAuditWriter.Start(cleanupCtx)
		// Prometheus collectors (Federation v1 F6.5, US-8.2): the federation_*
		// labeled metrics are registered into a dedicated registry exposed at
		// /metrics. The events endpoint records inbound + signature-failure counters;
		// a periodic feeder sets the outbox-depth gauge + per-peer last-contact gauge.
		federationMetrics = fedmetrics.New()
		federationSvc = fedsvc.NewService(
			sqlDB, projectRepo, federatedProjectRepo, federationKeysRepo, federationInviteRepo, federatedInstanceRepo, federationCipher, env.BaseURL,
		)
		// Joiner-side handshake collaborators (Federation v1 F2.2): the shared
		// peer-key cache (.well-known fetch-once, warmed on join), the outbound
		// signed-request sender, and the well-known fetcher. The owner handshake
		// endpoint is served by the signed group below.
		peerFetch := peerkeys.HTTPFetcher(nil)
		peerCache := peerkeys.NewCache(peerFetch)
		// Warm the in-memory peer-key cache from the durable federated_instances
		// directory at startup (Federation v1 F4.3 review fix). The cache is cold
		// after every restart; without this the first inbound event from a joined
		// peer would trigger a cold-cache .well-known fetch-on-miss, and a transient
		// fetch failure right then could be misread as a key rotation. Warming from
		// the pinned public_key means a real signature mismatch is a genuine key
		// change, not a cold-start fetch error.
		if insts, err := federatedInstanceRepo.List(context.Background()); err != nil {
			log.Warn("federation: warm peer-key cache failed", "err", err.Error())
		} else if len(insts) > 0 {
			warm := make([]peerkeys.Instance, 0, len(insts))
			for _, in := range insts {
				warm = append(warm, peerkeys.Instance{
					InstanceURL: in.InstanceURL, PublicKey: in.PublicKey, DisplayName: in.DisplayName,
				})
			}
			n := peerCache.Warm(warm)
			log.Info("federation: warmed peer-key cache", "peers", n)
		}
		fedSender := client.NewHTTPSender(nil)
		federationSvc.WithJoinDeps(fedSender, peerFetch, peerCache, nil)
		// Key-change detection + manual trust (Federation v1 F5.6b, US-6.4): wire the
		// security-incident log so the inbox signature-check records an incident on a
		// detected key change (AC2) and the manual "Trust new key" admin action can
		// fetch the peer's new .well-known key, overwrite the pinned key (durable +
		// peerCache.Trust), clear the sticky marker, and resolve the incident (AC3).
		// The peer-key cache it re-pins through is the same shared peerCache warmed
		// above, so no inbound event auto-refetches a pinned key (AC1 / R5).
		federationSvc.WithTrustKeyDeps(federationIncidentRepo)
		// Audit log (Federation v1 F6.3, US-7.4): the WRITE side records the
		// control-plane trust actions (handshake/revoke/trust-key/key-change) to the
		// async writer, and the READ side backs the JWT /federation/audit endpoint
		// (list + signature-failure "possible attack" alert). The threshold/window are
		// config-driven (defaults 10 / 60min).
		federationSvc.WithAuditor(federationAuditWriter).
			WithAuditReader(federationAuditRepo, cfg.FederationAuditAlertThreshold(), cfg.FederationAuditAlertWindow())
		// Snapshot bootstrap deps (Federation v1 F2.3): the owner streams a
		// buffer-first NDJSON snapshot under the 15-min token; the joiner applies it
		// into a brand-new local federated project.
		federationSvc.WithSnapshotDeps(taskRepo, sectionRepo, ctxRepo, federationSnapshotRepo)
		federationHandler.WithService(federationSvc)

		// Sync core (Federation v1 F3.2): the inbound event endpoint, the single
		// inbox-apply goroutine, and the outbox publisher worker.
		fedStore := store.New(sqlDB)
		// Surface the real per-peer pending-delivery count on the peers list, and the
		// per-project sync-status indicator (Federation v1 F4.3, US-4.3). The status
		// notifier publishes a ScopeFederation SSE when a peer's health transitions
		// (a key-mismatch is observed), so the owner's open tabs flip the badge.
		// The resume-flush hook (Federation v1 F5.3, US-6.1 AC2) wakes the publisher
		// the instant a paused peer is resumed so its accumulated events push promptly
		// rather than on the next safety-net tick; federationWorker is assigned below,
		// so the closure resolves it lazily at call time.
		federationSvc.WithSyncStore(fedStore).
			WithStatusNotifier(fedsvc.NewHubNotifier(eventsHub)).
			WithResumeFlush(func() {
				if federationWorker != nil {
					federationWorker.Ping()
				}
			}).
			// Direct revoke-event delivery (Federation v1 F5.4, US-6.2 AC1): the revoke
			// is pushed point-to-point to the now-revoked peer once, special-cased past
			// the publisher's revoked-skip fan-out filter. publisher is assigned below,
			// so the closure resolves it lazily at call time.
			WithRevokeSender(func(rctx context.Context, peerURL string, payloads []string) error {
				if federationPublisher == nil {
					return fmt.Errorf("federation publisher not wired")
				}
				return federationPublisher.Push(rctx, peerURL, payloads)
			}).
			// Direct leave-event delivery (Federation v1 F5.5, US-6.3 AC1): when this
			// instance voluntarily leaves a joined project it pushes the federation_leave
			// point-to-point to the owner; the fan-out skips the now-lost project, so this
			// direct push is how the owner learns. Resolved lazily at call time.
			WithLeaveSender(func(rctx context.Context, peerURL string, payloads []string) error {
				if federationPublisher == nil {
					return fmt.Errorf("federation publisher not wired")
				}
				return federationPublisher.Push(rctx, peerURL, payloads)
			})
		// Inbox-apply: applies received events per-field LWW off the HTTP path and
		// publishes a federation-origin SSE refresh (NOT echo-suppressed, US-3.1 AC2).
		// Owner-hub re-broadcast (Federation v1 F5.1, US-5.2 AC2): when THIS instance
		// owns the target project, an apply that changes an entity re-enqueues the
		// relayed event to the outbox (pre-stamped delivered-to-origin so it is never
		// pushed back to where it came from) so the owner fans a peer's edit out to the
		// OTHER peers — the hub-and-spoke leg (W-7). The commit-ping wakes the publisher
		// the instant a relay commits so re-broadcast pushes are immediate (NFR-1.1);
		// federationWorker is assigned below, so the closure resolves it lazily at call
		// time. A joined (non-owner) copy never re-broadcasts.
		applier := inbox.NewApplier(sqlDB, taskRepo, projectRepo, sectionRepo, federatedProjectRepo, fedStore).
			WithReBroadcast(fedStore, env.BaseURL, func() {
				if federationWorker != nil {
					federationWorker.Ping()
				}
			})
		federationQueue = inbox.NewQueue(applier, fedsvc.NewHubNotifier(eventsHub), inbox.NewStoreRecoverer(fedStore), log)
		federationQueue.Start(cleanupCtx)
		// Per-event payload validator (Federation v1 F3.2a): runs before any inbox
		// write so a forged/stale event leaves zero rows.
		eventValidator := inbox.NewDBValidator(sqlDB, federatedProjectRepo, peerCache, nil)
		// Inbound backpressure (Federation v1 F4.4, US-8.3): a per-peer token bucket
		// answers 429 + Retry-After when a peer floods the event endpoint, and a batch
		// over the per-request cap is rejected 413 WHOLE. The limiter is in-memory
		// (resets on restart — accepted v1 gap R18) and holds no DB connection (R1).
		federationRateLimiter = ratelimit.NewPeerLimiter(
			cfg.FederationInboundRatePerMinute(), cfg.FederationInboundBurst(), 10*time.Minute)
		federationHandler.WithEventsDeps(handlers.FederationEventsDeps{
			Store:          fedStore,
			Validator:      eventValidator,
			Queue:          federationQueue,
			Projects:       federatedProjectRepo,
			KeyMismatch:    federationSvc,
			BaseURL:        env.BaseURL,
			RateLimiter:    federationRateLimiter,
			MaxBatchEvents: cfg.FederationMaxBatchEvents(),
			// Refresh the sending peer's last_contact_at on a successful push so a
			// joiner's owner-offline flag clears when the owner reaches it again
			// (Federation v1 F5.6a, US-6.5 AC1/AC3 — the push touchpoint).
			Contact: federatedInstanceRepo,
			// Audit per-event rejections (Federation v1 F6.3, US-7.4 AC1): a
			// verified-but-wrong signature / author-origin mismatch / clock skew records
			// one audit row via the non-blocking writer.
			Auditor: federationAuditWriter,
			// Inbound Prometheus counters (Federation v1 F6.5, US-8.2): received events
			// by peer+result and per-peer signature failures.
			Metrics: federationMetrics,
		})
		// Outbox publisher: signs + POSTs batches to each non-revoked peer, stamps
		// delivered_to. The commit-ping (wired onto the emitter) makes push <5s. The
		// worker chunks delivery by the same event cap (and the 5 MB byte cap) so a
		// single POST never overruns the receiver's inbound 413 limit (US-4.4 AC4),
		// classifies a 429/5xx/4xx per F4.4, dead-letters permanent failures, and
		// persists the per-peer retry gate across restarts (RestoreBackoff on Start).
		publisher := fedsvc.NewPublisher(federatedProjectRepo, federationKeysRepo, federationCipher, fedSender, env.BaseURL, nil)
		// Expose the publisher to the WithRevokeSender closure above (direct revoke
		// delivery, US-6.2 AC1), resolved lazily at call time.
		federationPublisher = publisher
		federationWorker = outbox.NewWorker(fedStore, publisher, publisher, log).
			WithChunkLimits(cfg.FederationMaxBatchEvents(), 0).
			// Outbound Prometheus counter (Federation v1 F6.5, US-8.2):
			// federation_events_sent_total{peer,result}.
			WithSentObserver(federationMetrics)
		federationWorker.Start(cleanupCtx, cfg.FederationPublishInterval())

		// Recovery pull loop (Federation v1 F4.1, US-4.1): the symmetric backstop to
		// the push publisher. On startup + on a ticker it pulls each joined peer's
		// catch-up events from this instance's last_received_hlc cursor (signed by the
		// SAME publisher used for push), runs the SAME F3.2a per-event validator the
		// push handler uses over each event BEFORE recording it (per-event signature /
		// author-origin / clock-skew / membership — the transport response signature
		// authenticates only the relaying peer, not each relayed author, R22/§404),
		// records the survivors durably, feeds them to the same single inbox-apply
		// goroutine push uses, and advances the cursor — so an instance back from a
		// short offline gap auto-catches-up without loss, duplication, or admitting an
		// unauthenticated relayed event. Read-batch → release connection → network GET (R1).
		// 410-stale-pull consumer (Federation v1 F4.2, US-4.2 / US-3.7 AC4 consume
		// half): when a peer's catch-up pull is answered 410 because this instance
		// fell behind the owner's retention, the recovery loop hands the
		// {snapshot_url, as_of_hlc} the 410 carried to this consumer, which re-fetches
		// the owner snapshot and OVERWRITES local project state in one transaction —
		// preserving federation_outbox (the user's unsent edits survive — R3) and
		// stamping the re-bootstrap marker (cutoff X) so the UI surfaces the re-sync
		// banner. On success it publishes a federation-origin SSE refresh so open tabs
		// re-read the project. Without it a 410 would be re-pulled forever, never
		// converging.
		federationReBootstrap := fedsvc.NewReBootstrapConsumer(federationSvc, eventsHub, log)
		// The sink refreshes the peer's last_contact_at on every successful pull so a
		// joiner's owner-offline derivation (Federation v1 F5.6a, US-6.5 AC1/AC3)
		// clears the moment the owner is reachable again.
		recoverySink := recovery.NewStoreSink(fedStore, federationQueue).WithInstances(federatedInstanceRepo)
		federationRecovery = recovery.NewLoop(fedStore, publisher, recoverySink, log).
			WithValidator(eventValidator).
			WithStaleConsumer(federationReBootstrap).
			// Symmetric key-rotation detection (Federation v1 F4.3/F5.6b, US-4.3 AC4 /
			// US-6.4 AC2): a verified-and-rejected per-event signature observed on the
			// PULL leg stamps the SAME sticky key-mismatch marker + durable security
			// incident the push handler does, so a rotation first seen via pull raises
			// the same incident banner + red badge + "Trust new key" affordance — it is
			// not silently swallowed as a buried WARN. Same collaborator as the push
			// handler's KeyMismatch dep above; best-effort, never changes the rejection.
			WithKeyMismatch(federationSvc).
			WithInterval(cfg.FederationPullInterval()).
			WithBatchLimit(cfg.FederationPullBatchLimit())
		federationRecovery.Start(cleanupCtx)

		// Origin-emit hook (Federation v1 F3.1 EmitMutation / F3.3 EmitDeleteCascade):
		// wire the transactional Emitter into the task delete handler so a
		// user-originated delete of a federated task actually emits the op=delete
		// tombstone + child cascade to federation_outbox (US-3.7 AC3) — without this
		// the handler wrote straight to the repo and emitted nothing. EnsureKeys
		// guarantees the install node_id exists before the HLC store stamps its first
		// mutation; the commit-ping wakes the publisher so push is immediate (NFR-1.1).
		nodeID, err := federationSvc.EnsureKeys(context.Background())
		if err != nil {
			log.Error("ensure federation keys", "err", err)
			os.Exit(1)
		}
		federationEmitter := fedsvc.NewEmitter(sqlDB, federationKeysRepo, federationCipher, hlc.NewStore(sqlDB, nodeID), env.BaseURL).
			WithCommitPing(federationWorker.Ping)
		federationTaskMutator = fedsvc.NewTaskMutator(federationEmitter, taskRepo)
		federationProjectMutator = fedsvc.NewProjectMutator(federationEmitter, projectRepo)
		federationSectionMutator = fedsvc.NewSectionMutator(federationEmitter, sectionRepo)
		// Route task creates (inbox/context/project/section/subtask/duplicate) through
		// the same Emitter so a create in a federated project emits its op=create event
		// (US-3.1 AC1). The service holds the creator; the handler holds the mutator for
		// the PATCH/DELETE paths.
		taskSvc.WithFederation(federationTaskMutator)
		// Route task complete/uncomplete/cancel (and recurring advance) through the
		// Emitter so a status change on a federated task emits its op=update event
		// (US-3.2 AC1, TASK A) — the app's core action. The CompleteService keeps the
		// recurrence / troiki invariants; the CompleteMutator adds the transactional
		// emit sidecar (op=create for a new recurring occurrence snapshot).
		completeSvc.WithFederation(fedsvc.NewCompleteMutator(federationEmitter, taskRepo))

		// Runtime-reloadable retention (Federation v1 F6.5, US-8.4): the persisted
		// admin overrides are merged over the config defaults behind an atomic.Pointer
		// so an admin PATCH takes effect on the next GC pass without a restart. The
		// outbox window's EFFECTIVE value stays hard-capped at 30 days (§16.3). Loaded
		// once at startup; the admin handler Reloads on every change.
		federationRetentionSvc = fedsvc.NewRetentionService(
			repo.NewFederationRetentionSettingsRepo(sqlDB),
			cfg.FederationTombstoneRetention(),
			cfg.FederationOutboxRetention(),
			cfg.FederationInboxRetention(),
		)
		if err := federationRetentionSvc.Reload(context.Background()); err != nil {
			log.Warn("federation: load retention settings failed", "err", err.Error())
		}

		// Retention GC (Federation v1 F3.3, US-3.7 AC5): a daily goroutine hard-
		// deletes tombstones past the retention window (resurrection-safety horizon)
		// and purges aged outbox/applied-inbox rows. It runs entirely on the store's
		// own connection (no network I/O), so it never holds the lone connection
		// across anything slow (R1). The retention windows come from the live
		// retention service (F6.5) so a runtime change applies on the next pass.
		fedgc.NewCollector(fedStore, federationRetentionSvc.GCConfig(), log).
			WithConfigSource(federationRetentionSvc.GCConfig).
			// Audit-log retention (Federation v1 F6.3, US-7.4 AC2): the same daily pass
			// hard-deletes audit rows older than the 1-year window.
			WithAudit(federationAuditRepo, cfg.FederationAuditRetention()).
			Start(cleanupCtx, fedgc.DefaultInterval)

		// Per-peer handshake rate limiter (Federation v1 F7.7, NFR-3): a dedicated,
		// tighter token bucket guarding the signed /federation/handshake endpoint
		// against invite brute-force / handshake-flood DoS. It is DISTINCT from the
		// inbound-events limiter so a steady event stream cannot exhaust the handshake
		// budget. In-memory (resets on restart, R18); holds no DB connection (R1).
		federationHandshakeLimiter = ratelimit.NewPeerLimiter(
			cfg.FederationHandshakeRatePerMinute(), cfg.FederationHandshakeBurst(), 10*time.Minute)
		federationHandler.WithHandshakeRateLimiter(federationHandshakeLimiter)

		signed := app.Group("/federation", httpapi.HTTPSignatureMiddleware(httpapi.FederationSignatureDeps{
			Nonces:   nonce.NewCache(),
			PeerKeys: peerCache,
			// Audit transport rejections (Federation v1 F6.3, US-7.4 AC1): a replay /
			// stale-timestamp / digest-mismatch / bad-signature each records one audit
			// row via the non-blocking writer (the rejection itself is unaffected).
			Auditor: federationAuditWriter,
		}))
		federationHandler.RegisterSigned(signed)

		// Prometheus /metrics exposition (Federation v1 F6.5, US-8.2): mounted
		// publicly like /healthz (no secrets in the federation metrics). A periodic
		// gauge feeder sets the live outbox-depth + per-peer last-contact gauges off
		// the request path so a scrape reflects fresh values without a query per
		// scrape. Counters (sent/received/sig-failures) are updated inline at the
		// publisher / events endpoint.
		handlers.NewMetricsHandler(federationMetrics).RegisterPublic(app)
		startFederationMetricsFeeder(cleanupCtx, log, federationMetrics, fedStore, federatedProjectRepo, federatedInstanceRepo)

		// Restore identity check (Federation v1 F6.5, US-8.5 AC2, R27): if this DB was
		// restored under a NEW BASE_URL, the federation mappings are preserved as
		// read-only HISTORY (lost=instance_url_changed) rather than deleted, the
		// keypair is kept, and the user is WARNed to re-invite under the new URL. A
		// restore under the SAME BASE_URL is a no-op (identity intact, no re-handshake).
		if res, err := federationSvc.CheckRestoreIdentity(context.Background()); err != nil {
			log.Warn("federation: restore identity check failed", "err", err.Error())
		} else if res.Changed {
			log.Warn("federation: BASE_URL changed since backup — federation state kept as read-only history; re-invite peers under the new URL",
				"priorInstanceUrl", res.PriorInstanceURL,
				"currentInstanceUrl", env.BaseURL,
				"mappingsMarkedHistory", res.RowsMarked,
			)
		}

		log.Info("federation trust plane enabled")
	} else {
		log.Info("federation disabled (FEDERATION_KEY not set)")
	}

	api := httpapi.RegisterRoutes(app, deps)
	eventsHandler.Register(api)

	authHandler := handlers.NewAuthHandler(userRepo, sessionRepo, jwtIssuer, ipLimiter, env.Argon2Params)
	if totpSvc != nil {
		authHandler.WithTOTP(totpSvc)
	}
	authGroup := app.Group("/auth")
	authHandler.RegisterAuth(authGroup, jwtIssuer)
	if totpSvc != nil {
		totpHandler := handlers.NewTOTPHandler(userRepo, totpSvc, ipLimiter)
		totpHandler.RegisterTOTP(authGroup.Group("", httpapi.AuthMiddleware(jwtIssuer)))
	}
	handlers.NewContextHandler(ctxRepo, projectRepo, taskRepo, taskSvc, env.BaseURL).Register(api.Group("/contexts"))
	handlers.NewLabelHandler(labelRepo, projectRepo, taskRepo, env.BaseURL).Register(api.Group("/labels"))
	// The read-only federated-project guard rejects a local mutation against a
	// joined read-only federated project with 403 (Federation v1 F5.2, US-5.1
	// AC4). It is wired into EVERY task/section mutation entry point. The
	// federated-project repo is always constructed, so when no project is
	// federated the surface lookup is empty and the guard is a no-op — the
	// single-user path is untouched.
	fedReadOnlyGuard := handlers.NewFederationReadOnlyGuard(federatedProjectRepo, taskRepo, sectionRepo)
	handlers.NewSectionHandler(sectionRepo, projectRepo, taskRepo, taskSvc, env.BaseURL).
		WithFederation(federationSectionMutator).WithFederationGuard(fedReadOnlyGuard).Register(api.Group("/sections"))
	handlers.NewProjectHandler(projectRepo, sectionRepo, taskRepo, taskSvc, labelRepo, ctxRepo, pinSvc, federatedProjectRepo, env.BaseURL).
		WithFederation(federationProjectMutator, federationSectionMutator).
		WithOwnerTimeout(cfg.FederationOwnerTimeout()).Register(api)
	handlers.NewInboxHandler(taskRepo, taskSvc, cfg, env.BaseURL).Register(api.Group("/inbox"))
	handlers.NewTaskBulkHandler(completeSvc, moveSvc, groupSvc, env.BaseURL).
		WithFederationGuard(fedReadOnlyGuard).Register(api)
	handlers.NewTaskViewHandler(taskRepo, cfg, env.BaseURL).Register(api)
	handlers.NewTaskActionHandler(taskRepo, completeSvc, planSvc, pinSvc, moveSvc, env.BaseURL).
		WithFederationGuard(fedReadOnlyGuard).Register(api)
	handlers.NewTroikiHandler(troikiSvc, env.BaseURL).Register(api)
	handlers.NewTaskHandler(taskRepo, projectRepo, taskSvc, env.BaseURL).
		WithFederation(federationTaskMutator).WithFederationGuard(fedReadOnlyGuard).
		WithVisibility(federatedProjectRepo).Register(api)
	handlers.NewCommentHandler(commentRepo, taskRepo).Register(api)
	handlers.NewChecklistHandler(checklistRepo, taskRepo).Register(api)
	handlers.NewSearchHandler(searchRepo, env.BaseURL).Register(api)
	handlers.NewMetaHandler(cfg, totpSvc != nil, ctxRepo, projectRepo, labelRepo, taskRepo, userRepo, appSettingsRepo, troikiSvc, env.BaseURL).
		WithFederation(federatedProjectRepo).Register(api)
	handlers.NewStateHandler(userRepo).Register(api)
	handlers.NewSettingsHandler(userRepo).Register(api)
	handlers.NewAppSettingsHandler(appSettingsRepo, labelRepo).Register(api)
	handlers.NewAPITokensHandler(apiTokenRepo, []byte(env.APITokenSalt)).
		Register(api.Group("/api-tokens", httpapi.RequireJWTAuth()))
	handlers.NewSessionsHandler(sessionRepo).
		Register(api.Group("/sessions", httpapi.RequireJWTAuth()))
	handlers.NewBackupHandler(backupSvc).Register(api.Group("", httpapi.RequireJWTAuth()))
	// Federation admin control plane (Federation v1 F1.1) — JWT-only, web owner
	// UI. Registered even when federation is off so the route returns a clear
	// CodeFederationKeyMissing instead of a 404 (federationSvc is nil then). The
	// retention service (F6.5, US-8.4) backs the runtime-reloadable retention
	// GET/PATCH; nil when federation is off (those routes report key-missing).
	federationAdmin := handlers.NewFederationAdminHandler(federationSvc)
	if federationRetentionSvc != nil {
		federationAdmin.WithRetention(federationRetentionSvc, handlers.RetentionDefaults{
			TombstoneDays: int(cfg.FederationTombstoneRetention() / (24 * time.Hour)),
			OutboxDays:    int(cfg.FederationOutboxRetention() / (24 * time.Hour)),
			InboxDays:     int(cfg.FederationInboxRetention() / (24 * time.Hour)),
		}).
			// Federation-aware VACUUM INTO backup (Federation v1 F6.5, US-8.5): the temp
			// VACUUM file is written next to the live DB so it shares the data volume.
			WithBackup(backupSvc, filepath.Dir(env.DataPath))
	}
	federationAdmin.Register(api.Group("", httpapi.RequireJWTAuth()))
	calendarHandler.Register(api.Group("/calendars"))

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
		// Drain the outbox + flush the inbox-apply goroutine on shutdown so events
		// committed just before teardown are delivered/applied (Federation v1 F3.2,
		// NFR-2 at-least-once). The worker's run loop does a best-effort final drain
		// on ctx cancel; Stop blocks until both goroutines have returned.
		if federationWorker != nil {
			federationWorker.Stop()
		}
		if federationRecovery != nil {
			federationRecovery.Stop()
		}
		if federationQueue != nil {
			federationQueue.Stop()
		}
		if federationRateLimiter != nil {
			federationRateLimiter.Stop()
		}
		if federationHandshakeLimiter != nil {
			federationHandshakeLimiter.Stop()
		}
		// Drain the audit writer so events recorded just before teardown are
		// persisted (Federation v1 F6.3, US-7.4). cleanupCancel above stops its run
		// loop; Stop blocks until the final drain has returned.
		if federationAuditWriter != nil {
			federationAuditWriter.Stop()
		}
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

// startFederationMetricsFeeder launches a ctx-cancellable goroutine that refreshes
// the gauge-style federation metrics (Federation v1 F6.5, US-8.2): the live
// outbox-depth gauge and the per-peer seconds-since-last-contact gauge. Counters
// (sent/received/signature-failures) are updated inline at their event source; only
// the gauges need a periodic feed so a scrape reflects fresh values without a query
// per scrape. It runs entirely on the store/repo connection (no network I/O, R1).
func startFederationMetricsFeeder(
	ctx context.Context,
	log *slog.Logger,
	m *fedmetrics.Collectors,
	st *store.Store,
	fedProjects *repo.FederatedProjectRepo,
	instances *repo.FederatedInstanceRepo,
) {
	const interval = 15 * time.Second
	feed := func() {
		if depth, err := st.OutboxDepth(ctx); err != nil {
			log.Warn("federation: metrics outbox depth", "err", err.Error())
		} else {
			m.SetOutboxDepth(depth)
		}
		insts, err := instances.List(ctx)
		if err != nil {
			log.Warn("federation: metrics peer contact", "err", err.Error())
			return
		}
		now := time.Now()
		for _, in := range insts {
			if in.LastContactAt == nil {
				continue
			}
			m.SetPeerLastContactSeconds(in.InstanceURL, now.Sub(*in.LastContactAt).Seconds())
		}
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		feed()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				feed()
			}
		}
	}()
}

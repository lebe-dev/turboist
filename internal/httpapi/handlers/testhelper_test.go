package handlers_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/db"
	fedmetrics "github.com/lebe-dev/turboist/internal/federation/metrics"
	"github.com/lebe-dev/turboist/internal/federation/peerkeys"
	fedstore "github.com/lebe-dev/turboist/internal/federation/store"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/handlers"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
	calendarsvc "github.com/lebe-dev/turboist/internal/service/calendar"
	"github.com/lebe-dev/turboist/internal/service/events"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

// fedTestKey is the FEDERATION_KEY-equivalent used to derive the TokenCipher in
// handler tests; ≥32 bytes like the real env validation requires.
const fedTestKey = "federation-test-cipher-key-32-bytes-min!!"

const testBaseURL = "http://test"

// testNow is the fixed clock the handler test harness uses for owner-offline
// derivation (Federation v1 F5.6a, US-6.5) so a seeded owner last_contact_at can
// be placed deterministically inside or outside the owner-timeout window. It is
// far enough in the future that the pre-existing 2026-01-01 seed dates read as
// "long ago" for any owner-offline test that opts in (the default harness owner
// timeout is the config default, 30 days).
var testNow = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

// apiEnv is the shared test environment for resource handler tests.
type apiEnv struct {
	app          *fiber.App
	db           *sql.DB
	jwt          *auth.JWTIssuer
	ctxs         *repo.ContextRepo
	labels       *repo.LabelRepo
	sections     *repo.ProjectSectionRepo
	projects     *repo.ProjectRepo
	tasks        *repo.TaskRepo
	apiTokens    *repo.APITokenRepo
	apiTokenSalt []byte
	sessions     *repo.SessionRepo
	calendarRepo *repo.CalendarRepo
	eventsHub    *events.Hub
	eventsTix    *events.TicketStore
	fedKeys      *repo.FederationKeysRepo
	fedProjects  *repo.FederatedProjectRepo
	fedInvites   *repo.FederationInviteRepo
	fedInstances *repo.FederatedInstanceRepo
	fedIncidents *repo.FederationSecurityIncidentRepo
	fedAudit     *repo.FederationAuditLogRepo
	fedPeerCache *peerkeys.Cache
	fedSvc       *fedsvc.Service
	fedRetention *fedsvc.RetentionService
	fedMetrics   *fedmetrics.Collectors
	// fedFetch is the stub peer .well-known fetcher the trust-key tests (Federation
	// v1 F5.6b, US-6.4 AC3) override to return the peer's "new" key. Defaults to an
	// error fetcher so a test that does not set it cannot accidentally trust a key.
	fedFetch *func(ctx context.Context, instanceURL string) (*peerkeys.Instance, error)
}

func setupAPIEnv(t *testing.T) *apiEnv {
	t.Helper()
	return buildAPIEnvWithConfig(t, makeTestConfig())
}

func buildAPIEnvWithConfig(t *testing.T, cfg *config.Config) *apiEnv {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.RunMigrations(context.Background(), d); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	issuer := auth.NewJWTIssuer([]byte("test-secret-key-32-bytes-padding!"))

	plabels := repo.NewProjectLabelsRepo(d)
	tlabels := repo.NewTaskLabelsRepo(d)

	ctxs := repo.NewContextRepo(d)
	lbls := repo.NewLabelRepo(d)
	secs := repo.NewProjectSectionRepo(d)
	projs := repo.NewProjectRepo(d, plabels)
	tasks := repo.NewTaskRepo(d, tlabels)
	users := repo.NewUserRepo(d)
	if _, err := users.Create(context.Background(), "admin", "h"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	apiTokens := repo.NewAPITokenRepo(d)
	salt := []byte("test-api-token-salt-32-bytes-pad!")
	sessions := repo.NewSessionRepo(d)

	hub := events.NewHub(slog.Default())
	t.Cleanup(hub.Close)
	tix := events.NewTicketStore()

	deps := httpapi.Deps{
		JWTIssuer:    issuer,
		UserRepo:     users,
		APITokenRepo: apiTokens,
		APITokenSalt: salt,
		EventsHub:    hub,
	}
	app := httpapi.NewApp(deps)

	eventsHandler := handlers.NewEventsHandler(hub, tix)
	eventsHandler.RegisterPublic(app)

	api := httpapi.RegisterRoutes(app, deps)
	eventsHandler.Register(api)

	pinSvc := service.NewPinService(tasks, projs, cfg.MaxPinned)
	appSettings := repo.NewAppSettingsRepo(d)
	autoLabelsSvc := service.NewAutoLabelsService(lbls, appSettings)
	taskSvc := service.NewTaskService(tasks, projs, tlabels, autoLabelsSvc)
	completeSvc := service.NewCompleteService(tasks, projs, users)
	moveSvc := service.NewMoveService(tasks, projs)
	groupSvc := service.NewGroupService(taskSvc, moveSvc, tasks, tlabels)
	planSvc := service.NewPlanService(tasks, ctxs, cfg.Weekly.Limit, cfg.Backlog.Limit)
	searchRepo := repo.NewSearchRepo(tasks, projs)
	// Federation repos are constructed up front so the project handler can resolve
	// the federation surface + enforce the read-only guard (Federation v1 F2.4).
	fedKeys := repo.NewFederationKeysRepo(d)
	fedProjects := repo.NewFederatedProjectRepo(d)
	fedInvites := repo.NewFederationInviteRepo(d)
	fedInstances := repo.NewFederatedInstanceRepo(d)
	// The read-only federated-project guard is wired into every task/section
	// mutation handler (Federation v1 F5.2) so the local FederationGuard tests run
	// against the same enforcement seam production uses.
	fedGuard := handlers.NewFederationReadOnlyGuard(fedProjects, tasks, secs)
	handlers.NewContextHandler(ctxs, projs, tasks, taskSvc, testBaseURL).Register(api.Group("/contexts"))
	handlers.NewLabelHandler(lbls, projs, tasks, testBaseURL).Register(api.Group("/labels"))
	handlers.NewSectionHandler(secs, projs, tasks, taskSvc, testBaseURL).
		WithFederationGuard(fedGuard).Register(api.Group("/sections"))
	// The project handler is wired with the owner-death timeout + a fixed clock so
	// the owner-offline surface (Federation v1 F5.6a, US-6.5 AC1) is derived under
	// production-equivalent conditions in handler tests.
	handlers.NewProjectHandler(projs, secs, tasks, taskSvc, lbls, ctxs, pinSvc, fedProjects, testBaseURL).
		WithOwnerTimeout(cfg.FederationOwnerTimeout()).
		WithClock(func() time.Time { return testNow }).
		Register(api)
	handlers.NewInboxHandler(tasks, taskSvc, cfg, testBaseURL).Register(api.Group("/inbox"))
	handlers.NewTaskBulkHandler(completeSvc, moveSvc, groupSvc, testBaseURL).
		WithFederationGuard(fedGuard).Register(api)
	handlers.NewTaskViewHandler(tasks, cfg, testBaseURL).Register(api)
	handlers.NewTaskActionHandler(tasks, completeSvc, planSvc, pinSvc, moveSvc, testBaseURL).
		WithFederationGuard(fedGuard).Register(api)
	troikiSvc := service.NewTroikiService(tasks, projs, users)
	handlers.NewTroikiHandler(troikiSvc, testBaseURL).Register(api)
	handlers.NewTaskHandler(tasks, projs, taskSvc, testBaseURL).
		WithFederationGuard(fedGuard).
		WithVisibility(fedProjects).Register(api)
	handlers.NewCommentHandler(repo.NewCommentRepo(d), tasks).Register(api)
	handlers.NewChecklistHandler(repo.NewChecklistItemRepo(d), tasks).Register(api)
	handlers.NewSearchHandler(searchRepo, testBaseURL).Register(api)
	handlers.NewMetaHandler(cfg, false, ctxs, projs, lbls, tasks, users, appSettings, troikiSvc, testBaseURL).
		WithFederation(fedProjects).Register(api)
	handlers.NewSettingsHandler(users).Register(api)
	handlers.NewAppSettingsHandler(appSettings, lbls).Register(api)
	handlers.NewAPITokensHandler(apiTokens, salt).
		Register(api.Group("/api-tokens", httpapi.RequireJWTAuth()))
	handlers.NewSessionsHandler(sessions).
		Register(api.Group("/sessions", httpapi.RequireJWTAuth()))
	backupSvc := service.NewBackupService(d)
	handlers.NewBackupHandler(backupSvc).
		Register(api.Group("", httpapi.RequireJWTAuth()))

	// Federation trust plane + admin control plane (Federation v1 F0.3 / F1.1).
	// The sync store + status notifier are wired so the F4.3 status endpoint can
	// surface the overdue-pending signal + publish ScopeFederation on transition.
	fedCipher := crypto.NewTokenCipher(fedTestKey)
	fedIncidents := repo.NewFederationSecurityIncidentRepo(d)
	fedAudit := repo.NewFederationAuditLogRepo(d)
	// A per-env indirection so individual trust-key tests (Federation v1 F5.6b,
	// US-6.4 AC3) can swap in a fetcher returning the peer's "new" key. The default
	// errors so a test that forgets to set it can never silently trust a key.
	fedFetch := new(func(ctx context.Context, instanceURL string) (*peerkeys.Instance, error))
	*fedFetch = func(context.Context, string) (*peerkeys.Instance, error) {
		return nil, fmt.Errorf("test fetcher not configured")
	}
	fedPeerCache := peerkeys.NewCache(func(ctx context.Context, instanceURL string) (*peerkeys.Instance, error) {
		return (*fedFetch)(ctx, instanceURL)
	})
	federationSvc := fedsvc.NewService(d, projs, fedProjects, fedKeys, fedInvites, fedInstances, fedCipher, testBaseURL).
		WithSyncStore(fedstore.New(d)).
		WithStatusNotifier(fedsvc.NewHubNotifier(hub)).
		// Wire the join deps (peer-key cache + .well-known fetcher) and the trust-key
		// incident repo so the manual "Trust new key" admin route can run (US-6.4 AC3).
		WithJoinDeps(nil, func(ctx context.Context, instanceURL string) (*peerkeys.Instance, error) {
			return (*fedFetch)(ctx, instanceURL)
		}, fedPeerCache, nil).
		WithTrustKeyDeps(fedIncidents).
		// Wire the audit read side so the JWT /federation/audit endpoint lists rows
		// and flags signature-failure bursts (Federation v1 F6.3, US-7.4 AC1/AC3). A
		// low threshold + short window keep the alert test deterministic.
		WithAuditReader(fedAudit, 3, time.Hour)

	// F6.5 ops: the live retention service backs the GET/PATCH retention admin
	// endpoints; the metrics collectors back /metrics; the public federation handler
	// serves /federation/health. Defaults mirror the config (90/30/30 days).
	fedRetention := fedsvc.NewRetentionService(repo.NewFederationRetentionSettingsRepo(d),
		90*24*time.Hour, 30*24*time.Hour, 30*24*time.Hour)
	if err := fedRetention.Reload(context.Background()); err != nil {
		t.Fatalf("reload retention: %v", err)
	}
	fedMetrics := fedmetrics.New()
	federationAdmin := handlers.NewFederationAdminHandler(federationSvc).
		WithRetention(fedRetention, handlers.RetentionDefaults{TombstoneDays: 90, OutboxDays: 30, InboxDays: 30}).
		WithBackup(backupSvc, dir)
	federationAdmin.Register(api.Group("", httpapi.RequireJWTAuth()))
	// Public federation handler (.well-known + /federation/health, F6.5) and the
	// /metrics exposition.
	fedPublic := handlers.NewFederationHandler(fedKeys, fedCipher, testBaseURL).WithService(federationSvc)
	fedPublic.RegisterPublic(app)
	handlers.NewMetricsHandler(fedMetrics).RegisterPublic(app)

	calendarRepo := repo.NewCalendarRepo(d)
	calendarSvc := calendarsvc.NewService(calendarRepo, users, testBaseURL, "test-token-key-32bytes-padding!!", slog.Default())
	calendarHandler := handlers.NewCalendarHandler(calendarSvc, calendarRepo, users, testBaseURL, slog.Default())
	calendarHandler.RegisterPublic(app)
	calendarHandler.Register(api.Group("/calendars"))

	return &apiEnv{
		app:          app,
		db:           d,
		jwt:          issuer,
		ctxs:         ctxs,
		labels:       lbls,
		sections:     secs,
		projects:     projs,
		tasks:        tasks,
		apiTokens:    apiTokens,
		apiTokenSalt: salt,
		sessions:     sessions,
		calendarRepo: calendarRepo,
		eventsHub:    hub,
		eventsTix:    tix,
		fedKeys:      fedKeys,
		fedProjects:  fedProjects,
		fedInvites:   fedInvites,
		fedInstances: fedInstances,
		fedIncidents: fedIncidents,
		fedAudit:     fedAudit,
		fedPeerCache: fedPeerCache,
		fedFetch:     fedFetch,
		fedSvc:       federationSvc,
		fedRetention: fedRetention,
		fedMetrics:   fedMetrics,
	}
}

func (e *apiEnv) token(t *testing.T) string {
	t.Helper()
	tok, _, err := e.jwt.Issue(1, 1)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return tok
}

func (e *apiEnv) authedReq(t *testing.T, method, url string, body any) *http.Request {
	t.Helper()
	var buf *bytes.Buffer
	if body != nil {
		b, _ := json.Marshal(body)
		buf = bytes.NewBuffer(b)
	} else {
		buf = bytes.NewBuffer(nil)
	}
	req := httptest.NewRequest(method, url, buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.token(t))
	return req
}

func makeTestConfig() *config.Config {
	loc, _ := time.LoadLocation("UTC")
	return &config.Config{
		Timezone:  "UTC",
		MaxPinned: 5,
		Weekly:    config.WeeklyConfig{Limit: 7},
		Backlog:   config.BacklogConfig{Limit: 100},
		Inbox: config.InboxConfig{
			WarnThreshold: 5,
			OverflowTask:  config.OverflowTask{Title: "Clear inbox", Priority: "high"},
		},
		DayParts: map[string]config.DayPart{
			"morning": {Start: 6, End: 12},
		},
		Location: loc,
	}
}

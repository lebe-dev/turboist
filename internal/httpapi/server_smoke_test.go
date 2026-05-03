package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/handlers"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
	"golang.org/x/time/rate"
)

// smokeApp assembles the full application stack against a real SQLite database.
func smokeApp(t *testing.T) *smokeEnv {
	t.Helper()
	dir := t.TempDir()
	sqlDB, err := db.Open(filepath.Join(dir, "smoke.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.RunMigrations(context.Background(), sqlDB); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	loc, _ := time.LoadLocation("UTC")
	cfg := &config.Config{
		Timezone:  "UTC",
		MaxPinned: 5,
		Weekly:    config.WeeklyConfig{Limit: 7},
		Backlog:   config.BacklogConfig{Limit: 100},
		Inbox: config.InboxConfig{
			WarnThreshold: 5,
			OverflowTask:  config.OverflowTask{Title: "Clear inbox", Priority: "high"},
		},
		DayParts:   map[string]config.DayPart{"morning": {Start: 6, End: 12}},
		AutoLabels: []config.AutoLabel{},
		Location:   loc,
	}
	const baseURL = "http://test"

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

	jwtIssuer := auth.NewJWTIssuer([]byte("smoke-secret-key-32-bytes-padding"))
	ipLimiter := auth.NewIPLimiter(rate.Every(time.Second), 100, time.Minute)

	pinSvc := service.NewPinService(taskRepo, projectRepo, cfg.MaxPinned)
	autoLabelsSvc := service.NewAutoLabelsService(labelRepo, cfg)
	taskSvc := service.NewTaskService(taskRepo, tlabels, autoLabelsSvc)
	completeSvc := service.NewCompleteService(taskRepo, userRepo)
	moveSvc := service.NewMoveService(taskRepo)
	planSvc := service.NewPlanService(taskRepo, ctxRepo, cfg.Weekly.Limit, cfg.Backlog.Limit)

	deps := httpapi.Deps{
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
		BaseURL:     baseURL,
	}
	app := httpapi.NewApp(deps)
	api := httpapi.RegisterRoutes(app, deps)

	handlers.NewAuthHandler(userRepo, sessionRepo, jwtIssuer, ipLimiter, auth.DefaultArgon2Params()).RegisterAuth(app.Group("/auth"), jwtIssuer)
	handlers.NewContextHandler(ctxRepo, projectRepo, taskRepo, taskSvc, baseURL).Register(api.Group("/contexts"))
	handlers.NewLabelHandler(labelRepo, projectRepo, taskRepo, baseURL).Register(api.Group("/labels"))
	handlers.NewSectionHandler(sectionRepo, projectRepo, taskRepo, taskSvc, baseURL).Register(api.Group("/sections"))
	handlers.NewProjectHandler(projectRepo, sectionRepo, taskRepo, taskSvc, labelRepo, ctxRepo, pinSvc, baseURL).Register(api)
	handlers.NewInboxHandler(taskRepo, taskSvc, cfg, baseURL).Register(api.Group("/inbox"))
	handlers.NewTaskBulkHandler(completeSvc, moveSvc, baseURL).Register(api)
	handlers.NewTaskViewHandler(taskRepo, cfg, baseURL).Register(api)
	handlers.NewTaskActionHandler(taskRepo, completeSvc, planSvc, pinSvc, moveSvc, baseURL).Register(api)
	handlers.NewTaskHandler(taskRepo, taskSvc, baseURL).Register(api)
	handlers.NewSearchHandler(searchRepo, baseURL).Register(api)
	handlers.NewMetaHandler(cfg).Register(api)
	handlers.NewStateHandler(userRepo).Register(api)

	return &smokeEnv{app: app, jwt: jwtIssuer}
}

type smokeEnv struct {
	app *fiber.App
	jwt *auth.JWTIssuer
}

func (e *smokeEnv) do(t *testing.T, method, url string, body any, token string) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, url, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := e.app.Test(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	return resp
}

func decode(t *testing.T, resp *http.Response, dst any) {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func TestSmoke_FullFlow(t *testing.T) {
	e := smokeApp(t)

	// healthz — no auth required
	resp := e.do(t, "GET", "/healthz", nil, "")
	if resp.StatusCode != 200 {
		t.Fatalf("healthz: got %d, want 200", resp.StatusCode)
	}

	// setup — create the single user
	resp = e.do(t, "POST", "/auth/setup", map[string]string{
		"username":   "admin",
		"password":   "password123",
		"clientKind": "cli",
	}, "")
	if resp.StatusCode != 200 {
		t.Fatalf("setup: got %d, want 200", resp.StatusCode)
	}

	var setupResp struct {
		Access string `json:"access"`
	}
	decode(t, resp, &setupResp)
	if setupResp.Access == "" {
		t.Fatal("setup: empty access token")
	}

	// second setup must return 410
	resp = e.do(t, "POST", "/auth/setup", map[string]string{
		"username":   "admin2",
		"password":   "password123",
		"clientKind": "cli",
	}, "")
	if resp.StatusCode != 410 {
		t.Fatalf("second setup: got %d, want 410", resp.StatusCode)
	}

	// login
	resp = e.do(t, "POST", "/auth/login", map[string]string{
		"username":   "admin",
		"password":   "password123",
		"clientKind": "cli",
	}, "")
	if resp.StatusCode != 200 {
		t.Fatalf("login: got %d, want 200", resp.StatusCode)
	}
	var loginResp struct {
		Access string `json:"access"`
	}
	decode(t, resp, &loginResp)
	tok := loginResp.Access

	// GET /auth/me
	resp = e.do(t, "GET", "/auth/me", nil, tok)
	if resp.StatusCode != 200 {
		t.Fatalf("me: got %d, want 200", resp.StatusCode)
	}

	// create context
	resp = e.do(t, "POST", "/api/v1/contexts", map[string]string{"name": "Work", "color": "blue"}, tok)
	if resp.StatusCode != 201 {
		t.Fatalf("create context: got %d, want 201", resp.StatusCode)
	}
	var ctxResp struct {
		ID int64 `json:"id"`
	}
	decode(t, resp, &ctxResp)
	if ctxResp.ID == 0 {
		t.Fatal("create context: zero id")
	}

	// create project in context
	resp = e.do(t, "POST", "/api/v1/contexts/"+itoa(ctxResp.ID)+"/projects", map[string]any{
		"title": "My Project",
		"color": "blue",
	}, tok)
	if resp.StatusCode != 201 {
		t.Fatalf("create project: got %d, want 201", resp.StatusCode)
	}
	var projResp struct {
		ID int64 `json:"id"`
	}
	decode(t, resp, &projResp)
	if projResp.ID == 0 {
		t.Fatal("create project: zero id")
	}

	// create task in context
	resp = e.do(t, "POST", "/api/v1/contexts/"+itoa(ctxResp.ID)+"/tasks", map[string]any{
		"title": "My Task",
	}, tok)
	if resp.StatusCode != 201 {
		t.Fatalf("create task: got %d, want 201", resp.StatusCode)
	}
	var taskResp struct {
		ID int64 `json:"id"`
	}
	decode(t, resp, &taskResp)
	if taskResp.ID == 0 {
		t.Fatal("create task: zero id")
	}

	// complete task
	resp = e.do(t, "POST", "/api/v1/tasks/"+itoa(taskResp.ID)+"/complete", nil, tok)
	if resp.StatusCode != 200 {
		t.Fatalf("complete task: got %d, want 200", resp.StatusCode)
	}

	// check views
	for _, view := range []string{"today", "tomorrow", "overdue", "week", "backlog"} {
		resp = e.do(t, "GET", "/api/v1/tasks/"+view, nil, tok)
		if resp.StatusCode != 200 {
			t.Errorf("view %s: got %d, want 200", view, resp.StatusCode)
		} else {
			_ = resp.Body.Close()
		}
	}

	// logout
	resp = e.do(t, "POST", "/auth/logout", nil, tok)
	if resp.StatusCode != 204 {
		t.Fatalf("logout: got %d, want 204", resp.StatusCode)
	}
}

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}

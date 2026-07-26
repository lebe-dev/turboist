package handlers_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/handlers"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
)

// loggingEnv wraps apiEnv with the captureHandler so tests can inspect emitted
// records. It uses the same wiring as buildAPIEnvWithConfig but injects the
// capture handler through deps.Log so the request-scoped logger (set by
// RequestIDMiddleware) carries it down into every handler.
type loggingEnv struct {
	*apiEnv
	cap *captureHandler
}

func setupAPIEnvWithLog(t *testing.T) *loggingEnv {
	t.Helper()
	cfg := makeTestConfig()
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

	cap := newCaptureHandler()
	logger := slog.New(cap)

	deps := httpapi.Deps{Log: logger, JWTIssuer: issuer, UserRepo: users, APITokenRepo: apiTokens, APITokenSalt: salt}
	app := httpapi.NewApp(deps)
	api := httpapi.RegisterRoutes(app, deps)

	pinSvc := service.NewPinService(tasks, projs, cfg.MaxPinned)
	appSettings := repo.NewAppSettingsRepo(d)
	autoLabelsSvc := service.NewAutoLabelsService(lbls, appSettings)
	taskSvc := service.NewTaskService(tasks, projs, tlabels, autoLabelsSvc)
	completeSvc := service.NewCompleteService(tasks, projs, users)
	moveSvc := service.NewMoveService(tasks, projs)
	groupSvc := service.NewGroupService(taskSvc, moveSvc, tasks, tlabels)
	planSvc := service.NewPlanService(tasks, ctxs, cfg.Weekly.Limit, cfg.Backlog.Limit)
	searchRepo := repo.NewSearchRepo(tasks, projs)
	handlers.NewContextHandler(ctxs, projs, tasks, taskSvc, testBaseURL).Register(api.Group("/contexts"))
	handlers.NewLabelHandler(lbls, projs, tasks, testBaseURL).Register(api.Group("/labels"))
	handlers.NewSectionHandler(secs, projs, tasks, taskSvc, testBaseURL).Register(api.Group("/sections"))
	handlers.NewProjectHandler(projs, secs, tasks, taskSvc, lbls, ctxs, pinSvc, testBaseURL).Register(api)
	handlers.NewInboxHandler(tasks, taskSvc, cfg, testBaseURL).Register(api.Group("/inbox"))
	handlers.NewTaskBulkHandler(completeSvc, moveSvc, groupSvc, taskSvc, testBaseURL).Register(api)
	troikiSvc := service.NewTroikiService(tasks, projs, users)
	handlers.NewTaskViewHandler(tasks, users, troikiSvc, cfg, testBaseURL).Register(api)
	handlers.NewTaskActionHandler(tasks, completeSvc, planSvc, pinSvc, moveSvc, testBaseURL).Register(api)
	handlers.NewTroikiHandler(troikiSvc, testBaseURL).Register(api)
	handlers.NewTaskHandler(tasks, projs, taskSvc, testBaseURL).Register(api)
	handlers.NewSearchHandler(searchRepo, testBaseURL).Register(api)
	handlers.NewMetaHandler(cfg, false, ctxs, projs, lbls, tasks, users, appSettings, troikiSvc, testBaseURL).Register(api)
	handlers.NewSettingsHandler(users).Register(api)
	handlers.NewStateHandler(users).Register(api)
	handlers.NewAppSettingsHandler(appSettings, lbls, projs).Register(api)
	handlers.NewAPITokensHandler(apiTokens, salt).
		Register(api.Group("/api-tokens", httpapi.RequireJWTAuth()))
	handlers.NewBackupHandler(service.NewBackupService(d)).
		Register(api.Group("", httpapi.RequireJWTAuth()))

	env := &apiEnv{
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
	}
	return &loggingEnv{apiEnv: env, cap: cap}
}

// hasRecord reports whether any captured record matches the given level + message.
func hasRecord(records []slog.Record, level slog.Level, msg string) bool {
	_, ok := findRecord(records, level, msg)
	return ok
}

// createContextForTest creates a context row and returns its ID; used as a
// fixture for handler tests that need an existing parent.
func createContextForTest(t *testing.T, app *fiber.App, jwt string) int64 {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/contexts/",
		jsonBody(map[string]any{"name": "Work", "color": "blue"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwt)
	resp, body := doReq(t, app, req)
	if resp.StatusCode != 201 {
		t.Fatalf("seed context: status %d body %s", resp.StatusCode, body)
	}
	var out struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("seed context parse: %v", err)
	}
	return out.ID
}

// --- contexts.go ---

func TestContextHandler_Create_LogsInfo(t *testing.T) {
	env := setupAPIEnvWithLog(t)
	req := env.authedReq(t, http.MethodPost, "/api/v1/contexts/", map[string]any{
		"name": "Work", "color": "blue",
	})
	resp, _ := doReq(t, env.app, req)
	if resp.StatusCode != 201 {
		t.Fatalf("status: got %d, want 201", resp.StatusCode)
	}
	if !hasRecord(env.cap.snapshot(), slog.LevelInfo, "handler.Context.Create: ok") {
		t.Error("missing INFO record handler.Context.Create: ok")
	}
}

func TestContextHandler_Create_BadColor_LogsWarn(t *testing.T) {
	env := setupAPIEnvWithLog(t)
	req := env.authedReq(t, http.MethodPost, "/api/v1/contexts/", map[string]any{
		"name": "Work", "color": "neon",
	})
	resp, _ := doReq(t, env.app, req)
	if resp.StatusCode != 400 {
		t.Fatalf("status: got %d, want 400", resp.StatusCode)
	}
	if !hasRecord(env.cap.snapshot(), slog.LevelWarn, "handler.Context.Create: validation failed") {
		t.Error("missing WARN validation record for handler.Context.Create")
	}
}

// --- labels.go ---

func TestLabelHandler_Create_LogsInfo(t *testing.T) {
	env := setupAPIEnvWithLog(t)
	req := env.authedReq(t, http.MethodPost, "/api/v1/labels/", map[string]any{
		"name": "bug", "color": "red",
	})
	resp, _ := doReq(t, env.app, req)
	if resp.StatusCode != 201 {
		t.Fatalf("status: got %d, want 201", resp.StatusCode)
	}
	if !hasRecord(env.cap.snapshot(), slog.LevelInfo, "handler.Label.Create: ok") {
		t.Error("missing INFO handler.Label.Create: ok")
	}
}

func TestLabelHandler_Create_MissingName_LogsWarn(t *testing.T) {
	env := setupAPIEnvWithLog(t)
	req := env.authedReq(t, http.MethodPost, "/api/v1/labels/", map[string]any{"color": "red"})
	resp, _ := doReq(t, env.app, req)
	if resp.StatusCode != 400 {
		t.Fatalf("status: got %d, want 400", resp.StatusCode)
	}
	if !hasRecord(env.cap.snapshot(), slog.LevelWarn, "handler.Label.Create: validation failed") {
		t.Error("missing WARN validation record for handler.Label.Create")
	}
}

// --- projects.go ---

func TestProjectHandler_Create_LogsInfo(t *testing.T) {
	env := setupAPIEnvWithLog(t)
	jwt := env.token(t)
	ctxID := createContextForTest(t, env.app, jwt)

	req := env.authedReq(t, http.MethodPost,
		"/api/v1/contexts/"+itoa64(ctxID)+"/projects",
		map[string]any{"title": "P1", "color": "blue"})
	resp, body := doReq(t, env.app, req)
	if resp.StatusCode != 201 {
		t.Fatalf("status: got %d, want 201 body=%s", resp.StatusCode, body)
	}
	if !hasRecord(env.cap.snapshot(), slog.LevelInfo, "handler.Project.Create: ok") {
		t.Error("missing INFO handler.Project.Create: ok")
	}
}

func TestProjectHandler_Create_BadTitle_LogsWarn(t *testing.T) {
	env := setupAPIEnvWithLog(t)
	jwt := env.token(t)
	ctxID := createContextForTest(t, env.app, jwt)

	req := env.authedReq(t, http.MethodPost,
		"/api/v1/contexts/"+itoa64(ctxID)+"/projects",
		map[string]any{"title": ""})
	resp, _ := doReq(t, env.app, req)
	if resp.StatusCode != 400 {
		t.Fatalf("status: got %d, want 400", resp.StatusCode)
	}
	if !hasRecord(env.cap.snapshot(), slog.LevelWarn, "handler.Project.Create: validation failed") {
		t.Error("missing WARN validation record for handler.Project.Create")
	}
}

// --- sections.go ---

func TestSectionHandler_Reorder_NegativePosition_LogsWarn(t *testing.T) {
	env := setupAPIEnvWithLog(t)
	req := env.authedReq(t, http.MethodPost, "/api/v1/sections/1/reorder",
		map[string]any{"position": -1})
	resp, _ := doReq(t, env.app, req)
	if resp.StatusCode != 400 {
		t.Fatalf("status: got %d, want 400", resp.StatusCode)
	}
	if !hasRecord(env.cap.snapshot(), slog.LevelWarn, "handler.Section.Reorder: validation failed") {
		t.Error("missing WARN validation record for handler.Section.Reorder")
	}
}

// --- tasks.go ---

func TestTaskHandler_Patch_BadPriority_LogsWarn(t *testing.T) {
	env := setupAPIEnvWithLog(t)
	// Seed an inbox task.
	jwt := env.token(t)
	req0 := httptest.NewRequest(http.MethodPost, "/api/v1/inbox/tasks",
		jsonBody(map[string]any{"title": "T"}))
	req0.Header.Set("Content-Type", "application/json")
	req0.Header.Set("Authorization", "Bearer "+jwt)
	resp0, body := doReq(t, env.app, req0)
	if resp0.StatusCode != 201 {
		t.Fatalf("seed task: %d body %s", resp0.StatusCode, body)
	}
	var seeded struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(body, &seeded); err != nil {
		t.Fatalf("parse: %v", err)
	}

	req := env.authedReq(t, http.MethodPatch, "/api/v1/tasks/"+itoa64(seeded.ID),
		map[string]any{"priority": "nope"})
	resp, _ := doReq(t, env.app, req)
	if resp.StatusCode != 400 {
		t.Fatalf("status: got %d, want 400", resp.StatusCode)
	}
	if !hasRecord(env.cap.snapshot(), slog.LevelWarn, "handler.Task.Patch: validation failed") {
		t.Error("missing WARN validation record for handler.Task.Patch")
	}
}

// --- inbox.go ---

func TestInboxHandler_CreateTask_LogsInfo(t *testing.T) {
	env := setupAPIEnvWithLog(t)
	req := env.authedReq(t, http.MethodPost, "/api/v1/inbox/tasks",
		map[string]any{"title": "T1"})
	resp, _ := doReq(t, env.app, req)
	if resp.StatusCode != 201 {
		t.Fatalf("status: got %d, want 201", resp.StatusCode)
	}
	// doCreateTask emits handler.Task.Create on success.
	if !hasRecord(env.cap.snapshot(), slog.LevelInfo, "handler.Task.Create: ok") {
		t.Error("missing INFO handler.Task.Create: ok for inbox task")
	}
}

func TestInboxHandler_CreateTask_MissingTitle_LogsWarn(t *testing.T) {
	env := setupAPIEnvWithLog(t)
	req := env.authedReq(t, http.MethodPost, "/api/v1/inbox/tasks", map[string]any{})
	resp, _ := doReq(t, env.app, req)
	if resp.StatusCode != 400 {
		t.Fatalf("status: got %d, want 400", resp.StatusCode)
	}
	if !hasRecord(env.cap.snapshot(), slog.LevelWarn, "handler.Inbox.CreateTask: validation failed") {
		t.Error("missing WARN handler.Inbox.CreateTask validation")
	}
}

// --- task_actions.go ---

func TestTaskActions_Complete_LogsInfo(t *testing.T) {
	env := setupAPIEnvWithLog(t)
	// Seed a task in the inbox.
	id := seedInboxTask(t, env.apiEnv, "X")
	req := env.authedReq(t, http.MethodPost, "/api/v1/tasks/"+itoa64(id)+"/complete", nil)
	resp, _ := doReq(t, env.app, req)
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	if !hasRecord(env.cap.snapshot(), slog.LevelInfo, "handler.Task.Complete: ok") {
		t.Error("missing INFO handler.Task.Complete: ok")
	}
}

func TestTaskActions_Plan_InvalidState_LogsWarn(t *testing.T) {
	env := setupAPIEnvWithLog(t)
	id := seedInboxTask(t, env.apiEnv, "Y")
	req := env.authedReq(t, http.MethodPost, "/api/v1/tasks/"+itoa64(id)+"/plan",
		map[string]any{"state": "bogus"})
	resp, _ := doReq(t, env.app, req)
	if resp.StatusCode != 400 {
		t.Fatalf("status: got %d, want 400", resp.StatusCode)
	}
	if !hasRecord(env.cap.snapshot(), slog.LevelWarn, "handler.Task.Plan: validation failed") {
		t.Error("missing WARN handler.Task.Plan validation")
	}
}

// --- task_bulk.go ---

func TestTaskBulk_BulkComplete_TooMany_LogsWarn(t *testing.T) {
	env := setupAPIEnvWithLog(t)
	ids := make([]int64, 101)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	req := env.authedReq(t, http.MethodPost, "/api/v1/tasks/bulk/complete",
		map[string]any{"ids": ids})
	resp, _ := doReq(t, env.app, req)
	if resp.StatusCode != 400 {
		t.Fatalf("status: got %d, want 400", resp.StatusCode)
	}
	if !hasRecord(env.cap.snapshot(), slog.LevelWarn, "handler.Task.BulkComplete: validation failed") {
		t.Error("missing WARN handler.Task.BulkComplete validation")
	}
}

// --- task_views.go (read-only — exercise a code path to confirm no panic) ---

func TestTaskViews_Today_OK(t *testing.T) {
	env := setupAPIEnvWithLog(t)
	req := env.authedReq(t, http.MethodGet, "/api/v1/tasks/today", nil)
	resp, _ := doReq(t, env.app, req)
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
}

// --- troiki.go ---

func TestTroiki_SetCategory_InvalidCategory_LogsWarn(t *testing.T) {
	env := setupAPIEnvWithLog(t)
	req := env.authedReq(t, http.MethodPost, "/api/v1/projects/999/troiki",
		map[string]any{"category": "bogus"})
	resp, _ := doReq(t, env.app, req)
	if resp.StatusCode != 400 {
		t.Fatalf("status: got %d, want 400", resp.StatusCode)
	}
	if !hasRecord(env.cap.snapshot(), slog.LevelWarn, "handler.Troiki.SetCategory: validation failed") {
		t.Error("missing WARN handler.Troiki.SetCategory validation")
	}
}

// --- search.go ---

func TestSearch_QueryTooShort_LogsWarn(t *testing.T) {
	env := setupAPIEnvWithLog(t)
	req := env.authedReq(t, http.MethodGet, "/api/v1/search?q=a", nil)
	resp, _ := doReq(t, env.app, req)
	if resp.StatusCode != 400 {
		t.Fatalf("status: got %d, want 400", resp.StatusCode)
	}
	if !hasRecord(env.cap.snapshot(), slog.LevelWarn, "handler.Search: validation failed") {
		t.Error("missing WARN handler.Search validation")
	}
}

// --- settings.go ---

func TestSettings_Patch_BadLocale_LogsWarn(t *testing.T) {
	env := setupAPIEnvWithLog(t)
	req := env.authedReq(t, http.MethodPatch, "/api/v1/settings",
		map[string]any{"locale": "klingon"})
	resp, _ := doReq(t, env.app, req)
	if resp.StatusCode != 400 {
		t.Fatalf("status: got %d, want 400", resp.StatusCode)
	}
	if !hasRecord(env.cap.snapshot(), slog.LevelWarn, "handler.Settings.Patch: validation failed") {
		t.Error("missing WARN handler.Settings.Patch validation")
	}
}

// --- state.go ---

func TestState_Patch_InvalidJSON_LogsWarn(t *testing.T) {
	env := setupAPIEnvWithLog(t)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/state",
		strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+env.token(t))
	resp, _ := doReq(t, env.app, req)
	if resp.StatusCode != 400 {
		t.Fatalf("status: got %d, want 400", resp.StatusCode)
	}
	if !hasRecord(env.cap.snapshot(), slog.LevelWarn, "handler.State.Patch: validation failed") {
		t.Error("missing WARN handler.State.Patch validation")
	}
}

// --- app_settings.go ---

func TestAppSettings_PutAutoLabels_EmptyMask_LogsWarn(t *testing.T) {
	env := setupAPIEnvWithLog(t)
	req := env.authedReq(t, http.MethodPut, "/api/v1/app-settings/auto-labels",
		map[string]any{
			"autoLabels": []map[string]any{
				{"mask": "", "labelIds": []int{1}, "ignoreCase": false},
			},
		})
	resp, _ := doReq(t, env.app, req)
	if resp.StatusCode != 400 {
		t.Fatalf("status: got %d, want 400", resp.StatusCode)
	}
	if !hasRecord(env.cap.snapshot(), slog.LevelWarn, "handler.AppSettings.PutAutoLabels: validation failed") {
		t.Error("missing WARN handler.AppSettings.PutAutoLabels validation")
	}
}

// --- api_tokens.go ---

func TestAPITokens_Create_LogsInfoAndNoTokenValue(t *testing.T) {
	env := setupAPIEnvWithLog(t)
	req := env.authedReq(t, http.MethodPost, "/api/v1/api-tokens/",
		map[string]any{"name": "cli", "scopes": []string{"*"}})
	resp, body := doReq(t, env.app, req)
	if resp.StatusCode != 201 {
		t.Fatalf("status: got %d, want 201 body %s", resp.StatusCode, body)
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if out.Token == "" {
		t.Fatal("expected non-empty token in response")
	}

	rec, ok := findRecord(env.cap.snapshot(), slog.LevelInfo, "handler.APIToken.Create: ok")
	if !ok {
		t.Fatal("missing INFO handler.APIToken.Create: ok")
	}
	// Guarantee the plaintext token is not present in any attribute of the log.
	rec.Attrs(func(a slog.Attr) bool {
		if strings.Contains(a.Value.String(), out.Token) {
			t.Errorf("plaintext token leaked in log attr %q", a.Key)
		}
		return true
	})
}

func TestAPITokens_Create_BadName_LogsWarn(t *testing.T) {
	env := setupAPIEnvWithLog(t)
	req := env.authedReq(t, http.MethodPost, "/api/v1/api-tokens/",
		map[string]any{"name": ""})
	resp, _ := doReq(t, env.app, req)
	if resp.StatusCode != 400 {
		t.Fatalf("status: got %d, want 400", resp.StatusCode)
	}
	if !hasRecord(env.cap.snapshot(), slog.LevelWarn, "handler.APIToken.Create: validation failed") {
		t.Error("missing WARN handler.APIToken.Create validation")
	}
}

// --- backup.go ---

func TestBackup_Export_LogsInfoWithSize(t *testing.T) {
	env := setupAPIEnvWithLog(t)
	req := env.authedReq(t, http.MethodGet, "/api/v1/backup", nil)
	resp, _ := doReq(t, env.app, req)
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	rec, ok := findRecord(env.cap.snapshot(), slog.LevelInfo, "handler.Backup.Export: ok")
	if !ok {
		t.Fatal("missing INFO handler.Backup.Export: ok")
	}
	var hasSize bool
	rec.Attrs(func(a slog.Attr) bool {
		if a.Key == "raw_bytes" || a.Key == "gzip_bytes" {
			if a.Value.Int64() > 0 {
				hasSize = true
			}
		}
		return true
	})
	if !hasSize {
		t.Error("Backup.Export log missing positive size attr")
	}
}

func TestBackup_Restore_EmptyBody_LogsWarn(t *testing.T) {
	env := setupAPIEnvWithLog(t)
	req := env.authedReq(t, http.MethodPost, "/api/v1/restore", nil)
	resp, _ := doReq(t, env.app, req)
	if resp.StatusCode != 400 {
		t.Fatalf("status: got %d, want 400", resp.StatusCode)
	}
	if !hasRecord(env.cap.snapshot(), slog.LevelWarn, "handler.Backup.Restore: validation failed") {
		t.Error("missing WARN handler.Backup.Restore validation")
	}
}

// --- helpers.go (parseID) ---

func TestParseID_LogsWarnOnInvalid(t *testing.T) {
	env := setupAPIEnvWithLog(t)
	req := env.authedReq(t, http.MethodGet, "/api/v1/tasks/notanid", nil)
	resp, _ := doReq(t, env.app, req)
	if resp.StatusCode != 400 {
		t.Fatalf("status: got %d, want 400", resp.StatusCode)
	}
	if !hasRecord(env.cap.snapshot(), slog.LevelWarn, "handler.parseID: validation failed") {
		t.Error("missing WARN handler.parseID validation")
	}
}

func itoa64(n int64) string {
	return strconv.FormatInt(n, 10)
}

// seedInboxTask creates a task in the inbox via the repo layer and returns the id.
func seedInboxTask(t *testing.T, e *apiEnv, title string) int64 {
	t.Helper()
	inboxID := int64(1)
	task, err := e.tasks.Create(context.Background(), repo.CreateTask{
		Placement: repo.Placement{InboxID: &inboxID},
		Title:     title,
	})
	if err != nil {
		t.Fatalf("seed task: %v", err)
	}
	return task.ID
}

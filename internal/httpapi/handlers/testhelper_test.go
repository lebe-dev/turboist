package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
)

const testBaseURL = "http://test"

// apiEnv is the shared test environment for resource handler tests.
type apiEnv struct {
	app      *fiber.App
	jwt      *auth.JWTIssuer
	ctxs     *repo.ContextRepo
	labels   *repo.LabelRepo
	sections *repo.ProjectSectionRepo
	projects *repo.ProjectRepo
	tasks    *repo.TaskRepo
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

	deps := httpapi.Deps{JWTIssuer: issuer}
	app := httpapi.NewApp(deps)
	api := httpapi.RegisterRoutes(app, deps)

	pinSvc := service.NewPinService(tasks, projs, cfg.MaxPinned)
	autoLabelsSvc := service.NewAutoLabelsService(lbls, cfg)
	taskSvc := service.NewTaskService(tasks, tlabels, autoLabelsSvc)
	completeSvc := service.NewCompleteService(tasks)
	moveSvc := service.NewMoveService(tasks)
	planSvc := service.NewPlanService(tasks, ctxs, cfg.Weekly.Limit, cfg.Backlog.Limit)
	searchRepo := repo.NewSearchRepo(tasks, projs)
	handlers.NewContextHandler(ctxs, projs, tasks, taskSvc, testBaseURL).Register(api.Group("/contexts"))
	handlers.NewLabelHandler(lbls, projs, tasks, testBaseURL).Register(api.Group("/labels"))
	handlers.NewSectionHandler(secs, projs, tasks, taskSvc, testBaseURL).Register(api.Group("/sections"))
	handlers.NewProjectHandler(projs, secs, tasks, taskSvc, lbls, ctxs, pinSvc, testBaseURL).Register(api)
	handlers.NewInboxHandler(tasks, taskSvc, cfg, testBaseURL).Register(api.Group("/inbox"))
	handlers.NewTaskBulkHandler(completeSvc, moveSvc, testBaseURL).Register(api)
	handlers.NewTaskViewHandler(tasks, cfg, testBaseURL).Register(api)
	handlers.NewTaskActionHandler(tasks, completeSvc, planSvc, pinSvc, moveSvc, testBaseURL).Register(api)
	handlers.NewTaskHandler(tasks, taskSvc, testBaseURL).Register(api)
	handlers.NewSearchHandler(searchRepo, testBaseURL).Register(api)
	handlers.NewMetaHandler(cfg).Register(api)

	return &apiEnv{
		app:      app,
		jwt:      issuer,
		ctxs:     ctxs,
		labels:   lbls,
		sections: secs,
		projects: projs,
		tasks:    tasks,
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
		AutoLabels: []config.AutoLabel{},
		Location:   loc,
	}
}

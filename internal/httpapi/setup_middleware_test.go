package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v3"

	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/repo"
)

func TestSetupCheckMiddleware_BlocksWhenNoUser(t *testing.T) {
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.RunMigrations(context.Background(), d); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	users := repo.NewUserRepo(d)

	app := httpapi.NewApp(httpapi.Deps{})
	api := app.Group("/api/v1", httpapi.SetupCheckMiddleware(users))
	api.Get("/probe", func(c fiber.Ctx) error { return c.SendString("ok") })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/probe", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != 503 {
		t.Fatalf("status: got %d, want 503", resp.StatusCode)
	}
}

func TestSetupCheckMiddleware_PassesAfterSetup(t *testing.T) {
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.RunMigrations(context.Background(), d); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	users := repo.NewUserRepo(d)
	if _, err := users.Create(context.Background(), "admin", "hash"); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	app := httpapi.NewApp(httpapi.Deps{})
	api := app.Group("/api/v1", httpapi.SetupCheckMiddleware(users))
	api.Get("/probe", func(c fiber.Ctx) error { return c.SendString("ok") })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/probe", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
}

package auth

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func newTestApp(store *SessionStore) *fiber.App {
	app := fiber.New()
	app.Use(NewMiddleware(store))
	app.Get("/api/health", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })
	app.Post("/api/auth/login", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })
	app.Get("/api/tasks", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })
	return app
}

func TestMiddleware_SkipsHealth(t *testing.T) {
	app := newTestApp(NewSessionStore())
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestMiddleware_SkipsLogin(t *testing.T) {
	app := newTestApp(NewSessionStore())
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestMiddleware_UnauthorizedWithoutCookie(t *testing.T) {
	app := newTestApp(NewSessionStore())
	req := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestMiddleware_UnauthorizedWithInvalidToken(t *testing.T) {
	app := newTestApp(NewSessionStore())
	req := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: "invalid-token"})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestMiddleware_AuthorizedWithValidToken(t *testing.T) {
	store := NewSessionStore()
	token, err := store.CreateSession()
	if err != nil {
		t.Fatal(err)
	}

	app := newTestApp(store)
	req := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: token})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
}

func TestMiddleware_UnauthorizedAfterSessionDelete(t *testing.T) {
	store := NewSessionStore()
	token, err := store.CreateSession()
	if err != nil {
		t.Fatal(err)
	}
	store.DeleteSession(token)

	app := newTestApp(store)
	req := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: token})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 after logout, got %d", resp.StatusCode)
	}
}

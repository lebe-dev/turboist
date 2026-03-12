package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/auth"
)

const testPassword = "secret123"

func newTestAuthApp(dev bool) (*fiber.App, *auth.SessionStore) {
	store := auth.NewSessionStore()
	h := NewAuthHandler(store, testPassword, dev)
	mw := auth.NewMiddleware(store)

	app := fiber.New()
	app.Use(mw)
	app.Post("/api/auth/login", h.Login)
	app.Post("/api/auth/logout", h.Logout)
	app.Get("/api/auth/me", h.Me)
	return app, store
}

func TestLogin_Success(t *testing.T) {
	app, _ := newTestAuthApp(true)

	body, _ := json.Marshal(map[string]string{"password": testPassword})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var cookieFound bool
	for _, c := range resp.Cookies() {
		if c.Name == cookieName && c.Value != "" && c.HttpOnly {
			cookieFound = true
		}
	}
	if !cookieFound {
		t.Fatal("expected httpOnly cookie to be set")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	app, _ := newTestAuthApp(true)

	body, _ := json.Marshal(map[string]string{"password": "wrong"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestLogin_InvalidBody(t *testing.T) {
	app, _ := newTestAuthApp(true)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader([]byte("not-json")))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestLogout_ClearsCookie(t *testing.T) {
	app, store := newTestAuthApp(true)

	token, _ := store.CreateSession()

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: token})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	if store.ValidateSession(token) {
		t.Fatal("expected session to be deleted after logout")
	}
}

func TestMe_Authorized(t *testing.T) {
	app, store := newTestAuthApp(true)

	token, _ := store.CreateSession()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: token})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestMe_Unauthorized(t *testing.T) {
	app, _ := newTestAuthApp(true)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/storage"
)

func newTestStateApp(t *testing.T, cfg *config.AppConfig) *fiber.App {
	t.Helper()
	store, err := storage.New(":memory:")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	sessStore := auth.NewSessionStore()
	mw := auth.NewMiddleware(sessStore)

	h := NewStateHandler(store, cfg)

	app := fiber.New()
	app.Use(mw)
	app.Patch("/api/state", h.Update)

	// Create a valid session cookie for all requests
	token, _ := sessStore.CreateSession()
	app.Use(func(c fiber.Ctx) error {
		c.Request().Header.SetCookie(cookieName, token)
		return c.Next()
	})

	// Re-register with cookie middleware active
	app2 := fiber.New()
	app2.Use(func(c fiber.Ctx) error {
		c.Request().Header.SetCookie(cookieName, token)
		return c.Next()
	})
	app2.Use(mw)
	app2.Patch("/api/state", h.Update)

	return app2
}

func defaultTestCfg() *config.AppConfig {
	return &config.AppConfig{
		MaxPinned: 3,
		Contexts: []config.ContextConfig{
			{ID: "work", DisplayName: "Work"},
			{ID: "personal", DisplayName: "Personal"},
		},
		Today: config.TodayConfig{
			DayParts: []config.DayPartConfig{
				{Label: "morning", Start: 8, End: 13},
				{Label: "afternoon", Start: 13, End: 17},
			},
			MaxDayPartNoteLength: 200,
		},
	}
}

func patchState(t *testing.T, app *fiber.App, body any) *http.Response {
	t.Helper()
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/api/state", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	return resp
}

func TestStateUpdate_ValidView(t *testing.T) {
	app := newTestStateApp(t, defaultTestCfg())
	view := "today"
	resp := patchState(t, app, map[string]any{"active_view": view})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("got %d, want 204", resp.StatusCode)
	}
}

func TestStateUpdate_InvalidView(t *testing.T) {
	app := newTestStateApp(t, defaultTestCfg())
	resp := patchState(t, app, map[string]any{"active_view": "nonexistent"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", resp.StatusCode)
	}
}

func TestStateUpdate_UnknownContext(t *testing.T) {
	app := newTestStateApp(t, defaultTestCfg())
	resp := patchState(t, app, map[string]any{"active_context_id": "unknown"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", resp.StatusCode)
	}
}

func TestStateUpdate_ValidContext(t *testing.T) {
	app := newTestStateApp(t, defaultTestCfg())
	resp := patchState(t, app, map[string]any{"active_context_id": "work"})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("got %d, want 204", resp.StatusCode)
	}
}

func TestStateUpdate_EmptyContext(t *testing.T) {
	app := newTestStateApp(t, defaultTestCfg())
	// Empty context means "clear context" — should be allowed
	resp := patchState(t, app, map[string]any{"active_context_id": ""})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("got %d, want 204", resp.StatusCode)
	}
}

func TestStateUpdate_TooManyPinned(t *testing.T) {
	app := newTestStateApp(t, defaultTestCfg())
	pinned := []storage.PinnedTask{
		{ID: "1", Content: "a"},
		{ID: "2", Content: "b"},
		{ID: "3", Content: "c"},
		{ID: "4", Content: "d"},
	}
	resp := patchState(t, app, map[string]any{"pinned_tasks": pinned})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", resp.StatusCode)
	}
}

func TestStateUpdate_PinnedWithinLimit(t *testing.T) {
	app := newTestStateApp(t, defaultTestCfg())
	pinned := []storage.PinnedTask{
		{ID: "1", Content: "a"},
		{ID: "2", Content: "b"},
	}
	resp := patchState(t, app, map[string]any{"pinned_tasks": pinned})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("got %d, want 204", resp.StatusCode)
	}
}

func TestStateUpdate_InvalidJSON(t *testing.T) {
	app := newTestStateApp(t, defaultTestCfg())
	req := httptest.NewRequest(http.MethodPatch, "/api/state", bytes.NewReader([]byte("not-json")))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", resp.StatusCode)
	}
}

func TestStateUpdate_PersistAndRead(t *testing.T) {
	store, err := storage.New(":memory:")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	cfg := defaultTestCfg()
	sessStore := auth.NewSessionStore()
	token, _ := sessStore.CreateSession()
	mw := auth.NewMiddleware(sessStore)
	h := NewStateHandler(store, cfg)

	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Request().Header.SetCookie(cookieName, token)
		return c.Next()
	})
	app.Use(mw)
	app.Patch("/api/state", h.Update)

	// PATCH active_view and sidebar_collapsed
	body, _ := json.Marshal(map[string]any{
		"active_view":       "weekly",
		"sidebar_collapsed": true,
		"active_context_id": "work",
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/state", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("got %d, want 204", resp.StatusCode)
	}

	// Verify via storage
	state, err := store.GetState()
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if state.ActiveView != "weekly" {
		t.Errorf("got active_view %q, want %q", state.ActiveView, "weekly")
	}
	if !state.SidebarCollapsed {
		t.Error("got sidebar_collapsed false, want true")
	}
	if state.ActiveContextID != "work" {
		t.Errorf("got active_context_id %q, want %q", state.ActiveContextID, "work")
	}
}

func TestStateUpdate_DayPartNotes_Valid(t *testing.T) {
	app := newTestStateApp(t, defaultTestCfg())
	notes := map[string]string{"morning": "focus block", "afternoon": "meetings"}
	resp := patchState(t, app, map[string]any{"day_part_notes": notes})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("got %d, want 204", resp.StatusCode)
	}
}

func TestStateUpdate_DayPartNotes_UnknownLabel(t *testing.T) {
	app := newTestStateApp(t, defaultTestCfg())
	notes := map[string]string{"night": "sleep"}
	resp := patchState(t, app, map[string]any{"day_part_notes": notes})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", resp.StatusCode)
	}
}

func TestStateUpdate_DayPartNotes_TooLong(t *testing.T) {
	cfg := defaultTestCfg()
	cfg.Today.MaxDayPartNoteLength = 10
	app := newTestStateApp(t, cfg)
	notes := map[string]string{"morning": "this note is way too long"}
	resp := patchState(t, app, map[string]any{"day_part_notes": notes})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", resp.StatusCode)
	}
}

func TestStateUpdate_DayPartNotes_PersistAndRead(t *testing.T) {
	store, err := storage.New(":memory:")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	cfg := defaultTestCfg()
	sessStore := auth.NewSessionStore()
	token, _ := sessStore.CreateSession()
	mw := auth.NewMiddleware(sessStore)
	h := NewStateHandler(store, cfg)

	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Request().Header.SetCookie(cookieName, token)
		return c.Next()
	})
	app.Use(mw)
	app.Patch("/api/state", h.Update)

	notes := map[string]string{"morning": "emails first"}
	body, _ := json.Marshal(map[string]any{"day_part_notes": notes})
	req := httptest.NewRequest(http.MethodPatch, "/api/state", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("got %d, want 204", resp.StatusCode)
	}

	state, err := store.GetState()
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if state.DayPartNotes["morning"] != "emails first" {
		t.Errorf("got day_part_notes[morning] %q, want %q", state.DayPartNotes["morning"], "emails first")
	}
}

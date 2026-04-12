package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/storage"
	"github.com/lebe-dev/turboist/internal/todoist"
)

func newTestConfigApp(t *testing.T, cfg *config.AppConfig, store *storage.Store) *fiber.App {
	t.Helper()
	cache := todoist.NewTestCache(nil, nil, nil, nil)

	sessStore := auth.NewSessionStore()
	token, _ := sessStore.CreateSession()
	mw := auth.NewMiddleware(sessStore)

	h := NewConfigHandler(cache, cfg, store, nil)

	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Request().Header.SetCookie(cookieName, token)
		return c.Next()
	})
	app.Use(mw)
	app.Get("/api/config", h.Config)

	return app
}

func getConfig(t *testing.T, app *fiber.App) (int, appConfigResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	var body appConfigResponse
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
	}
	return resp.StatusCode, body
}

func newTestStore(t *testing.T) *storage.Store {
	t.Helper()
	store, err := storage.New(":memory:")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestConfig_ConstraintsEnabled(t *testing.T) {
	store := newTestStore(t)
	cfg := &config.AppConfig{
		Location: time.UTC,
		Constraints: config.ConstraintsConfig{
			Enabled:        true,
			PriorityFloor:  3,
			PostponeBudget: 5,
			DayPartCaps: []config.DayPartCapConfig{
				{Label: "morning", MaxTasks: 4},
			},
		},
	}
	app := newTestConfigApp(t, cfg, store)
	status, body := getConfig(t, app)
	if status != http.StatusOK {
		t.Fatalf("got %d, want 200", status)
	}
	if !body.Constraints.Enabled {
		t.Error("got constraints.enabled false, want true")
	}
	if body.Constraints.PriorityFloor != 3 {
		t.Errorf("got priority_floor %d, want 3", body.Constraints.PriorityFloor)
	}
	if body.Constraints.PostponeBudget != 5 {
		t.Errorf("got postpone_budget %d, want 5", body.Constraints.PostponeBudget)
	}
	if len(body.Constraints.DayPartCaps) != 1 {
		t.Fatalf("got %d day_part_caps, want 1", len(body.Constraints.DayPartCaps))
	}
	if body.Constraints.DayPartCaps[0].Label != "morning" || body.Constraints.DayPartCaps[0].MaxTasks != 4 {
		t.Errorf("got day_part_cap %+v, want morning/4", body.Constraints.DayPartCaps[0])
	}
}

func TestConfig_ConstraintsDisabled(t *testing.T) {
	store := newTestStore(t)
	cfg := &config.AppConfig{
		Location: time.UTC,
		Constraints: config.ConstraintsConfig{
			Enabled: false,
		},
	}
	app := newTestConfigApp(t, cfg, store)
	status, body := getConfig(t, app)
	if status != http.StatusOK {
		t.Fatalf("got %d, want 200", status)
	}
	if body.Constraints.Enabled {
		t.Error("got constraints.enabled true, want false")
	}
}

func TestConfig_LabelBlocksRemainingSeconds(t *testing.T) {
	store := newTestStore(t)

	// Insert a label block that started 2 hours ago
	startedAt := time.Now().Add(-2 * time.Hour)
	if err := store.UpsertLabelBlock("focus", startedAt); err != nil {
		t.Fatalf("upsert label block: %v", err)
	}

	cfg := &config.AppConfig{
		Location: time.UTC,
		Constraints: config.ConstraintsConfig{
			Enabled: true,
			LabelBlocks: []config.LabelBlockConfig{
				{Label: "focus", Duration: 24 * time.Hour}, // 24h total, 2h elapsed = ~22h remaining
			},
			PriorityFloor: 4,
		},
	}
	app := newTestConfigApp(t, cfg, store)
	status, body := getConfig(t, app)
	if status != http.StatusOK {
		t.Fatalf("got %d, want 200", status)
	}
	if len(body.Constraints.LabelBlocks) != 1 {
		t.Fatalf("got %d label_blocks, want 1", len(body.Constraints.LabelBlocks))
	}
	lb := body.Constraints.LabelBlocks[0]
	if lb.Label != "focus" {
		t.Errorf("got label %q, want %q", lb.Label, "focus")
	}
	// Should be approximately 22 hours in seconds (79200), with some tolerance
	expectedMin := int((22*time.Hour - 10*time.Second).Seconds())
	expectedMax := int((22*time.Hour + 10*time.Second).Seconds())
	if lb.RemainingSeconds < expectedMin || lb.RemainingSeconds > expectedMax {
		t.Errorf("got remaining_seconds %d, want between %d and %d", lb.RemainingSeconds, expectedMin, expectedMax)
	}
}

func TestConfig_LabelBlocksExpiredNotReturned(t *testing.T) {
	store := newTestStore(t)

	// Insert a label block that started 25 hours ago with a 24h duration — expired
	startedAt := time.Now().Add(-25 * time.Hour)
	if err := store.UpsertLabelBlock("expired-label", startedAt); err != nil {
		t.Fatalf("upsert label block: %v", err)
	}

	cfg := &config.AppConfig{
		Location: time.UTC,
		Constraints: config.ConstraintsConfig{
			Enabled: true,
			LabelBlocks: []config.LabelBlockConfig{
				{Label: "expired-label", Duration: 24 * time.Hour},
			},
			PriorityFloor: 4,
		},
	}
	app := newTestConfigApp(t, cfg, store)
	_, body := getConfig(t, app)
	if len(body.Constraints.LabelBlocks) != 0 {
		t.Errorf("got %d label_blocks, want 0 (expired block should be excluded)", len(body.Constraints.LabelBlocks))
	}
}

func TestConfig_LabelBlocksUnconfiguredNotReturned(t *testing.T) {
	store := newTestStore(t)

	// Insert a label block for a label that's NOT in the config
	if err := store.UpsertLabelBlock("removed-label", time.Now()); err != nil {
		t.Fatalf("upsert label block: %v", err)
	}

	cfg := &config.AppConfig{
		Location: time.UTC,
		Constraints: config.ConstraintsConfig{
			Enabled:       true,
			LabelBlocks:   []config.LabelBlockConfig{}, // no configured blocks
			PriorityFloor: 4,
		},
	}
	app := newTestConfigApp(t, cfg, store)
	_, body := getConfig(t, app)
	if len(body.Constraints.LabelBlocks) != 0 {
		t.Errorf("got %d label_blocks, want 0 (unconfigured block should be excluded)", len(body.Constraints.LabelBlocks))
	}
}

func TestConfig_PostponeBudgetUsedToday(t *testing.T) {
	store := newTestStore(t)
	today := time.Now().Format("2006-01-02")
	if err := store.SetPostponeBudget(&storage.PostponeBudgetState{Date: today, Used: 3}); err != nil {
		t.Fatalf("set postpone budget: %v", err)
	}

	cfg := &config.AppConfig{
		Location: time.UTC,
		Constraints: config.ConstraintsConfig{
			Enabled:        true,
			PostponeBudget: 5,
			PriorityFloor:  4,
		},
	}
	app := newTestConfigApp(t, cfg, store)
	_, body := getConfig(t, app)
	if body.Constraints.PostponeBudgetUsed != 3 {
		t.Errorf("got postpone_budget_used %d, want 3", body.Constraints.PostponeBudgetUsed)
	}
}

func TestConfig_PostponeBudgetResetsOnNewDay(t *testing.T) {
	store := newTestStore(t)
	// Set budget for yesterday
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	if err := store.SetPostponeBudget(&storage.PostponeBudgetState{Date: yesterday, Used: 3}); err != nil {
		t.Fatalf("set postpone budget: %v", err)
	}

	cfg := &config.AppConfig{
		Location: time.UTC,
		Constraints: config.ConstraintsConfig{
			Enabled:        true,
			PostponeBudget: 5,
			PriorityFloor:  4,
		},
	}
	app := newTestConfigApp(t, cfg, store)
	_, body := getConfig(t, app)
	if body.Constraints.PostponeBudgetUsed != 0 {
		t.Errorf("got postpone_budget_used %d, want 0 (should reset on new day)", body.Constraints.PostponeBudgetUsed)
	}
}

func TestConfig_PostponeBudgetNoState(t *testing.T) {
	store := newTestStore(t)

	cfg := &config.AppConfig{
		Location: time.UTC,
		Constraints: config.ConstraintsConfig{
			Enabled:        true,
			PostponeBudget: 5,
			PriorityFloor:  4,
		},
	}
	app := newTestConfigApp(t, cfg, store)
	_, body := getConfig(t, app)
	if body.Constraints.PostponeBudgetUsed != 0 {
		t.Errorf("got postpone_budget_used %d, want 0 (no state = zero used)", body.Constraints.PostponeBudgetUsed)
	}
}

func TestConfig_ConstraintsEmptyArraysNotNull(t *testing.T) {
	store := newTestStore(t)
	cfg := &config.AppConfig{
		Location: time.UTC,
		Constraints: config.ConstraintsConfig{
			Enabled:       true,
			PriorityFloor: 4,
		},
	}
	app := newTestConfigApp(t, cfg, store)
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var constraints map[string]json.RawMessage
	if err := json.Unmarshal(raw["constraints"], &constraints); err != nil {
		t.Fatalf("decode constraints: %v", err)
	}
	// Verify label_blocks and day_part_caps are [] not null
	if string(constraints["label_blocks"]) == "null" {
		t.Error("label_blocks is null, want empty array")
	}
	if string(constraints["day_part_caps"]) == "null" {
		t.Error("day_part_caps is null, want empty array")
	}
}

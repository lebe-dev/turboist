package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/auth"
	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/storage"
)

func newTestConstraintsApp(t *testing.T, cfg *config.AppConfig, store *storage.Store) *fiber.App {
	t.Helper()

	sessStore := auth.NewSessionStore()
	token, _ := sessStore.CreateSession()
	mw := auth.NewMiddleware(sessStore)

	h := NewConstraintsHandler(store, cfg)

	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Request().Header.SetCookie(cookieName, token)
		return c.Next()
	})
	app.Use(mw)
	app.Get("/api/constraints/daily", h.Daily)
	app.Post("/api/constraints/daily/roll", h.Roll)
	app.Post("/api/constraints/daily/swap", h.Swap)
	app.Post("/api/constraints/daily/confirm", h.Confirm)

	return app
}

func defaultConstraintsCfg() *config.AppConfig {
	return &config.AppConfig{
		Location: time.UTC,
		Constraints: config.ConstraintsConfig{
			Enabled: true,
			Daily: config.DailyConstraintsConfig{
				MaxConstraints: 3,
				MaxRerolls:     2,
			},
			PriorityFloor: 4,
		},
	}
}

func getDailyConstraints(t *testing.T, app *fiber.App) (int, dailyConstraintsResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/constraints/daily", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	var body dailyConstraintsResponse
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
	}
	return resp.StatusCode, body
}

func postRoll(t *testing.T, app *fiber.App) (int, dailyConstraintsResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/constraints/daily/roll", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	var body dailyConstraintsResponse
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
	}
	return resp.StatusCode, body
}

func postSwap(t *testing.T, app *fiber.App, index int) (int, dailyConstraintsResponse) {
	t.Helper()
	payload, _ := json.Marshal(swapRequest{Index: index})
	req := httptest.NewRequest(http.MethodPost, "/api/constraints/daily/swap", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	var body dailyConstraintsResponse
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
	}
	return resp.StatusCode, body
}

func postConfirm(t *testing.T, app *fiber.App) (int, dailyConstraintsResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/constraints/daily/confirm", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	var body dailyConstraintsResponse
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
	}
	return resp.StatusCode, body
}

// --- GET /api/constraints/daily ---

func TestDaily_NoStateNeedsSelection(t *testing.T) {
	store := newTestStore(t)
	cfg := defaultConstraintsCfg()
	if err := store.SetConstraintPool([]string{"a", "b", "c", "d"}); err != nil {
		t.Fatalf("set pool: %v", err)
	}
	app := newTestConstraintsApp(t, cfg, store)

	status, body := getDailyConstraints(t, app)
	if status != http.StatusOK {
		t.Fatalf("got %d, want 200", status)
	}
	if !body.NeedsSelection {
		t.Error("got needs_selection false, want true")
	}
	if body.PoolSize != 4 {
		t.Errorf("got pool_size %d, want 4", body.PoolSize)
	}
	if body.MaxRerolls != 2 {
		t.Errorf("got max_rerolls %d, want 2", body.MaxRerolls)
	}
}

func TestDaily_ExistingStateTodayReturnsItems(t *testing.T) {
	store := newTestStore(t)
	cfg := defaultConstraintsCfg()
	today := time.Now().Format("2006-01-02")
	if err := store.SetDailyConstraints(&storage.DailyConstraintsState{
		Date:        today,
		Items:       []string{"focus", "no-phone"},
		RerollsUsed: 1,
		Confirmed:   false,
	}); err != nil {
		t.Fatalf("set daily constraints: %v", err)
	}
	app := newTestConstraintsApp(t, cfg, store)

	status, body := getDailyConstraints(t, app)
	if status != http.StatusOK {
		t.Fatalf("got %d, want 200", status)
	}
	if body.NeedsSelection {
		t.Error("got needs_selection true, want false")
	}
	if len(body.Items) != 2 {
		t.Fatalf("got %d items, want 2", len(body.Items))
	}
	if body.RerollsUsed != 1 {
		t.Errorf("got rerolls_used %d, want 1", body.RerollsUsed)
	}
}

func TestDaily_DateRolloverNeedsSelection(t *testing.T) {
	store := newTestStore(t)
	cfg := defaultConstraintsCfg()
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	if err := store.SetDailyConstraints(&storage.DailyConstraintsState{
		Date:      yesterday,
		Items:     []string{"old-constraint"},
		Confirmed: true,
	}); err != nil {
		t.Fatalf("set daily constraints: %v", err)
	}
	app := newTestConstraintsApp(t, cfg, store)

	status, body := getDailyConstraints(t, app)
	if status != http.StatusOK {
		t.Fatalf("got %d, want 200", status)
	}
	if !body.NeedsSelection {
		t.Error("got needs_selection false, want true (date rollover)")
	}
}

// --- POST /api/constraints/daily/roll ---

func TestRoll_FirstRollHappyPath(t *testing.T) {
	store := newTestStore(t)
	cfg := defaultConstraintsCfg()
	pool := []string{"a", "b", "c", "d", "e"}
	if err := store.SetConstraintPool(pool); err != nil {
		t.Fatalf("set pool: %v", err)
	}
	app := newTestConstraintsApp(t, cfg, store)

	status, body := postRoll(t, app)
	if status != http.StatusOK {
		t.Fatalf("got %d, want 200", status)
	}
	if body.NeedsSelection {
		t.Error("got needs_selection true after roll")
	}
	if len(body.Items) != 3 {
		t.Errorf("got %d items, want 3 (max_constraints)", len(body.Items))
	}
	if body.RerollsUsed != 0 {
		t.Errorf("got rerolls_used %d, want 0 (first roll)", body.RerollsUsed)
	}
	// All items should come from the pool
	poolSet := make(map[string]bool, len(pool))
	for _, p := range pool {
		poolSet[p] = true
	}
	for _, item := range body.Items {
		if !poolSet[item] {
			t.Errorf("item %q not in pool", item)
		}
	}
}

func TestRoll_RerollIncrementsCount(t *testing.T) {
	store := newTestStore(t)
	cfg := defaultConstraintsCfg()
	if err := store.SetConstraintPool([]string{"a", "b", "c", "d", "e"}); err != nil {
		t.Fatalf("set pool: %v", err)
	}
	app := newTestConstraintsApp(t, cfg, store)

	// First roll
	status, _ := postRoll(t, app)
	if status != http.StatusOK {
		t.Fatalf("first roll: got %d, want 200", status)
	}

	// Second roll (reroll)
	status, body := postRoll(t, app)
	if status != http.StatusOK {
		t.Fatalf("reroll: got %d, want 200", status)
	}
	if body.RerollsUsed != 1 {
		t.Errorf("got rerolls_used %d, want 1", body.RerollsUsed)
	}
}

func TestRoll_RerollExhausted(t *testing.T) {
	store := newTestStore(t)
	cfg := defaultConstraintsCfg()
	if err := store.SetConstraintPool([]string{"a", "b", "c", "d", "e"}); err != nil {
		t.Fatalf("set pool: %v", err)
	}
	today := time.Now().Format("2006-01-02")
	// State with max rerolls already used
	if err := store.SetDailyConstraints(&storage.DailyConstraintsState{
		Date:        today,
		Items:       []string{"a", "b", "c"},
		RerollsUsed: 2, // max_rerolls = 2
	}); err != nil {
		t.Fatalf("set daily constraints: %v", err)
	}
	app := newTestConstraintsApp(t, cfg, store)

	status, _ := postRoll(t, app)
	if status != http.StatusBadRequest {
		t.Fatalf("got %d, want 400 (rerolls exhausted)", status)
	}
}

func TestRoll_EmptyPool(t *testing.T) {
	store := newTestStore(t)
	cfg := defaultConstraintsCfg()
	// No pool set
	app := newTestConstraintsApp(t, cfg, store)

	status, _ := postRoll(t, app)
	if status != http.StatusBadRequest {
		t.Fatalf("got %d, want 400 (empty pool)", status)
	}
}

func TestRoll_PoolSmallerThanMaxConstraints(t *testing.T) {
	store := newTestStore(t)
	cfg := defaultConstraintsCfg()
	// Pool has fewer items than max_constraints (3)
	if err := store.SetConstraintPool([]string{"x", "y"}); err != nil {
		t.Fatalf("set pool: %v", err)
	}
	app := newTestConstraintsApp(t, cfg, store)

	status, body := postRoll(t, app)
	if status != http.StatusOK {
		t.Fatalf("got %d, want 200", status)
	}
	if len(body.Items) != 2 {
		t.Errorf("got %d items, want 2 (pool size)", len(body.Items))
	}
}

func TestRoll_RejectsAfterConfirm(t *testing.T) {
	store := newTestStore(t)
	cfg := defaultConstraintsCfg()
	if err := store.SetConstraintPool([]string{"a", "b", "c", "d"}); err != nil {
		t.Fatalf("set pool: %v", err)
	}
	today := time.Now().Format("2006-01-02")
	if err := store.SetDailyConstraints(&storage.DailyConstraintsState{
		Date:      today,
		Items:     []string{"a", "b", "c"},
		Confirmed: true,
	}); err != nil {
		t.Fatalf("set daily constraints: %v", err)
	}
	app := newTestConstraintsApp(t, cfg, store)

	status, _ := postRoll(t, app)
	if status != http.StatusBadRequest {
		t.Fatalf("got %d, want 400 (already confirmed)", status)
	}
}

// --- POST /api/constraints/daily/swap ---

func TestSwap_HappyPath(t *testing.T) {
	store := newTestStore(t)
	cfg := defaultConstraintsCfg()
	if err := store.SetConstraintPool([]string{"a", "b", "c", "d", "e"}); err != nil {
		t.Fatalf("set pool: %v", err)
	}
	today := time.Now().Format("2006-01-02")
	if err := store.SetDailyConstraints(&storage.DailyConstraintsState{
		Date:        today,
		Items:       []string{"a", "b", "c"},
		RerollsUsed: 0,
	}); err != nil {
		t.Fatalf("set daily constraints: %v", err)
	}
	app := newTestConstraintsApp(t, cfg, store)

	status, body := postSwap(t, app, 1)
	if status != http.StatusOK {
		t.Fatalf("got %d, want 200", status)
	}
	if body.RerollsUsed != 1 {
		t.Errorf("got rerolls_used %d, want 1", body.RerollsUsed)
	}
	swapped := body.Items[1]
	if swapped != "d" && swapped != "e" {
		t.Errorf("swapped item: got %q, want one of {d, e}", swapped)
	}
	if body.Items[0] != "a" {
		t.Errorf("items[0]: got %q, want %q", body.Items[0], "a")
	}
	if body.Items[2] != "c" {
		t.Errorf("items[2]: got %q, want %q", body.Items[2], "c")
	}
}

func TestSwap_IndexOutOfBounds(t *testing.T) {
	store := newTestStore(t)
	cfg := defaultConstraintsCfg()
	if err := store.SetConstraintPool([]string{"a", "b", "c", "d"}); err != nil {
		t.Fatalf("set pool: %v", err)
	}
	today := time.Now().Format("2006-01-02")
	if err := store.SetDailyConstraints(&storage.DailyConstraintsState{
		Date:  today,
		Items: []string{"a", "b", "c"},
	}); err != nil {
		t.Fatalf("set daily constraints: %v", err)
	}
	app := newTestConstraintsApp(t, cfg, store)

	status, _ := postSwap(t, app, 5)
	if status != http.StatusBadRequest {
		t.Fatalf("got %d, want 400 (index out of bounds)", status)
	}

	status, _ = postSwap(t, app, -1)
	if status != http.StatusBadRequest {
		t.Fatalf("got %d, want 400 (negative index)", status)
	}
}

func TestSwap_RerollsExhausted(t *testing.T) {
	store := newTestStore(t)
	cfg := defaultConstraintsCfg()
	if err := store.SetConstraintPool([]string{"a", "b", "c", "d"}); err != nil {
		t.Fatalf("set pool: %v", err)
	}
	today := time.Now().Format("2006-01-02")
	if err := store.SetDailyConstraints(&storage.DailyConstraintsState{
		Date:        today,
		Items:       []string{"a", "b", "c"},
		RerollsUsed: 2,
	}); err != nil {
		t.Fatalf("set daily constraints: %v", err)
	}
	app := newTestConstraintsApp(t, cfg, store)

	status, _ := postSwap(t, app, 0)
	if status != http.StatusBadRequest {
		t.Fatalf("got %d, want 400 (rerolls exhausted)", status)
	}
}

func TestSwap_NoStateForToday(t *testing.T) {
	store := newTestStore(t)
	cfg := defaultConstraintsCfg()
	app := newTestConstraintsApp(t, cfg, store)

	status, _ := postSwap(t, app, 0)
	if status != http.StatusBadRequest {
		t.Fatalf("got %d, want 400 (no state for today)", status)
	}
}

func TestSwap_AlreadyConfirmed(t *testing.T) {
	store := newTestStore(t)
	cfg := defaultConstraintsCfg()
	if err := store.SetConstraintPool([]string{"a", "b", "c", "d"}); err != nil {
		t.Fatalf("set pool: %v", err)
	}
	today := time.Now().Format("2006-01-02")
	if err := store.SetDailyConstraints(&storage.DailyConstraintsState{
		Date:      today,
		Items:     []string{"a", "b", "c"},
		Confirmed: true,
	}); err != nil {
		t.Fatalf("set daily constraints: %v", err)
	}
	app := newTestConstraintsApp(t, cfg, store)

	status, _ := postSwap(t, app, 0)
	if status != http.StatusBadRequest {
		t.Fatalf("got %d, want 400 (already confirmed)", status)
	}
}

func TestSwap_NoCandidates(t *testing.T) {
	store := newTestStore(t)
	cfg := defaultConstraintsCfg()
	// Pool exactly matches current items — no alternatives available.
	if err := store.SetConstraintPool([]string{"a", "b", "c"}); err != nil {
		t.Fatalf("set pool: %v", err)
	}
	today := time.Now().Format("2006-01-02")
	if err := store.SetDailyConstraints(&storage.DailyConstraintsState{
		Date:  today,
		Items: []string{"a", "b", "c"},
	}); err != nil {
		t.Fatalf("set daily constraints: %v", err)
	}
	app := newTestConstraintsApp(t, cfg, store)

	status, _ := postSwap(t, app, 0)
	if status != http.StatusBadRequest {
		t.Fatalf("got %d, want 400 (no candidates)", status)
	}
}

// --- POST /api/constraints/daily/confirm ---

func TestConfirm_HappyPath(t *testing.T) {
	store := newTestStore(t)
	cfg := defaultConstraintsCfg()
	if err := store.SetConstraintPool([]string{"a", "b", "c"}); err != nil {
		t.Fatalf("set pool: %v", err)
	}
	today := time.Now().Format("2006-01-02")
	if err := store.SetDailyConstraints(&storage.DailyConstraintsState{
		Date:  today,
		Items: []string{"a", "b"},
	}); err != nil {
		t.Fatalf("set daily constraints: %v", err)
	}
	app := newTestConstraintsApp(t, cfg, store)

	status, body := postConfirm(t, app)
	if status != http.StatusOK {
		t.Fatalf("got %d, want 200", status)
	}
	if !body.Confirmed {
		t.Error("got confirmed false, want true")
	}
	if len(body.Items) != 2 {
		t.Errorf("got %d items, want 2", len(body.Items))
	}
}

func TestConfirm_Idempotent(t *testing.T) {
	store := newTestStore(t)
	cfg := defaultConstraintsCfg()
	if err := store.SetConstraintPool([]string{"a", "b", "c"}); err != nil {
		t.Fatalf("set pool: %v", err)
	}
	today := time.Now().Format("2006-01-02")
	if err := store.SetDailyConstraints(&storage.DailyConstraintsState{
		Date:      today,
		Items:     []string{"a", "b"},
		Confirmed: true,
	}); err != nil {
		t.Fatalf("set daily constraints: %v", err)
	}
	app := newTestConstraintsApp(t, cfg, store)

	// Confirm again — should succeed
	status, body := postConfirm(t, app)
	if status != http.StatusOK {
		t.Fatalf("got %d, want 200 (idempotent confirm)", status)
	}
	if !body.Confirmed {
		t.Error("got confirmed false, want true")
	}
}

func TestConfirm_NoStateForToday(t *testing.T) {
	store := newTestStore(t)
	cfg := defaultConstraintsCfg()
	app := newTestConstraintsApp(t, cfg, store)

	status, _ := postConfirm(t, app)
	if status != http.StatusBadRequest {
		t.Fatalf("got %d, want 400 (no state for today)", status)
	}
}

// --- Full flow test ---

func TestDailyConstraints_FullFlow(t *testing.T) {
	store := newTestStore(t)
	cfg := defaultConstraintsCfg()
	pool := []string{"focus", "no-phone", "exercise", "read", "meditate"}
	if err := store.SetConstraintPool(pool); err != nil {
		t.Fatalf("set pool: %v", err)
	}
	app := newTestConstraintsApp(t, cfg, store)

	// Step 1: Check initial state — needs selection
	status, body := getDailyConstraints(t, app)
	if status != http.StatusOK {
		t.Fatalf("step1: got %d, want 200", status)
	}
	if !body.NeedsSelection {
		t.Fatal("step1: needs_selection should be true")
	}

	// Step 2: First roll
	status, body = postRoll(t, app)
	if status != http.StatusOK {
		t.Fatalf("step2: got %d, want 200", status)
	}
	if body.RerollsUsed != 0 {
		t.Errorf("step2: got rerolls_used %d, want 0", body.RerollsUsed)
	}
	if len(body.Items) != 3 {
		t.Fatalf("step2: got %d items, want 3", len(body.Items))
	}

	// Step 3: Swap one item
	status, body = postSwap(t, app, 0)
	if status != http.StatusOK {
		t.Fatalf("step3: got %d, want 200", status)
	}
	if body.RerollsUsed != 1 {
		t.Errorf("step3: got rerolls_used %d, want 1", body.RerollsUsed)
	}

	// Step 4: Reroll all
	status, body = postRoll(t, app)
	if status != http.StatusOK {
		t.Fatalf("step4: got %d, want 200", status)
	}
	if body.RerollsUsed != 2 {
		t.Errorf("step4: got rerolls_used %d, want 2", body.RerollsUsed)
	}

	// Step 5: Try another reroll — should fail (exhausted)
	status, _ = postRoll(t, app)
	if status != http.StatusBadRequest {
		t.Fatalf("step5: got %d, want 400 (rerolls exhausted)", status)
	}

	// Step 6: Confirm
	status, body = postConfirm(t, app)
	if status != http.StatusOK {
		t.Fatalf("step6: got %d, want 200", status)
	}
	if !body.Confirmed {
		t.Error("step6: confirmed should be true")
	}

	// Step 7: Verify GET returns confirmed state
	status, body = getDailyConstraints(t, app)
	if status != http.StatusOK {
		t.Fatalf("step7: got %d, want 200", status)
	}
	if body.NeedsSelection {
		t.Error("step7: needs_selection should be false")
	}
	if !body.Confirmed {
		t.Error("step7: confirmed should be true")
	}
}

package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

func TestCalendarHandler_ListRequiresAuth(t *testing.T) {
	env := setupAPIEnv(t)
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/calendars/", nil)
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestCalendarHandler_ListEmpty(t *testing.T) {
	env := setupAPIEnv(t)
	req := env.authedReq(t, http.MethodGet, "/api/v1/calendars/", nil)
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if v, ok := body["enabled"].(bool); !ok || v {
		t.Errorf("enabled: got %v, want false", body["enabled"])
	}
	accounts, ok := body["accounts"].([]any)
	if !ok {
		t.Fatalf("accounts: missing or wrong type: %T", body["accounts"])
	}
	if len(accounts) != 0 {
		t.Errorf("accounts: got %d items, want 0", len(accounts))
	}
	sources, ok := body["sources"].([]any)
	if !ok {
		t.Fatalf("sources: missing or wrong type: %T", body["sources"])
	}
	if len(sources) != 0 {
		t.Errorf("sources: got %d items, want 0", len(sources))
	}
	if v, ok := body["googleConfigured"].(bool); !ok || v {
		t.Errorf("googleConfigured: got %v, want false", body["googleConfigured"])
	}
}

func TestCalendarHandler_PatchSettingsToggle(t *testing.T) {
	env := setupAPIEnv(t)

	// Enable
	resp, err := env.app.Test(env.authedReq(t, http.MethodPatch, "/api/v1/calendars/settings", map[string]any{
		"enabled": true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if v, ok := body["enabled"].(bool); !ok || !v {
		t.Errorf("enabled: got %v, want true", body["enabled"])
	}

	// Disable
	resp2, err := env.app.Test(env.authedReq(t, http.MethodPatch, "/api/v1/calendars/settings", map[string]any{
		"enabled": false,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("patch status: got %d, want %d", resp2.StatusCode, http.StatusOK)
	}
	var body2 map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&body2); err != nil {
		t.Fatal(err)
	}
	if v, ok := body2["enabled"].(bool); !ok || v {
		t.Errorf("enabled: got %v, want false", body2["enabled"])
	}
}

func TestCalendarHandler_DeleteAccountNotFound(t *testing.T) {
	env := setupAPIEnv(t)
	req := env.authedReq(t, http.MethodDelete, "/api/v1/calendars/accounts/9999", nil)
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestCalendarHandler_List_WithSeededAccount(t *testing.T) {
	env := setupAPIEnv(t)
	ctx := context.Background()
	if _, err := env.calendarRepo.UpsertAccount(ctx, &model.CalendarAccount{
		UserID: 1, Provider: model.CalendarProviderGoogle,
		Email: "u@example.com", DisplayName: "Test", AccessToken: "at", RefreshToken: "rt",
		Expiry: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("seed account: %v", err)
	}

	resp, err := env.app.Test(env.authedReq(t, http.MethodGet, "/api/v1/calendars/", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	accounts, ok := body["accounts"].([]any)
	if !ok || len(accounts) != 1 {
		t.Fatalf("accounts: got %v, want one entry", body["accounts"])
	}
	acc := accounts[0].(map[string]any)
	if acc["email"] != "u@example.com" || acc["displayName"] != "Test" || acc["provider"] != "google" {
		t.Errorf("account mapping: got %+v", acc)
	}
}

// --- patchGoogleConfig ---

func TestCalendarHandler_PatchGoogleConfig_Create(t *testing.T) {
	env := setupAPIEnv(t)
	resp, err := env.app.Test(env.authedReq(t, http.MethodPatch, "/api/v1/calendars/google/config",
		map[string]any{"clientId": "cid", "clientSecret": "secret"}))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if v, ok := body["googleConfigured"].(bool); !ok || !v {
		t.Errorf("googleConfigured: got %v, want true", body["googleConfigured"])
	}
}

func TestCalendarHandler_PatchGoogleConfig_MissingClientID(t *testing.T) {
	env := setupAPIEnv(t)
	resp, err := env.app.Test(env.authedReq(t, http.MethodPatch, "/api/v1/calendars/google/config",
		map[string]any{"clientSecret": "secret"}))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode == http.StatusOK {
		t.Errorf("expected 4xx for missing clientId, got %d", resp.StatusCode)
	}
}

func TestCalendarHandler_PatchGoogleConfig_EmptyClientIDNoExisting(t *testing.T) {
	env := setupAPIEnv(t)
	resp, err := env.app.Test(env.authedReq(t, http.MethodPatch, "/api/v1/calendars/google/config",
		map[string]any{"clientId": "   ", "clientSecret": "secret"}))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode == http.StatusOK {
		t.Errorf("expected 4xx for empty clientId with no existing, got %d", resp.StatusCode)
	}
}

func TestCalendarHandler_PatchGoogleConfig_InvalidJSON(t *testing.T) {
	env := setupAPIEnv(t)
	req := env.authedReq(t, http.MethodPatch, "/api/v1/calendars/google/config", nil)
	req.Body = http.NoBody
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", resp.StatusCode)
	}
}

// --- deleteGoogleConfig ---

func TestCalendarHandler_DeleteGoogleConfig_NoConfig(t *testing.T) {
	env := setupAPIEnv(t)
	// Delete when nothing exists should succeed (ErrNotFound swallowed).
	resp, err := env.app.Test(env.authedReq(t, http.MethodDelete, "/api/v1/calendars/google/config", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want 200", resp.StatusCode)
	}
}

func TestCalendarHandler_DeleteGoogleConfig_ExistingConfig(t *testing.T) {
	env := setupAPIEnv(t)
	// Seed config first.
	_, err := env.app.Test(env.authedReq(t, http.MethodPatch, "/api/v1/calendars/google/config",
		map[string]any{"clientId": "cid", "clientSecret": "secret"}))
	if err != nil {
		t.Fatal(err)
	}

	resp, err := env.app.Test(env.authedReq(t, http.MethodDelete, "/api/v1/calendars/google/config", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if v, ok := body["googleConfigured"].(bool); !ok || v {
		t.Errorf("after delete googleConfigured: got %v, want false", body["googleConfigured"])
	}
}

// --- googleStart ---

func TestCalendarHandler_GoogleStart_NotConfigured(t *testing.T) {
	env := setupAPIEnv(t)
	resp, err := env.app.Test(env.authedReq(t, http.MethodGet, "/api/v1/calendars/google/start", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode == http.StatusOK {
		t.Errorf("expected 4xx when OAuth not configured, got %d", resp.StatusCode)
	}
}

func TestCalendarHandler_GoogleStart_Configured(t *testing.T) {
	env := setupAPIEnv(t)
	// Seed encrypted creds via the patch endpoint, which handles encryption.
	if _, err := env.app.Test(env.authedReq(t, http.MethodPatch, "/api/v1/calendars/google/config",
		map[string]any{"clientId": "cid", "clientSecret": "secret"})); err != nil {
		t.Fatal(err)
	}

	resp, err := env.app.Test(env.authedReq(t, http.MethodGet, "/api/v1/calendars/google/start", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.URL == "" {
		t.Errorf("URL must be returned")
	}
}

// --- googleCallback (public) ---

func TestCalendarHandler_GoogleCallback_MissingStateAndCode(t *testing.T) {
	env := setupAPIEnv(t)
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/calendars/google/callback", nil)
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Errorf("status: got %d, want 302", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc == "" {
		t.Errorf("expected Location header")
	}
}

func TestCalendarHandler_GoogleCallback_ProviderError(t *testing.T) {
	env := setupAPIEnv(t)
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/calendars/google/callback?error=access_denied", nil)
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Errorf("status: got %d, want 302", resp.StatusCode)
	}
}

func TestCalendarHandler_GoogleCallback_InvalidState(t *testing.T) {
	env := setupAPIEnv(t)
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/calendars/google/callback?state=bogus&code=xyz", nil)
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Errorf("status: got %d, want 302", resp.StatusCode)
	}
}

// --- googleSync ---

func TestCalendarHandler_GoogleSync_NotConfigured(t *testing.T) {
	env := setupAPIEnv(t)
	resp, err := env.app.Test(env.authedReq(t, http.MethodPost, "/api/v1/calendars/google/sync", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode == http.StatusOK {
		t.Errorf("expected 4xx when not configured, got %d", resp.StatusCode)
	}
}

func TestCalendarHandler_GoogleSync_ConfiguredButNoAccount(t *testing.T) {
	env := setupAPIEnv(t)
	if _, err := env.app.Test(env.authedReq(t, http.MethodPatch, "/api/v1/calendars/google/config",
		map[string]any{"clientId": "cid", "clientSecret": "secret"})); err != nil {
		t.Fatal(err)
	}
	resp, err := env.app.Test(env.authedReq(t, http.MethodPost, "/api/v1/calendars/google/sync", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status: got %d, want 404 (no connected account)", resp.StatusCode)
	}
}

// --- patchSource ---

func TestCalendarHandler_PatchSource_NotFound(t *testing.T) {
	env := setupAPIEnv(t)
	resp, err := env.app.Test(env.authedReq(t, http.MethodPatch, "/api/v1/calendars/sources/9999",
		map[string]any{"selected": true}))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", resp.StatusCode)
	}
}

func TestCalendarHandler_PatchSource_MissingSelected(t *testing.T) {
	env := setupAPIEnv(t)
	resp, err := env.app.Test(env.authedReq(t, http.MethodPatch, "/api/v1/calendars/sources/1",
		map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode == http.StatusOK {
		t.Errorf("expected 4xx for missing selected, got %d", resp.StatusCode)
	}
}

func TestCalendarHandler_PatchSource_TogglesSelected(t *testing.T) {
	env := setupAPIEnv(t)
	// Seed account and one source directly via the repo.
	ctx := context.Background()
	acc, err := env.calendarRepo.UpsertAccount(ctx, &model.CalendarAccount{
		UserID: 1, Provider: model.CalendarProviderGoogle,
		Email: "u@e", AccessToken: "at", RefreshToken: "rt",
		Expiry: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("seed account: %v", err)
	}
	if err := env.calendarRepo.UpsertSources(ctx, acc, []model.CalendarSource{
		{ExternalID: "cal-x", Summary: "X", Selected: true},
	}); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	srcs, err := env.calendarRepo.ListSources(ctx, 1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(srcs) != 1 {
		t.Fatalf("expected 1 source, got %d", len(srcs))
	}

	url := "/api/v1/calendars/sources/" + intToStr(srcs[0].ID)
	resp, err := env.app.Test(env.authedReq(t, http.MethodPatch, url,
		map[string]any{"selected": false}))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if v, ok := body["selected"].(bool); !ok || v {
		t.Errorf("selected: got %v, want false", body["selected"])
	}
}

// --- events ---

func TestCalendarHandler_Events_DisabledReturnsEmpty(t *testing.T) {
	env := setupAPIEnv(t)
	// Default is disabled — no need to toggle.
	resp, err := env.app.Test(env.authedReq(t, http.MethodGet,
		"/api/v1/calendars/events?start=2026-05-01T00:00:00.000Z&end=2026-05-08T00:00:00.000Z", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	var body struct {
		Items []any `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 0 {
		t.Errorf("expected empty items when disabled, got %d", len(body.Items))
	}
}

func TestCalendarHandler_Events_MissingRange(t *testing.T) {
	env := setupAPIEnv(t)
	// Enable calendar first to bypass the early disabled-short-circuit.
	if _, err := env.app.Test(env.authedReq(t, http.MethodPatch, "/api/v1/calendars/settings",
		map[string]any{"enabled": true})); err != nil {
		t.Fatal(err)
	}
	resp, err := env.app.Test(env.authedReq(t, http.MethodGet, "/api/v1/calendars/events", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode == http.StatusOK {
		t.Errorf("expected 4xx for missing range, got %d", resp.StatusCode)
	}
}

func TestCalendarHandler_Events_InvalidRange(t *testing.T) {
	env := setupAPIEnv(t)
	if _, err := env.app.Test(env.authedReq(t, http.MethodPatch, "/api/v1/calendars/settings",
		map[string]any{"enabled": true})); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		url  string
	}{
		{
			name: "start after end",
			url:  "/api/v1/calendars/events?start=2026-05-08T00:00:00.000Z&end=2026-05-01T00:00:00.000Z",
		},
		{
			name: "range too large",
			url:  "/api/v1/calendars/events?start=2026-01-01T00:00:00.000Z&end=2026-12-01T00:00:00.000Z",
		},
		{
			name: "invalid start",
			url:  "/api/v1/calendars/events?start=not-a-time&end=2026-05-08T00:00:00.000Z",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := env.app.Test(env.authedReq(t, http.MethodGet, tc.url, nil))
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode == http.StatusOK {
				t.Errorf("expected 4xx, got %d", resp.StatusCode)
			}
		})
	}
}

func intToStr(n int64) string {
	const digits = "0123456789"
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	out := make([]byte, 0, 20)
	for n > 0 {
		out = append([]byte{digits[n%10]}, out...)
		n /= 10
	}
	if neg {
		out = append([]byte{'-'}, out...)
	}
	return string(out)
}

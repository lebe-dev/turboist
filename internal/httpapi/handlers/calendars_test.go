package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"
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

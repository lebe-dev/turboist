package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMetaConfig_Success(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/config", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v — body: %s", err, body)
	}
	if result["timezone"] != "UTC" {
		t.Errorf("timezone: got %v, want UTC", result["timezone"])
	}
	if result["maxPinned"] == nil {
		t.Error("maxPinned missing from config")
	}
	if result["weekly"] == nil {
		t.Error("weekly missing from config")
	}
	if result["inbox"] == nil {
		t.Error("inbox missing from config")
	}
	for _, key := range []string{"contexts", "projects", "labels", "settings", "appSettings", "userState", "troiki"} {
		if _, ok := result[key]; !ok {
			t.Errorf("%s missing from config", key)
		}
	}
	if _, ok := result["contexts"].([]any); !ok {
		t.Errorf("contexts: got %T, want []any", result["contexts"])
	}
	if _, ok := result["projects"].([]any); !ok {
		t.Errorf("projects: got %T, want []any", result["projects"])
	}
	if _, ok := result["labels"].([]any); !ok {
		t.Errorf("labels: got %T, want []any", result["labels"])
	}
	settings, ok := result["settings"].(map[string]any)
	if !ok {
		t.Fatalf("settings: got %T, want map[string]any", result["settings"])
	}
	if _, ok := settings["locale"]; !ok {
		t.Error("settings.locale missing")
	}
	appSettings, ok := result["appSettings"].(map[string]any)
	if !ok {
		t.Fatalf("appSettings: got %T, want map[string]any", result["appSettings"])
	}
	if _, ok := appSettings["autoLabels"]; !ok {
		t.Error("appSettings.autoLabels missing")
	}
	if _, ok := result["userState"].(map[string]any); !ok {
		t.Errorf("userState: got %T, want map[string]any", result["userState"])
	}
	troiki, ok := result["troiki"].(map[string]any)
	if !ok {
		t.Fatalf("troiki: got %T, want map[string]any", result["troiki"])
	}
	for _, slot := range []string{"important", "medium", "rest"} {
		if _, ok := troiki[slot]; !ok {
			t.Errorf("troiki.%s missing", slot)
		}
	}
}

func TestMetaConfig_RequiresAuth(t *testing.T) {
	e := setupAPIEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	resp, body := doReq(t, e.app, req)
	if resp.StatusCode != 401 {
		t.Fatalf("got %d, want 401; body: %s", resp.StatusCode, body)
	}
}

func TestHealthz_Public(t *testing.T) {
	e := setupAPIEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	resp, _ := doReq(t, e.app, req)
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
}

func TestVersion_Public(t *testing.T) {
	e := setupAPIEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	resp, body := doReq(t, e.app, req)
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result["version"] == nil {
		t.Error("version field missing")
	}
}

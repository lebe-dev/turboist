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

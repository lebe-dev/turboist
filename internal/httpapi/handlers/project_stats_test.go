package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestProjectStats_RouteIsNotSwallowedByID(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/projects/stats", nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /projects/stats: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var out struct {
		Ranges map[string]struct {
			Days int `json:"days"`
		} `json:"ranges"`
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("parse: %v", err)
	}
	for key, days := range map[string]int{"week": 7, "month": 30, "quarter": 90} {
		if out.Ranges[key].Days != days {
			t.Errorf("range %s days: got %d, want %d", key, out.Ranges[key].Days, days)
		}
	}
	if out.Items == nil {
		t.Errorf("items: got null, want an empty array")
	}
}

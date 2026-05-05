package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestSettingsHandler_GetDefault(t *testing.T) {
	env := setupAPIEnv(t)
	req := env.authedReq(t, http.MethodGet, "/api/v1/settings", nil)
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
	ids, ok := body["weeklyUnplannedExcludedLabelIds"]
	if !ok {
		t.Fatal("missing weeklyUnplannedExcludedLabelIds")
	}
	slice, ok := ids.([]any)
	if !ok {
		t.Fatalf("weeklyUnplannedExcludedLabelIds: got %T, want []any", ids)
	}
	if len(slice) != 0 {
		t.Errorf("weeklyUnplannedExcludedLabelIds: got %v, want []", slice)
	}
}

func TestSettingsHandler_Patch(t *testing.T) {
	env := setupAPIEnv(t)

	req := env.authedReq(t, http.MethodPatch, "/api/v1/settings", map[string]any{
		"weeklyUnplannedExcludedLabelIds": []int{10, 20},
	})
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// verify persisted via GET
	req2 := env.authedReq(t, http.MethodGet, "/api/v1/settings", nil)
	resp2, err := env.app.Test(req2)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	ids, _ := body["weeklyUnplannedExcludedLabelIds"].([]any)
	if len(ids) != 2 {
		t.Fatalf("weeklyUnplannedExcludedLabelIds: got %v, want [10 20]", ids)
	}
}

func TestSettingsHandler_PatchClear(t *testing.T) {
	env := setupAPIEnv(t)

	env.authedReq(t, http.MethodPatch, "/api/v1/settings", map[string]any{
		"weeklyUnplannedExcludedLabelIds": []int{5},
	})
	req := env.authedReq(t, http.MethodPatch, "/api/v1/settings", map[string]any{
		"weeklyUnplannedExcludedLabelIds": []int{},
	})
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	req2 := env.authedReq(t, http.MethodGet, "/api/v1/settings", nil)
	resp2, err := env.app.Test(req2)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	ids, _ := body["weeklyUnplannedExcludedLabelIds"].([]any)
	if len(ids) != 0 {
		t.Fatalf("weeklyUnplannedExcludedLabelIds: got %v, want []", ids)
	}
}

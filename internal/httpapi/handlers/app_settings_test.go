package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestAppSettings_GetDefault(t *testing.T) {
	env := setupAPIEnv(t)
	req := env.authedReq(t, http.MethodGet, "/api/v1/app-settings", nil)
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
	rules, ok := body["autoLabels"].([]any)
	if !ok {
		t.Fatalf("autoLabels: got %T, want []any", body["autoLabels"])
	}
	if len(rules) != 0 {
		t.Errorf("autoLabels: got %v, want []", rules)
	}
}

func TestAppSettings_PutAutoLabels(t *testing.T) {
	env := setupAPIEnv(t)
	ctx := context.Background()

	l1, err := env.labels.Create(ctx, "shopping", "blue", false)
	if err != nil {
		t.Fatalf("seed label: %v", err)
	}
	l2, err := env.labels.Create(ctx, "project", "green", false)
	if err != nil {
		t.Fatalf("seed label: %v", err)
	}

	req := env.authedReq(t, http.MethodPut, "/api/v1/app-settings/auto-labels", map[string]any{
		"autoLabels": []map[string]any{
			{"mask": "buy", "labelIds": []int64{l1.ID}, "ignoreCase": true},
			{"mask": "Proj -", "labelIds": []int64{l2.ID, l1.ID}, "ignoreCase": false},
		},
	})
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
	rules := body["autoLabels"].([]any)
	if len(rules) != 2 {
		t.Fatalf("autoLabels count: got %d, want 2", len(rules))
	}
	r1 := rules[1].(map[string]any)
	if ids, ok := r1["labelIds"].([]any); !ok || len(ids) != 2 {
		t.Errorf("rule[1].labelIds: got %v, want 2 ids", r1["labelIds"])
	}
}

func TestAppSettings_PutAutoLabels_EmptyMaskRejected(t *testing.T) {
	env := setupAPIEnv(t)
	l, _ := env.labels.Create(context.Background(), "x", "blue", false)

	req := env.authedReq(t, http.MethodPut, "/api/v1/app-settings/auto-labels", map[string]any{
		"autoLabels": []map[string]any{
			{"mask": "", "labelIds": []int64{l.ID}, "ignoreCase": true},
		},
	})
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", resp.StatusCode)
	}
}

func TestAppSettings_PutAutoLabels_EmptyLabelIDsRejected(t *testing.T) {
	env := setupAPIEnv(t)

	req := env.authedReq(t, http.MethodPut, "/api/v1/app-settings/auto-labels", map[string]any{
		"autoLabels": []map[string]any{
			{"mask": "x", "labelIds": []int64{}, "ignoreCase": true},
		},
	})
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", resp.StatusCode)
	}
}

func TestAppSettings_PutAutoLabels_UnknownLabelRejected(t *testing.T) {
	env := setupAPIEnv(t)

	req := env.authedReq(t, http.MethodPut, "/api/v1/app-settings/auto-labels", map[string]any{
		"autoLabels": []map[string]any{
			{"mask": "x", "labelIds": []int64{9999}, "ignoreCase": true},
		},
	})
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", resp.StatusCode)
	}
}

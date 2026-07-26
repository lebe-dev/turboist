package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/lebe-dev/turboist/internal/repo"
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
	suggestions, ok := body["projectSuggestions"].([]any)
	if !ok {
		t.Fatalf("projectSuggestions: got %T, want []any", body["projectSuggestions"])
	}
	if len(suggestions) != 0 {
		t.Errorf("projectSuggestions: got %v, want []", suggestions)
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

// seedSuggestionProject seeds a context + project and returns the project id.
func seedSuggestionProject(t *testing.T, env *apiEnv, title string) int64 {
	t.Helper()
	ctx := context.Background()
	c, err := env.ctxs.Create(ctx, title+" ctx", "blue", false)
	if err != nil {
		t.Fatalf("seed context: %v", err)
	}
	p, err := env.projects.Create(ctx, repo.CreateProject{ContextID: c.ID, Title: title, Color: "red"})
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}
	return p.ID
}

func TestAppSettings_PutProjectSuggestions(t *testing.T) {
	env := setupAPIEnv(t)
	p1 := seedSuggestionProject(t, env, "Infra")
	p2 := seedSuggestionProject(t, env, "DevOps")

	req := env.authedReq(t, http.MethodPut, "/api/v1/app-settings/project-suggestions", map[string]any{
		"projectSuggestions": []map[string]any{
			{"mask": "deploy", "projectIds": []int64{p1, p2}, "ignoreCase": true},
			{"mask": "Bill", "projectIds": []int64{p2}, "ignoreCase": false},
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
	rules, ok := body["projectSuggestions"].([]any)
	if !ok {
		t.Fatalf("projectSuggestions: got %T, want []any", body["projectSuggestions"])
	}
	if len(rules) != 2 {
		t.Fatalf("projectSuggestions count: got %d, want 2", len(rules))
	}
	r0 := rules[0].(map[string]any)
	if ids, ok := r0["projectIds"].([]any); !ok || len(ids) != 2 {
		t.Errorf("rule[0].projectIds: got %v, want 2 ids", r0["projectIds"])
	}
	if r0["ignoreCase"] != true {
		t.Errorf("rule[0].ignoreCase: got %v, want true", r0["ignoreCase"])
	}
}

func TestAppSettings_PutProjectSuggestions_PreservesAutoLabels(t *testing.T) {
	env := setupAPIEnv(t)
	l, err := env.labels.Create(context.Background(), "shopping", "blue", false)
	if err != nil {
		t.Fatalf("seed label: %v", err)
	}
	p := seedSuggestionProject(t, env, "Infra")

	req := env.authedReq(t, http.MethodPut, "/api/v1/app-settings/auto-labels", map[string]any{
		"autoLabels": []map[string]any{{"mask": "buy", "labelIds": []int64{l.ID}, "ignoreCase": true}},
	})
	if _, err := env.app.Test(req); err != nil {
		t.Fatal(err)
	}

	req = env.authedReq(t, http.MethodPut, "/api/v1/app-settings/project-suggestions", map[string]any{
		"projectSuggestions": []map[string]any{{"mask": "deploy", "projectIds": []int64{p}, "ignoreCase": true}},
	})
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if labels, ok := body["autoLabels"].([]any); !ok || len(labels) != 1 {
		t.Errorf("autoLabels: got %v, want 1 rule preserved", body["autoLabels"])
	}
}

func TestAppSettings_PutProjectSuggestions_DedupesProjectIDs(t *testing.T) {
	env := setupAPIEnv(t)
	p := seedSuggestionProject(t, env, "Infra")

	req := env.authedReq(t, http.MethodPut, "/api/v1/app-settings/project-suggestions", map[string]any{
		"projectSuggestions": []map[string]any{
			{"mask": "deploy", "projectIds": []int64{p, p}, "ignoreCase": true},
		},
	})
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	r0 := body["projectSuggestions"].([]any)[0].(map[string]any)
	if ids, ok := r0["projectIds"].([]any); !ok || len(ids) != 1 {
		t.Errorf("rule[0].projectIds: got %v, want 1 id", r0["projectIds"])
	}
}

func TestAppSettings_PutProjectSuggestions_EmptyMaskRejected(t *testing.T) {
	env := setupAPIEnv(t)
	p := seedSuggestionProject(t, env, "Infra")

	req := env.authedReq(t, http.MethodPut, "/api/v1/app-settings/project-suggestions", map[string]any{
		"projectSuggestions": []map[string]any{
			{"mask": "   ", "projectIds": []int64{p}, "ignoreCase": true},
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

func TestAppSettings_PutProjectSuggestions_EmptyProjectIDsRejected(t *testing.T) {
	env := setupAPIEnv(t)

	req := env.authedReq(t, http.MethodPut, "/api/v1/app-settings/project-suggestions", map[string]any{
		"projectSuggestions": []map[string]any{
			{"mask": "x", "projectIds": []int64{}, "ignoreCase": true},
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

func TestAppSettings_PutProjectSuggestions_UnknownProjectRejected(t *testing.T) {
	env := setupAPIEnv(t)

	req := env.authedReq(t, http.MethodPut, "/api/v1/app-settings/project-suggestions", map[string]any{
		"projectSuggestions": []map[string]any{
			{"mask": "x", "projectIds": []int64{9999}, "ignoreCase": true},
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

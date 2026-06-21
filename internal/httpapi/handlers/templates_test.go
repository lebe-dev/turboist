package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/repo"
)

func TestTemplateCreate_Success(t *testing.T) {
	e := setupAPIEnv(t)
	l, _ := e.labels.Create(context.Background(), "work", "blue", false)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/task-templates/", map[string]any{
		"name":        "Onboard",
		"description": "desc",
		"priority":    "high",
		"dayPart":     "morning",
		"labelIds":    []int64{l.ID},
		"subtasks": []map[string]any{
			{"title": "Call", "priority": "medium", "labelIds": []int64{l.ID}},
			{"title": "Email"},
		},
	}))
	if resp.StatusCode != 201 {
		t.Fatalf("got %d, want 201; body: %s", resp.StatusCode, body)
	}
	var result dto.TaskTemplateDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Name != "Onboard" || result.Priority != "high" {
		t.Errorf("root: got %+v", result)
	}
	if len(result.Subtasks) != 2 || result.Subtasks[0].Title != "Call" {
		t.Errorf("subtasks: got %+v", result.Subtasks)
	}
	if len(result.Labels) != 1 {
		t.Errorf("labels: got %d, want 1", len(result.Labels))
	}
}

func TestTemplateCreate_Validation(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/task-templates/", map[string]any{
		"name": "",
	}))
	if resp.StatusCode != 400 {
		t.Fatalf("got %d, want 400; body: %s", resp.StatusCode, body)
	}
}

func TestTemplateCreate_InvalidPriority(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/task-templates/", map[string]any{
		"name": "x", "priority": "bogus",
	}))
	if resp.StatusCode != 400 {
		t.Fatalf("got %d, want 400; body: %s", resp.StatusCode, body)
	}
}

func TestTemplateCreate_SubtaskTitleRequired(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/task-templates/", map[string]any{
		"name":     "x",
		"subtasks": []map[string]any{{"title": ""}},
	}))
	if resp.StatusCode != 400 {
		t.Fatalf("got %d, want 400; body: %s", resp.StatusCode, body)
	}
}

func TestTemplateList(t *testing.T) {
	e := setupAPIEnv(t)
	if _, err := e.templates.Create(context.Background(), repo.TemplateInput{Name: "A"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/task-templates/", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.PagedResponse[dto.TaskTemplateDTO]
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(result.Items) != 1 {
		t.Errorf("items: got %d, want 1", len(result.Items))
	}
}

func TestTemplatePatch_Replace(t *testing.T) {
	e := setupAPIEnv(t)
	tmpl, err := e.templates.Create(context.Background(), repo.TemplateInput{
		Name:     "v1",
		Subtasks: []repo.TemplateSubtaskInput{{Title: "old"}},
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch, "/api/v1/task-templates/"+itoa(tmpl.ID), map[string]any{
		"name":     "v2",
		"subtasks": []map[string]any{{"title": "new"}},
	}))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.TaskTemplateDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Name != "v2" || len(result.Subtasks) != 1 || result.Subtasks[0].Title != "new" {
		t.Errorf("replace: got %+v", result)
	}
}

func TestTemplateDelete(t *testing.T) {
	e := setupAPIEnv(t)
	tmpl, err := e.templates.Create(context.Background(), repo.TemplateInput{Name: "t"})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodDelete, "/api/v1/task-templates/"+itoa(tmpl.ID), nil))
	if resp.StatusCode != 204 {
		t.Fatalf("got %d, want 204; body: %s", resp.StatusCode, body)
	}
	resp, _ = doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/task-templates/"+itoa(tmpl.ID), nil))
	if resp.StatusCode != 404 {
		t.Errorf("get after delete: got %d, want 404", resp.StatusCode)
	}
}

func TestTemplateInstantiate_Success(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := context.Background()
	work, err := e.ctxs.Create(ctx, "work", "blue", false)
	if err != nil {
		t.Fatalf("ctx: %v", err)
	}
	proj, err := e.projects.Create(ctx, repo.CreateProject{ContextID: work.ID, Title: "Q3", Color: "purple"})
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	tmpl, err := e.templates.Create(ctx, repo.TemplateInput{
		Name:     "Sprint",
		Subtasks: []repo.TemplateSubtaskInput{{Title: "Plan"}, {Title: "Review"}},
	})
	if err != nil {
		t.Fatalf("template: %v", err)
	}

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/task-templates/"+itoa(tmpl.ID)+"/instantiate", map[string]any{
		"projectId": proj.ID,
	}))
	if resp.StatusCode != 201 {
		t.Fatalf("got %d, want 201; body: %s", resp.StatusCode, body)
	}
	var result dto.InstantiateTemplateResponse
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Root.Title != "Sprint" {
		t.Errorf("root title: got %q, want Sprint", result.Root.Title)
	}
	if result.Root.ProjectID == nil || *result.Root.ProjectID != proj.ID {
		t.Errorf("root project: got %v, want %d", result.Root.ProjectID, proj.ID)
	}
	if len(result.Subtasks) != 2 {
		t.Fatalf("subtasks: got %d, want 2", len(result.Subtasks))
	}
	for _, st := range result.Subtasks {
		if st.ParentID == nil || *st.ParentID != result.Root.ID {
			t.Errorf("subtask parent: got %v, want %d", st.ParentID, result.Root.ID)
		}
	}
}

func TestTemplateInstantiate_ProjectRequired(t *testing.T) {
	e := setupAPIEnv(t)
	tmpl, err := e.templates.Create(context.Background(), repo.TemplateInput{Name: "t"})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/task-templates/"+itoa(tmpl.ID)+"/instantiate", map[string]any{}))
	if resp.StatusCode != 400 {
		t.Fatalf("got %d, want 400; body: %s", resp.StatusCode, body)
	}
}

func TestTemplateInstantiate_TemplateNotFound(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := context.Background()
	work, _ := e.ctxs.Create(ctx, "work", "blue", false)
	proj, _ := e.projects.Create(ctx, repo.CreateProject{ContextID: work.ID, Title: "P", Color: "blue"})
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/task-templates/999/instantiate", map[string]any{
		"projectId": proj.ID,
	}))
	if resp.StatusCode != 404 {
		t.Fatalf("got %d, want 404; body: %s", resp.StatusCode, body)
	}
}

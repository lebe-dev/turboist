package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
)

func TestLabelList_Empty(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/labels/", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.PagedResponse[dto.LabelDTO]
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(result.Items) != 0 {
		t.Errorf("items: got %d, want 0", len(result.Items))
	}
}

func TestLabelCreate_Success(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/labels/", map[string]any{
		"name":        "urgent",
		"color":       "red",
		"isFavourite": false,
	}))
	if resp.StatusCode != 201 {
		t.Fatalf("got %d, want 201; body: %s", resp.StatusCode, body)
	}
	var result dto.LabelDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Name != "urgent" {
		t.Errorf("name: got %q, want urgent", result.Name)
	}
}

func TestLabelCreate_Conflict(t *testing.T) {
	e := setupAPIEnv(t)
	body := map[string]any{"name": "dup-label", "color": "blue"}
	doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/labels/", body))
	resp, b := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/labels/", body))
	if resp.StatusCode != 409 {
		t.Fatalf("got %d, want 409; body: %s", resp.StatusCode, b)
	}
	er := parseErr(t, b)
	if er.Error.Code != httpapi.CodeConflict {
		t.Errorf("code: got %q, want %q", er.Error.Code, httpapi.CodeConflict)
	}
}

func TestLabelCreate_Validation(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/labels/", map[string]any{
		"name": "", "color": "red",
	}))
	if resp.StatusCode != 400 {
		t.Fatalf("got %d, want 400; body: %s", resp.StatusCode, body)
	}
}

func TestLabelGet_Found(t *testing.T) {
	e := setupAPIEnv(t)
	l, err := e.labels.Create(context.Background(), "work", "blue", false)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/labels/"+itoa(l.ID), nil))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.LabelDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.ID != l.ID {
		t.Errorf("id: got %d, want %d", result.ID, l.ID)
	}
}

func TestLabelGet_NotFound(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/labels/9999", nil))
	if resp.StatusCode != 404 {
		t.Fatalf("got %d, want 404; body: %s", resp.StatusCode, body)
	}
	er := parseErr(t, body)
	if er.Error.Code != httpapi.CodeNotFound {
		t.Errorf("code: got %q, want %q", er.Error.Code, httpapi.CodeNotFound)
	}
}

func TestLabelPatch_Success(t *testing.T) {
	e := setupAPIEnv(t)
	l, err := e.labels.Create(context.Background(), "old-label", "red", false)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	name := "new-label"
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch, "/api/v1/labels/"+itoa(l.ID), map[string]any{
		"name": name,
	}))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.LabelDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Name != name {
		t.Errorf("name: got %q, want %q", result.Name, name)
	}
}

func TestLabelDelete_Success(t *testing.T) {
	e := setupAPIEnv(t)
	l, err := e.labels.Create(context.Background(), "to-delete", "green", false)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	resp, _ := doReq(t, e.app, e.authedReq(t, http.MethodDelete, "/api/v1/labels/"+itoa(l.ID), nil))
	if resp.StatusCode != 204 {
		t.Fatalf("delete: got %d, want 204", resp.StatusCode)
	}
	resp2, _ := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/labels/"+itoa(l.ID), nil))
	if resp2.StatusCode != 404 {
		t.Fatalf("after delete: got %d, want 404", resp2.StatusCode)
	}
}

func TestLabelDelete_NotFound(t *testing.T) {
	e := setupAPIEnv(t)
	resp, _ := doReq(t, e.app, e.authedReq(t, http.MethodDelete, "/api/v1/labels/9999", nil))
	if resp.StatusCode != 404 {
		t.Fatalf("got %d, want 404", resp.StatusCode)
	}
}

func TestLabelListTasks_Empty(t *testing.T) {
	e := setupAPIEnv(t)
	l, err := e.labels.Create(context.Background(), "my-label", "teal", false)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/labels/"+itoa(l.ID)+"/tasks", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.PagedResponse[dto.TaskDTO]
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(result.Items) != 0 {
		t.Errorf("items: got %d, want 0", len(result.Items))
	}
}

func TestLabelListProjects_Empty(t *testing.T) {
	e := setupAPIEnv(t)
	l, err := e.labels.Create(context.Background(), "proj-label", "purple", false)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/labels/"+itoa(l.ID)+"/projects", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.PagedResponse[dto.ProjectDTO]
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(result.Items) != 0 {
		t.Errorf("items: got %d, want 0", len(result.Items))
	}
}

func TestLabelListProjects_LabelNotFound(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/labels/9999/projects", nil))
	if resp.StatusCode != 404 {
		t.Fatalf("got %d, want 404; body: %s", resp.StatusCode, body)
	}
}

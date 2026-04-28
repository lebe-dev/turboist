package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
)

func TestContextList_Empty(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/contexts/", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.PagedResponse[dto.ContextDTO]
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse response: %v — body: %s", err, body)
	}
	if len(result.Items) != 0 {
		t.Errorf("items: got %d, want 0", len(result.Items))
	}
	if result.Total != 0 {
		t.Errorf("total: got %d, want 0", result.Total)
	}
}

func TestContextCreate_Success(t *testing.T) {
	e := setupAPIEnv(t)
	req := e.authedReq(t, http.MethodPost, "/api/v1/contexts/", map[string]any{
		"name":        "Work",
		"color":       "blue",
		"isFavourite": true,
	})
	resp, body := doReq(t, e.app, req)
	if resp.StatusCode != 201 {
		t.Fatalf("got %d, want 201; body: %s", resp.StatusCode, body)
	}
	var result dto.ContextDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v — body: %s", err, body)
	}
	if result.Name != "Work" {
		t.Errorf("name: got %q, want %q", result.Name, "Work")
	}
	if result.Color != "blue" {
		t.Errorf("color: got %q, want %q", result.Color, "blue")
	}
	if !result.IsFavourite {
		t.Error("isFavourite: got false, want true")
	}
	if result.ID == 0 {
		t.Error("id must not be zero")
	}
}

func TestContextCreate_Conflict(t *testing.T) {
	e := setupAPIEnv(t)
	body1 := map[string]any{"name": "Dup", "color": "red", "isFavourite": false}
	resp1, _ := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/contexts/", body1))
	if resp1.StatusCode != 201 {
		t.Fatalf("first create: got %d, want 201", resp1.StatusCode)
	}
	resp2, b2 := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/contexts/", body1))
	if resp2.StatusCode != 409 {
		t.Fatalf("duplicate: got %d, want 409; body: %s", resp2.StatusCode, b2)
	}
	er := parseErr(t, b2)
	if er.Error.Code != httpapi.CodeConflict {
		t.Errorf("code: got %q, want %q", er.Error.Code, httpapi.CodeConflict)
	}
}

func TestContextCreate_Validation(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/contexts/", map[string]any{
		"name": "", "color": "red",
	}))
	if resp.StatusCode != 400 {
		t.Fatalf("got %d, want 400; body: %s", resp.StatusCode, body)
	}
	er := parseErr(t, body)
	if er.Error.Code != httpapi.CodeValidationFailed {
		t.Errorf("code: got %q, want %q", er.Error.Code, httpapi.CodeValidationFailed)
	}
}

func TestContextGet_Found(t *testing.T) {
	e := setupAPIEnv(t)
	ctx, err := e.ctxs.Create(context.Background(), "Personal", "green", false)
	if err != nil {
		t.Fatalf("create context: %v", err)
	}
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/contexts/"+itoa(ctx.ID), nil))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.ContextDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.ID != ctx.ID {
		t.Errorf("id: got %d, want %d", result.ID, ctx.ID)
	}
}

func TestContextGet_NotFound(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/contexts/9999", nil))
	if resp.StatusCode != 404 {
		t.Fatalf("got %d, want 404; body: %s", resp.StatusCode, body)
	}
	er := parseErr(t, body)
	if er.Error.Code != httpapi.CodeNotFound {
		t.Errorf("code: got %q, want %q", er.Error.Code, httpapi.CodeNotFound)
	}
}

func TestContextPatch_Success(t *testing.T) {
	e := setupAPIEnv(t)
	ctx, err := e.ctxs.Create(context.Background(), "OldName", "blue", false)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	name := "NewName"
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch, "/api/v1/contexts/"+itoa(ctx.ID), map[string]any{
		"name": name,
	}))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.ContextDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Name != name {
		t.Errorf("name: got %q, want %q", result.Name, name)
	}
}

func TestContextDelete_Success(t *testing.T) {
	e := setupAPIEnv(t)
	ctx, err := e.ctxs.Create(context.Background(), "ToDelete", "red", false)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	resp, _ := doReq(t, e.app, e.authedReq(t, http.MethodDelete, "/api/v1/contexts/"+itoa(ctx.ID), nil))
	if resp.StatusCode != 204 {
		t.Fatalf("delete: got %d, want 204", resp.StatusCode)
	}
	// Verify it's gone.
	resp2, body2 := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/contexts/"+itoa(ctx.ID), nil))
	if resp2.StatusCode != 404 {
		t.Fatalf("after delete: got %d, want 404; body: %s", resp2.StatusCode, body2)
	}
}

func TestContextDelete_NotFound(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodDelete, "/api/v1/contexts/9999", nil))
	if resp.StatusCode != 404 {
		t.Fatalf("got %d, want 404; body: %s", resp.StatusCode, body)
	}
}

func TestContextListProjects_Empty(t *testing.T) {
	e := setupAPIEnv(t)
	ctx, err := e.ctxs.Create(context.Background(), "Ctx", "blue", false)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/contexts/"+itoa(ctx.ID)+"/projects", nil))
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

func TestContextListProjects_ContextNotFound(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/contexts/9999/projects", nil))
	if resp.StatusCode != 404 {
		t.Fatalf("got %d, want 404; body: %s", resp.StatusCode, body)
	}
}

func TestContextListTasks_Empty(t *testing.T) {
	e := setupAPIEnv(t)
	ctx, err := e.ctxs.Create(context.Background(), "Ctx2", "green", false)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/contexts/"+itoa(ctx.ID)+"/tasks", nil))
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

func TestContextCreateTask_Success(t *testing.T) {
	e := setupAPIEnv(t)
	ctx, err := e.ctxs.Create(context.Background(), "CtxTask", "blue", false)
	if err != nil {
		t.Fatalf("create context: %v", err)
	}
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/contexts/"+itoa(ctx.ID)+"/tasks", map[string]any{
		"title":       "My task",
		"description": "desc",
	}))
	if resp.StatusCode != 201 {
		t.Fatalf("got %d, want 201; body: %s", resp.StatusCode, body)
	}
	var result dto.TaskDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Title != "My task" {
		t.Errorf("title: got %q, want %q", result.Title, "My task")
	}
	if result.ContextID == nil || *result.ContextID != ctx.ID {
		t.Errorf("contextId: got %v, want %d", result.ContextID, ctx.ID)
	}
}

func TestContextCreateTask_ContextNotFound(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/contexts/9999/tasks", map[string]any{
		"title": "Task",
	}))
	if resp.StatusCode != 404 {
		t.Fatalf("got %d, want 404; body: %s", resp.StatusCode, body)
	}
}

func TestContextRequiresAuth(t *testing.T) {
	e := setupAPIEnv(t)
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/contexts/", nil)
	resp, body := doReq(t, e.app, req)
	if resp.StatusCode != 401 {
		t.Fatalf("got %d, want 401; body: %s", resp.StatusCode, body)
	}
}

func itoa(id int64) string {
	return strconv.FormatInt(id, 10)
}

package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
)

type searchResp struct {
	Tasks    *dto.PagedResponse[dto.TaskDTO]    `json:"tasks,omitempty"`
	Projects *dto.PagedResponse[dto.ProjectDTO] `json:"projects,omitempty"`
}

func TestSearch_ShortQuery(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/search?q=a", nil))
	if resp.StatusCode != 400 {
		t.Fatalf("got %d, want 400; body: %s", resp.StatusCode, body)
	}
	er := parseErr(t, body)
	if er.Error.Code != httpapi.CodeValidationFailed {
		t.Errorf("code: got %q, want %q", er.Error.Code, httpapi.CodeValidationFailed)
	}
}

func TestSearch_Tasks(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	createTestTask(t, e, ctx.ID, "Buy groceries")
	createTestTask(t, e, ctx.ID, "Buy milk")
	createTestTask(t, e, ctx.ID, "Something else")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet,
		"/api/v1/search?q=Buy&type=tasks", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result searchResp
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Tasks == nil {
		t.Fatal("tasks should be present when type=tasks")
	}
	if result.Tasks.Total != 2 {
		t.Errorf("tasks total: got %d, want 2", result.Tasks.Total)
	}
	if result.Projects != nil {
		t.Error("projects should be absent when type=tasks")
	}
}

func TestSearch_Projects(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	createTestProject(t, e, ctx.ID, "Alpha project")
	createTestProject(t, e, ctx.ID, "Beta project")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet,
		"/api/v1/search?q=project&type=projects", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result searchResp
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Projects == nil {
		t.Fatal("projects should be present when type=projects")
	}
	if result.Projects.Total != 2 {
		t.Errorf("projects total: got %d, want 2", result.Projects.Total)
	}
	if result.Tasks != nil {
		t.Error("tasks should be absent when type=projects")
	}
}

func TestSearch_All(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	createTestTask(t, e, ctx.ID, "Alpha task")
	createTestProject(t, e, ctx.ID, "Alpha project")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet,
		"/api/v1/search?q=Alpha", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result searchResp
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Tasks == nil || result.Tasks.Total != 1 {
		t.Errorf("tasks: got %v", result.Tasks)
	}
	if result.Projects == nil || result.Projects.Total != 1 {
		t.Errorf("projects: got %v", result.Projects)
	}
}

func TestSearch_InvalidType(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet,
		"/api/v1/search?q=ab&type=unknown", nil))
	if resp.StatusCode != 400 {
		t.Fatalf("got %d, want 400; body: %s", resp.StatusCode, body)
	}
}

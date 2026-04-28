package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/repo"
)

// helpers to create test fixtures

func createTestContext(t *testing.T, e *apiEnv, name string) dto.ContextDTO {
	t.Helper()
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/contexts/", map[string]any{
		"name": name, "color": "blue",
	}))
	if resp.StatusCode != 201 {
		t.Fatalf("create context: got %d, want 201; body: %s", resp.StatusCode, body)
	}
	var result dto.ContextDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse context: %v", err)
	}
	return result
}

func createTestProject(t *testing.T, e *apiEnv, contextID int64, title string) dto.ProjectDTO {
	t.Helper()
	url := fmt.Sprintf("/api/v1/contexts/%d/projects", contextID)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, url, map[string]any{"title": title, "color": "blue"}))
	if resp.StatusCode != 201 {
		t.Fatalf("create project: got %d, want 201; body: %s", resp.StatusCode, body)
	}
	var result dto.ProjectDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse project: %v", err)
	}
	return result
}

func TestProjectList_Empty(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/projects/", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.PagedResponse[dto.ProjectDTO]
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("total: got %d, want 0", result.Total)
	}
	if len(result.Items) != 0 {
		t.Errorf("items: got %d, want 0", len(result.Items))
	}
}

func TestProjectCreate_Success(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")

	url := fmt.Sprintf("/api/v1/contexts/%d/projects", ctx.ID)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, url, map[string]any{
		"title":       "My Project",
		"description": "desc",
		"color":       "green",
	}))
	if resp.StatusCode != 201 {
		t.Fatalf("got %d, want 201; body: %s", resp.StatusCode, body)
	}
	var result dto.ProjectDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Title != "My Project" {
		t.Errorf("title: got %q, want %q", result.Title, "My Project")
	}
	if result.ContextID != ctx.ID {
		t.Errorf("contextId: got %d, want %d", result.ContextID, ctx.ID)
	}
	if result.Status != "open" {
		t.Errorf("status: got %q, want %q", result.Status, "open")
	}
	if result.IsPinned {
		t.Error("isPinned: got true, want false")
	}
	if result.ID == 0 {
		t.Error("id must not be zero")
	}
}

func TestProjectCreate_ContextNotFound(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/contexts/9999/projects", map[string]any{
		"title": "X",
	}))
	if resp.StatusCode != 404 {
		t.Fatalf("got %d, want 404; body: %s", resp.StatusCode, body)
	}
}

func TestProjectCreate_ValidationNoTitle(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	url := fmt.Sprintf("/api/v1/contexts/%d/projects", ctx.ID)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, url, map[string]any{"color": "blue"}))
	if resp.StatusCode != 400 {
		t.Fatalf("got %d, want 400; body: %s", resp.StatusCode, body)
	}
	er := parseErr(t, body)
	if er.Error.Code != httpapi.CodeValidationFailed {
		t.Errorf("code: got %q, want %q", er.Error.Code, httpapi.CodeValidationFailed)
	}
}

func TestProjectCreate_WithLabels(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")

	// Create a label first
	lblResp, lblBody := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/labels/", map[string]any{"name": "frontend", "color": "blue"}))
	if lblResp.StatusCode != 201 {
		t.Fatalf("create label: got %d, want 201; body: %s", lblResp.StatusCode, lblBody)
	}

	url := fmt.Sprintf("/api/v1/contexts/%d/projects", ctx.ID)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, url, map[string]any{
		"title":  "Labeled",
		"color":  "purple",
		"labels": []string{"frontend"},
	}))
	if resp.StatusCode != 201 {
		t.Fatalf("got %d, want 201; body: %s", resp.StatusCode, body)
	}
	var result dto.ProjectDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(result.Labels) != 1 {
		t.Errorf("labels: got %d, want 1", len(result.Labels))
	}
	if result.Labels[0].Name != "frontend" {
		t.Errorf("label name: got %q, want %q", result.Labels[0].Name, "frontend")
	}
}

func TestProjectCreate_UnknownLabel(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	url := fmt.Sprintf("/api/v1/contexts/%d/projects", ctx.ID)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, url, map[string]any{
		"title":  "X",
		"labels": []string{"nonexistent"},
	}))
	if resp.StatusCode != 400 {
		t.Fatalf("got %d, want 400; body: %s", resp.StatusCode, body)
	}
	er := parseErr(t, body)
	if er.Error.Code != httpapi.CodeValidationFailed {
		t.Errorf("code: got %q, want %q", er.Error.Code, httpapi.CodeValidationFailed)
	}
}

func TestProjectGet_Success(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	proj := createTestProject(t, e, ctx.ID, "Alpha")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, fmt.Sprintf("/api/v1/projects/%d", proj.ID), nil))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.ProjectDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.ID != proj.ID {
		t.Errorf("id: got %d, want %d", result.ID, proj.ID)
	}
}

func TestProjectGet_NotFound(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/projects/9999", nil))
	if resp.StatusCode != 404 {
		t.Fatalf("got %d, want 404; body: %s", resp.StatusCode, body)
	}
}

func TestProjectPatch_Success(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	proj := createTestProject(t, e, ctx.ID, "Alpha")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch,
		fmt.Sprintf("/api/v1/projects/%d", proj.ID),
		map[string]any{"title": "Beta", "description": "updated"}))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.ProjectDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Title != "Beta" {
		t.Errorf("title: got %q, want %q", result.Title, "Beta")
	}
	if result.Description != "updated" {
		t.Errorf("description: got %q, want %q", result.Description, "updated")
	}
}

func TestProjectPatch_IgnoresStatusAndPin(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	proj := createTestProject(t, e, ctx.ID, "Alpha")

	// Send status and isPinned — these fields should be silently ignored
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch,
		fmt.Sprintf("/api/v1/projects/%d", proj.ID),
		map[string]any{"title": "Beta", "status": "completed", "isPinned": true}))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.ProjectDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Status != "open" {
		t.Errorf("status: got %q, want %q (status must not change via PATCH)", result.Status, "open")
	}
	if result.IsPinned {
		t.Error("isPinned: got true, want false (pin must not change via PATCH)")
	}
}

func TestProjectPatch_UpdateLabels(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	proj := createTestProject(t, e, ctx.ID, "Alpha")

	// Create labels
	doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/labels/", map[string]any{"name": "backend", "color": "green"}))

	// Patch with labels
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch,
		fmt.Sprintf("/api/v1/projects/%d", proj.ID),
		map[string]any{"labels": []string{"backend"}}))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.ProjectDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(result.Labels) != 1 || result.Labels[0].Name != "backend" {
		t.Errorf("labels: got %v, want [{backend}]", result.Labels)
	}

	// Patch with empty labels clears them
	resp2, body2 := doReq(t, e.app, e.authedReq(t, http.MethodPatch,
		fmt.Sprintf("/api/v1/projects/%d", proj.ID),
		map[string]any{"labels": []string{}}))
	if resp2.StatusCode != 200 {
		t.Fatalf("clear labels: got %d, want 200; body: %s", resp2.StatusCode, body2)
	}
	var result2 dto.ProjectDTO
	if err := json.Unmarshal(body2, &result2); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(result2.Labels) != 0 {
		t.Errorf("labels after clear: got %d, want 0", len(result2.Labels))
	}
}

func TestProjectDelete_Success(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	proj := createTestProject(t, e, ctx.ID, "Alpha")

	resp, _ := doReq(t, e.app, e.authedReq(t, http.MethodDelete, fmt.Sprintf("/api/v1/projects/%d", proj.ID), nil))
	if resp.StatusCode != 204 {
		t.Fatalf("got %d, want 204", resp.StatusCode)
	}

	// Verify it's gone
	resp2, _ := doReq(t, e.app, e.authedReq(t, http.MethodGet, fmt.Sprintf("/api/v1/projects/%d", proj.ID), nil))
	if resp2.StatusCode != 404 {
		t.Fatalf("after delete: got %d, want 404", resp2.StatusCode)
	}
}

func TestProjectDelete_NotFound(t *testing.T) {
	e := setupAPIEnv(t)
	resp, _ := doReq(t, e.app, e.authedReq(t, http.MethodDelete, "/api/v1/projects/9999", nil))
	if resp.StatusCode != 404 {
		t.Fatalf("got %d, want 404", resp.StatusCode)
	}
}

func TestProjectStatus_CompleteAndUncomplete(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	proj := createTestProject(t, e, ctx.ID, "Alpha")

	// complete
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, fmt.Sprintf("/api/v1/projects/%d/complete", proj.ID), nil))
	if resp.StatusCode != 200 {
		t.Fatalf("complete: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.ProjectDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Status != "completed" {
		t.Errorf("status after complete: got %q, want %q", result.Status, "completed")
	}

	// uncomplete
	resp2, body2 := doReq(t, e.app, e.authedReq(t, http.MethodPost, fmt.Sprintf("/api/v1/projects/%d/uncomplete", proj.ID), nil))
	if resp2.StatusCode != 200 {
		t.Fatalf("uncomplete: got %d, want 200; body: %s", resp2.StatusCode, body2)
	}
	var result2 dto.ProjectDTO
	if err := json.Unmarshal(body2, &result2); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result2.Status != "open" {
		t.Errorf("status after uncomplete: got %q, want %q", result2.Status, "open")
	}
}

func TestProjectStatus_CancelAndArchive(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	proj := createTestProject(t, e, ctx.ID, "Alpha")

	for _, tc := range []struct {
		action, wantStatus string
	}{
		{"cancel", "cancelled"},
		{"archive", "archived"},
	} {
		// reset to open first
		doReq(t, e.app, e.authedReq(t, http.MethodPost, fmt.Sprintf("/api/v1/projects/%d/uncomplete", proj.ID), nil))

		resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
			fmt.Sprintf("/api/v1/projects/%d/%s", proj.ID, tc.action), nil))
		if resp.StatusCode != 200 {
			t.Fatalf("%s: got %d, want 200; body: %s", tc.action, resp.StatusCode, body)
		}
		var result dto.ProjectDTO
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if result.Status != tc.wantStatus {
			t.Errorf("%s: status got %q, want %q", tc.action, result.Status, tc.wantStatus)
		}
	}
}

func TestProjectStatus_Unarchive(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	proj := createTestProject(t, e, ctx.ID, "Alpha")

	doReq(t, e.app, e.authedReq(t, http.MethodPost, fmt.Sprintf("/api/v1/projects/%d/archive", proj.ID), nil))
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, fmt.Sprintf("/api/v1/projects/%d/unarchive", proj.ID), nil))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.ProjectDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Status != "open" {
		t.Errorf("status: got %q, want %q", result.Status, "open")
	}
}

func TestProjectPin_Success(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	proj := createTestProject(t, e, ctx.ID, "Alpha")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, fmt.Sprintf("/api/v1/projects/%d/pin", proj.ID), nil))
	if resp.StatusCode != 200 {
		t.Fatalf("pin: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.ProjectDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !result.IsPinned {
		t.Error("isPinned: got false, want true")
	}
	if result.PinnedAt == nil {
		t.Error("pinnedAt must not be nil after pin")
	}
}

func TestProjectPin_LimitExceeded(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	cfg := makeTestConfig() // MaxPinned = 5

	// Pin exactly cfg.MaxPinned projects directly through repo to reach the limit
	for i := 0; i < cfg.MaxPinned; i++ {
		p, err := e.projects.Create(context.Background(), repo.CreateProject{
			ContextID: ctx.ID,
			Title:     fmt.Sprintf("Proj%d", i),
			Color:     "blue",
		})
		if err != nil {
			t.Fatalf("create project: %v", err)
		}
		if err := e.projects.SetPinned(context.Background(), p.ID, true); err != nil {
			t.Fatalf("set pinned: %v", err)
		}
	}

	// Create one more project and try to pin it — should fail
	extra, err := e.projects.Create(context.Background(), repo.CreateProject{
		ContextID: ctx.ID,
		Title:     "Extra",
		Color:     "blue",
	})
	if err != nil {
		t.Fatalf("create extra project: %v", err)
	}
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, fmt.Sprintf("/api/v1/projects/%d/pin", extra.ID), nil))
	if resp.StatusCode != 422 {
		t.Fatalf("got %d, want 422; body: %s", resp.StatusCode, body)
	}
	er := parseErr(t, body)
	if er.Error.Code != httpapi.CodeLimitExceeded {
		t.Errorf("code: got %q, want %q", er.Error.Code, httpapi.CodeLimitExceeded)
	}
}

func TestProjectUnpin_Success(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	proj := createTestProject(t, e, ctx.ID, "Alpha")

	doReq(t, e.app, e.authedReq(t, http.MethodPost, fmt.Sprintf("/api/v1/projects/%d/pin", proj.ID), nil))

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, fmt.Sprintf("/api/v1/projects/%d/unpin", proj.ID), nil))
	if resp.StatusCode != 200 {
		t.Fatalf("unpin: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.ProjectDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.IsPinned {
		t.Error("isPinned: got true, want false after unpin")
	}
}

func TestProjectListSections_Success(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	proj := createTestProject(t, e, ctx.ID, "Alpha")

	// Create a section via the project route
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/projects/%d/sections", proj.ID),
		map[string]any{"title": "Sprint 1"}))
	if resp.StatusCode != 201 {
		t.Fatalf("create section: got %d, want 201; body: %s", resp.StatusCode, body)
	}

	// List sections
	resp2, body2 := doReq(t, e.app, e.authedReq(t, http.MethodGet,
		fmt.Sprintf("/api/v1/projects/%d/sections", proj.ID), nil))
	if resp2.StatusCode != 200 {
		t.Fatalf("list sections: got %d, want 200; body: %s", resp2.StatusCode, body2)
	}
	var result dto.PagedResponse[dto.SectionDTO]
	if err := json.Unmarshal(body2, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("total: got %d, want 1", result.Total)
	}
	if result.Items[0].Title != "Sprint 1" {
		t.Errorf("title: got %q, want %q", result.Items[0].Title, "Sprint 1")
	}
}

func TestProjectCreateSection_ProjectNotFound(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/projects/9999/sections", map[string]any{"title": "X"}))
	if resp.StatusCode != 404 {
		t.Fatalf("got %d, want 404; body: %s", resp.StatusCode, body)
	}
}

func TestProjectListTasks_Success(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	proj := createTestProject(t, e, ctx.ID, "Alpha")

	// Create a task in the project
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/projects/%d/tasks", proj.ID),
		map[string]any{"title": "Task One"}))
	if resp.StatusCode != 201 {
		t.Fatalf("create task: got %d, want 201; body: %s", resp.StatusCode, body)
	}

	// List tasks
	resp2, body2 := doReq(t, e.app, e.authedReq(t, http.MethodGet,
		fmt.Sprintf("/api/v1/projects/%d/tasks", proj.ID), nil))
	if resp2.StatusCode != 200 {
		t.Fatalf("list tasks: got %d, want 200; body: %s", resp2.StatusCode, body2)
	}
	var result dto.PagedResponse[dto.TaskDTO]
	if err := json.Unmarshal(body2, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("total: got %d, want 1", result.Total)
	}
}

func TestProjectCreateTask_Success(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	proj := createTestProject(t, e, ctx.ID, "Alpha")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/projects/%d/tasks", proj.ID),
		map[string]any{"title": "Do something", "priority": "high"}))
	if resp.StatusCode != 201 {
		t.Fatalf("got %d, want 201; body: %s", resp.StatusCode, body)
	}
	var result dto.TaskDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Title != "Do something" {
		t.Errorf("title: got %q, want %q", result.Title, "Do something")
	}
	if result.ProjectID == nil || *result.ProjectID != proj.ID {
		t.Errorf("projectId: got %v, want %d", result.ProjectID, proj.ID)
	}
}

func TestProjectListFilter_ByStatus(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	proj := createTestProject(t, e, ctx.ID, "Alpha")

	// complete the project
	doReq(t, e.app, e.authedReq(t, http.MethodPost, fmt.Sprintf("/api/v1/projects/%d/complete", proj.ID), nil))

	// filter by completed
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/projects/?status=completed", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.PagedResponse[dto.ProjectDTO]
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("total: got %d, want 1", result.Total)
	}

	// filter by open — should be 0
	resp2, body2 := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/projects/?status=open", nil))
	if resp2.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp2.StatusCode, body2)
	}
	var result2 dto.PagedResponse[dto.ProjectDTO]
	if err := json.Unmarshal(body2, &result2); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result2.Total != 0 {
		t.Errorf("open count: got %d, want 0", result2.Total)
	}
}

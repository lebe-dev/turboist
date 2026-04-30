package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/repo"
)

// --- fixtures ---

func createTestTask(t *testing.T, e *apiEnv, contextID int64, title string) dto.TaskDTO {
	t.Helper()
	url := fmt.Sprintf("/api/v1/contexts/%d/tasks", contextID)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, url, map[string]any{"title": title}))
	if resp.StatusCode != 201 {
		t.Fatalf("create task: got %d, want 201; body: %s", resp.StatusCode, body)
	}
	var result dto.TaskDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse task: %v", err)
	}
	return result
}

func createTestLabel(t *testing.T, e *apiEnv, name string) dto.LabelDTO {
	t.Helper()
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/labels/", map[string]any{"name": name, "color": "blue"}))
	if resp.StatusCode != 201 {
		t.Fatalf("create label: got %d, want 201; body: %s", resp.StatusCode, body)
	}
	var result dto.LabelDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse label: %v", err)
	}
	return result
}

func setupEnvWithAutoLabels(t *testing.T, autoLabels []config.AutoLabel) *apiEnv {
	t.Helper()
	cfg := makeTestConfig()
	cfg.AutoLabels = autoLabels
	return buildAPIEnvWithConfig(t, cfg)
}

// --- GET /tasks/:id ---

func TestTaskGet_Success(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	task := createTestTask(t, e, ctx.ID, "Read book")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, fmt.Sprintf("/api/v1/tasks/%d", task.ID), nil))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.TaskDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.ID != task.ID {
		t.Errorf("id: got %d, want %d", result.ID, task.ID)
	}
	if result.Title != "Read book" {
		t.Errorf("title: got %q, want %q", result.Title, "Read book")
	}
	if result.URL == "" {
		t.Error("url must not be empty")
	}
}

func TestTaskGet_NotFound(t *testing.T) {
	e := setupAPIEnv(t)
	resp, _ := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/tasks/9999", nil))
	if resp.StatusCode != 404 {
		t.Fatalf("got %d, want 404", resp.StatusCode)
	}
}

// --- DELETE /tasks/:id ---

func TestTaskDelete_Success(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	task := createTestTask(t, e, ctx.ID, "Temp task")

	resp, _ := doReq(t, e.app, e.authedReq(t, http.MethodDelete, fmt.Sprintf("/api/v1/tasks/%d", task.ID), nil))
	if resp.StatusCode != 204 {
		t.Fatalf("delete: got %d, want 204", resp.StatusCode)
	}
	resp2, _ := doReq(t, e.app, e.authedReq(t, http.MethodGet, fmt.Sprintf("/api/v1/tasks/%d", task.ID), nil))
	if resp2.StatusCode != 404 {
		t.Fatalf("after delete: got %d, want 404", resp2.StatusCode)
	}
}

func TestTaskDelete_NotFound(t *testing.T) {
	e := setupAPIEnv(t)
	resp, _ := doReq(t, e.app, e.authedReq(t, http.MethodDelete, "/api/v1/tasks/9999", nil))
	if resp.StatusCode != 404 {
		t.Fatalf("got %d, want 404", resp.StatusCode)
	}
}

// --- PATCH /tasks/:id ---

func TestTaskPatch_UpdateTitle(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	task := createTestTask(t, e, ctx.ID, "Old title")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch,
		fmt.Sprintf("/api/v1/tasks/%d", task.ID),
		map[string]any{"title": "New title"}))
	if resp.StatusCode != 200 {
		t.Fatalf("patch: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.TaskDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Title != "New title" {
		t.Errorf("title: got %q, want %q", result.Title, "New title")
	}
}

func TestTaskPatch_UpdatePriority(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	task := createTestTask(t, e, ctx.ID, "Task")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch,
		fmt.Sprintf("/api/v1/tasks/%d", task.ID),
		map[string]any{"priority": "high"}))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.TaskDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Priority != "high" {
		t.Errorf("priority: got %q, want %q", result.Priority, "high")
	}
}

func TestTaskPatch_ClearDueAt(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")

	// Create task with dueAt.
	url := fmt.Sprintf("/api/v1/contexts/%d/tasks", ctx.ID)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, url, map[string]any{
		"title": "Task with due", "dueAt": "2030-01-15T10:00:00.000Z",
	}))
	if resp.StatusCode != 201 {
		t.Fatalf("create: got %d; body: %s", resp.StatusCode, body)
	}
	var created dto.TaskDTO
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if created.DueAt == nil {
		t.Fatal("dueAt should be set after create")
	}

	// Clear with JSON null: map[string]any{"dueAt": nil} → {"dueAt":null}
	resp2, body2 := doReq(t, e.app, e.authedReq(t, http.MethodPatch,
		fmt.Sprintf("/api/v1/tasks/%d", created.ID),
		map[string]any{"dueAt": nil}))
	if resp2.StatusCode != 200 {
		t.Fatalf("patch: got %d, want 200; body: %s", resp2.StatusCode, body2)
	}
	var patched dto.TaskDTO
	if err := json.Unmarshal(body2, &patched); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if patched.DueAt != nil {
		t.Errorf("dueAt: got %v, want nil after clear", patched.DueAt)
	}
}

func TestTaskPatch_DoesNotTouchPlacement(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	task := createTestTask(t, e, ctx.ID, "Task")

	// Attempt to change placement fields via PATCH — they should be silently ignored.
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch,
		fmt.Sprintf("/api/v1/tasks/%d", task.ID),
		map[string]any{"title": "Updated", "contextId": 9999, "projectId": 9999}))
	if resp.StatusCode != 200 {
		t.Fatalf("patch: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.TaskDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.ContextID == nil || *result.ContextID != ctx.ID {
		t.Errorf("contextId: got %v, want %d (placement must not change via PATCH)", result.ContextID, ctx.ID)
	}
}

func TestTaskPatch_DoesNotTouchStatusOrPin(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	task := createTestTask(t, e, ctx.ID, "Task")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch,
		fmt.Sprintf("/api/v1/tasks/%d", task.ID),
		map[string]any{"title": "Updated", "status": "completed", "isPinned": true}))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.TaskDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Status != "open" {
		t.Errorf("status: got %q, want %q (must not change via PATCH)", result.Status, "open")
	}
	if result.IsPinned {
		t.Error("isPinned: got true, want false (must not change via PATCH)")
	}
}

func TestTaskPatch_NotFound(t *testing.T) {
	e := setupAPIEnv(t)
	resp, _ := doReq(t, e.app, e.authedReq(t, http.MethodPatch, "/api/v1/tasks/9999",
		map[string]any{"title": "X"}))
	if resp.StatusCode != 404 {
		t.Fatalf("got %d, want 404", resp.StatusCode)
	}
}

func TestTaskPatch_InvalidPriority(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	task := createTestTask(t, e, ctx.ID, "Task")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch,
		fmt.Sprintf("/api/v1/tasks/%d", task.ID),
		map[string]any{"priority": "super-high"}))
	if resp.StatusCode != 400 {
		t.Fatalf("got %d, want 400; body: %s", resp.StatusCode, body)
	}
	er := parseErr(t, body)
	if er.Error.Code != httpapi.CodeValidationFailed {
		t.Errorf("code: got %q, want %q", er.Error.Code, httpapi.CodeValidationFailed)
	}
}

func TestTaskPatch_WithLabels(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	task := createTestTask(t, e, ctx.ID, "Task")
	createTestLabel(t, e, "work")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch,
		fmt.Sprintf("/api/v1/tasks/%d", task.ID),
		map[string]any{"labels": []string{"work"}}))
	if resp.StatusCode != 200 {
		t.Fatalf("patch with label: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.TaskDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(result.Labels) != 1 || result.Labels[0].Name != "work" {
		t.Errorf("labels: got %v, want [{work}]", result.Labels)
	}

	// Clear labels by setting empty array.
	resp2, body2 := doReq(t, e.app, e.authedReq(t, http.MethodPatch,
		fmt.Sprintf("/api/v1/tasks/%d", task.ID),
		map[string]any{"labels": []string{}}))
	if resp2.StatusCode != 200 {
		t.Fatalf("clear labels: got %d, want 200; body: %s", resp2.StatusCode, body2)
	}
	var result2 dto.TaskDTO
	if err := json.Unmarshal(body2, &result2); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(result2.Labels) != 0 {
		t.Errorf("labels after clear: got %d, want 0", len(result2.Labels))
	}
}

func TestTaskPatch_UnknownLabel(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	task := createTestTask(t, e, ctx.ID, "Task")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch,
		fmt.Sprintf("/api/v1/tasks/%d", task.ID),
		map[string]any{"labels": []string{"nonexistent"}}))
	if resp.StatusCode != 400 {
		t.Fatalf("got %d, want 400; body: %s", resp.StatusCode, body)
	}
	er := parseErr(t, body)
	if er.Error.Code != httpapi.CodeValidationFailed {
		t.Errorf("code: got %q, want %q", er.Error.Code, httpapi.CodeValidationFailed)
	}
}

// --- POST /tasks/:id/subtasks ---

func TestSubtaskCreate_InContext(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	parent := createTestTask(t, e, ctx.ID, "Parent task")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/subtasks", parent.ID),
		map[string]any{"title": "Child task", "priority": "low"}))
	if resp.StatusCode != 201 {
		t.Fatalf("create subtask: got %d, want 201; body: %s", resp.StatusCode, body)
	}
	var result dto.TaskDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Title != "Child task" {
		t.Errorf("title: got %q, want %q", result.Title, "Child task")
	}
	if result.ParentID == nil || *result.ParentID != parent.ID {
		t.Errorf("parentId: got %v, want %d", result.ParentID, parent.ID)
	}
	if result.ContextID == nil || *result.ContextID != ctx.ID {
		t.Errorf("contextId: inherited incorrectly: got %v", result.ContextID)
	}
	if result.Priority != "low" {
		t.Errorf("priority: got %q, want %q", result.Priority, "low")
	}
}

func TestSubtaskCreate_InProject(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	proj := createTestProject(t, e, ctx.ID, "My Project")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/projects/%d/tasks", proj.ID),
		map[string]any{"title": "Parent"}))
	if resp.StatusCode != 201 {
		t.Fatalf("create parent: got %d; body: %s", resp.StatusCode, body)
	}
	var parent dto.TaskDTO
	if err := json.Unmarshal(body, &parent); err != nil {
		t.Fatalf("parse parent: %v", err)
	}

	resp2, body2 := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/subtasks", parent.ID),
		map[string]any{"title": "Child"}))
	if resp2.StatusCode != 201 {
		t.Fatalf("create subtask: got %d, want 201; body: %s", resp2.StatusCode, body2)
	}
	var child dto.TaskDTO
	if err := json.Unmarshal(body2, &child); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if child.ProjectID == nil || *child.ProjectID != proj.ID {
		t.Errorf("projectId: got %v, want %d", child.ProjectID, proj.ID)
	}
}

func TestSubtaskCreate_InInbox_Forbidden(t *testing.T) {
	e := setupAPIEnv(t)

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/inbox/tasks",
		map[string]any{"title": "Inbox parent"}))
	if resp.StatusCode != 201 {
		t.Fatalf("create inbox task: got %d; body: %s", resp.StatusCode, body)
	}
	var parent dto.TaskDTO
	if err := json.Unmarshal(body, &parent); err != nil {
		t.Fatalf("parse: %v", err)
	}

	resp2, body2 := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/subtasks", parent.ID),
		map[string]any{"title": "Forbidden child"}))
	if resp2.StatusCode != 422 {
		t.Fatalf("inbox subtask: got %d, want 422; body: %s", resp2.StatusCode, body2)
	}
	er := parseErr(t, body2)
	if er.Error.Code != httpapi.CodeForbiddenPlacement {
		t.Errorf("code: got %q, want %q", er.Error.Code, httpapi.CodeForbiddenPlacement)
	}
}

func TestSubtaskCreate_ParentNotFound(t *testing.T) {
	e := setupAPIEnv(t)
	resp, _ := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/tasks/9999/subtasks",
		map[string]any{"title": "Child"}))
	if resp.StatusCode != 404 {
		t.Fatalf("got %d, want 404", resp.StatusCode)
	}
}

func TestSubtaskCreate_MissingTitle(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	parent := createTestTask(t, e, ctx.ID, "Parent")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/subtasks", parent.ID),
		map[string]any{"title": ""}))
	if resp.StatusCode != 400 {
		t.Fatalf("got %d, want 400; body: %s", resp.StatusCode, body)
	}
}

// --- task creation in all containers ---

func TestCreateTask_InAllContainers(t *testing.T) {
	e := setupAPIEnv(t)
	ctx, err := e.ctxs.Create(context.Background(), "Personal", "blue", false)
	if err != nil {
		t.Fatal(err)
	}
	proj, err := e.projects.Create(context.Background(), repo.CreateProject{
		ContextID: ctx.ID, Title: "Project", Color: "green",
	})
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		url  string
	}{
		{"context", fmt.Sprintf("/api/v1/contexts/%d/tasks", ctx.ID)},
		{"project", fmt.Sprintf("/api/v1/projects/%d/tasks", proj.ID)},
		{"inbox", "/api/v1/inbox/tasks"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, tc.url,
				map[string]any{"title": "Test task from " + tc.name}))
			if resp.StatusCode != 201 {
				t.Fatalf("create in %s: got %d, want 201; body: %s", tc.name, resp.StatusCode, body)
			}
		})
	}
}

func TestCreateTask_ExplicitLabel(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	createTestLabel(t, e, "urgent")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/contexts/%d/tasks", ctx.ID),
		map[string]any{"title": "Important task", "labels": []string{"urgent"}}))
	if resp.StatusCode != 201 {
		t.Fatalf("got %d, want 201; body: %s", resp.StatusCode, body)
	}
	var result dto.TaskDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(result.Labels) != 1 || result.Labels[0].Name != "urgent" {
		t.Errorf("labels: got %v, want [{urgent}]", result.Labels)
	}
}

func TestCreateTask_UnknownLabel(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/contexts/%d/tasks", ctx.ID),
		map[string]any{"title": "Task", "labels": []string{"nonexistent"}}))
	if resp.StatusCode != 400 {
		t.Fatalf("got %d, want 400; body: %s", resp.StatusCode, body)
	}
	er := parseErr(t, body)
	if er.Error.Code != httpapi.CodeValidationFailed {
		t.Errorf("code: got %q, want %q", er.Error.Code, httpapi.CodeValidationFailed)
	}
}

// --- Auto-labels ---

func TestAutoLabels_MatchedOnCreate(t *testing.T) {
	e := setupEnvWithAutoLabels(t, []config.AutoLabel{
		{Mask: "buy", Label: "shopping"},
	})
	ctx := createTestContext(t, e, "Personal")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/contexts/%d/tasks", ctx.ID),
		map[string]any{"title": "buy groceries"}))
	if resp.StatusCode != 201 {
		t.Fatalf("create: got %d; body: %s", resp.StatusCode, body)
	}
	var result dto.TaskDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(result.Labels) != 1 || result.Labels[0].Name != "shopping" {
		t.Errorf("auto-labels: got %v, want [{shopping}]", result.Labels)
	}
}

func TestAutoLabels_UnmatchedOnCreate(t *testing.T) {
	e := setupEnvWithAutoLabels(t, []config.AutoLabel{
		{Mask: "buy", Label: "shopping"},
	})
	ctx := createTestContext(t, e, "Personal")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/contexts/%d/tasks", ctx.ID),
		map[string]any{"title": "read a book"}))
	if resp.StatusCode != 201 {
		t.Fatalf("create: got %d; body: %s", resp.StatusCode, body)
	}
	var result dto.TaskDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(result.Labels) != 0 {
		t.Errorf("auto-labels: got %d labels, want 0", len(result.Labels))
	}
}

func TestAutoLabels_CaseSensitive(t *testing.T) {
	f := false
	e := setupEnvWithAutoLabels(t, []config.AutoLabel{
		{Mask: "BUY", Label: "shopping", IgnoreCase: &f},
	})
	ctx := createTestContext(t, e, "Personal")

	// Lowercase "buy" should NOT match case-sensitive mask "BUY".
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/contexts/%d/tasks", ctx.ID),
		map[string]any{"title": "buy milk"}))
	if resp.StatusCode != 201 {
		t.Fatalf("create lowercase: got %d; body: %s", resp.StatusCode, body)
	}
	var result dto.TaskDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(result.Labels) != 0 {
		t.Errorf("case-sensitive mismatch: got %d labels, want 0", len(result.Labels))
	}

	// Uppercase "BUY" should match.
	resp2, body2 := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/contexts/%d/tasks", ctx.ID),
		map[string]any{"title": "BUY milk"}))
	if resp2.StatusCode != 201 {
		t.Fatalf("create uppercase: got %d; body: %s", resp2.StatusCode, body2)
	}
	var result2 dto.TaskDTO
	if err := json.Unmarshal(body2, &result2); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(result2.Labels) != 1 || result2.Labels[0].Name != "shopping" {
		t.Errorf("case-sensitive match: got %v, want [{shopping}]", result2.Labels)
	}
}

func TestAutoLabels_RemovedAutoLabels_OnCreate(t *testing.T) {
	e := setupEnvWithAutoLabels(t, []config.AutoLabel{
		{Mask: "buy", Label: "shopping"},
	})
	ctx := createTestContext(t, e, "Personal")

	// User explicitly rejects the auto-label via removedAutoLabels.
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/contexts/%d/tasks", ctx.ID),
		map[string]any{
			"title":             "buy milk",
			"removedAutoLabels": []string{"shopping"},
		}))
	if resp.StatusCode != 201 {
		t.Fatalf("create: got %d; body: %s", resp.StatusCode, body)
	}
	var result dto.TaskDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(result.Labels) != 0 {
		t.Errorf("removedAutoLabels: got %d labels, want 0", len(result.Labels))
	}
}

func TestAutoLabels_MatchedOnTitleChange(t *testing.T) {
	e := setupEnvWithAutoLabels(t, []config.AutoLabel{
		{Mask: "buy", Label: "shopping"},
	})
	ctx := createTestContext(t, e, "Personal")
	task := createTestTask(t, e, ctx.ID, "read a book")

	if len(task.Labels) != 0 {
		t.Errorf("initial: got %d labels, want 0", len(task.Labels))
	}

	// Rename title to include "buy" — auto-label should be applied on patch.
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch,
		fmt.Sprintf("/api/v1/tasks/%d", task.ID),
		map[string]any{"title": "buy milk"}))
	if resp.StatusCode != 200 {
		t.Fatalf("patch: got %d; body: %s", resp.StatusCode, body)
	}
	var patched dto.TaskDTO
	if err := json.Unmarshal(body, &patched); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(patched.Labels) != 1 || patched.Labels[0].Name != "shopping" {
		t.Errorf("auto-label on rename: got %v, want [{shopping}]", patched.Labels)
	}
}

func TestAutoLabels_AutoCreatesLabel(t *testing.T) {
	e := setupEnvWithAutoLabels(t, []config.AutoLabel{
		{Mask: "urgent", Label: "urgent-flag"},
	})
	ctx := createTestContext(t, e, "Personal")

	// Label "urgent-flag" doesn't exist yet — should be auto-created.
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/contexts/%d/tasks", ctx.ID),
		map[string]any{"title": "urgent task"}))
	if resp.StatusCode != 201 {
		t.Fatalf("create: got %d; body: %s", resp.StatusCode, body)
	}
	var result dto.TaskDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(result.Labels) != 1 || result.Labels[0].Name != "urgent-flag" {
		t.Errorf("auto-created label: got %v, want [{urgent-flag}]", result.Labels)
	}
}

// --- POST /tasks/:id/duplicate ---

func TestTaskDuplicate_Simple(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	task := createTestTask(t, e, ctx.ID, "Source Task")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/duplicate", task.ID), nil))
	if resp.StatusCode != 201 {
		t.Fatalf("duplicate: got %d, want 201; body: %s", resp.StatusCode, body)
	}
	var result dto.TaskDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Title != "Source Task (2)" {
		t.Errorf("title: got %q, want %q", result.Title, "Source Task (2)")
	}
	if result.ID == task.ID {
		t.Error("duplicate must have a different ID")
	}
	if result.ContextID == nil || *result.ContextID != *task.ContextID {
		t.Errorf("contextId: got %v, want %v", result.ContextID, task.ContextID)
	}
}

func TestTaskDuplicate_AlreadyNumbered(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")

	// Create a task with "(2)" suffix and duplicate it → should become "(3)".
	task := createTestTask(t, e, ctx.ID, "Task (2)")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/duplicate", task.ID), nil))
	if resp.StatusCode != 201 {
		t.Fatalf("duplicate: got %d, want 201; body: %s", resp.StatusCode, body)
	}
	var result dto.TaskDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Title != "Task (3)" {
		t.Errorf("title: got %q, want %q", result.Title, "Task (3)")
	}
}

func TestTaskDuplicate_NotFound(t *testing.T) {
	e := setupAPIEnv(t)
	resp, _ := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/tasks/9999/duplicate", nil))
	if resp.StatusCode != 404 {
		t.Fatalf("got %d, want 404", resp.StatusCode)
	}
}

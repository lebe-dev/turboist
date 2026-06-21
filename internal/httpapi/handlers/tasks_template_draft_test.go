package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/lebe-dev/turboist/internal/httpapi/dto"
)

func TestTaskTemplateDraft_FlattensSubtreeWithLabels(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	createTestLabel(t, e, "alpha")
	createTestLabel(t, e, "beta")

	// Root task — give it a priority and a label.
	root := createTestTask(t, e, ctx.ID, "Root")
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch,
		fmt.Sprintf("/api/v1/tasks/%d", root.ID),
		map[string]any{"priority": "high", "labels": []string{"alpha"}}))
	if resp.StatusCode != 200 {
		t.Fatalf("patch root: got %d; body: %s", resp.StatusCode, body)
	}

	// Direct subtask with its own (explicit) label.
	resp, body = doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/subtasks", root.ID),
		map[string]any{"title": "Child", "labels": []string{"beta"}}))
	if resp.StatusCode != 201 {
		t.Fatalf("create child: got %d; body: %s", resp.StatusCode, body)
	}
	var child dto.TaskDTO
	if err := json.Unmarshal(body, &child); err != nil {
		t.Fatalf("parse child: %v", err)
	}

	// Grandchild — deeper nesting that must collapse into the single subtask level.
	resp, body = doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/subtasks", child.ID),
		map[string]any{"title": "Grandchild", "labels": []string{}}))
	if resp.StatusCode != 201 {
		t.Fatalf("create grandchild: got %d; body: %s", resp.StatusCode, body)
	}

	resp, body = doReq(t, e.app, e.authedReq(t, http.MethodGet,
		fmt.Sprintf("/api/v1/tasks/%d/template-draft", root.ID), nil))
	if resp.StatusCode != 200 {
		t.Fatalf("draft: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var draft dto.TaskTemplateDTO
	if err := json.Unmarshal(body, &draft); err != nil {
		t.Fatalf("parse draft: %v", err)
	}

	if draft.Name != "Root" || draft.Priority != "high" {
		t.Errorf("root: got name=%q priority=%q, want Root/high", draft.Name, draft.Priority)
	}
	if draft.ID != 0 {
		t.Errorf("draft id: got %d, want 0 (unsaved draft)", draft.ID)
	}
	if len(draft.Labels) != 1 || draft.Labels[0].Name != "alpha" {
		t.Errorf("root labels: got %+v, want [alpha]", draft.Labels)
	}
	if len(draft.Subtasks) != 2 {
		t.Fatalf("subtasks: got %d, want 2 (child + flattened grandchild)", len(draft.Subtasks))
	}
	// Depth-first pre-order: child first, then its grandchild.
	if draft.Subtasks[0].Title != "Child" || draft.Subtasks[1].Title != "Grandchild" {
		t.Errorf("order: got %q,%q; want Child,Grandchild", draft.Subtasks[0].Title, draft.Subtasks[1].Title)
	}
	if len(draft.Subtasks[0].Labels) != 1 || draft.Subtasks[0].Labels[0].Name != "beta" {
		t.Errorf("child labels: got %+v, want [beta]", draft.Subtasks[0].Labels)
	}
}

func TestTaskTemplateDraft_NoSubtasks(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	root := createTestTask(t, e, ctx.ID, "Solo")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet,
		fmt.Sprintf("/api/v1/tasks/%d/template-draft", root.ID), nil))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var draft dto.TaskTemplateDTO
	if err := json.Unmarshal(body, &draft); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if draft.Name != "Solo" || len(draft.Subtasks) != 0 {
		t.Errorf("draft: got name=%q subtasks=%d, want Solo/0", draft.Name, len(draft.Subtasks))
	}
}

func TestTaskTemplateDraft_NotFound(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/tasks/9999/template-draft", nil))
	if resp.StatusCode != 404 {
		t.Fatalf("got %d, want 404; body: %s", resp.StatusCode, body)
	}
}

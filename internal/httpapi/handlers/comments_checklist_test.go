package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/lebe-dev/turboist/internal/httpapi/dto"
)

// TestComment_CreateThenList_Immutable asserts comments can be created and
// listed but there is no PATCH route to mutate a body (US-3.5 AC2 — comments are
// immutable). The handler exposes only GET/POST/DELETE.
func TestComment_CreateThenList_Immutable(t *testing.T) {
	e := setupAPIEnv(t)
	ctxID := seedOfflineSyncContext(t, e, "Comments")
	task := createTestTask(t, e, ctxID, "host")

	base := fmt.Sprintf("/api/v1/tasks/%d/comments", task.ID)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, base, map[string]any{"body": "first"}))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create comment: got %d, want 201; body: %s", resp.StatusCode, body)
	}
	var created dto.CommentDTO
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("parse comment: %v", err)
	}
	if created.Body != "first" || created.ClientID == "" {
		t.Errorf("created comment: body=%q clientId=%q", created.Body, created.ClientID)
	}

	// No PATCH route exists for comments — a PATCH must not succeed.
	patchURL := fmt.Sprintf("/api/v1/tasks/%d/comments/%d", task.ID, created.ID)
	resp, _ = doReq(t, e.app, e.authedReq(t, http.MethodPatch, patchURL, map[string]any{"body": "changed"}))
	if resp.StatusCode == http.StatusOK {
		t.Errorf("comment PATCH must not succeed (comments are immutable); got 200")
	}
}

// TestComment_TwoCommentsOrdered asserts two comments are returned oldest-first
// (US-3.5 AC3 precursor).
func TestComment_TwoCommentsOrdered(t *testing.T) {
	e := setupAPIEnv(t)
	ctxID := seedOfflineSyncContext(t, e, "Ordered")
	task := createTestTask(t, e, ctxID, "host")
	base := fmt.Sprintf("/api/v1/tasks/%d/comments", task.ID)

	first := createComment(t, e, base, "older")
	second := createComment(t, e, base, "newer")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, base, nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list comments: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var page struct {
		Items []dto.CommentDTO `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatalf("parse list: %v", err)
	}
	if page.Total != 2 || len(page.Items) != 2 {
		t.Fatalf("list count: got total=%d len=%d, want 2/2", page.Total, len(page.Items))
	}
	if page.Items[0].ID != first.ID || page.Items[1].ID != second.ID {
		t.Errorf("order: got [%d,%d], want [%d,%d]", page.Items[0].ID, page.Items[1].ID, first.ID, second.ID)
	}
}

// TestComment_AuthorShape asserts the comment DTO carries the federated author
// fields (authorDisplayName / authorInstance) so the "display_name @ origin"
// line (US-3.5 AC4) has a wire home; they are null for locally-authored
// comments since no peer display_name exists yet (F0.3).
func TestComment_AuthorShape(t *testing.T) {
	e := setupAPIEnv(t)
	ctxID := seedOfflineSyncContext(t, e, "Author")
	task := createTestTask(t, e, ctxID, "host")
	base := fmt.Sprintf("/api/v1/tasks/%d/comments", task.ID)

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, base, map[string]any{"body": "hi"}))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create comment: got %d, want 201; body: %s", resp.StatusCode, body)
	}
	// The keys must be present in the JSON (null is fine for a local comment).
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("parse raw comment: %v", err)
	}
	for _, key := range []string{"authorDisplayName", "authorInstance"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("comment DTO missing %q field for federated author line", key)
		}
	}
}

// TestComment_Delete asserts a comment can be soft-deleted and disappears from
// the list.
func TestComment_Delete(t *testing.T) {
	e := setupAPIEnv(t)
	ctxID := seedOfflineSyncContext(t, e, "DelComment")
	task := createTestTask(t, e, ctxID, "host")
	base := fmt.Sprintf("/api/v1/tasks/%d/comments", task.ID)
	c := createComment(t, e, base, "doomed")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodDelete, fmt.Sprintf("%s/%d", base, c.ID), nil))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete comment: got %d, want 204; body: %s", resp.StatusCode, body)
	}
	resp, body = doReq(t, e.app, e.authedReq(t, http.MethodGet, base, nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list comments: got %d; body: %s", resp.StatusCode, body)
	}
	var page struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatalf("parse list: %v", err)
	}
	if page.Total != 0 {
		t.Errorf("list after delete: got total=%d, want 0", page.Total)
	}
}

// TestChecklist_ToggleIsolatesSiblings asserts toggling one item's completion
// via PATCH leaves siblings unaffected (US-3.6 AC1 precursor).
func TestChecklist_ToggleIsolatesSiblings(t *testing.T) {
	e := setupAPIEnv(t)
	ctxID := seedOfflineSyncContext(t, e, "Checklist")
	task := createTestTask(t, e, ctxID, "host")
	base := fmt.Sprintf("/api/v1/tasks/%d/checklist", task.ID)

	a := createChecklistItem(t, e, base, "a")
	b := createChecklistItem(t, e, base, "b")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch,
		fmt.Sprintf("%s/%d", base, a.ID), map[string]any{"isCompleted": true}))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("toggle a: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var updated dto.ChecklistItemDTO
	if err := json.Unmarshal(body, &updated); err != nil {
		t.Fatalf("parse updated: %v", err)
	}
	if !updated.IsCompleted {
		t.Errorf("item a should be completed after toggle")
	}

	// List and assert b is still uncompleted.
	resp, body = doReq(t, e.app, e.authedReq(t, http.MethodGet, base, nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list checklist: got %d; body: %s", resp.StatusCode, body)
	}
	var page struct {
		Items []dto.ChecklistItemDTO `json:"items"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatalf("parse list: %v", err)
	}
	for _, item := range page.Items {
		if item.ID == b.ID && item.IsCompleted {
			t.Errorf("sibling b must remain uncompleted after toggling a")
		}
	}
}

// TestChecklist_CreateAssignsPosition asserts appended items get increasing
// positions.
func TestChecklist_CreateAssignsPosition(t *testing.T) {
	e := setupAPIEnv(t)
	ctxID := seedOfflineSyncContext(t, e, "Position")
	task := createTestTask(t, e, ctxID, "host")
	base := fmt.Sprintf("/api/v1/tasks/%d/checklist", task.ID)

	a := createChecklistItem(t, e, base, "a")
	b := createChecklistItem(t, e, base, "b")
	if a.Position != 0 || b.Position != 1 {
		t.Errorf("positions: got a=%d b=%d, want 0 and 1", a.Position, b.Position)
	}
	if a.ClientID == "" || b.ClientID == "" {
		t.Errorf("expected non-empty clientId on checklist items")
	}
}

// TestComment_Delete_WrongTask_NotFound asserts a comment cannot be deleted
// through a sibling task's URL: the parent-scoped delete returns 404 and the
// comment survives under its real task (Federation v1 F0.2 follow-up).
func TestComment_Delete_WrongTask_NotFound(t *testing.T) {
	e := setupAPIEnv(t)
	ctxID := seedOfflineSyncContext(t, e, "WrongTaskC")
	owner := createTestTask(t, e, ctxID, "owner")
	other := createTestTask(t, e, ctxID, "other")
	base := fmt.Sprintf("/api/v1/tasks/%d/comments", owner.ID)
	c := createComment(t, e, base, "mine")

	wrongURL := fmt.Sprintf("/api/v1/tasks/%d/comments/%d", other.ID, c.ID)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodDelete, wrongURL, nil))
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-task delete: got %d, want 404; body: %s", resp.StatusCode, body)
	}

	resp, body = doReq(t, e.app, e.authedReq(t, http.MethodGet, base, nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list comments: got %d; body: %s", resp.StatusCode, body)
	}
	var page struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatalf("parse list: %v", err)
	}
	if page.Total != 1 {
		t.Errorf("comment must survive cross-task delete: got total=%d, want 1", page.Total)
	}
}

// TestChecklist_Patch_WrongTask_NotFound asserts a checklist item cannot be
// patched through a sibling task's URL.
func TestChecklist_Patch_WrongTask_NotFound(t *testing.T) {
	e := setupAPIEnv(t)
	ctxID := seedOfflineSyncContext(t, e, "WrongTaskCk")
	owner := createTestTask(t, e, ctxID, "owner")
	other := createTestTask(t, e, ctxID, "other")
	base := fmt.Sprintf("/api/v1/tasks/%d/checklist", owner.ID)
	it := createChecklistItem(t, e, base, "step")

	wrongURL := fmt.Sprintf("/api/v1/tasks/%d/checklist/%d", other.ID, it.ID)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch, wrongURL, map[string]any{"isCompleted": true}))
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-task patch: got %d, want 404; body: %s", resp.StatusCode, body)
	}
}

func createComment(t *testing.T, e *apiEnv, base, body string) dto.CommentDTO {
	t.Helper()
	resp, raw := doReq(t, e.app, e.authedReq(t, http.MethodPost, base, map[string]any{"body": body}))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create comment: got %d, want 201; body: %s", resp.StatusCode, raw)
	}
	var c dto.CommentDTO
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatalf("parse comment: %v", err)
	}
	return c
}

func createChecklistItem(t *testing.T, e *apiEnv, base, title string) dto.ChecklistItemDTO {
	t.Helper()
	resp, raw := doReq(t, e.app, e.authedReq(t, http.MethodPost, base, map[string]any{"title": title}))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create checklist item: got %d, want 201; body: %s", resp.StatusCode, raw)
	}
	var it dto.ChecklistItemDTO
	if err := json.Unmarshal(raw, &it); err != nil {
		t.Fatalf("parse checklist item: %v", err)
	}
	return it
}

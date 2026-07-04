package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/lebe-dev/turboist/internal/httpapi/dto"
)

type bulkResp struct {
	Succeeded []int64 `json:"succeeded"`
	Failed    []struct {
		ID    int64 `json:"id"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	} `json:"failed"`
}

func TestBulkComplete_AllSucceed(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	t1 := createTestTask(t, e, ctx.ID, "Task 1")
	t2 := createTestTask(t, e, ctx.ID, "Task 2")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		"/api/v1/tasks/bulk/complete",
		map[string]any{"ids": []int64{t1.ID, t2.ID}}))
	if resp.StatusCode != 200 {
		t.Fatalf("bulk complete: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result bulkResp
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(result.Succeeded) != 2 {
		t.Errorf("succeeded: got %d, want 2", len(result.Succeeded))
	}
	if len(result.Failed) != 0 {
		t.Errorf("failed: got %d, want 0", len(result.Failed))
	}
}

func TestBulkComplete_PartialFailure(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	task := createTestTask(t, e, ctx.ID, "Existing task")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		"/api/v1/tasks/bulk/complete",
		map[string]any{"ids": []int64{task.ID, 99999}}))
	if resp.StatusCode != 200 {
		t.Fatalf("bulk complete: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result bulkResp
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(result.Succeeded) != 1 || result.Succeeded[0] != task.ID {
		t.Errorf("succeeded: got %v, want [%d]", result.Succeeded, task.ID)
	}
	if len(result.Failed) != 1 || result.Failed[0].ID != 99999 {
		t.Errorf("failed: got %v, want [{id:99999}]", result.Failed)
	}
}

func TestBulkMove_AllSucceed(t *testing.T) {
	e := setupAPIEnv(t)
	ctx1 := createTestContext(t, e, "Work")
	ctx2 := createTestContext(t, e, "Personal")
	t1 := createTestTask(t, e, ctx1.ID, "Task 1")
	t2 := createTestTask(t, e, ctx1.ID, "Task 2")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		"/api/v1/tasks/bulk/move",
		map[string]any{"ids": []int64{t1.ID, t2.ID}, "contextId": ctx2.ID}))
	if resp.StatusCode != 200 {
		t.Fatalf("bulk move: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result bulkResp
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(result.Succeeded) != 2 {
		t.Errorf("succeeded: got %d, want 2", len(result.Succeeded))
	}

	// Verify tasks moved.
	resp2, body2 := doReq(t, e.app, e.authedReq(t, http.MethodGet,
		fmt.Sprintf("/api/v1/tasks/%d", t1.ID), nil))
	if resp2.StatusCode != 200 {
		t.Fatalf("get task: got %d; body: %s", resp2.StatusCode, body2)
	}
	var moved dto.TaskDTO
	if err := json.Unmarshal(body2, &moved); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if moved.ContextID == nil || *moved.ContextID != ctx2.ID {
		t.Errorf("contextId: got %v, want %d", moved.ContextID, ctx2.ID)
	}
}

func TestBulkPriority_AllSucceed(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	t1 := createTestTask(t, e, ctx.ID, "Task 1")
	t2 := createTestTask(t, e, ctx.ID, "Task 2")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		"/api/v1/tasks/bulk/priority",
		map[string]any{"ids": []int64{t1.ID, t2.ID}, "priority": "high"}))
	if resp.StatusCode != 200 {
		t.Fatalf("bulk priority: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result bulkResp
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(result.Succeeded) != 2 {
		t.Errorf("succeeded: got %d, want 2", len(result.Succeeded))
	}
	if len(result.Failed) != 0 {
		t.Errorf("failed: got %d, want 0", len(result.Failed))
	}

	// Verify the priority was applied.
	_, b := doReq(t, e.app, e.authedReq(t, http.MethodGet,
		fmt.Sprintf("/api/v1/tasks/%d", t1.ID), nil))
	var got dto.TaskDTO
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("parse task: %v", err)
	}
	if got.Priority != "high" {
		t.Errorf("priority: got %s, want high", got.Priority)
	}
}

func TestBulkPriority_InvalidPriority(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	t1 := createTestTask(t, e, ctx.ID, "Task 1")

	resp, _ := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		"/api/v1/tasks/bulk/priority",
		map[string]any{"ids": []int64{t1.ID}, "priority": "urgent"}))
	if resp.StatusCode != 400 {
		t.Errorf("status: got %d, want 400", resp.StatusCode)
	}
}

func TestBulkPriority_PartialFailure(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	t1 := createTestTask(t, e, ctx.ID, "Task 1")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		"/api/v1/tasks/bulk/priority",
		map[string]any{"ids": []int64{t1.ID, 99999}, "priority": "medium"}))
	if resp.StatusCode != 200 {
		t.Fatalf("bulk priority: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result bulkResp
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(result.Succeeded) != 1 || result.Succeeded[0] != t1.ID {
		t.Errorf("succeeded: got %v, want [%d]", result.Succeeded, t1.ID)
	}
	if len(result.Failed) != 1 || result.Failed[0].ID != 99999 {
		t.Errorf("failed: got %v, want [{id:99999}]", result.Failed)
	}
}

// A task in a Troiki-categorised project has its priority pinned by the
// category; a bulk override to a mismatching priority must be rejected per-id.
func TestBulkPriority_TroikiManagedRejected(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	proj := createTestProject(t, e, ctx.ID, "Categorised")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/projects/%d/troiki", proj.ID),
		map[string]any{"category": "important"}))
	if resp.StatusCode != 200 {
		t.Fatalf("set troiki category: got %d; body: %s", resp.StatusCode, body)
	}

	task := createTestTask(t, e, ctx.ID, "Task")
	mv, mb := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		"/api/v1/tasks/bulk/move",
		map[string]any{"ids": []int64{task.ID}, "contextId": ctx.ID, "projectId": proj.ID}))
	if mv.StatusCode != 200 {
		t.Fatalf("bulk move: got %d; body: %s", mv.StatusCode, mb)
	}

	resp2, body2 := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		"/api/v1/tasks/bulk/priority",
		map[string]any{"ids": []int64{task.ID}, "priority": "low"}))
	if resp2.StatusCode != 200 {
		t.Fatalf("bulk priority: got %d; body: %s", resp2.StatusCode, body2)
	}
	var result bulkResp
	if err := json.Unmarshal(body2, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(result.Succeeded) != 0 {
		t.Errorf("succeeded: got %v, want none", result.Succeeded)
	}
	if len(result.Failed) != 1 || result.Failed[0].ID != task.ID {
		t.Fatalf("failed: got %v, want one for %d", result.Failed, task.ID)
	}
	if result.Failed[0].Error.Code != "validation_failed" {
		t.Errorf("failed code: got %s, want validation_failed", result.Failed[0].Error.Code)
	}
}

type groupResp struct {
	Parent    dto.TaskDTO `json:"parent"`
	Succeeded []int64     `json:"succeeded"`
	Failed    []struct {
		ID    int64 `json:"id"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	} `json:"failed"`
}

func TestGroupTasks_HappyPath(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	createTestLabel(t, e, "umbrella")
	t1 := createTestTask(t, e, ctx.ID, "Task 1")
	t2 := createTestTask(t, e, ctx.ID, "Task 2")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		"/api/v1/tasks/group",
		map[string]any{
			"title":     "Wrap-up",
			"priority":  "high",
			"labels":    []string{"umbrella"},
			"contextId": ctx.ID,
			"childIds":  []int64{t1.ID, t2.ID},
		}))
	if resp.StatusCode != 201 {
		t.Fatalf("group: got %d, want 201; body: %s", resp.StatusCode, body)
	}
	var result groupResp
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Parent.Title != "Wrap-up" || result.Parent.Priority != "high" {
		t.Errorf("parent: got %+v", result.Parent)
	}
	if len(result.Succeeded) != 2 || len(result.Failed) != 0 {
		t.Fatalf("outcomes: succeeded=%v failed=%v", result.Succeeded, result.Failed)
	}

	for _, id := range []int64{t1.ID, t2.ID} {
		_, b := doReq(t, e.app, e.authedReq(t, http.MethodGet,
			fmt.Sprintf("/api/v1/tasks/%d", id), nil))
		var got dto.TaskDTO
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("parse child: %v", err)
		}
		if got.ParentID == nil || *got.ParentID != result.Parent.ID {
			t.Errorf("child %d parentId: got %v, want %d", id, got.ParentID, result.Parent.ID)
		}
		if got.Priority != "high" {
			t.Errorf("child %d priority: got %s, want high", id, got.Priority)
		}
		if len(got.Labels) != 1 || got.Labels[0].Name != "umbrella" {
			t.Errorf("child %d labels: got %v, want [umbrella]", id, got.Labels)
		}
	}
}

func TestGroupTasks_RejectsInboxTarget(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	t1 := createTestTask(t, e, ctx.ID, "Task 1")

	resp, _ := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		"/api/v1/tasks/group",
		map[string]any{
			"title":    "Wrap-up",
			"inboxId":  2,
			"childIds": []int64{t1.ID},
		}))
	if resp.StatusCode != 403 && resp.StatusCode != 422 {
		t.Errorf("status: got %d, want 403 or 422", resp.StatusCode)
	}
}

func TestGroupTasks_RejectsEmptyChildIDs(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")

	resp, _ := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		"/api/v1/tasks/group",
		map[string]any{
			"title":     "Wrap-up",
			"contextId": ctx.ID,
			"childIds":  []int64{},
		}))
	if resp.StatusCode != 400 {
		t.Errorf("status: got %d, want 400", resp.StatusCode)
	}
}

func TestGroupTasks_PartialFailureRecorded(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	t1 := createTestTask(t, e, ctx.ID, "Task 1")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		"/api/v1/tasks/group",
		map[string]any{
			"title":     "Wrap-up",
			"contextId": ctx.ID,
			"childIds":  []int64{t1.ID, 99999},
		}))
	if resp.StatusCode != 201 {
		t.Fatalf("status: got %d; body: %s", resp.StatusCode, body)
	}
	var result groupResp
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(result.Succeeded) != 1 || result.Succeeded[0] != t1.ID {
		t.Errorf("succeeded: got %v, want [%d]", result.Succeeded, t1.ID)
	}
	if len(result.Failed) != 1 || result.Failed[0].ID != 99999 {
		t.Errorf("failed: got %v, want one for 99999", result.Failed)
	}
}

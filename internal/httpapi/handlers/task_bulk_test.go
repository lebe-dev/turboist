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

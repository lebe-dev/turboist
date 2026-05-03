package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/service"
)

// --- complete / uncomplete / cancel ---

func TestTaskComplete_Simple(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	task := createTestTask(t, e, ctx.ID, "Do thing")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/complete", task.ID), nil))
	if resp.StatusCode != 200 {
		t.Fatalf("complete: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.TaskDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Status != "completed" {
		t.Errorf("status: got %q, want %q", result.Status, "completed")
	}
}

func TestTaskComplete_NotFound(t *testing.T) {
	e := setupAPIEnv(t)
	resp, _ := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/tasks/9999/complete", nil))
	if resp.StatusCode != 404 {
		t.Fatalf("got %d, want 404", resp.StatusCode)
	}
}

func TestTaskComplete_Recurring_AdvancesDueAt(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Personal")

	// Create task with a daily recurrence rule and future due date.
	dueAt := "2099-01-01T10:00:00.000Z"
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/contexts/%d/tasks", ctx.ID),
		map[string]any{
			"title":          "Daily standup",
			"dueAt":          dueAt,
			"recurrenceRule": "FREQ=DAILY;INTERVAL=1",
		}))
	if resp.StatusCode != 201 {
		t.Fatalf("create: got %d; body: %s", resp.StatusCode, body)
	}
	var task dto.TaskDTO
	if err := json.Unmarshal(body, &task); err != nil {
		t.Fatalf("parse task: %v", err)
	}

	resp2, body2 := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/complete", task.ID), nil))
	if resp2.StatusCode != 200 {
		t.Fatalf("complete recurring: got %d; body: %s", resp2.StatusCode, body2)
	}
	var result dto.TaskDTO
	if err := json.Unmarshal(body2, &result); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	// Status should still be open since RRULE is infinite (no COUNT/UNTIL).
	if result.Status != "open" {
		t.Errorf("status after recurring complete: got %q, want %q", result.Status, "open")
	}
	// due_at should have advanced (not be the same as original).
	if result.DueAt == nil {
		t.Fatal("dueAt should not be nil after recurring complete")
	}
	if *result.DueAt == dueAt {
		t.Errorf("dueAt should advance after recurring complete; got %q same as before", *result.DueAt)
	}
}

func TestTaskComplete_Recurring_CountExhausted(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Personal")

	// COUNT=1 means only one occurrence — after advancing, rule is exhausted → completed.
	dueAt := "2099-01-01T10:00:00.000Z"
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/contexts/%d/tasks", ctx.ID),
		map[string]any{
			"title":          "One-shot recurrence",
			"dueAt":          dueAt,
			"recurrenceRule": "FREQ=DAILY;COUNT=1",
		}))
	if resp.StatusCode != 201 {
		t.Fatalf("create: got %d; body: %s", resp.StatusCode, body)
	}
	var task dto.TaskDTO
	if err := json.Unmarshal(body, &task); err != nil {
		t.Fatalf("parse task: %v", err)
	}

	resp2, body2 := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/complete", task.ID), nil))
	if resp2.StatusCode != 200 {
		t.Fatalf("complete exhausted: got %d; body: %s", resp2.StatusCode, body2)
	}
	var result dto.TaskDTO
	if err := json.Unmarshal(body2, &result); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if result.Status != "completed" {
		t.Errorf("status: got %q, want %q (COUNT=1 should exhaust after advance)", result.Status, "completed")
	}
}

func TestTaskUncomplete(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	task := createTestTask(t, e, ctx.ID, "Task")

	// Complete first.
	doReq(t, e.app, e.authedReq(t, http.MethodPost, fmt.Sprintf("/api/v1/tasks/%d/complete", task.ID), nil))

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/uncomplete", task.ID), nil))
	if resp.StatusCode != 200 {
		t.Fatalf("uncomplete: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.TaskDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Status != "open" {
		t.Errorf("status: got %q, want %q", result.Status, "open")
	}
}

func TestTaskUncomplete_SlotFull(t *testing.T) {
	// A previously-completed Important task whose slot was refilled while it
	// was completed must surface the slot-full conflict (409) instead of 500.
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	original := createTestTask(t, e, ctx.ID, "orig")

	if resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/troiki", original.ID),
		map[string]any{"category": "important"})); resp.StatusCode != 200 {
		t.Fatalf("seed cat: got %d; body: %s", resp.StatusCode, body)
	}
	if resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/complete", original.ID), nil)); resp.StatusCode != 200 {
		t.Fatalf("complete: got %d; body: %s", resp.StatusCode, body)
	}
	for i := range service.TroikiImportantCap {
		fill := createTestTask(t, e, ctx.ID, fmt.Sprintf("fill-%d", i))
		if resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
			fmt.Sprintf("/api/v1/tasks/%d/troiki", fill.ID),
			map[string]any{"category": "important"})); resp.StatusCode != 200 {
			t.Fatalf("fill %d: got %d; body: %s", i, resp.StatusCode, body)
		}
	}

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/uncomplete", original.ID), nil))
	if resp.StatusCode != 409 {
		t.Fatalf("uncomplete overflow: got %d, want 409; body: %s", resp.StatusCode, body)
	}
	er := parseErr(t, body)
	if er.Error.Code != httpapi.CodeTroikiSlotFull {
		t.Errorf("code: got %q, want %q", er.Error.Code, httpapi.CodeTroikiSlotFull)
	}
}

func TestTaskCancel(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	task := createTestTask(t, e, ctx.ID, "Task")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/cancel", task.ID), nil))
	if resp.StatusCode != 200 {
		t.Fatalf("cancel: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.TaskDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Status != "cancelled" {
		t.Errorf("status: got %q, want %q", result.Status, "cancelled")
	}
}

// --- move ---

func TestTaskMove_ToContext(t *testing.T) {
	e := setupAPIEnv(t)
	ctx1 := createTestContext(t, e, "Work")
	ctx2 := createTestContext(t, e, "Personal")
	task := createTestTask(t, e, ctx1.ID, "Task")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/move", task.ID),
		map[string]any{"contextId": ctx2.ID}))
	if resp.StatusCode != 200 {
		t.Fatalf("move: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.TaskDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.ContextID == nil || *result.ContextID != ctx2.ID {
		t.Errorf("contextId: got %v, want %d", result.ContextID, ctx2.ID)
	}
}

func TestTaskMove_CycleDetected(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	parent := createTestTask(t, e, ctx.ID, "Parent")

	// Create subtask.
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/subtasks", parent.ID),
		map[string]any{"title": "Child"}))
	if resp.StatusCode != 201 {
		t.Fatalf("create subtask: got %d; body: %s", resp.StatusCode, body)
	}
	var child dto.TaskDTO
	if err := json.Unmarshal(body, &child); err != nil {
		t.Fatalf("parse: %v", err)
	}

	// Try to move parent under its own child → cycle.
	resp2, body2 := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/move", parent.ID),
		map[string]any{"parentId": child.ID}))
	if resp2.StatusCode != 422 {
		t.Fatalf("cycle move: got %d, want 422; body: %s", resp2.StatusCode, body2)
	}
	er := parseErr(t, body2)
	if er.Error.Code != httpapi.CodeForbiddenPlacement {
		t.Errorf("code: got %q, want %q", er.Error.Code, httpapi.CodeForbiddenPlacement)
	}
}

func TestTaskMove_SubtreeMovedTogether(t *testing.T) {
	e := setupAPIEnv(t)
	ctx1 := createTestContext(t, e, "Work")
	ctx2 := createTestContext(t, e, "Personal")
	parent := createTestTask(t, e, ctx1.ID, "Parent")

	// Create subtask.
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/subtasks", parent.ID),
		map[string]any{"title": "Child"}))
	if resp.StatusCode != 201 {
		t.Fatalf("create subtask: got %d; body: %s", resp.StatusCode, body)
	}
	var child dto.TaskDTO
	if err := json.Unmarshal(body, &child); err != nil {
		t.Fatalf("parse child: %v", err)
	}

	// Move parent to ctx2.
	doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/move", parent.ID),
		map[string]any{"contextId": ctx2.ID}))

	// Verify child moved too.
	resp2, body2 := doReq(t, e.app, e.authedReq(t, http.MethodGet,
		fmt.Sprintf("/api/v1/tasks/%d", child.ID), nil))
	if resp2.StatusCode != 200 {
		t.Fatalf("get child: got %d; body: %s", resp2.StatusCode, body2)
	}
	var updated dto.TaskDTO
	if err := json.Unmarshal(body2, &updated); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if updated.ContextID == nil || *updated.ContextID != ctx2.ID {
		t.Errorf("child contextId: got %v, want %d (subtree should move)", updated.ContextID, ctx2.ID)
	}
}

// --- plan ---

func TestTaskPlan_SetWeek(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	task := createTestTask(t, e, ctx.ID, "Plan task")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/plan", task.ID),
		map[string]any{"state": "week"}))
	if resp.StatusCode != 200 {
		t.Fatalf("plan: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.TaskDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.PlanState != "week" {
		t.Errorf("planState: got %q, want %q", result.PlanState, "week")
	}
}

func TestTaskPlan_WeeklyLimit(t *testing.T) {
	// Config has Weekly.Limit = 7; create 7 tasks in week, 8th should fail.
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")

	for i := 0; i < 7; i++ {
		task := createTestTask(t, e, ctx.ID, fmt.Sprintf("Task %d", i))
		resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
			fmt.Sprintf("/api/v1/tasks/%d/plan", task.ID),
			map[string]any{"state": "week"}))
		if resp.StatusCode != 200 {
			t.Fatalf("plan task %d: got %d; body: %s", i, resp.StatusCode, body)
		}
	}

	task8 := createTestTask(t, e, ctx.ID, "Task 8")
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/plan", task8.ID),
		map[string]any{"state": "week"}))
	if resp.StatusCode != 422 {
		t.Fatalf("8th task: got %d, want 422; body: %s", resp.StatusCode, body)
	}
	er := parseErr(t, body)
	if er.Error.Code != httpapi.CodeLimitExceeded {
		t.Errorf("code: got %q, want %q", er.Error.Code, httpapi.CodeLimitExceeded)
	}
}

func TestTaskPlan_InvalidState(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	task := createTestTask(t, e, ctx.ID, "Task")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/plan", task.ID),
		map[string]any{"state": "monthly"}))
	if resp.StatusCode != 400 {
		t.Fatalf("invalid state: got %d, want 400; body: %s", resp.StatusCode, body)
	}
}

// --- pin / unpin ---

func TestTaskPin_Success(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	task := createTestTask(t, e, ctx.ID, "Pin me")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/pin", task.ID), nil))
	if resp.StatusCode != 200 {
		t.Fatalf("pin: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.TaskDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !result.IsPinned {
		t.Error("isPinned: got false, want true")
	}
}

func TestTaskPin_LimitExceeded(t *testing.T) {
	// Config MaxPinned = 5; pin 5 tasks then try 6th.
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")

	for i := 0; i < 5; i++ {
		task := createTestTask(t, e, ctx.ID, fmt.Sprintf("Pin task %d", i))
		resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
			fmt.Sprintf("/api/v1/tasks/%d/pin", task.ID), nil))
		if resp.StatusCode != 200 {
			t.Fatalf("pin task %d: got %d; body: %s", i, resp.StatusCode, body)
		}
	}

	task6 := createTestTask(t, e, ctx.ID, "Pin task 6")
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/pin", task6.ID), nil))
	if resp.StatusCode != 422 {
		t.Fatalf("6th pin: got %d, want 422; body: %s", resp.StatusCode, body)
	}
	er := parseErr(t, body)
	if er.Error.Code != httpapi.CodeLimitExceeded {
		t.Errorf("code: got %q, want %q", er.Error.Code, httpapi.CodeLimitExceeded)
	}
}

func TestTaskUnpin(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	task := createTestTask(t, e, ctx.ID, "Task")

	doReq(t, e.app, e.authedReq(t, http.MethodPost, fmt.Sprintf("/api/v1/tasks/%d/pin", task.ID), nil))

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/unpin", task.ID), nil))
	if resp.StatusCode != 200 {
		t.Fatalf("unpin: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.TaskDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.IsPinned {
		t.Error("isPinned: got true, want false after unpin")
	}
}

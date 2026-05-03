package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/service"
)

type troikiSlotResp struct {
	Capacity int           `json:"capacity"`
	Tasks    []dto.TaskDTO `json:"tasks"`
}

type troikiViewResp struct {
	Important troikiSlotResp `json:"important"`
	Medium    troikiSlotResp `json:"medium"`
	Rest      troikiSlotResp `json:"rest"`
}

func TestTroikiView_Empty(t *testing.T) {
	e := setupAPIEnv(t)

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/troiki", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("view: got %d; body: %s", resp.StatusCode, body)
	}
	var v troikiViewResp
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if v.Important.Capacity != service.TroikiImportantCap {
		t.Errorf("important capacity: got %d, want %d", v.Important.Capacity, service.TroikiImportantCap)
	}
	if len(v.Important.Tasks) != 0 {
		t.Errorf("important tasks: got %d, want 0", len(v.Important.Tasks))
	}
	if v.Medium.Capacity != 0 {
		t.Errorf("medium capacity: got %d, want 0", v.Medium.Capacity)
	}
	if v.Rest.Capacity != 0 {
		t.Errorf("rest capacity: got %d, want 0", v.Rest.Capacity)
	}
}

func TestTroikiSetCategory_Important(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	task := createTestTask(t, e, ctx.ID, "Do thing")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/troiki", task.ID),
		map[string]any{"category": "important"}))
	if resp.StatusCode != 200 {
		t.Fatalf("set: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.TaskDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.TroikiCategory == nil || *result.TroikiCategory != "important" {
		t.Errorf("category: got %v, want important", result.TroikiCategory)
	}
}

func TestTroikiSetCategory_Clear(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	task := createTestTask(t, e, ctx.ID, "x")

	doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/troiki", task.ID),
		map[string]any{"category": "important"}))

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/troiki", task.ID),
		map[string]any{"category": nil}))
	if resp.StatusCode != 200 {
		t.Fatalf("clear: got %d; body: %s", resp.StatusCode, body)
	}
	var result dto.TaskDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.TroikiCategory != nil {
		t.Errorf("category: got %v, want nil", *result.TroikiCategory)
	}
}

func TestTroikiSetCategory_SlotFull(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")

	for i := range service.TroikiImportantCap {
		task := createTestTask(t, e, ctx.ID, fmt.Sprintf("imp-%d", i))
		resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
			fmt.Sprintf("/api/v1/tasks/%d/troiki", task.ID),
			map[string]any{"category": "important"}))
		if resp.StatusCode != 200 {
			t.Fatalf("seed %d: got %d; body: %s", i, resp.StatusCode, body)
		}
	}
	extra := createTestTask(t, e, ctx.ID, "extra")
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/troiki", extra.ID),
		map[string]any{"category": "important"}))
	if resp.StatusCode != 409 {
		t.Fatalf("overflow: got %d, want 409; body: %s", resp.StatusCode, body)
	}
	er := parseErr(t, body)
	if er.Error.Code != httpapi.CodeTroikiSlotFull {
		t.Errorf("code: got %q, want %q", er.Error.Code, httpapi.CodeTroikiSlotFull)
	}
}

func TestTroikiSetCategory_Subtask(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	parent := createTestTask(t, e, ctx.ID, "parent")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/subtasks", parent.ID),
		map[string]any{"title": "child"}))
	if resp.StatusCode != 201 {
		t.Fatalf("create subtask: got %d; body: %s", resp.StatusCode, body)
	}
	var child dto.TaskDTO
	if err := json.Unmarshal(body, &child); err != nil {
		t.Fatalf("parse child: %v", err)
	}

	resp2, body2 := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/troiki", child.ID),
		map[string]any{"category": "important"}))
	if resp2.StatusCode != 422 {
		t.Fatalf("subtask: got %d, want 422; body: %s", resp2.StatusCode, body2)
	}
}

func TestTroikiSetCategory_InvalidCategory(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	task := createTestTask(t, e, ctx.ID, "x")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/troiki", task.ID),
		map[string]any{"category": "bogus"}))
	if resp.StatusCode != 400 {
		t.Fatalf("invalid: got %d, want 400; body: %s", resp.StatusCode, body)
	}
}

func TestTroikiSetCategory_NotFound(t *testing.T) {
	e := setupAPIEnv(t)
	resp, _ := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		"/api/v1/tasks/9999/troiki",
		map[string]any{"category": "important"}))
	if resp.StatusCode != 404 {
		t.Fatalf("got %d, want 404", resp.StatusCode)
	}
}

func TestTroikiView_AfterCompletion(t *testing.T) {
	// Completing an Important task grants +1 Medium capacity (via CompleteService hook).
	// View should reflect that capacity bump and the assigned Medium task.
	e := setupAPIEnv(t)
	ctxObj := createTestContext(t, e, "Work")

	imp := createTestTask(t, e, ctxObj.ID, "important task")
	if r, b := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/troiki", imp.ID),
		map[string]any{"category": "important"})); r.StatusCode != 200 {
		t.Fatalf("set important: got %d; body: %s", r.StatusCode, b)
	}
	if r, b := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/complete", imp.ID), nil)); r.StatusCode != 200 {
		t.Fatalf("complete: got %d; body: %s", r.StatusCode, b)
	}

	med := createTestTask(t, e, ctxObj.ID, "medium task")
	if r, b := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/troiki", med.ID),
		map[string]any{"category": "medium"})); r.StatusCode != 200 {
		t.Fatalf("set medium: got %d; body: %s", r.StatusCode, b)
	}

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/troiki", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("view: got %d; body: %s", resp.StatusCode, body)
	}
	var v troikiViewResp
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if v.Medium.Capacity != 1 {
		t.Errorf("medium capacity: got %d, want 1", v.Medium.Capacity)
	}
	if len(v.Medium.Tasks) != 1 {
		t.Fatalf("medium tasks: got %d, want 1", len(v.Medium.Tasks))
	}
	if v.Medium.Tasks[0].TroikiCategory == nil || *v.Medium.Tasks[0].TroikiCategory != string(model.TroikiCategoryMedium) {
		t.Errorf("medium task category: got %v, want medium", v.Medium.Tasks[0].TroikiCategory)
	}
	// Important slot should now be empty (its only task was completed).
	if len(v.Important.Tasks) != 0 {
		t.Errorf("important tasks: got %d, want 0", len(v.Important.Tasks))
	}
}

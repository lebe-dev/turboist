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

type troikiProjectResp struct {
	dto.ProjectDTO
	Tasks []dto.TaskDTO `json:"tasks"`
}

type troikiSlotResp struct {
	Capacity int                 `json:"capacity"`
	Projects []troikiProjectResp `json:"projects"`
}

type troikiViewResp struct {
	Important troikiSlotResp `json:"important"`
	Medium    troikiSlotResp `json:"medium"`
	Rest      troikiSlotResp `json:"rest"`
	Started   bool           `json:"started"`
}

func setProjectTroiki(t *testing.T, e *apiEnv, projectID int64, cat any) (int, []byte) {
	t.Helper()
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/projects/%d/troiki", projectID),
		map[string]any{"category": cat}))
	return resp.StatusCode, body
}

func createTaskInProject(t *testing.T, e *apiEnv, projectID int64, title string) dto.TaskDTO {
	t.Helper()
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/projects/%d/tasks", projectID),
		map[string]any{"title": title}))
	if resp.StatusCode != 201 {
		t.Fatalf("create task in project: got %d; body: %s", resp.StatusCode, body)
	}
	var result dto.TaskDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse task: %v", err)
	}
	return result
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
	if len(v.Important.Projects) != 0 {
		t.Errorf("important projects: got %d, want 0", len(v.Important.Projects))
	}
	if v.Medium.Capacity != 0 {
		t.Errorf("medium capacity: got %d, want 0", v.Medium.Capacity)
	}
	if v.Rest.Capacity != 0 {
		t.Errorf("rest capacity: got %d, want 0", v.Rest.Capacity)
	}
	if v.Started {
		t.Errorf("started: got true, want false on fresh user")
	}
}

func TestTroikiSetCategory_Important(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	proj := createTestProject(t, e, ctx.ID, "Alpha")

	code, body := setProjectTroiki(t, e, proj.ID, "important")
	if code != 200 {
		t.Fatalf("set: got %d, want 200; body: %s", code, body)
	}
	var result dto.ProjectDTO
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.TroikiCategory == nil || *result.TroikiCategory != "important" {
		t.Errorf("category: got %v, want important", result.TroikiCategory)
	}
}

func TestTroikiSetCategory_AppliesPriorityToProjectTasks(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	proj := createTestProject(t, e, ctx.ID, "Alpha")
	root := createTaskInProject(t, e, proj.ID, "root")
	subResp, subBody := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/subtasks", root.ID),
		map[string]any{"title": "child"}))
	if subResp.StatusCode != 201 {
		t.Fatalf("create subtask: got %d; body: %s", subResp.StatusCode, subBody)
	}
	var sub dto.TaskDTO
	if err := json.Unmarshal(subBody, &sub); err != nil {
		t.Fatalf("parse sub: %v", err)
	}

	if code, body := setProjectTroiki(t, e, proj.ID, "important"); code != 200 {
		t.Fatalf("set: got %d; body: %s", code, body)
	}

	for _, id := range []int64{root.ID, sub.ID} {
		resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet,
			fmt.Sprintf("/api/v1/tasks/%d", id), nil))
		if resp.StatusCode != 200 {
			t.Fatalf("get task %d: %d; body: %s", id, resp.StatusCode, body)
		}
		var got dto.TaskDTO
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if got.Priority != "high" {
			t.Errorf("task %d priority: got %q, want high", id, got.Priority)
		}
	}
}

func TestTroikiSetCategory_Clear(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	proj := createTestProject(t, e, ctx.ID, "Alpha")

	if code, body := setProjectTroiki(t, e, proj.ID, "important"); code != 200 {
		t.Fatalf("seed: got %d; body: %s", code, body)
	}
	code, body := setProjectTroiki(t, e, proj.ID, nil)
	if code != 200 {
		t.Fatalf("clear: got %d; body: %s", code, body)
	}
	var result dto.ProjectDTO
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
		p := createTestProject(t, e, ctx.ID, fmt.Sprintf("imp-%d", i))
		if code, body := setProjectTroiki(t, e, p.ID, "important"); code != 200 {
			t.Fatalf("seed %d: got %d; body: %s", i, code, body)
		}
	}
	extra := createTestProject(t, e, ctx.ID, "extra")
	code, body := setProjectTroiki(t, e, extra.ID, "important")
	if code != 409 {
		t.Fatalf("overflow: got %d, want 409; body: %s", code, body)
	}
	er := parseErr(t, body)
	if er.Error.Code != httpapi.CodeTroikiSlotFull {
		t.Errorf("code: got %q, want %q", er.Error.Code, httpapi.CodeTroikiSlotFull)
	}
}

func TestTroikiSetCategory_InvalidCategory(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	proj := createTestProject(t, e, ctx.ID, "Alpha")

	code, body := setProjectTroiki(t, e, proj.ID, "bogus")
	if code != 400 {
		t.Fatalf("invalid: got %d, want 400; body: %s", code, body)
	}
}

func TestTroikiSetCategory_NotFound(t *testing.T) {
	e := setupAPIEnv(t)
	code, _ := setProjectTroiki(t, e, 9999, "important")
	if code != 404 {
		t.Fatalf("got %d, want 404", code)
	}
}

func TestTroikiStart_SnapshotsAndFlips(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")

	// Pre-fill Medium with 2 projects while still in initial mode.
	for i := 0; i < 2; i++ {
		p := createTestProject(t, e, ctx.ID, fmt.Sprintf("m-%d", i))
		if code, body := setProjectTroiki(t, e, p.ID, "medium"); code != 200 {
			t.Fatalf("seed medium %d: got %d; body: %s", i, code, body)
		}
	}

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/troiki/start", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("start: got %d; body: %s", resp.StatusCode, body)
	}
	var v troikiViewResp
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !v.Started {
		t.Errorf("started: got false, want true")
	}
	if v.Medium.Capacity != 2 {
		t.Errorf("medium cap snapshot: got %d, want 2", v.Medium.Capacity)
	}

	// After start, adding a third Medium without earned capacity fails.
	extra := createTestProject(t, e, ctx.ID, "extra-m")
	code, body2 := setProjectTroiki(t, e, extra.ID, "medium")
	if code != 409 {
		t.Fatalf("post-start medium overflow: got %d, want 409; body: %s", code, body2)
	}
}

func TestTroikiView_ReturnsProjectsWithTasks(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	proj := createTestProject(t, e, ctx.ID, "Alpha")
	root := createTaskInProject(t, e, proj.ID, "root")
	if r, b := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/subtasks", root.ID),
		map[string]any{"title": "child"})); r.StatusCode != 201 {
		t.Fatalf("create subtask: %d; body: %s", r.StatusCode, b)
	}
	if code, body := setProjectTroiki(t, e, proj.ID, "important"); code != 200 {
		t.Fatalf("seed: %d; body: %s", code, body)
	}

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/troiki", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("view: %d; body: %s", resp.StatusCode, body)
	}
	var v troikiViewResp
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(v.Important.Projects) != 1 {
		t.Fatalf("important projects: got %d, want 1", len(v.Important.Projects))
	}
	got := v.Important.Projects[0]
	if got.ID != proj.ID {
		t.Errorf("project id: got %d, want %d", got.ID, proj.ID)
	}
	if got.TroikiCategory == nil || *got.TroikiCategory != "important" {
		t.Errorf("project category: got %v, want important", got.TroikiCategory)
	}
	if len(got.Tasks) != 2 {
		t.Errorf("project tasks: got %d, want 2 (root+subtask)", len(got.Tasks))
	}
}

func TestTroikiView_AfterCompletion(t *testing.T) {
	// Completing a task in an Important project grants +1 Medium capacity.
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	imp := createTestProject(t, e, ctx.ID, "Alpha")
	if code, body := setProjectTroiki(t, e, imp.ID, "important"); code != 200 {
		t.Fatalf("set important: %d; body: %s", code, body)
	}
	tk := createTaskInProject(t, e, imp.ID, "imp task")
	if r, b := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/complete", tk.ID), nil)); r.StatusCode != 200 {
		t.Fatalf("complete: %d; body: %s", r.StatusCode, b)
	}

	med := createTestProject(t, e, ctx.ID, "Beta")
	if code, body := setProjectTroiki(t, e, med.ID, "medium"); code != 200 {
		t.Fatalf("set medium: %d; body: %s", code, body)
	}

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/troiki", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("view: %d; body: %s", resp.StatusCode, body)
	}
	var v troikiViewResp
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if v.Medium.Capacity != 1 {
		t.Errorf("medium capacity: got %d, want 1", v.Medium.Capacity)
	}
	if len(v.Medium.Projects) != 1 {
		t.Errorf("medium projects: got %d, want 1", len(v.Medium.Projects))
	}
}

func TestPatchTask_PriorityRejectedWhenProjectInTroiki(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	proj := createTestProject(t, e, ctx.ID, "Alpha")
	tk := createTaskInProject(t, e, proj.ID, "root")
	if code, body := setProjectTroiki(t, e, proj.ID, "important"); code != 200 {
		t.Fatalf("set: %d; body: %s", code, body)
	}

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch,
		fmt.Sprintf("/api/v1/tasks/%d", tk.ID),
		map[string]any{"priority": "low"}))
	if resp.StatusCode != 400 {
		t.Fatalf("patch: got %d, want 400; body: %s", resp.StatusCode, body)
	}

	// Setting the derived priority is allowed.
	resp2, body2 := doReq(t, e.app, e.authedReq(t, http.MethodPatch,
		fmt.Sprintf("/api/v1/tasks/%d", tk.ID),
		map[string]any{"priority": "high"}))
	if resp2.StatusCode != 200 {
		t.Fatalf("patch derived: got %d; body: %s", resp2.StatusCode, body2)
	}
}

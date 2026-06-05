package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/model"
)

// assertReadOnly403 sends a request and asserts it is rejected 403
// federation_read_only — the F5.2 local FederationGuard enforcement seam
// (US-5.1 AC4 backend leg).
func assertReadOnly403(t *testing.T, e *apiEnv, method, path string, body any) {
	t.Helper()
	resp, raw := doReq(t, e.app, e.authedReq(t, method, path, body))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("%s %s: got %d, want 403; body: %s", method, path, resp.StatusCode, raw)
	}
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("parse error envelope: %v", err)
	}
	if env.Error.Code != "federation_read_only" {
		t.Errorf("%s %s code: got %q, want federation_read_only", method, path, env.Error.Code)
	}
}

// TestTaskMutations_ReadOnlyFederated403 asserts that EVERY task-level mutation
// entry point against a task in a joined read-only federated project is rejected
// 403 federation_read_only by the local FederationGuard (Federation v1 F5.2,
// US-5.1 AC4 backend leg). The guard must hook every entry point — the task
// patch/delete, the action verbs (complete/uncomplete/cancel/pin/unpin/move/
// plan), the sub-resource creates (subtask/duplicate/decompose), and the bulk
// verbs (bulk complete/move, group) — not just the project-keyed routes F2.4
// covered. UI disabling is insufficient; this is the authoritative seam.
func TestTaskMutations_ReadOnlyFederated403(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Read only")
	task := createTaskInProject(t, e, p.ID, "Locked task")
	// Mark the project a joined read-only federated copy AFTER the task exists, so
	// the task already maps to it through projectId.
	seedJoinedProject(t, e, p.ID, "https://owner.example", model.FederationPermissionRead)

	tid := task.ID
	cases := []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"patch", http.MethodPatch, fmt.Sprintf("/api/v1/tasks/%d", tid), map[string]any{"title": "renamed"}},
		{"delete", http.MethodDelete, fmt.Sprintf("/api/v1/tasks/%d", tid), nil},
		{"complete", http.MethodPost, fmt.Sprintf("/api/v1/tasks/%d/complete", tid), nil},
		{"uncomplete", http.MethodPost, fmt.Sprintf("/api/v1/tasks/%d/uncomplete", tid), nil},
		{"cancel", http.MethodPost, fmt.Sprintf("/api/v1/tasks/%d/cancel", tid), nil},
		{"pin", http.MethodPost, fmt.Sprintf("/api/v1/tasks/%d/pin", tid), nil},
		{"unpin", http.MethodPost, fmt.Sprintf("/api/v1/tasks/%d/unpin", tid), nil},
		{"plan", http.MethodPost, fmt.Sprintf("/api/v1/tasks/%d/plan", tid), map[string]any{"state": "week"}},
		{"subtask", http.MethodPost, fmt.Sprintf("/api/v1/tasks/%d/subtasks", tid), map[string]any{"title": "child"}},
		{"duplicate", http.MethodPost, fmt.Sprintf("/api/v1/tasks/%d/duplicate", tid), nil},
		{"decompose", http.MethodPost, fmt.Sprintf("/api/v1/tasks/%d/decompose", tid), map[string]any{"titles": []string{"a", "b"}}},
		{"bulkComplete", http.MethodPost, "/api/v1/tasks/bulk/complete", map[string]any{"ids": []int64{tid}}},
		{"group", http.MethodPost, "/api/v1/tasks/group", map[string]any{"title": "Group", "childIds": []int64{tid}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertReadOnly403(t, e, tc.method, tc.path, tc.body)
		})
	}
}

// TestTaskMove_IntoReadOnlyFederated403 asserts that moving a plain local task
// INTO a read-only federated project is rejected (Federation v1 F5.2): the guard
// keys on the destination project of a move, not only the task's current project,
// so a writable task cannot be smuggled into a read-only federated container.
func TestTaskMove_IntoReadOnlyFederated403(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	readonly := createTestProject(t, e, ctx.ID, "Read only target")
	seedJoinedProject(t, e, readonly.ID, "https://owner.example", model.FederationPermissionRead)
	// A plain task in the inbox (no project) — always locally editable.
	task := createTestTask(t, e, ctx.ID, "Free task")

	assertReadOnly403(t, e, http.MethodPost, fmt.Sprintf("/api/v1/tasks/%d/move", task.ID),
		map[string]any{"contextId": ctx.ID, "projectId": readonly.ID})

	assertReadOnly403(t, e, http.MethodPost, "/api/v1/tasks/bulk/move",
		map[string]any{"ids": []int64{task.ID}, "contextId": ctx.ID, "projectId": readonly.ID})
}

// TestTaskMutations_WriteFederatedSucceeds asserts a task in a joined WRITE
// federated project is NOT locked: its mutations go through (Federation v1 F5.2,
// US-5.1 write leg). Only read-only peers are blocked.
func TestTaskMutations_WriteFederatedSucceeds(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Writable")
	task := createTaskInProject(t, e, p.ID, "Editable task")
	seedJoinedProject(t, e, p.ID, "https://owner.example", model.FederationPermissionWrite)

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch,
		fmt.Sprintf("/api/v1/tasks/%d", task.ID), map[string]any{"title": "renamed"}))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("write-task patch: got %d, want 200; body: %s", resp.StatusCode, body)
	}
}

// TestTaskMutations_NonProjectTaskNotGuarded asserts a task with no project
// (an inbox task) is never subject to the federation guard (Federation v1 F5.2
// risk note: inbox/non-project tasks are always local). The read-only seam must
// never affect the non-federated path.
func TestTaskMutations_NonProjectTaskNotGuarded(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	// Seed an unrelated read-only federated project so the guard is "armed" but
	// must not bleed onto a task that lives outside it.
	other := createTestProject(t, e, ctx.ID, "Read only elsewhere")
	seedJoinedProject(t, e, other.ID, "https://owner.example", model.FederationPermissionRead)

	task := createTestTask(t, e, ctx.ID, "Inbox task")
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch,
		fmt.Sprintf("/api/v1/tasks/%d", task.ID), map[string]any{"title": "renamed"}))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("non-project task patch: got %d, want 200; body: %s", resp.StatusCode, body)
	}
}

// TestSectionMutations_ReadOnlyFederated403 asserts a section in a joined
// read-only federated project cannot be patched, deleted, reordered, or have a
// task created under it (Federation v1 F5.2 — the section entry points are part
// of "every mutation entry point").
func TestSectionMutations_ReadOnlyFederated403(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	p := createTestProject(t, e, ctx.ID, "Read only")
	secResp, secBody := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/projects/%d/sections", p.ID), map[string]any{"title": "Sec"}))
	if secResp.StatusCode != http.StatusCreated {
		t.Fatalf("create section: got %d, want 201; body: %s", secResp.StatusCode, secBody)
	}
	var sec dto.SectionDTO
	if err := json.Unmarshal(secBody, &sec); err != nil {
		t.Fatalf("parse section: %v", err)
	}
	seedJoinedProject(t, e, p.ID, "https://owner.example", model.FederationPermissionRead)

	assertReadOnly403(t, e, http.MethodPatch, fmt.Sprintf("/api/v1/sections/%d", sec.ID), map[string]any{"title": "renamed"})
	assertReadOnly403(t, e, http.MethodPost, fmt.Sprintf("/api/v1/sections/%d/reorder", sec.ID), map[string]any{"position": 0})
	assertReadOnly403(t, e, http.MethodPost, fmt.Sprintf("/api/v1/sections/%d/tasks", sec.ID), map[string]any{"title": "t"})
	assertReadOnly403(t, e, http.MethodDelete, fmt.Sprintf("/api/v1/sections/%d", sec.ID), nil)
}

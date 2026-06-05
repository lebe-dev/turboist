package handlers_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/lebe-dev/turboist/internal/repo"
)

func seedOfflineSyncContext(t *testing.T, e *apiEnv, name string) int64 {
	t.Helper()
	c, err := e.ctxs.Create(context.Background(), name, "blue", false)
	if err != nil {
		t.Fatalf("seed context: %v", err)
	}
	return c.ID
}

// TestTaskPatch_Tombstone_Returns410 asserts that PATCHing a soft-deleted task
// returns HTTP 410 Gone (Federation v1 F0.1, US-3.7 AC2 foundation) — the
// tombstone is final and must not be silently resurrected.
func TestTaskPatch_Tombstone_Returns410(t *testing.T) {
	e := setupAPIEnv(t)
	ctxID := seedOfflineSyncContext(t, e, "Tomb")
	task := createTestTask(t, e, ctxID, "doomed")

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodDelete, fmt.Sprintf("/api/v1/tasks/%d", task.ID), nil))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: got %d, want 204; body: %s", resp.StatusCode, body)
	}

	resp, body = doReq(t, e.app, e.authedReq(t, http.MethodPatch,
		fmt.Sprintf("/api/v1/tasks/%d", task.ID), map[string]any{"title": "resurrected"}))
	if resp.StatusCode != http.StatusGone {
		t.Fatalf("patch tombstone: got %d, want 410; body: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "gone") {
		t.Errorf("expected error code 'gone' in body; body: %s", body)
	}
}

// TestProjectPatch_Tombstone_Returns410 asserts the same 410-on-tombstone
// contract for projects.
func TestProjectPatch_Tombstone_Returns410(t *testing.T) {
	e := setupAPIEnv(t)
	ctxID := seedOfflineSyncContext(t, e, "ProjTomb")
	p, err := e.projects.Create(context.Background(), repo.CreateProject{ContextID: ctxID, Title: "p", Color: "blue"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodDelete, fmt.Sprintf("/api/v1/projects/%d", p.ID), nil))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete project: got %d, want 204; body: %s", resp.StatusCode, body)
	}

	resp, body = doReq(t, e.app, e.authedReq(t, http.MethodPatch,
		fmt.Sprintf("/api/v1/projects/%d", p.ID), map[string]any{"title": "back"}))
	if resp.StatusCode != http.StatusGone {
		t.Fatalf("patch tombstoned project: got %d, want 410; body: %s", resp.StatusCode, body)
	}
}

// TestTaskList_ExcludesTombstones asserts a soft-deleted task is no longer
// returned by a list endpoint (US-3.7 AC1 foundation — tombstone is hidden).
func TestTaskList_ExcludesTombstones(t *testing.T) {
	e := setupAPIEnv(t)
	ctxID := seedOfflineSyncContext(t, e, "ListCtx")
	_ = createTestTask(t, e, ctxID, "keepTask")
	gone := createTestTask(t, e, ctxID, "goneTask")

	resp, _ := doReq(t, e.app, e.authedReq(t, http.MethodDelete, fmt.Sprintf("/api/v1/tasks/%d", gone.ID), nil))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: got %d, want 204", resp.StatusCode)
	}

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet,
		fmt.Sprintf("/api/v1/contexts/%d/tasks", ctxID), nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "keepTask") {
		t.Errorf("expected live task 'keepTask' in list; body: %s", bodyStr)
	}
	if strings.Contains(bodyStr, "goneTask") {
		t.Errorf("tombstoned task 'goneTask' must not appear in list; body: %s", bodyStr)
	}
}

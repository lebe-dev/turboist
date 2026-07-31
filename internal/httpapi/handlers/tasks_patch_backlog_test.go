package handlers_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

// planTask moves a task into the given plan state via the dedicated plan endpoint.
func planTask(t *testing.T, e *apiEnv, id int64, state string) {
	t.Helper()
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/plan", id), map[string]any{"state": state}))
	if resp.StatusCode != 200 {
		t.Fatalf("plan task %d as %q: got %d, want 200; body: %s", id, state, resp.StatusCode, body)
	}
}

// Setting a due date on a backlog task pulls it out of the backlog (planState → none):
// a scheduled task no longer belongs to the unscheduled backlog.
func TestTaskPatch_SetDueOnBacklog_ClearsBacklog(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	task := createTestTask(t, e, ctx.ID, "Backlog task")
	planTask(t, e, task.ID, "backlog")

	tomorrow := time.Now().Add(24 * time.Hour)
	updated := patchTask(t, e, task.ID, map[string]any{"dueAt": model.FormatUTC(tomorrow)})

	if updated.PlanState != "none" {
		t.Errorf("planState: got %q, want %q", updated.PlanState, "none")
	}
	if updated.DueAt == nil {
		t.Fatal("dueAt: got nil, want a date")
	}
}

// Clearing the due date on a backlog task must NOT move it out of the backlog —
// only assigning a date does.
func TestTaskPatch_ClearDueOnBacklog_KeepsBacklog(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	task := createTestTask(t, e, ctx.ID, "Backlog task")
	planTask(t, e, task.ID, "backlog")

	updated := patchTask(t, e, task.ID, map[string]any{"dueAt": nil})

	if updated.PlanState != "backlog" {
		t.Errorf("planState: got %q, want %q", updated.PlanState, "backlog")
	}
}

// Scope is backlog-only: setting a due date on a week-planned task leaves it in week.
func TestTaskPatch_SetDueOnWeek_KeepsWeek(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	task := createTestTask(t, e, ctx.ID, "Week task")
	planTask(t, e, task.ID, "week")

	tomorrow := time.Now().Add(24 * time.Hour)
	updated := patchTask(t, e, task.ID, map[string]any{"dueAt": model.FormatUTC(tomorrow)})

	if updated.PlanState != "week" {
		t.Errorf("planState: got %q, want %q", updated.PlanState, "week")
	}
}

// An explicit planState in the same PATCH wins over the implicit backlog clearing.
func TestTaskPatch_SetDueWithExplicitPlanState_RespectsRequest(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	task := createTestTask(t, e, ctx.ID, "Backlog task")
	planTask(t, e, task.ID, "backlog")

	tomorrow := time.Now().Add(24 * time.Hour)
	updated := patchTask(t, e, task.ID, map[string]any{
		"dueAt":     model.FormatUTC(tomorrow),
		"planState": "backlog",
	})

	if updated.PlanState != "backlog" {
		t.Errorf("planState: got %q, want %q", updated.PlanState, "backlog")
	}
}

// Subtasks follow their parent into the backlog whichever endpoint parks it — the
// plan endpoint and a PATCH that sets planState directly must behave alike.
func TestTaskPatch_SetBacklog_CascadesToSubtasks(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	parent := createTestTask(t, e, ctx.ID, "Parent")
	child := createTestSubtask(t, e, parent.ID, "Child")

	patchTask(t, e, parent.ID, map[string]any{"planState": "backlog"})

	got := fetchTask(t, e, child.ID)
	if got.PlanState != "backlog" {
		t.Errorf("child planState: got %q, want %q", got.PlanState, "backlog")
	}
}

// The plan endpoint is the path the SPA uses; it cascades through PlanService.
func TestTaskPlan_SetBacklog_CascadesToSubtasks(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	parent := createTestTask(t, e, ctx.ID, "Parent")
	child := createTestSubtask(t, e, parent.ID, "Child")
	grandchild := createTestSubtask(t, e, child.ID, "Grandchild")

	planTask(t, e, parent.ID, "backlog")

	for _, id := range []int64{child.ID, grandchild.ID} {
		got := fetchTask(t, e, id)
		if got.PlanState != "backlog" {
			t.Errorf("task %d planState: got %q, want %q", id, got.PlanState, "backlog")
		}
	}
}

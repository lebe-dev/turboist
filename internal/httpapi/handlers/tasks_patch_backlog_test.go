package handlers_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/httpapi/dto"
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

// schedule gives a task a due date with a time, as a planned task would have.
func schedule(t *testing.T, e *apiEnv, id int64) {
	t.Helper()
	tomorrow := time.Now().Add(24 * time.Hour)
	updated := patchTask(t, e, id, map[string]any{"dueAt": model.FormatUTC(tomorrow), "dueHasTime": true})
	if updated.DueAt == nil {
		t.Fatalf("schedule task %d: dueAt not set", id)
	}
}

// assertUnscheduled fails unless the task is parked with no day and no time.
func assertUnscheduled(t *testing.T, got dto.TaskDTO) {
	t.Helper()
	if got.PlanState != "backlog" {
		t.Errorf("task %q planState: got %q, want %q", got.Title, got.PlanState, "backlog")
	}
	if got.DueAt != nil {
		t.Errorf("task %q dueAt: got %v, want nil", got.Title, *got.DueAt)
	}
	if got.DueHasTime {
		t.Errorf("task %q dueHasTime: got true, want false", got.Title)
	}
}

// Parking a parent unschedules the whole open subtree, whichever endpoint parks
// it: a subtask keeping its day would surface in the day views for postponed work.
func TestTaskBacklog_UnschedulesSubtasks(t *testing.T) {
	for _, via := range []string{"plan", "patch"} {
		t.Run(via, func(t *testing.T) {
			e := setupAPIEnv(t)
			ctx := createTestContext(t, e, "Work")
			parent := createTestTask(t, e, ctx.ID, "Parent")
			child := createTestSubtask(t, e, parent.ID, "Child")
			grandchild := createTestSubtask(t, e, child.ID, "Grandchild")
			for _, id := range []int64{parent.ID, child.ID, grandchild.ID} {
				schedule(t, e, id)
			}

			if via == "plan" {
				planTask(t, e, parent.ID, "backlog")
			} else {
				patchTask(t, e, parent.ID, map[string]any{"planState": "backlog"})
			}

			for _, id := range []int64{parent.ID, child.ID, grandchild.ID} {
				assertUnscheduled(t, fetchTask(t, e, id))
			}
		})
	}
}

// A subtask planned for the week with a day of its own is parked and unscheduled
// with its parent too — the cascade does not skip work that was already planned.
func TestTaskBacklog_UnschedulesSubtaskPlannedForWeek(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	parent := createTestTask(t, e, ctx.ID, "Parent")
	child := createTestSubtask(t, e, parent.ID, "Child")
	planTask(t, e, child.ID, "week")
	schedule(t, e, child.ID)

	planTask(t, e, parent.ID, "backlog")

	assertUnscheduled(t, fetchTask(t, e, child.ID))
}

// A PATCH that parks a task and names a day in the same request keeps that day:
// the request said so explicitly.
func TestTaskPatch_SetBacklogWithDue_KeepsDue(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	task := createTestTask(t, e, ctx.ID, "Task")

	tomorrow := time.Now().Add(24 * time.Hour)
	updated := patchTask(t, e, task.ID, map[string]any{
		"dueAt":     model.FormatUTC(tomorrow),
		"planState": "backlog",
	})

	if updated.DueAt == nil {
		t.Error("dueAt: got nil, want the date named in the request")
	}
}

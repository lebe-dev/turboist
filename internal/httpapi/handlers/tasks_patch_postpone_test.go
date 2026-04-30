package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/model"
)

// shiftCreatedAt rewinds a task's created_at by d so postpone grace logic can be exercised.
func shiftCreatedAt(t *testing.T, e *apiEnv, taskID int64, d time.Duration) {
	t.Helper()
	newCreated := model.FormatUTC(time.Now().Add(-d))
	if _, err := e.db.ExecContext(context.Background(),
		`UPDATE tasks SET created_at = ? WHERE id = ?`, newCreated, taskID); err != nil {
		t.Fatalf("shift created_at: %v", err)
	}
}

func patchTask(t *testing.T, e *apiEnv, id int64, body map[string]any) dto.TaskDTO {
	t.Helper()
	resp, raw := doReq(t, e.app, e.authedReq(t, http.MethodPatch, fmt.Sprintf("/api/v1/tasks/%d", id), body))
	if resp.StatusCode != 200 {
		t.Fatalf("patch task: got %d, want 200; body: %s", resp.StatusCode, raw)
	}
	var result dto.TaskDTO
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	return result
}

func TestTaskPatch_Postpone_FutureToFurtherFuture_Increments(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	tomorrow := time.Now().Add(24 * time.Hour)
	created := createTestTask(t, e, ctx.ID, "Read book")
	patchTask(t, e, created.ID, map[string]any{"dueAt": model.FormatUTC(tomorrow)})

	shiftCreatedAt(t, e, created.ID, 10*time.Minute)

	nextWeek := time.Now().Add(7 * 24 * time.Hour)
	updated := patchTask(t, e, created.ID, map[string]any{"dueAt": model.FormatUTC(nextWeek)})

	if updated.PostponeCount != 1 {
		t.Errorf("postponeCount: got %d, want 1", updated.PostponeCount)
	}
}

func TestTaskPatch_Postpone_FutureToPast_DoesNotIncrement(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	tomorrow := time.Now().Add(24 * time.Hour)
	created := createTestTask(t, e, ctx.ID, "Read book")
	patchTask(t, e, created.ID, map[string]any{"dueAt": model.FormatUTC(tomorrow)})

	shiftCreatedAt(t, e, created.ID, 10*time.Minute)

	yesterday := time.Now().Add(-24 * time.Hour)
	updated := patchTask(t, e, created.ID, map[string]any{"dueAt": model.FormatUTC(yesterday)})

	if updated.PostponeCount != 0 {
		t.Errorf("postponeCount: got %d, want 0", updated.PostponeCount)
	}
}

func TestTaskPatch_Postpone_FreshTask_DoesNotIncrement(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	tomorrow := time.Now().Add(24 * time.Hour)
	created := createTestTask(t, e, ctx.ID, "Read book")
	patchTask(t, e, created.ID, map[string]any{"dueAt": model.FormatUTC(tomorrow)})

	// no shiftCreatedAt — the task is fresh, within grace period

	nextWeek := time.Now().Add(7 * 24 * time.Hour)
	updated := patchTask(t, e, created.ID, map[string]any{"dueAt": model.FormatUTC(nextWeek)})

	if updated.PostponeCount != 0 {
		t.Errorf("postponeCount: got %d, want 0", updated.PostponeCount)
	}
}

func TestTaskPatch_Postpone_FirstAssignment_DoesNotIncrement(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	created := createTestTask(t, e, ctx.ID, "Read book")
	// task created without dueAt
	shiftCreatedAt(t, e, created.ID, 10*time.Minute)

	tomorrow := time.Now().Add(24 * time.Hour)
	updated := patchTask(t, e, created.ID, map[string]any{"dueAt": model.FormatUTC(tomorrow)})

	if updated.PostponeCount != 0 {
		t.Errorf("postponeCount: got %d, want 0", updated.PostponeCount)
	}
}

func TestTaskPatch_Postpone_RepeatedPostpones_Accumulate(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	tomorrow := time.Now().Add(24 * time.Hour)
	created := createTestTask(t, e, ctx.ID, "Read book")
	patchTask(t, e, created.ID, map[string]any{"dueAt": model.FormatUTC(tomorrow)})

	shiftCreatedAt(t, e, created.ID, 10*time.Minute)

	d2 := time.Now().Add(7 * 24 * time.Hour)
	if got := patchTask(t, e, created.ID, map[string]any{"dueAt": model.FormatUTC(d2)}).PostponeCount; got != 1 {
		t.Fatalf("after 1st postpone: got %d, want 1", got)
	}
	d3 := time.Now().Add(14 * 24 * time.Hour)
	if got := patchTask(t, e, created.ID, map[string]any{"dueAt": model.FormatUTC(d3)}).PostponeCount; got != 2 {
		t.Fatalf("after 2nd postpone: got %d, want 2", got)
	}
	d4 := time.Now().Add(21 * 24 * time.Hour)
	if got := patchTask(t, e, created.ID, map[string]any{"dueAt": model.FormatUTC(d4)}).PostponeCount; got != 3 {
		t.Fatalf("after 3rd postpone: got %d, want 3", got)
	}
}

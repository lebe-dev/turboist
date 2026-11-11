package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/httpapi/dto"
	"github.com/lebe-dev/turboist/internal/model"
)

// The test config runs on UTC, so "today" is anything before the next UTC
// midnight and "tomorrow" anything 24h out.
func todayWire() string    { return model.FormatUTC(time.Now().UTC()) }
func tomorrowWire() string { return model.FormatUTC(time.Now().UTC().Add(24 * time.Hour)) }

func nextUTCMidnightWire() string {
	now := time.Now().UTC()
	return model.FormatUTC(time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC))
}

type lockErrBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Details struct {
			LockedUntil string `json:"lockedUntil"`
			ProjectID   *int64 `json:"projectId"`
		} `json:"details"`
	} `json:"error"`
}

type todayLockFixture struct {
	e       *apiEnv
	ctxID   int64
	focus   dto.ProjectDTO
	other   dto.ProjectDTO
	focusOn dto.TaskDTO // the task that puts focus on Today
}

// setupTodayLockEnv builds a workspace with one project on Today ("focus") and
// one that is not ("other"), and turns the lock on.
func setupTodayLockEnv(t *testing.T) *todayLockFixture {
	t.Helper()
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")
	f := &todayLockFixture{
		e:     e,
		ctxID: c.ID,
		focus: createTestProject(t, e, c.ID, "Focus"),
		other: createTestProject(t, e, c.ID, "Other"),
	}
	f.focusOn = createInProject(t, e, f.focus.ID, map[string]any{"title": "today", "dueAt": todayWire()})
	lockToday(t, e)
	return f
}

func lockToday(t *testing.T, e *apiEnv) string {
	t.Helper()
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/today/lock", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("lock: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var out struct {
		LockedUntil string `json:"lockedUntil"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("parse lock: %v", err)
	}
	return out.LockedUntil
}

func createInProject(t *testing.T, e *apiEnv, projectID int64, body map[string]any) dto.TaskDTO {
	t.Helper()
	resp, raw := doReq(t, e.app, e.authedReq(t, http.MethodPost, fmt.Sprintf("/api/v1/projects/%d/tasks", projectID), body))
	if resp.StatusCode != 201 {
		t.Fatalf("create in project: got %d, want 201; body: %s", resp.StatusCode, raw)
	}
	var out dto.TaskDTO
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("parse task: %v", err)
	}
	return out
}

func assertLockedResponse(t *testing.T, resp *http.Response, body []byte, wantProject *int64) {
	t.Helper()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status: got %d, want 409; body: %s", resp.StatusCode, body)
	}
	var got lockErrBody
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if got.Error.Code != "today_locked" {
		t.Errorf("code: got %q, want today_locked", got.Error.Code)
	}
	if got.Error.Details.LockedUntil != nextUTCMidnightWire() {
		t.Errorf("lockedUntil: got %q, want %q", got.Error.Details.LockedUntil, nextUTCMidnightWire())
	}
	if (got.Error.Details.ProjectID == nil) != (wantProject == nil) ||
		(wantProject != nil && *got.Error.Details.ProjectID != *wantProject) {
		t.Errorf("projectId: got %v, want %v", got.Error.Details.ProjectID, wantProject)
	}
}

func getSettingsLock(t *testing.T, e *apiEnv) *string {
	t.Helper()
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/settings", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("get settings: got %d; body: %s", resp.StatusCode, body)
	}
	var out struct {
		TodayLockUntil *string `json:"todayLockUntil"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("parse settings: %v", err)
	}
	return out.TodayLockUntil
}

// --- the lock endpoint -------------------------------------------------------

func TestTodayLock_SettingsHaveNoLockBeforeOne(t *testing.T) {
	e := setupAPIEnv(t)
	if got := getSettingsLock(t, e); got != nil {
		t.Errorf("todayLockUntil: got %q, want null", *got)
	}
}

func TestTodayLock_LockRunsUntilNextMidnightAndShowsInSettings(t *testing.T) {
	e := setupAPIEnv(t)
	until := lockToday(t, e)
	if until != nextUTCMidnightWire() {
		t.Errorf("lockedUntil: got %q, want %q", until, nextUTCMidnightWire())
	}
	got := getSettingsLock(t, e)
	if got == nil || *got != until {
		t.Errorf("settings todayLockUntil: got %v, want %q", got, until)
	}
	// Locking again changes nothing.
	if again := lockToday(t, e); again != until {
		t.Errorf("second lock: got %q, want %q", again, until)
	}
}

func TestTodayLock_SettingsPatchCannotLiftTheLock(t *testing.T) {
	e := setupAPIEnv(t)
	until := lockToday(t, e)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch, "/api/v1/settings",
		map[string]any{"todayLockUntil": nil, "bannerText": "hi"}))
	if resp.StatusCode != 200 {
		t.Fatalf("patch settings: got %d; body: %s", resp.StatusCode, body)
	}
	if got := getSettingsLock(t, e); got == nil || *got != until {
		t.Errorf("todayLockUntil after patch: got %v, want %q", got, until)
	}
}

func TestTodayLock_ConfigCarriesTheLock(t *testing.T) {
	e := setupAPIEnv(t)
	until := lockToday(t, e)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/config", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("config: got %d; body: %s", resp.StatusCode, body)
	}
	var cfg struct {
		Settings struct {
			TodayLockUntil *string `json:"todayLockUntil"`
		} `json:"settings"`
	}
	if err := json.Unmarshal(body, &cfg); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if cfg.Settings.TodayLockUntil == nil || *cfg.Settings.TodayLockUntil != until {
		t.Errorf("config settings.todayLockUntil: got %v, want %q", cfg.Settings.TodayLockUntil, until)
	}
}

func TestTodayLock_ReplicatesThroughSyncChanges(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/sync/snapshot", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("snapshot: got %d; body: %s", resp.StatusCode, body)
	}
	var snap struct {
		Epoch  int64 `json:"epoch"`
		Cursor int64 `json:"cursor"`
	}
	if err := json.Unmarshal(body, &snap); err != nil {
		t.Fatalf("parse snapshot: %v", err)
	}
	until := lockToday(t, e)
	resp, body = doReq(t, e.app, e.authedReq(t, http.MethodGet,
		fmt.Sprintf("/api/v1/sync/changes?since=%d&epoch=%d", snap.Cursor, snap.Epoch), nil))
	if resp.StatusCode != 200 {
		t.Fatalf("changes: got %d; body: %s", resp.StatusCode, body)
	}
	var page struct {
		Changes []struct {
			Entity string          `json:"entity"`
			Op     string          `json:"op"`
			Data   json.RawMessage `json:"data"`
		} `json:"changes"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatalf("parse changes: %v; body: %s", err, body)
	}
	for _, ch := range page.Changes {
		if ch.Entity != "user_settings" {
			continue
		}
		var s struct {
			TodayLockUntil *string `json:"todayLockUntil"`
		}
		if err := json.Unmarshal(ch.Data, &s); err != nil {
			t.Fatalf("parse user_settings: %v", err)
		}
		if s.TodayLockUntil == nil || *s.TodayLockUntil != until {
			t.Errorf("replicated todayLockUntil: got %v, want %q", s.TodayLockUntil, until)
		}
		return
	}
	t.Fatalf("no user_settings change after locking; body: %s", body)
}

// --- creating ----------------------------------------------------------------

func TestTodayLock_CreateTodayInOtherProjectIsRefused(t *testing.T) {
	f := setupTodayLockEnv(t)
	resp, body := doReq(t, f.e.app, f.e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/projects/%d/tasks", f.other.ID), map[string]any{"title": "x", "dueAt": todayWire()}))
	assertLockedResponse(t, resp, body, &f.other.ID)
}

func TestTodayLock_CreateTodayInTodaysProjectIsAllowed(t *testing.T) {
	f := setupTodayLockEnv(t)
	createInProject(t, f.e, f.focus.ID, map[string]any{"title": "more", "dueAt": todayWire()})
}

func TestTodayLock_CreateForTomorrowOrUndatedInOtherProjectIsAllowed(t *testing.T) {
	f := setupTodayLockEnv(t)
	createInProject(t, f.e, f.other.ID, map[string]any{"title": "later", "dueAt": tomorrowWire()})
	createInProject(t, f.e, f.other.ID, map[string]any{"title": "someday"})
}

func TestTodayLock_CreateTodayInInboxIsRefused(t *testing.T) {
	f := setupTodayLockEnv(t)
	resp, body := doReq(t, f.e.app, f.e.authedReq(t, http.MethodPost, "/api/v1/inbox/tasks",
		map[string]any{"title": "capture", "dueAt": todayWire()}))
	assertLockedResponse(t, resp, body, nil)
	resp, body = doReq(t, f.e.app, f.e.authedReq(t, http.MethodPost, "/api/v1/inbox/tasks",
		map[string]any{"title": "capture"}))
	if resp.StatusCode != 201 {
		t.Errorf("undated inbox capture: got %d, want 201; body: %s", resp.StatusCode, body)
	}
}

func TestTodayLock_CreateSubtaskTodayUnderOtherProjectIsRefused(t *testing.T) {
	f := setupTodayLockEnv(t)
	parent := createInProject(t, f.e, f.other.ID, map[string]any{"title": "parent"})
	resp, body := doReq(t, f.e.app, f.e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/subtasks", parent.ID), map[string]any{"title": "child", "dueAt": todayWire()}))
	assertLockedResponse(t, resp, body, &f.other.ID)
}

func TestTodayLock_DuplicateOntoTodayInOtherProjectIsRefused(t *testing.T) {
	f := setupTodayLockEnv(t)
	src := createInProject(t, f.e, f.other.ID, map[string]any{"title": "src", "dueAt": tomorrowWire()})
	resp, body := doReq(t, f.e.app, f.e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/duplicate", src.ID), map[string]any{"dueAt": todayWire()}))
	assertLockedResponse(t, resp, body, &f.other.ID)
}

func TestTodayLock_DuplicateOfATodayTaskIsAllowed(t *testing.T) {
	f := setupTodayLockEnv(t)
	resp, body := doReq(t, f.e.app, f.e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/duplicate", f.focusOn.ID), nil))
	if resp.StatusCode != 201 {
		t.Errorf("duplicate: got %d, want 201; body: %s", resp.StatusCode, body)
	}
}

// --- editing -----------------------------------------------------------------

func TestTodayLock_PatchDueTodayInOtherProjectIsRefused(t *testing.T) {
	f := setupTodayLockEnv(t)
	task := createInProject(t, f.e, f.other.ID, map[string]any{"title": "later", "dueAt": tomorrowWire()})
	resp, body := doReq(t, f.e.app, f.e.authedReq(t, http.MethodPatch,
		fmt.Sprintf("/api/v1/tasks/%d", task.ID), map[string]any{"dueAt": todayWire(), "title": "renamed"}))
	assertLockedResponse(t, resp, body, &f.other.ID)
	// Nothing of the refused edit was applied.
	resp, body = doReq(t, f.e.app, f.e.authedReq(t, http.MethodGet, fmt.Sprintf("/api/v1/tasks/%d", task.ID), nil))
	if resp.StatusCode != 200 {
		t.Fatalf("get task: got %d; body: %s", resp.StatusCode, body)
	}
	var got dto.TaskDTO
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("parse task: %v", err)
	}
	if got.Title != "later" || got.DueAt == nil || *got.DueAt != *task.DueAt {
		t.Errorf("task after refused patch: got title %q due %v, want unchanged", got.Title, got.DueAt)
	}
}

func TestTodayLock_PatchOfTaskAlreadyOnTodayIsAllowed(t *testing.T) {
	f := setupTodayLockEnv(t)
	updated := patchTask(t, f.e, f.focusOn.ID, map[string]any{"title": "renamed", "dueAt": todayWire()})
	if updated.Title != "renamed" {
		t.Errorf("title: got %q, want renamed", updated.Title)
	}
}

func TestTodayLock_PatchOffTodayIsAllowed(t *testing.T) {
	f := setupTodayLockEnv(t)
	patchTask(t, f.e, f.focusOn.ID, map[string]any{"dueAt": tomorrowWire()})
	patchTask(t, f.e, f.focusOn.ID, map[string]any{"dueAt": nil})
}

func TestTodayLock_PatchUnrelatedFieldsOfOtherProjectTaskIsAllowed(t *testing.T) {
	f := setupTodayLockEnv(t)
	task := createInProject(t, f.e, f.other.ID, map[string]any{"title": "later"})
	patchTask(t, f.e, task.ID, map[string]any{"title": "renamed", "priority": "high"})
}

// --- moving ------------------------------------------------------------------

func TestTodayLock_MoveTodayTaskIntoOtherProjectIsRefused(t *testing.T) {
	f := setupTodayLockEnv(t)
	resp, body := doReq(t, f.e.app, f.e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/move", f.focusOn.ID),
		map[string]any{"contextId": f.ctxID, "projectId": f.other.ID}))
	assertLockedResponse(t, resp, body, &f.other.ID)
}

func TestTodayLock_BulkMoveReportsLockedItems(t *testing.T) {
	f := setupTodayLockEnv(t)
	undated := createInProject(t, f.e, f.focus.ID, map[string]any{"title": "undated"})
	resp, body := doReq(t, f.e.app, f.e.authedReq(t, http.MethodPost, "/api/v1/tasks/bulk/move",
		map[string]any{"ids": []int64{f.focusOn.ID, undated.ID}, "contextId": f.ctxID, "projectId": f.other.ID}))
	if resp.StatusCode != 200 {
		t.Fatalf("bulk move: got %d; body: %s", resp.StatusCode, body)
	}
	var out struct {
		Succeeded []int64 `json:"succeeded"`
		Failed    []struct {
			ID    int64 `json:"id"`
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		} `json:"failed"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("parse bulk: %v", err)
	}
	if len(out.Succeeded) != 1 || out.Succeeded[0] != undated.ID {
		t.Errorf("succeeded: got %v, want [%d]", out.Succeeded, undated.ID)
	}
	if len(out.Failed) != 1 || out.Failed[0].ID != f.focusOn.ID || out.Failed[0].Error.Code != "today_locked" {
		t.Errorf("failed: got %+v, want task %d refused as today_locked", out.Failed, f.focusOn.ID)
	}
}

// --- reopening ---------------------------------------------------------------

func TestTodayLock_UncompleteTodayTaskOfOtherProjectIsRefused(t *testing.T) {
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")
	other := createTestProject(t, e, c.ID, "Other")
	task := createInProject(t, e, other.ID, map[string]any{"title": "done", "dueAt": todayWire()})
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, fmt.Sprintf("/api/v1/tasks/%d/complete", task.ID), nil))
	if resp.StatusCode != 200 {
		t.Fatalf("complete: got %d; body: %s", resp.StatusCode, body)
	}
	lockToday(t, e)
	resp, body = doReq(t, e.app, e.authedReq(t, http.MethodPost, fmt.Sprintf("/api/v1/tasks/%d/uncomplete", task.ID), nil))
	assertLockedResponse(t, resp, body, &other.ID)
}

// --- without the lock --------------------------------------------------------

func TestTodayLock_WithoutLockEverythingIsAllowed(t *testing.T) {
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")
	other := createTestProject(t, e, c.ID, "Other")
	createInProject(t, e, other.ID, map[string]any{"title": "x", "dueAt": todayWire()})
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost, "/api/v1/inbox/tasks",
		map[string]any{"title": "capture", "dueAt": todayWire()}))
	if resp.StatusCode != 201 {
		t.Errorf("inbox create: got %d, want 201; body: %s", resp.StatusCode, body)
	}
}

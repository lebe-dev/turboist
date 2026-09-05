package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/httpapi/dto"
)

type viewResp struct {
	Items []dto.TaskDTO `json:"items"`
	Total int           `json:"total"`
}

type todayBundleResp struct {
	Today          dto.PagedResponse[dto.TaskDTO] `json:"today"`
	Overdue        dto.PagedResponse[dto.TaskDTO] `json:"overdue"`
	CompletedToday dto.PagedResponse[dto.TaskDTO] `json:"completedToday"`
}

type sidebarStatsResp struct {
	PlanStats struct {
		Week    int `json:"week"`
		Backlog int `json:"backlog"`
	} `json:"planStats"`
	InboxStats struct {
		Count                 int  `json:"count"`
		WarnThresholdExceeded bool `json:"warnThresholdExceeded"`
	} `json:"inboxStats"`
	Pinned viewResp `json:"pinned"`
}

func TestTaskViews_StatsSidebar_Empty(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/stats/sidebar", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result sidebarStatsResp
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.PlanStats.Week != 0 || result.PlanStats.Backlog != 0 {
		t.Errorf("plan stats: got %+v, want zero", result.PlanStats)
	}
	if result.InboxStats.Count != 0 || result.InboxStats.WarnThresholdExceeded {
		t.Errorf("inbox stats: got %+v, want zero", result.InboxStats)
	}
	if result.Pinned.Total != 0 || len(result.Pinned.Items) != 0 {
		t.Errorf("pinned: got total=%d items=%d, want empty", result.Pinned.Total, len(result.Pinned.Items))
	}
}

type weekSummaryResp struct {
	Range struct {
		Start string `json:"start"`
		End   string `json:"end"`
	} `json:"range"`
	Stats struct {
		CompletedCount int `json:"completedCount"`
		PlannedOpen    int `json:"plannedOpen"`
		Overdue        int `json:"overdue"`
	} `json:"stats"`
	Completed []dto.TaskDTO       `json:"completed"`
	Troiki    *weekSummaryTroikiR `json:"troiki"`
}

type weekSummaryTroikiR struct {
	Started bool `json:"started"`
	Slots   []struct {
		Category  string `json:"category"`
		Capacity  int    `json:"capacity"`
		Projects  int    `json:"projects"`
		Open      int    `json:"open"`
		Completed int    `json:"completed"`
	} `json:"slots"`
}

func TestTaskViews_WeekSummary_Empty(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/stats/week-summary", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result weekSummaryResp
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Stats.CompletedCount != 0 || result.Stats.PlannedOpen != 0 || result.Stats.Overdue != 0 {
		t.Errorf("stats: got %+v, want zero", result.Stats)
	}
	if len(result.Completed) != 0 {
		t.Errorf("completed: got %d items, want 0", len(result.Completed))
	}
	if result.Range.Start == "" || result.Range.End == "" {
		t.Errorf("range: got %+v, want populated bounds", result.Range)
	}
	if result.Troiki != nil {
		t.Errorf("troiki: got %+v, want nil when system disabled", result.Troiki)
	}
}

// TestTaskViews_WeekSummary_Troiki verifies the prioritised Troiki progress
// block: when the system is enabled, each category reports its capacity,
// project/open-task counts and how many of its tasks were completed this week.
func TestTaskViews_WeekSummary_Troiki(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")

	// Enable the Troiki system.
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPatch, "/api/v1/settings",
		map[string]any{"troikiEnabled": true}))
	if resp.StatusCode != 200 {
		t.Fatalf("enable troiki: got %d; body: %s", resp.StatusCode, body)
	}

	// Important project with one completed + one open task.
	important := createTestProject(t, e, ctx.ID, "Important project")
	if code, b := setProjectTroiki(t, e, important.ID, "important"); code != 200 {
		t.Fatalf("assign important: got %d; body: %s", code, b)
	}
	done := createTaskInProject(t, e, important.ID, "Done important")
	if r, b := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/complete", done.ID), nil)); r.StatusCode != 200 {
		t.Fatalf("complete: got %d; body: %s", r.StatusCode, b)
	}
	createTaskInProject(t, e, important.ID, "Open important")

	resp2, body2 := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/stats/week-summary", nil))
	if resp2.StatusCode != 200 {
		t.Fatalf("week-summary: got %d; body: %s", resp2.StatusCode, body2)
	}
	var result weekSummaryResp
	if err := json.Unmarshal(body2, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Troiki == nil {
		t.Fatal("troiki: got nil, want progress block when enabled")
	}
	if len(result.Troiki.Slots) != 3 {
		t.Fatalf("slots: got %d, want 3 (important/medium/rest)", len(result.Troiki.Slots))
	}
	imp := result.Troiki.Slots[0]
	if imp.Category != "important" {
		t.Errorf("first slot category: got %q, want important", imp.Category)
	}
	if imp.Projects != 1 {
		t.Errorf("important projects: got %d, want 1", imp.Projects)
	}
	if imp.Open != 1 {
		t.Errorf("important open: got %d, want 1", imp.Open)
	}
	if imp.Completed != 1 {
		t.Errorf("important completed: got %d, want 1", imp.Completed)
	}
	// Completing an Important task grants +1 Medium capacity.
	if med := result.Troiki.Slots[1]; med.Category != "medium" || med.Completed != 0 {
		t.Errorf("medium slot: got %+v, want category=medium completed=0", med)
	}
}

func TestTaskViews_WeekSummary_Counts(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")

	// One task completed this week.
	done := createTestTask(t, e, ctx.ID, "Done this week")
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/complete", done.ID), nil))
	if resp.StatusCode != 200 {
		t.Fatalf("complete: got %d; body: %s", resp.StatusCode, body)
	}

	// One open task planned for the week.
	planned := createTestTask(t, e, ctx.ID, "Planned for week")
	doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/plan", planned.ID),
		map[string]any{"state": "week"}))

	// One overdue open task (due in the past).
	doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/contexts/%d/tasks", ctx.ID),
		map[string]any{"title": "Overdue", "dueAt": "2020-01-01T00:00:00.000Z"}))

	resp2, body2 := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/stats/week-summary", nil))
	if resp2.StatusCode != 200 {
		t.Fatalf("week-summary: got %d; body: %s", resp2.StatusCode, body2)
	}
	var result weekSummaryResp
	if err := json.Unmarshal(body2, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Stats.CompletedCount != 1 {
		t.Errorf("completedCount: got %d, want 1", result.Stats.CompletedCount)
	}
	if len(result.Completed) != 1 || result.Completed[0].Title != "Done this week" {
		t.Errorf("completed: got %+v, want [Done this week]", result.Completed)
	}
	if result.Stats.PlannedOpen != 1 {
		t.Errorf("plannedOpen: got %d, want 1", result.Stats.PlannedOpen)
	}
	if result.Stats.Overdue != 1 {
		t.Errorf("overdue: got %d, want 1", result.Stats.Overdue)
	}
}

// TestTaskViews_Today_Empty hits the today bundle endpoint with an empty DB and
// expects all three sub-lists to be empty.
func TestTaskViews_Today_Empty(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/tasks/today", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result todayBundleResp
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Today.Total != 0 {
		t.Errorf("today total: got %d, want 0", result.Today.Total)
	}
	if result.Overdue.Total != 0 {
		t.Errorf("overdue total: got %d, want 0", result.Overdue.Total)
	}
	if result.CompletedToday.Total != 0 {
		t.Errorf("completedToday total: got %d, want 0", result.CompletedToday.Total)
	}
}

func TestTaskViews_Tomorrow_Empty(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/tasks/tomorrow", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.PagedResponse[dto.TaskDTO]
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("total: got %d, want 0", result.Total)
	}
}

func TestTaskViews_Overdue_Empty(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/tasks/overdue", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result dto.PagedResponse[dto.TaskDTO]
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("total: got %d, want 0", result.Total)
	}
}

func TestTaskViews_Overdue_HasTask(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")

	// Create a task due in the past.
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/contexts/%d/tasks", ctx.ID),
		map[string]any{"title": "Past task", "dueAt": "2020-01-01T00:00:00.000Z"}))
	if resp.StatusCode != 201 {
		t.Fatalf("create: got %d; body: %s", resp.StatusCode, body)
	}

	resp2, body2 := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/tasks/overdue", nil))
	if resp2.StatusCode != 200 {
		t.Fatalf("overdue: got %d; body: %s", resp2.StatusCode, body2)
	}
	var result dto.PagedResponse[dto.TaskDTO]
	if err := json.Unmarshal(body2, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("total: got %d, want 1", result.Total)
	}
	if len(result.Items) != 1 || result.Items[0].Title != "Past task" {
		t.Errorf("items: got %v", result.Items)
	}
}

func TestTaskViews_Week_Empty(t *testing.T) {
	e := setupAPIEnv(t)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/tasks/week", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result viewResp
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("total: got %d, want 0", result.Total)
	}
}

func TestTaskViews_Week_HasTask(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	task := createTestTask(t, e, ctx.ID, "Week task")

	// Plan the task to week.
	doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/plan", task.ID),
		map[string]any{"state": "week"}))

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/tasks/week", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result viewResp
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("total: got %d, want 1", result.Total)
	}
}

// Tasks with due_at in the current week but not planned must appear in /week,
// but the `total` (which drives the weekly limit badge) must count only
// plan_state='week' tasks.
func TestTaskViews_Week_IncludesDueInRange(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")

	// Compute Wednesday of the current week (UTC).
	now := time.Now().UTC()
	daysFromMonday := (int(now.Weekday()) + 6) % 7
	monday := time.Date(now.Year(), now.Month(), now.Day()-daysFromMonday, 0, 0, 0, 0, time.UTC)
	wed := monday.AddDate(0, 0, 2).Add(9 * time.Hour)

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/contexts/%d/tasks", ctx.ID),
		map[string]any{"title": "Unplanned due this week", "dueAt": wed.Format("2006-01-02T15:04:05.000Z")}))
	if resp.StatusCode != 201 {
		t.Fatalf("create: got %d; body: %s", resp.StatusCode, body)
	}

	resp2, body2 := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/tasks/week", nil))
	if resp2.StatusCode != 200 {
		t.Fatalf("week: got %d; body: %s", resp2.StatusCode, body2)
	}
	var result viewResp
	if err := json.Unmarshal(body2, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].Title != "Unplanned due this week" {
		t.Errorf("items: got %+v, want 1 unplanned due-this-week task", result.Items)
	}
	if result.Total != 0 {
		t.Errorf("total: got %d, want 0 (due-in-range tasks must not consume weekly limit)", result.Total)
	}
}

func TestTaskViews_Backlog_HasTask(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	task := createTestTask(t, e, ctx.ID, "Backlog task")

	doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/plan", task.ID),
		map[string]any{"state": "backlog"}))

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/tasks/backlog", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result viewResp
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("total: got %d, want 1", result.Total)
	}
}

func TestTaskViews_FilterByContext(t *testing.T) {
	e := setupAPIEnv(t)
	ctx1 := createTestContext(t, e, "Work")
	ctx2 := createTestContext(t, e, "Personal")

	// Create overdue task in ctx1.
	doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/contexts/%d/tasks", ctx1.ID),
		map[string]any{"title": "Work task", "dueAt": "2020-01-01T00:00:00.000Z"}))

	// Filter by ctx2 → 0 results.
	url := fmt.Sprintf("/api/v1/tasks/overdue?contextId=%d", ctx2.ID)
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, url, nil))
	if resp.StatusCode != 200 {
		t.Fatalf("got %d; body: %s", resp.StatusCode, body)
	}
	var result dto.PagedResponse[dto.TaskDTO]
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("total filtered by ctx2: got %d, want 0", result.Total)
	}
}

// Planning a parent for the week pulls its backlog subtasks along, so the backlog
// view must stop listing them as standalone rows. It also covers the PATCH path,
// which cascades the same way the plan endpoint does.
func TestTaskViews_Backlog_ExcludesSubtasksPlannedWithParent(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := createTestContext(t, e, "Work")
	parent := createTestTask(t, e, ctx.ID, "Parent")

	respS, bodyS := doReq(t, e.app, e.authedReq(t, http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%d/subtasks", parent.ID), map[string]any{"title": "Child"}))
	if respS.StatusCode != 201 {
		t.Fatalf("create subtask: got %d; body: %s", respS.StatusCode, bodyS)
	}
	var child dto.TaskDTO
	if err := json.Unmarshal(bodyS, &child); err != nil {
		t.Fatalf("parse subtask: %v", err)
	}
	for _, id := range []int64{parent.ID, child.ID} {
		resp, body := doReq(t, e.app, e.authedReq(t, http.MethodPost,
			fmt.Sprintf("/api/v1/tasks/%d/plan", id), map[string]any{"state": "backlog"}))
		if resp.StatusCode != 200 {
			t.Fatalf("plan backlog %d: got %d; body: %s", id, resp.StatusCode, body)
		}
	}

	respP, bodyP := doReq(t, e.app, e.authedReq(t, http.MethodPatch,
		fmt.Sprintf("/api/v1/tasks/%d", parent.ID), map[string]any{"planState": "week"}))
	if respP.StatusCode != 200 {
		t.Fatalf("patch planState: got %d; body: %s", respP.StatusCode, bodyP)
	}

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/tasks/backlog", nil))
	if resp.StatusCode != 200 {
		t.Fatalf("backlog: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result viewResp
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("backlog total: got %d, want 0; items: %+v", result.Total, result.Items)
	}

	respW, bodyW := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/tasks/week", nil))
	if respW.StatusCode != 200 {
		t.Fatalf("week: got %d, want 200; body: %s", respW.StatusCode, bodyW)
	}
	var week viewResp
	if err := json.Unmarshal(bodyW, &week); err != nil {
		t.Fatalf("parse week: %v", err)
	}
	if len(week.Items) != 2 {
		t.Errorf("week items: got %d, want parent + subtask; items: %+v", len(week.Items), week.Items)
	}
	if week.Total != 1 {
		t.Errorf("week total: got %d, want 1 (the cascaded subtask must not consume the weekly limit)", week.Total)
	}
}

// completedTitles fetches GET /tasks/completed with the given query and returns
// the titles it answered with, in order.
func completedTitles(t *testing.T, e *apiEnv, query string) []string {
	t.Helper()
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/tasks/completed"+query, nil))
	if resp.StatusCode != 200 {
		t.Fatalf("completed%s: got %d, want 200; body: %s", query, resp.StatusCode, body)
	}
	var result dto.PagedResponse[dto.TaskDTO]
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	titles := make([]string, 0, len(result.Items))
	for _, item := range result.Items {
		titles = append(titles, item.Title)
	}
	return titles
}

// A client whose local copy of the history is bounded reads everything past that
// bound from this endpoint, so `days` has to be able to name a window wider than
// the one such a copy covers. Anything smaller than that and the endpoint can
// only ever hand back what the caller already has.
func TestTaskViews_Completed_ReachesPastTheReplicatedHistory(t *testing.T) {
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")
	recent := createTestTask(t, e, c.ID, "Finished last week")
	ancient := createTestTask(t, e, c.ID, "Finished two years ago")
	now := time.Now()
	completeTaskAt(t, e, recent.ID, now.Add(-7*24*time.Hour))
	completeTaskAt(t, e, ancient.ID, now.Add(-730*24*time.Hour))

	got := completedTitles(t, e, "?days=3650&limit=50")
	if len(got) != 2 {
		t.Fatalf("days=3650: got %v, want both completions", got)
	}

	inWindow := completedTitles(t, e, "?days=90&limit=50")
	if len(inWindow) != 1 || inWindow[0] != recent.Title {
		t.Errorf("days=90: got %v, want only %q — a narrower window must still be honored", inWindow, recent.Title)
	}
}

// The window is measured by subtracting days from today, so an absurd number
// must be capped rather than run off the end of the clock.
func TestTaskViews_Completed_AbsurdWindowIsCapped(t *testing.T) {
	e := setupAPIEnv(t)
	c := createTestContext(t, e, "Work")
	task := createTestTask(t, e, c.ID, "Finished long ago")
	completeTaskAt(t, e, task.ID, time.Now().Add(-3650*24*time.Hour))

	got := completedTitles(t, e, "?days=999999999&limit=50")
	if len(got) != 1 || got[0] != task.Title {
		t.Errorf("days=999999999: got %v, want %q", got, task.Title)
	}
}

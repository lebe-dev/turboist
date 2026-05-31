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

package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

type labelStatsPeriodJSON struct {
	Applied         int `json:"applied"`
	PreviousApplied int `json:"previousApplied"`
	Completed       int `json:"completed"`
}

type labelStatsItemJSON struct {
	Label struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"label"`
	TotalTasks int                             `json:"totalTasks"`
	OpenTasks  int                             `json:"openTasks"`
	Overdue    int                             `json:"overdue"`
	Projects   int                             `json:"projects"`
	LastUsedAt *string                         `json:"lastUsedAt"`
	Periods    map[string]labelStatsPeriodJSON `json:"periods"`
}

type labelStatsJSON struct {
	Ranges map[string]struct {
		Start string `json:"start"`
		End   string `json:"end"`
		Days  int    `json:"days"`
	} `json:"ranges"`
	Items []labelStatsItemJSON `json:"items"`
}

// tagTaskAt attaches a label to a task and backdates the tagging event so it
// lands in a specific rolling window.
func tagTaskAt(t *testing.T, e *apiEnv, taskID, labelID int64, at time.Time) {
	t.Helper()
	if _, err := e.db.Exec(`INSERT INTO task_labels (task_id, label_id, created_at) VALUES (?, ?, ?)`,
		taskID, labelID, model.FormatUTC(at)); err != nil {
		t.Fatalf("tag task: %v", err)
	}
}

// getLabelStats GETs the usage report and decodes it, failing the test on
// anything but a 200.
func getLabelStats(t *testing.T, e *apiEnv) labelStatsJSON {
	t.Helper()
	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/labels/stats", nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /labels/stats: got %d, want 200; body: %s", resp.StatusCode, body)
	}
	var out labelStatsJSON
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("parse: %v", err)
	}
	return out
}

func TestLabelStats_PeriodsAndTotals(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := context.Background()
	now := time.Now()

	c, err := e.ctxs.Create(ctx, "work", "blue", false)
	if err != nil {
		t.Fatalf("create context: %v", err)
	}
	hot, err := e.labels.Create(ctx, "hot", "red", false)
	if err != nil {
		t.Fatalf("create label: %v", err)
	}
	idle, err := e.labels.Create(ctx, "idle", "grey", false)
	if err != nil {
		t.Fatalf("create label: %v", err)
	}

	// Two applications inside the week window, one only inside the quarter.
	for _, at := range []time.Time{now.Add(-2 * time.Hour), now.AddDate(0, 0, -2), now.AddDate(0, 0, -45)} {
		task, err := e.tasks.Create(ctx, repo.CreateTask{
			Placement: repo.Placement{ContextID: &c.ID},
			Title:     "t",
		})
		if err != nil {
			t.Fatalf("create task: %v", err)
		}
		tagTaskAt(t, e, task.ID, hot.ID, at)
	}

	got := getLabelStats(t, e)

	for key, days := range map[string]int{"week": 7, "month": 30, "quarter": 90} {
		rng, ok := got.Ranges[key]
		if !ok {
			t.Fatalf("ranges: %q missing", key)
		}
		if rng.Days != days {
			t.Errorf("range %s days: got %d, want %d", key, rng.Days, days)
		}
		if rng.Start == "" || rng.End == "" {
			t.Errorf("range %s: empty bounds %+v", key, rng)
		}
	}

	if len(got.Items) != 2 {
		t.Fatalf("items: got %d, want 2 (both labels, used or not)", len(got.Items))
	}

	var hotRow, idleRow labelStatsItemJSON
	for _, item := range got.Items {
		switch item.Label.ID {
		case hot.ID:
			hotRow = item
		case idle.ID:
			idleRow = item
		}
	}

	if hotRow.Periods["week"].Applied != 2 {
		t.Errorf("hot week applied: got %d, want 2", hotRow.Periods["week"].Applied)
	}
	if hotRow.Periods["month"].Applied != 2 {
		t.Errorf("hot month applied: got %d, want 2", hotRow.Periods["month"].Applied)
	}
	if hotRow.Periods["quarter"].Applied != 3 {
		t.Errorf("hot quarter applied: got %d, want 3", hotRow.Periods["quarter"].Applied)
	}
	if hotRow.TotalTasks != 3 || hotRow.OpenTasks != 3 {
		t.Errorf("hot totals: got total=%d open=%d, want 3/3", hotRow.TotalTasks, hotRow.OpenTasks)
	}
	if hotRow.LastUsedAt == nil {
		t.Error("hot lastUsedAt: got nil, want a timestamp")
	}

	if idleRow.Label.Name != "idle" {
		t.Fatalf("idle row missing: %+v", got.Items)
	}
	if idleRow.TotalTasks != 0 || idleRow.Periods["week"].Applied != 0 || idleRow.LastUsedAt != nil {
		t.Errorf("idle row: got %+v, want zeros and nil lastUsedAt", idleRow)
	}
}

// The stats route is static and must win over /labels/:id, which would otherwise
// answer with a 400 for the unparseable id "stats".
func TestLabelStats_RouteNotShadowedByID(t *testing.T) {
	e := setupAPIEnv(t)
	got := getLabelStats(t, e)
	if len(got.Items) != 0 {
		t.Errorf("items: got %d, want 0", len(got.Items))
	}
	if len(got.Ranges) != 3 {
		t.Errorf("ranges: got %d, want 3", len(got.Ranges))
	}
}

// The handler maps repo period slots onto the `week`/`month`/`quarter` keys by
// index; a single application 10 days back pins that mapping from every side —
// it is outside the week window, inside the week's *previous* window (days
// 8..14 back), and inside both longer windows.
func TestLabelStats_PeriodKeysMapToTheirOwnWindows(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := context.Background()

	c, err := e.ctxs.Create(ctx, "work", "blue", false)
	if err != nil {
		t.Fatalf("create context: %v", err)
	}
	label, err := e.labels.Create(ctx, "tenDaysAgo", "red", false)
	if err != nil {
		t.Fatalf("create label: %v", err)
	}
	task, err := e.tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &c.ID},
		Title:     "t",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	tagTaskAt(t, e, task.ID, label.ID, time.Now().AddDate(0, 0, -10))

	got := getLabelStats(t, e)
	if len(got.Items) != 1 {
		t.Fatalf("items: got %d, want 1", len(got.Items))
	}
	periods := got.Items[0].Periods

	want := map[string]labelStatsPeriodJSON{
		"week":    {Applied: 0, PreviousApplied: 1},
		"month":   {Applied: 1, PreviousApplied: 0},
		"quarter": {Applied: 1, PreviousApplied: 0},
	}
	for key, w := range want {
		if periods[key].Applied != w.Applied {
			t.Errorf("%s applied: got %d, want %d", key, periods[key].Applied, w.Applied)
		}
		if periods[key].PreviousApplied != w.PreviousApplied {
			t.Errorf("%s previousApplied: got %d, want %d", key, periods[key].PreviousApplied, w.PreviousApplied)
		}
	}
}

// Windows are anchored to the configured timezone and end at the *start of
// tomorrow* there, so today's activity counts in full instead of being cut off
// at the current clock time.
func TestLabelStats_RangesAnchoredToConfiguredTimezone(t *testing.T) {
	cfg := makeTestConfig()
	loc, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	cfg.Timezone = "Asia/Tokyo"
	cfg.Location = loc
	e := buildAPIEnvWithConfig(t, cfg)

	got := getLabelStats(t, e)

	now := time.Now().In(loc)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	wantEnd := model.FormatUTC(todayStart.AddDate(0, 0, 1).UTC())

	for key, days := range map[string]int{"week": 7, "month": 30, "quarter": 90} {
		rng := got.Ranges[key]
		if rng.End != wantEnd {
			t.Errorf("%s end: got %s, want %s (start of tomorrow in %s)", key, rng.End, wantEnd, cfg.Timezone)
		}
		wantStart := model.FormatUTC(todayStart.AddDate(0, 0, 1-days).UTC())
		if rng.Start != wantStart {
			t.Errorf("%s start: got %s, want %s", key, rng.Start, wantStart)
		}
	}
}

// Privacy filtering for the public view is a frontend concern (`isLabelVisible`),
// exactly like the sidebar's label list: the report itself carries every label so
// the owner's own numbers stay complete.
func TestLabelStats_IncludesPrivateLabels(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := context.Background()

	secret, err := e.labels.Create(ctx, "secret", "grey", false)
	if err != nil {
		t.Fatalf("create label: %v", err)
	}
	private := true
	if _, err := e.labels.Update(ctx, secret.ID, repo.LabelUpdate{IsPrivate: &private}); err != nil {
		t.Fatalf("mark private: %v", err)
	}

	got := getLabelStats(t, e)
	if len(got.Items) != 1 || got.Items[0].Label.Name != "secret" {
		t.Fatalf("items: got %+v, want the private label", got.Items)
	}
}

// A label attached to a task that is later deleted loses the tagging event with
// it (the FK cascades), so the report must not keep counting it.
func TestLabelStats_DeletedTaskDropsItsApplications(t *testing.T) {
	e := setupAPIEnv(t)
	ctx := context.Background()

	c, err := e.ctxs.Create(ctx, "work", "blue", false)
	if err != nil {
		t.Fatalf("create context: %v", err)
	}
	label, err := e.labels.Create(ctx, "temp", "red", false)
	if err != nil {
		t.Fatalf("create label: %v", err)
	}
	task, err := e.tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &c.ID},
		Title:     "doomed",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	tagTaskAt(t, e, task.ID, label.ID, time.Now())

	if got := getLabelStats(t, e); got.Items[0].Periods["week"].Applied != 1 {
		t.Fatalf("before delete: got %d applications, want 1", got.Items[0].Periods["week"].Applied)
	}

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodDelete, "/api/v1/tasks/"+strconv.FormatInt(task.ID, 10), nil))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete task: got %d, want 204; body: %s", resp.StatusCode, body)
	}

	got := getLabelStats(t, e)
	row := got.Items[0]
	if row.Periods["week"].Applied != 0 || row.TotalTasks != 0 || row.LastUsedAt != nil {
		t.Errorf("after delete: got applied=%d total=%d lastUsed=%v, want 0/0/nil",
			row.Periods["week"].Applied, row.TotalTasks, row.LastUsedAt)
	}
}

// A repo failure must come back as the standard 500 envelope, not a partial
// report the page would render as "no labels used".
func TestLabelStats_RepoFailureReturns500(t *testing.T) {
	e := setupAPIEnv(t)
	if _, err := e.db.Exec(`DROP TABLE task_labels`); err != nil {
		t.Fatalf("drop table: %v", err)
	}

	resp, body := doReq(t, e.app, e.authedReq(t, http.MethodGet, "/api/v1/labels/stats", nil))
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("got %d, want 500; body: %s", resp.StatusCode, body)
	}
	if er := parseErr(t, body); er.Error.Code == "" {
		t.Errorf("error envelope: got %+v, want a code", er.Error)
	}
}

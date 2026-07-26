package repo

import (
	"context"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

// setTagTime rewrites the tagging timestamp of a task↔label edge so a test can
// place an application inside or outside a rolling window.
func setTagTime(t *testing.T, f *taskFixture, taskID, labelID int64, at time.Time) {
	t.Helper()
	if _, err := f.db.Exec(`UPDATE task_labels SET created_at = ? WHERE task_id = ? AND label_id = ?`,
		model.FormatUTC(at), taskID, labelID); err != nil {
		t.Fatalf("set tag time: %v", err)
	}
}

// completeTaskAt marks a task completed at a given instant. The stats query
// only reads status + completed_at, so the repo update is enough — no need for
// the completion service and its recurrence handling.
func completeTaskAt(t *testing.T, f *taskFixture, taskID int64, at time.Time) {
	t.Helper()
	if _, err := f.tasks.Update(context.Background(), taskID, TaskUpdate{
		Status:      ptr(model.TaskStatusCompleted),
		CompletedAt: &at,
	}); err != nil {
		t.Fatalf("complete task: %v", err)
	}
}

func (f *taskFixture) newTask(t *testing.T, title string) *model.Task {
	t.Helper()
	task, err := f.tasks.Create(context.Background(), CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     title,
	})
	if err != nil {
		t.Fatalf("create task %s: %v", title, err)
	}
	return task
}

// Indices into the ranges built by labelUsageRanges, mirroring the production
// period order (see handlers.labelStatsPeriods).
const (
	weekIdx = iota
	monthIdx
	quarterIdx
)

// labelUsageRanges builds the same [week, month, quarter] windows the handler
// passes in — 7 / 30 / 90 days back — ending an hour ahead of `now` so a "right
// now" tagging event is inside them.
func labelUsageRanges(now time.Time) []LabelUsageRange {
	end := now.Add(time.Hour)
	return []LabelUsageRange{
		{Start: end.AddDate(0, 0, -7), End: end},
		{Start: end.AddDate(0, 0, -30), End: end},
		{Start: end.AddDate(0, 0, -90), End: end},
	}
}

func findUsage(t *testing.T, rows []LabelUsage, name string) LabelUsage {
	t.Helper()
	for _, r := range rows {
		if r.Label.Name == name {
			return r
		}
	}
	t.Fatalf("label %q missing from usage stats", name)
	return LabelUsage{}
}

func TestLabelRepo_UsageStats_AppliedPerPeriod(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	now := time.Now()

	hot, _ := f.labels.Create(ctx, "hot", "red", false)
	cold, _ := f.labels.Create(ctx, "cold", "blue", false)
	_, _ = f.labels.Create(ctx, "unused", "grey", false)

	// hot: two applications this week, one three weeks ago (month window only).
	for i, at := range []time.Time{now.AddDate(0, 0, -1), now.AddDate(0, 0, -3), now.AddDate(0, 0, -21)} {
		task := f.newTask(t, "hot-"+string(rune('a'+i)))
		if err := f.tlabels.SetForTask(ctx, task.ID, []int64{hot.ID}); err != nil {
			t.Fatalf("tag: %v", err)
		}
		setTagTime(t, f, task.ID, hot.ID, at)
	}

	// cold: a single application 45 days ago — outside both windows, but inside
	// the month window's comparison bucket (days 31..60 back).
	coldTask := f.newTask(t, "cold-a")
	if err := f.tlabels.SetForTask(ctx, coldTask.ID, []int64{cold.ID}); err != nil {
		t.Fatalf("tag: %v", err)
	}
	setTagTime(t, f, coldTask.ID, cold.ID, now.AddDate(0, 0, -45))

	rows, err := f.labels.UsageStats(ctx, labelUsageRanges(now), now, 100)
	if err != nil {
		t.Fatalf("usage stats: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows: got %d, want 3 (every label, used or not)", len(rows))
	}

	hotRow := findUsage(t, rows, "hot")
	if got := hotRow.Periods[weekIdx].Applied; got != 2 {
		t.Errorf("hot week applied: got %d, want 2", got)
	}
	if got := hotRow.Periods[monthIdx].Applied; got != 3 {
		t.Errorf("hot month applied: got %d, want 3", got)
	}
	// The week's previous window is days 8..14 back — the 21-day-old tag is not in it.
	if got := hotRow.Periods[weekIdx].PreviousApplied; got != 0 {
		t.Errorf("hot previous week applied: got %d, want 0", got)
	}
	// The month's previous window is days 31..60 back — empty for hot.
	if got := hotRow.Periods[monthIdx].PreviousApplied; got != 0 {
		t.Errorf("hot previous month applied: got %d, want 0", got)
	}
	if hotRow.TotalTasks != 3 {
		t.Errorf("hot total tasks: got %d, want 3", hotRow.TotalTasks)
	}
	if hotRow.LastUsedAt == nil {
		t.Fatal("hot last used at: got nil, want the most recent application")
	}

	coldRow := findUsage(t, rows, "cold")
	if got := coldRow.Periods[weekIdx].Applied; got != 0 {
		t.Errorf("cold week applied: got %d, want 0", got)
	}
	if got := coldRow.Periods[monthIdx].Applied; got != 0 {
		t.Errorf("cold month applied: got %d, want 0", got)
	}
	// 45 days back lands in the month window's previous bucket (days 31..60).
	if got := coldRow.Periods[monthIdx].PreviousApplied; got != 1 {
		t.Errorf("cold previous month applied: got %d, want 1", got)
	}
	if coldRow.TotalTasks != 1 {
		t.Errorf("cold total tasks: got %d, want 1", coldRow.TotalTasks)
	}

	unusedRow := findUsage(t, rows, "unused")
	if unusedRow.TotalTasks != 0 || unusedRow.LastUsedAt != nil {
		t.Errorf("unused label: got total=%d lastUsed=%v, want 0/nil", unusedRow.TotalTasks, unusedRow.LastUsedAt)
	}
}

func TestLabelRepo_UsageStats_OpenCompletedOverdue(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	label, _ := f.labels.Create(ctx, "work", "blue", false)

	open := f.newTask(t, "open")
	overdue := f.newTask(t, "overdue")
	done := f.newTask(t, "done")
	for _, task := range []*model.Task{open, overdue, done} {
		if err := f.tlabels.SetForTask(ctx, task.ID, []int64{label.ID}); err != nil {
			t.Fatalf("tag: %v", err)
		}
		setTagTime(t, f, task.ID, label.ID, now.AddDate(0, 0, -2))
	}

	due := todayStart.AddDate(0, 0, -3)
	if _, err := f.tasks.Update(ctx, overdue.ID, TaskUpdate{DueAt: &due}); err != nil {
		t.Fatalf("set due: %v", err)
	}
	completeTaskAt(t, f, done.ID, now.AddDate(0, 0, -1))

	rows, err := f.labels.UsageStats(ctx, labelUsageRanges(now), todayStart, 100)
	if err != nil {
		t.Fatalf("usage stats: %v", err)
	}
	row := findUsage(t, rows, "work")

	if row.OpenTasks != 2 {
		t.Errorf("open tasks: got %d, want 2", row.OpenTasks)
	}
	if row.Overdue != 1 {
		t.Errorf("overdue: got %d, want 1", row.Overdue)
	}
	if got := row.Periods[weekIdx].Completed; got != 1 {
		t.Errorf("week completed: got %d, want 1", got)
	}
	if row.TotalTasks != 3 {
		t.Errorf("total tasks: got %d, want 3", row.TotalTasks)
	}
}

func TestLabelRepo_UsageStats_ProjectSpread(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	now := time.Now()

	label, _ := f.labels.Create(ctx, "spread", "green", false)
	second, err := f.projects.Create(ctx, CreateProject{ContextID: f.contextID, Title: "beta", Color: "red"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	for _, pid := range []int64{f.projectID, f.projectID, second.ID} {
		task, err := f.tasks.Create(ctx, CreateTask{
			Placement: Placement{ContextID: &f.contextID, ProjectID: &pid},
			Title:     "t",
		})
		if err != nil {
			t.Fatalf("create task: %v", err)
		}
		if err := f.tlabels.SetForTask(ctx, task.ID, []int64{label.ID}); err != nil {
			t.Fatalf("tag: %v", err)
		}
	}
	// One inbox task — NULL project_id must not inflate the spread.
	inboxID := int64(1)
	inboxTask, err := f.tasks.Create(ctx, CreateTask{Placement: Placement{InboxID: &inboxID}, Title: "inbox"})
	if err != nil {
		t.Fatalf("create inbox task: %v", err)
	}
	if err := f.tlabels.SetForTask(ctx, inboxTask.ID, []int64{label.ID}); err != nil {
		t.Fatalf("tag: %v", err)
	}

	rows, err := f.labels.UsageStats(ctx, labelUsageRanges(now), now, 100)
	if err != nil {
		t.Fatalf("usage stats: %v", err)
	}
	row := findUsage(t, rows, "spread")
	if row.Projects != 2 {
		t.Errorf("projects: got %d, want 2", row.Projects)
	}
	if row.TotalTasks != 4 {
		t.Errorf("total tasks: got %d, want 4", row.TotalTasks)
	}
}

// Re-saving a task's label set must not reset the tagging time of labels that
// were already attached — otherwise every edit would move all of the task's
// applications into the current week and the stats would drift.
func TestTaskLabelsRepo_SetForTask_PreservesExistingTagTime(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()

	keep, _ := f.labels.Create(ctx, "keep", "blue", false)
	added, _ := f.labels.Create(ctx, "added", "red", false)
	task := f.newTask(t, "t")

	if err := f.tlabels.SetForTask(ctx, task.ID, []int64{keep.ID}); err != nil {
		t.Fatalf("first set: %v", err)
	}
	old := time.Now().AddDate(0, 0, -40)
	setTagTime(t, f, task.ID, keep.ID, old)

	if err := f.tlabels.SetForTask(ctx, task.ID, []int64{keep.ID, added.ID}); err != nil {
		t.Fatalf("second set: %v", err)
	}

	var keepAt, addedAt string
	if err := f.db.QueryRow(`SELECT created_at FROM task_labels WHERE task_id = ? AND label_id = ?`,
		task.ID, keep.ID).Scan(&keepAt); err != nil {
		t.Fatalf("read keep: %v", err)
	}
	if err := f.db.QueryRow(`SELECT created_at FROM task_labels WHERE task_id = ? AND label_id = ?`,
		task.ID, added.ID).Scan(&addedAt); err != nil {
		t.Fatalf("read added: %v", err)
	}
	if keepAt != model.FormatUTC(old) {
		t.Errorf("kept label tag time: got %s, want %s", keepAt, model.FormatUTC(old))
	}
	if addedAt <= keepAt {
		t.Errorf("added label tag time: got %s, want later than %s", addedAt, keepAt)
	}
}

// A row written before migration 047 (or restored from a pre-047 backup whose
// task is missing) can carry a NULL created_at. It must count in the all-time
// totals but land in no window, and must not become a bogus LastUsedAt.
func TestLabelRepo_UsageStats_NullTagTimeCountsOnlyInTotals(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	now := time.Now()

	label, _ := f.labels.Create(ctx, "legacy", "grey", false)
	task := f.newTask(t, "legacy-task")
	if err := f.tlabels.SetForTask(ctx, task.ID, []int64{label.ID}); err != nil {
		t.Fatalf("tag: %v", err)
	}
	if _, err := f.db.Exec(`UPDATE task_labels SET created_at = NULL WHERE task_id = ?`, task.ID); err != nil {
		t.Fatalf("null out tag time: %v", err)
	}

	rows, err := f.labels.UsageStats(ctx, labelUsageRanges(now), now, 100)
	if err != nil {
		t.Fatalf("usage stats: %v", err)
	}
	row := findUsage(t, rows, "legacy")

	if row.TotalTasks != 1 || row.OpenTasks != 1 {
		t.Errorf("totals: got total=%d open=%d, want 1/1", row.TotalTasks, row.OpenTasks)
	}
	for i, p := range row.Periods {
		if p.Applied != 0 || p.PreviousApplied != 0 {
			t.Errorf("period %d: got applied=%d previous=%d, want 0/0", i, p.Applied, p.PreviousApplied)
		}
	}
	if row.LastUsedAt != nil {
		t.Errorf("last used at: got %v, want nil", row.LastUsedAt)
	}
}

// A cancelled task is neither open (it needs no attention) nor completed (no work
// was finished), but it still happened — so it stays in the all-time total and
// its tagging event still counts as an application.
func TestLabelRepo_UsageStats_CancelledTaskIsNeitherOpenNorCompleted(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	now := time.Now()

	label, _ := f.labels.Create(ctx, "dropped", "brown", false)
	task := f.newTask(t, "cancelled")
	if err := f.tlabels.SetForTask(ctx, task.ID, []int64{label.ID}); err != nil {
		t.Fatalf("tag: %v", err)
	}
	setTagTime(t, f, task.ID, label.ID, now.AddDate(0, 0, -2))
	cancelledAt := now.AddDate(0, 0, -1)
	if _, err := f.tasks.Update(ctx, task.ID, TaskUpdate{
		Status:      ptr(model.TaskStatusCancelled),
		CompletedAt: &cancelledAt,
	}); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	rows, err := f.labels.UsageStats(ctx, labelUsageRanges(now), now, 100)
	if err != nil {
		t.Fatalf("usage stats: %v", err)
	}
	row := findUsage(t, rows, "dropped")

	if row.TotalTasks != 1 {
		t.Errorf("total tasks: got %d, want 1", row.TotalTasks)
	}
	if row.OpenTasks != 0 {
		t.Errorf("open tasks: got %d, want 0", row.OpenTasks)
	}
	if got := row.Periods[weekIdx].Applied; got != 1 {
		t.Errorf("week applied: got %d, want 1", got)
	}
	if got := row.Periods[weekIdx].Completed; got != 0 {
		t.Errorf("week completed: got %d, want 0 (cancelled is not completed)", got)
	}
}

// Completions are bucketed by completed_at, independently of when the label was
// attached: a task tagged 100 days ago but finished yesterday counts as this
// week's completion and as nobody's application.
func TestLabelRepo_UsageStats_CompletionBucketedIndependentlyOfTagging(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	now := time.Now()

	label, _ := f.labels.Create(ctx, "longhaul", "teal", false)
	task := f.newTask(t, "old-task")
	if err := f.tlabels.SetForTask(ctx, task.ID, []int64{label.ID}); err != nil {
		t.Fatalf("tag: %v", err)
	}
	setTagTime(t, f, task.ID, label.ID, now.AddDate(0, 0, -100))
	completeTaskAt(t, f, task.ID, now.AddDate(0, 0, -1))

	rows, err := f.labels.UsageStats(ctx, labelUsageRanges(now), now, 100)
	if err != nil {
		t.Fatalf("usage stats: %v", err)
	}
	row := findUsage(t, rows, "longhaul")

	for i, want := range map[int]int{weekIdx: 1, monthIdx: 1, quarterIdx: 1} {
		if got := row.Periods[i].Completed; got != want {
			t.Errorf("period %d completed: got %d, want %d", i, got, want)
		}
		if got := row.Periods[i].Applied; got != 0 {
			t.Errorf("period %d applied: got %d, want 0 (tagged 100 days ago)", i, got)
		}
	}
}

// Completions outside a window do not leak into it.
func TestLabelRepo_UsageStats_CompletionOutsideWindowExcluded(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	now := time.Now()

	label, _ := f.labels.Create(ctx, "stale", "pink", false)
	task := f.newTask(t, "finished-long-ago")
	if err := f.tlabels.SetForTask(ctx, task.ID, []int64{label.ID}); err != nil {
		t.Fatalf("tag: %v", err)
	}
	setTagTime(t, f, task.ID, label.ID, now.AddDate(0, 0, -50))
	completeTaskAt(t, f, task.ID, now.AddDate(0, 0, -40))

	rows, err := f.labels.UsageStats(ctx, labelUsageRanges(now), now, 100)
	if err != nil {
		t.Fatalf("usage stats: %v", err)
	}
	row := findUsage(t, rows, "stale")

	if got := row.Periods[weekIdx].Completed; got != 0 {
		t.Errorf("week completed: got %d, want 0", got)
	}
	if got := row.Periods[monthIdx].Completed; got != 0 {
		t.Errorf("month completed: got %d, want 0 (finished 40 days ago)", got)
	}
	if got := row.Periods[quarterIdx].Completed; got != 1 {
		t.Errorf("quarter completed: got %d, want 1", got)
	}
	if got := row.Periods[quarterIdx].Applied; got != 1 {
		t.Errorf("quarter applied: got %d, want 1", got)
	}
}

func TestLabelRepo_UsageStats_LastUsedAtIsMostRecentApplication(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	now := time.Now()

	label, _ := f.labels.Create(ctx, "recent", "orange", false)
	newest := now.AddDate(0, 0, -2).Truncate(time.Millisecond)
	for _, at := range []time.Time{now.AddDate(0, 0, -30), newest, now.AddDate(0, 0, -9)} {
		task := f.newTask(t, "t")
		if err := f.tlabels.SetForTask(ctx, task.ID, []int64{label.ID}); err != nil {
			t.Fatalf("tag: %v", err)
		}
		setTagTime(t, f, task.ID, label.ID, at)
	}

	rows, err := f.labels.UsageStats(ctx, labelUsageRanges(now), now, 100)
	if err != nil {
		t.Fatalf("usage stats: %v", err)
	}
	row := findUsage(t, rows, "recent")
	if row.LastUsedAt == nil {
		t.Fatal("last used at: got nil, want the newest application")
	}
	if got, want := model.FormatUTC(*row.LastUsedAt), model.FormatUTC(newest); got != want {
		t.Errorf("last used at: got %s, want %s", got, want)
	}
}

// The row cap keeps a pathological label set from blowing up the response; rows
// are ordered by name, so the cap is deterministic.
func TestLabelRepo_UsageStats_LimitCapsRows(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()

	for _, name := range []string{"c-third", "a-first", "b-second"} {
		if _, err := f.labels.Create(ctx, name, "blue", false); err != nil {
			t.Fatalf("create label %s: %v", name, err)
		}
	}

	rows, err := f.labels.UsageStats(ctx, labelUsageRanges(time.Now()), time.Now(), 2)
	if err != nil {
		t.Fatalf("usage stats: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows: got %d, want 2", len(rows))
	}
	if rows[0].Label.Name != "a-first" || rows[1].Label.Name != "b-second" {
		t.Errorf("order: got %s, %s; want a-first, b-second", rows[0].Label.Name, rows[1].Label.Name)
	}
}

func TestLabelRepo_UsageStats_NoLabels(t *testing.T) {
	f := newTaskFixture(t)
	rows, err := f.labels.UsageStats(context.Background(), labelUsageRanges(time.Now()), time.Now(), 100)
	if err != nil {
		t.Fatalf("usage stats: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("rows: got %d, want 0", len(rows))
	}
}

func TestTaskLabelsRepo_SetForTask_StampsNewRowsAndClearsRemoved(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()

	first, _ := f.labels.Create(ctx, "first", "blue", false)
	second, _ := f.labels.Create(ctx, "second", "red", false)
	task := f.newTask(t, "t")

	if err := f.tlabels.SetForTask(ctx, task.ID, []int64{first.ID, second.ID}); err != nil {
		t.Fatalf("set: %v", err)
	}
	var stamped int
	if err := f.db.QueryRow(
		`SELECT COUNT(*) FROM task_labels WHERE task_id = ? AND created_at IS NOT NULL`,
		task.ID).Scan(&stamped); err != nil {
		t.Fatalf("count stamped: %v", err)
	}
	if stamped != 2 {
		t.Errorf("stamped rows: got %d, want 2", stamped)
	}

	// Dropping one label removes only its row.
	if err := f.tlabels.SetForTask(ctx, task.ID, []int64{second.ID}); err != nil {
		t.Fatalf("re-set: %v", err)
	}
	var remaining int64
	if err := f.db.QueryRow(`SELECT label_id FROM task_labels WHERE task_id = ?`, task.ID).
		Scan(&remaining); err != nil {
		t.Fatalf("read remaining: %v", err)
	}
	if remaining != second.ID {
		t.Errorf("remaining label: got %d, want %d", remaining, second.ID)
	}

	// An empty set clears everything.
	if err := f.tlabels.SetForTask(ctx, task.ID, nil); err != nil {
		t.Fatalf("clear: %v", err)
	}
	var left int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM task_labels WHERE task_id = ?`, task.ID).Scan(&left); err != nil {
		t.Fatalf("count left: %v", err)
	}
	if left != 0 {
		t.Errorf("rows after clear: got %d, want 0", left)
	}
}

// Deleting a label must take its stats row with it — the FK cascade is what keeps
// the usage report from resurrecting deleted labels.
func TestLabelRepo_UsageStats_DeletedLabelDisappears(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	now := time.Now()

	label, _ := f.labels.Create(ctx, "doomed", "red", false)
	task := f.newTask(t, "t")
	if err := f.tlabels.SetForTask(ctx, task.ID, []int64{label.ID}); err != nil {
		t.Fatalf("tag: %v", err)
	}
	if err := f.labels.Delete(ctx, label.ID); err != nil {
		t.Fatalf("delete label: %v", err)
	}

	rows, err := f.labels.UsageStats(ctx, labelUsageRanges(now), now, 100)
	if err != nil {
		t.Fatalf("usage stats: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("rows: got %d, want 0", len(rows))
	}
	var orphans int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM task_labels WHERE label_id = ?`, label.ID).Scan(&orphans); err != nil {
		t.Fatalf("count orphans: %v", err)
	}
	if orphans != 0 {
		t.Errorf("orphan task_labels rows: got %d, want 0", orphans)
	}
}

// Corrupt timestamps must surface as an error rather than a silently zeroed row:
// the report is only meaningful if its dates parse.
func TestLabelRepo_UsageStats_RejectsUnparsableTimestamps(t *testing.T) {
	cases := []struct {
		name  string
		table string
		col   string
	}{
		{"label created_at", "labels", "created_at"},
		{"label updated_at", "labels", "updated_at"},
		{"tag created_at", "task_labels", "created_at"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newTaskFixture(t)
			ctx := context.Background()
			label, _ := f.labels.Create(ctx, "broken", "red", false)
			task := f.newTask(t, "t")
			if err := f.tlabels.SetForTask(ctx, task.ID, []int64{label.ID}); err != nil {
				t.Fatalf("tag: %v", err)
			}
			if _, err := f.db.Exec(`UPDATE ` + tc.table + ` SET ` + tc.col + ` = 'not-a-timestamp'`); err != nil {
				t.Fatalf("corrupt %s.%s: %v", tc.table, tc.col, err)
			}

			if _, err := f.labels.UsageStats(ctx, labelUsageRanges(time.Now()), time.Now(), 100); err == nil {
				t.Errorf("usage stats: got nil error, want a parse failure for %s.%s", tc.table, tc.col)
			}
		})
	}
}

// A missing join table is the cheapest stand-in for "the query blew up"; the repo
// must report it instead of returning a half-built report.
func TestLabelRepo_UsageStats_ReturnsQueryError(t *testing.T) {
	f := newTaskFixture(t)
	if _, err := f.db.Exec(`DROP TABLE task_labels`); err != nil {
		t.Fatalf("drop table: %v", err)
	}
	if _, err := f.labels.UsageStats(context.Background(), labelUsageRanges(time.Now()), time.Now(), 100); err == nil {
		t.Error("usage stats: got nil error, want a query failure")
	}
}

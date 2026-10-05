package repo

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

// snapshotFixture seeds one context plus the repos a snapshot test needs.
func snapshotFixture(t *testing.T) (*sql.DB, *ChangeLogRepo, *TaskRepo, int64) {
	t.Helper()
	d := setupTestDB(t)
	ctx := context.Background()
	c, err := NewContextRepo(d).Create(ctx, "Work", "blue", false)
	if err != nil {
		t.Fatalf("create context: %v", err)
	}
	tasks := NewTaskRepo(d, NewTaskLabelsRepo(d), NewTaskRelationsRepo(d))
	return d, NewChangeLogRepo(d), tasks, c.ID
}

func newSnapshotTask(t *testing.T, tasks *TaskRepo, contextID int64, title string) *model.Task {
	t.Helper()
	task, err := tasks.Create(context.Background(), CreateTask{
		Placement: Placement{ContextID: &contextID},
		Title:     title,
	})
	if err != nil {
		t.Fatalf("create task %q: %v", title, err)
	}
	return task
}

// completeAt marks a task completed at a chosen moment, which is how the window
// tests place a task on either side of the cutoff without waiting 90 days.
func completeAt(t *testing.T, tasks *TaskRepo, id int64, at time.Time) {
	t.Helper()
	status := model.TaskStatusCompleted
	if _, err := tasks.Update(context.Background(), id, TaskUpdate{Status: &status, CompletedAt: &at}); err != nil {
		t.Fatalf("complete task %d: %v", id, err)
	}
}

func snapshotTaskIDs(snap *SyncSnapshot) map[int64]bool {
	ids := make(map[int64]bool, len(snap.Tasks))
	for _, task := range snap.Tasks {
		ids[task.ID] = true
	}
	return ids
}

// The window is measured back from the caller's clock, so a task completed
// exactly on the boundary is still carried — a boundary that excluded its own
// edge would drop a day of history every time the cutoff was recomputed.
func TestChangeLogSnapshot_CompletedWindowBoundary(t *testing.T) {
	_, log, tasks, contextID := snapshotFixture(t)
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	cutoff := now.Add(-SyncHistoryWindow)

	onBoundary := newSnapshotTask(t, tasks, contextID, "Completed on the cutoff")
	justInside := newSnapshotTask(t, tasks, contextID, "Completed a day after the cutoff")
	justOutside := newSnapshotTask(t, tasks, contextID, "Completed a millisecond before the cutoff")
	open := newSnapshotTask(t, tasks, contextID, "Still open")

	completeAt(t, tasks, onBoundary.ID, cutoff)
	completeAt(t, tasks, justInside.ID, cutoff.Add(24*time.Hour))
	completeAt(t, tasks, justOutside.ID, cutoff.Add(-time.Millisecond))

	snap, err := log.Snapshot(context.Background(), SyncSnapshotQuery{UserID: 1, Now: now})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	ids := snapshotTaskIDs(snap)

	for _, want := range []*model.Task{onBoundary, justInside, open} {
		if !ids[want.ID] {
			t.Errorf("task %q missing from the snapshot", want.Title)
		}
	}
	if ids[justOutside.ID] {
		t.Errorf("task %q is outside the window but was carried", justOutside.Title)
	}
	if got, want := snap.CompletedSince.UTC(), cutoff.UTC(); !got.Equal(want) {
		t.Errorf("completedSince: got %v, want %v", got, want)
	}
}

// A cancelled task never gets a completion time, so it is history the window
// cannot age out. It stays in the snapshot, which is what the replica needs: a
// cancelled blocker releases its dependents, and a client that cannot see it
// would keep the padlock on forever.
func TestChangeLogSnapshot_KeepsCancelledTasks(t *testing.T) {
	_, log, tasks, contextID := snapshotFixture(t)
	cancelled := newSnapshotTask(t, tasks, contextID, "Abandoned")
	status := model.TaskStatusCancelled
	if _, err := tasks.Update(context.Background(), cancelled.ID, TaskUpdate{Status: &status}); err != nil {
		t.Fatalf("cancel task: %v", err)
	}

	snap, err := log.Snapshot(context.Background(), SyncSnapshotQuery{UserID: 1})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if !snapshotTaskIDs(snap)[cancelled.ID] {
		t.Error("cancelled task missing from the snapshot")
	}
}

// An open subtask hanging off a long-completed parent drags the parent in with
// it. Without that the replica would hold a task whose parent_id names a row it
// has never seen, and would draw it as a detached top-level task.
func TestChangeLogSnapshot_KeepsAncestorsOfWindowedTasks(t *testing.T) {
	_, log, tasks, contextID := snapshotFixture(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	ancient := now.Add(-SyncHistoryWindow).Add(-365 * 24 * time.Hour)

	grandparent := newSnapshotTask(t, tasks, contextID, "Grandparent")
	parent, err := tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &contextID, ParentID: &grandparent.ID},
		Title:     "Parent",
	})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	child, err := tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &contextID, ParentID: &parent.ID},
		Title:     "Open child",
	})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	completeAt(t, tasks, parent.ID, ancient)
	completeAt(t, tasks, grandparent.ID, ancient)

	snap, err := log.Snapshot(ctx, SyncSnapshotQuery{UserID: 1, Now: now})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	ids := snapshotTaskIDs(snap)
	for _, want := range []*model.Task{child, parent, grandparent} {
		if !ids[want.ID] {
			t.Errorf("task %q missing from the snapshot", want.Title)
		}
	}
}

// An edge is only meaningful when the replica holds both of its ends, so an
// edge reaching out of the window is left behind rather than handed over as a
// dangling reference.
func TestChangeLogSnapshot_DropsRelationsReachingOutOfTheWindow(t *testing.T) {
	d, log, tasks, contextID := snapshotFixture(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	ancient := now.Add(-SyncHistoryWindow).Add(-365 * 24 * time.Hour)

	relations := NewTaskRelationsRepo(d)
	inside := newSnapshotTask(t, tasks, contextID, "Inside the window")
	partner := newSnapshotTask(t, tasks, contextID, "Also inside")
	outside := newSnapshotTask(t, tasks, contextID, "Long completed")

	kept, err := relations.Create(ctx, inside.ID, partner.ID, model.RelationTypeRelated)
	if err != nil {
		t.Fatalf("create kept relation: %v", err)
	}
	dropped, err := relations.Create(ctx, inside.ID, outside.ID, model.RelationTypeRelated)
	if err != nil {
		t.Fatalf("create dropped relation: %v", err)
	}
	completeAt(t, tasks, outside.ID, ancient)

	snap, err := log.Snapshot(ctx, SyncSnapshotQuery{UserID: 1, Now: now})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	seen := map[int64]bool{}
	for _, rel := range snap.TaskRelations {
		seen[rel.ID] = true
	}
	if !seen[kept.ID] {
		t.Error("relation between two in-window tasks is missing")
	}
	if seen[dropped.ID] {
		t.Error("relation pointing outside the window was carried")
	}
}

func TestChangeLogSnapshot_CursorIsTheLatestLoggedChange(t *testing.T) {
	d, log, tasks, contextID := snapshotFixture(t)
	newSnapshotTask(t, tasks, contextID, "Something to log")

	snap, err := log.Snapshot(context.Background(), SyncSnapshotQuery{UserID: 1})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if want := maxChangeSeq(t, d); snap.Cursor != want {
		t.Errorf("cursor: got %d, want %d", snap.Cursor, want)
	}
}

// A snapshot taken against a log that pruning has emptied must still hand back a
// cursor the delta feed accepts. Reporting 0 would be read as "I have seen
// nothing", and the very next call would be refused as expired.
func TestChangeLogSnapshot_CursorSurvivesAnEmptiedLog(t *testing.T) {
	d, log, tasks, contextID := snapshotFixture(t)
	ctx := context.Background()
	newSnapshotTask(t, tasks, contextID, "Logged then pruned")
	head := maxChangeSeq(t, d)
	if _, err := d.Exec(`DELETE FROM change_log`); err != nil {
		t.Fatalf("prune log: %v", err)
	}

	snap, err := log.Snapshot(ctx, SyncSnapshotQuery{UserID: 1})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snap.Cursor != head {
		t.Errorf("cursor: got %d, want %d", snap.Cursor, head)
	}
	if _, err := log.Changes(ctx, SyncChangesQuery{Since: snap.Cursor}); err != nil {
		t.Errorf("changes after an emptied log: %v", err)
	}
}

// Nothing has ever been written on a fresh database, so the cursor starts at
// zero and the first delta call asks for everything from the beginning.
func TestChangeLogSnapshot_FreshDatabaseStartsAtZero(t *testing.T) {
	d := setupTestDB(t)
	snap, err := NewChangeLogRepo(d).Snapshot(context.Background(), SyncSnapshotQuery{UserID: 1})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snap.Cursor != 0 {
		t.Errorf("cursor: got %d, want 0", snap.Cursor)
	}
	if snap.Epoch != 1 {
		t.Errorf("epoch: got %d, want 1", snap.Epoch)
	}
	if snap.UserSettings == nil {
		t.Error("user settings are nil on a fresh database")
	}
	if snap.AppSettings == nil {
		t.Error("app settings are nil on a fresh database")
	}
	if snap.UserState == "" {
		t.Error("user state is empty rather than an empty JSON object")
	}
}

func TestChangeLogSnapshot_CarriesEveryEntityKind(t *testing.T) {
	d, log, tasks, contextID := snapshotFixture(t)
	ctx := context.Background()

	project, err := NewProjectRepo(d, NewProjectLabelsRepo(d)).Create(ctx, CreateProject{ContextID: contextID, Title: "Website", Color: "blue"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := NewProjectSectionRepo(d).Create(ctx, project.ID, "Backlog"); err != nil {
		t.Fatalf("create section: %v", err)
	}
	if _, err := NewLabelRepo(d).Create(ctx, "urgent", "red", false); err != nil {
		t.Fatalf("create label: %v", err)
	}
	if _, err := NewTemplateRepo(d).Create(ctx, TemplateInput{Name: "Weekly review"}); err != nil {
		t.Fatalf("create template: %v", err)
	}
	first := newSnapshotTask(t, tasks, contextID, "First")
	second := newSnapshotTask(t, tasks, contextID, "Second")
	if _, err := NewTaskRelationsRepo(d).Create(ctx, first.ID, second.ID, model.RelationTypeRelated); err != nil {
		t.Fatalf("create relation: %v", err)
	}

	snap, err := log.Snapshot(ctx, SyncSnapshotQuery{UserID: 1})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	counts := map[string]int{
		"tasks":          len(snap.Tasks),
		"projects":       len(snap.Projects),
		"sections":       len(snap.Sections),
		"contexts":       len(snap.Contexts),
		"labels":         len(snap.Labels),
		"task relations": len(snap.TaskRelations),
		"task templates": len(snap.TaskTemplates),
	}
	for name, got := range counts {
		if got == 0 {
			t.Errorf("%s: got 0 rows, want at least one", name)
		}
	}
}

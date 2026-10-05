package repo

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

type inboxProcFixture struct {
	*taskFixture
	proc *InboxProcessingRepo
}

func newInboxProcFixture(t *testing.T) *inboxProcFixture {
	t.Helper()
	f := newTaskFixture(t)
	return &inboxProcFixture{taskFixture: f, proc: NewInboxProcessingRepo(f.db, f.tlabels)}
}

// inboxTask creates an Inbox task and pins its created_at so ordering is exact.
func (f *inboxProcFixture) inboxTask(t *testing.T, title string, createdAt time.Time) *model.Task {
	t.Helper()
	inboxID := int64(1)
	task, err := f.tasks.Create(context.Background(), CreateTask{Placement: Placement{InboxID: &inboxID}, Title: title})
	if err != nil {
		t.Fatalf("create %q: %v", title, err)
	}
	if _, err := f.db.Exec(`UPDATE tasks SET created_at = ? WHERE id = ?`, model.FormatUTC(createdAt), task.ID); err != nil {
		t.Fatalf("pin created_at: %v", err)
	}
	return task
}

func pendingIDs(p []PendingInboxTask) []int64 {
	out := make([]int64, len(p))
	for i, x := range p {
		out[i] = x.Task.ID
	}
	return out
}

func equalIDs(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestInboxProcessingRepo_ListPending_Rules(t *testing.T) {
	f := newInboxProcFixture(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	base := now.Add(-24 * time.Hour)

	fresh := f.inboxTask(t, "fresh", base.Add(1*time.Minute))
	kept := f.inboxTask(t, "kept", base.Add(2*time.Minute))
	edited := f.inboxTask(t, "edited", base.Add(3*time.Minute))
	failedDue := f.inboxTask(t, "failed due", base.Add(4*time.Minute))
	failedLater := f.inboxTask(t, "failed later", base.Add(5*time.Minute))
	failedAsleep := f.inboxTask(t, "failed asleep", base.Add(6*time.Minute))
	reverted := f.inboxTask(t, "reverted", base.Add(7*time.Minute))
	closed := f.inboxTask(t, "closed", base.Add(8*time.Minute))
	if _, err := f.tasks.Update(ctx, closed.ID, TaskUpdate{Status: ptr(model.TaskStatusCompleted)}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if _, err := f.tasks.Create(ctx, CreateTask{Placement: Placement{ContextID: &f.contextID}, Title: "not inbox"}); err != nil {
		t.Fatalf("create context task: %v", err)
	}

	state := func(task *model.Task, status model.InboxStateStatus, fp string, next *time.Time) {
		t.Helper()
		if err := f.proc.UpsertState(ctx, model.InboxProcessingState{
			TaskID: task.ID, Fingerprint: fp, Status: status, Attempts: 1, NextAttemptAt: next, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("upsert state: %v", err)
		}
	}
	fp := func(task *model.Task) string { return model.InboxFingerprint(task.Title, task.Description) }
	state(kept, model.InboxStateKept, fp(kept), nil)
	state(edited, model.InboxStateKept, "stale-fingerprint", nil)
	state(failedDue, model.InboxStateFailed, fp(failedDue), ptr(now.Add(-time.Second)))
	state(failedLater, model.InboxStateFailed, fp(failedLater), ptr(now.Add(time.Minute)))
	state(failedAsleep, model.InboxStateFailed, fp(failedAsleep), nil)
	state(reverted, model.InboxStateReverted, fp(reverted), nil)

	got, err := f.proc.ListPending(ctx, now, 10)
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	want := []int64{fresh.ID, edited.ID, failedDue.ID}
	if !equalIDs(pendingIDs(got), want) {
		t.Fatalf("pending: got %v, want %v", pendingIDs(got), want)
	}
	if got[0].State != nil {
		t.Errorf("fresh task state: got %+v, want nil", got[0].State)
	}
	if got[2].State == nil || got[2].State.Status != model.InboxStateFailed {
		t.Errorf("failed task state: got %+v, want the failed row", got[2].State)
	}

	count, err := f.proc.PendingCount(ctx, now)
	if err != nil {
		t.Fatalf("pending count: %v", err)
	}
	if count != 3 {
		t.Errorf("pending count: got %d, want 3", count)
	}
}

func TestInboxProcessingRepo_ListPending_LimitAndOrder(t *testing.T) {
	f := newInboxProcFixture(t)
	ctx := context.Background()
	now := time.Now()
	f.inboxTask(t, "c", now.Add(-1*time.Hour))
	a := f.inboxTask(t, "a", now.Add(-3*time.Hour))
	b := f.inboxTask(t, "b", now.Add(-2*time.Hour))

	got, err := f.proc.ListPending(ctx, now, 2)
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if want := []int64{a.ID, b.ID}; !equalIDs(pendingIDs(got), want) {
		t.Errorf("pending: got %v, want %v (oldest first, limited)", pendingIDs(got), want)
	}
}

func TestInboxProcessingRepo_ListPending_HydratesLabels(t *testing.T) {
	f := newInboxProcFixture(t)
	ctx := context.Background()
	label, err := f.labels.Create(ctx, "home", "green", false)
	if err != nil {
		t.Fatalf("create label: %v", err)
	}
	task := f.inboxTask(t, "with label", time.Now().Add(-time.Minute))
	if err := f.tlabels.SetForTask(ctx, task.ID, []int64{label.ID}); err != nil {
		t.Fatalf("set labels: %v", err)
	}
	got, err := f.proc.ListPending(ctx, time.Now(), 10)
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(got) != 1 || len(got[0].Task.Labels) != 1 || got[0].Task.Labels[0].ID != label.ID {
		t.Errorf("labels: got %+v, want [%d]", got, label.ID)
	}
}

func TestInboxProcessingRepo_StateUpsertAndDelete(t *testing.T) {
	f := newInboxProcFixture(t)
	ctx := context.Background()
	task := f.inboxTask(t, "x", time.Now())
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	errText := "boom"
	st := model.InboxProcessingState{TaskID: task.ID, Fingerprint: "f", Status: model.InboxStateFailed, Attempts: 2,
		NextAttemptAt: ptr(now.Add(time.Minute)), LastError: &errText, UpdatedAt: now}
	if err := f.proc.UpsertState(ctx, st); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	st.Status = model.InboxStateKept
	st.Attempts = 0
	st.NextAttemptAt = nil
	st.LastError = nil
	if err := f.proc.UpsertState(ctx, st); err != nil {
		t.Fatalf("upsert again: %v", err)
	}
	got, err := f.proc.GetState(ctx, task.ID)
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if got.Status != model.InboxStateKept || got.Attempts != 0 || got.NextAttemptAt != nil || got.LastError != nil {
		t.Errorf("state: got %+v, want the overwritten kept row", got)
	}
	if err := f.proc.DeleteState(ctx, task.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := f.proc.GetState(ctx, task.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("get after delete: got %v, want ErrNotFound", err)
	}
}

func TestInboxProcessingRepo_LogAppendListGetRevert(t *testing.T) {
	f := newInboxProcFixture(t)
	ctx := context.Background()
	task := f.inboxTask(t, "log me", time.Now())
	t0 := time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC)
	due := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)

	confidence := 0.9
	sortedID, err := f.proc.AppendLog(ctx, model.InboxProcessingLogEntry{
		TaskID: &task.ID, TaskTitle: task.Title, Outcome: model.InboxOutcomeSorted, Model: "m", Reason: "fits",
		Confidence: &confidence,
		Before:     model.InboxTaskBefore{LabelIDs: []int64{}, Priority: model.PriorityNone},
		After: &model.InboxTaskAfter{ContextID: f.contextID, ProjectID: f.projectID, LabelIDs: []int64{3},
			Priority: model.PriorityHigh, DueAt: &due},
		PromptTokens: ptr(100), CompletionTokens: ptr(20), CreatedAt: t0,
	})
	if err != nil {
		t.Fatalf("append sorted: %v", err)
	}
	errText := "bad json"
	if _, err := f.proc.AppendLog(ctx, model.InboxProcessingLogEntry{
		TaskID: &task.ID, TaskTitle: task.Title, Outcome: model.InboxOutcomeFailed, Model: "m",
		Before: model.InboxTaskBefore{Priority: model.PriorityNone}, Error: &errText, CreatedAt: t0.Add(time.Minute),
	}); err != nil {
		t.Fatalf("append failed: %v", err)
	}

	entries, total, err := f.proc.ListLog(ctx, Page{Limit: 1})
	if err != nil {
		t.Fatalf("list log: %v", err)
	}
	if total != 2 || len(entries) != 1 || entries[0].Outcome != model.InboxOutcomeFailed {
		t.Fatalf("list log: got total=%d entries=%+v, want newest first", total, entries)
	}

	got, err := f.proc.GetLog(ctx, sortedID)
	if err != nil {
		t.Fatalf("get log: %v", err)
	}
	if got.After == nil || got.After.ProjectID != f.projectID || got.After.DueAt == nil || !got.After.DueAt.Equal(due) {
		t.Errorf("after state: got %+v, want project %d due %v", got.After, f.projectID, due)
	}
	if got.Confidence == nil || *got.Confidence != 0.9 || got.PromptTokens == nil || *got.PromptTokens != 100 {
		t.Errorf("metrics: got %+v", got)
	}
	if !got.CreatedAt.Equal(t0) {
		t.Errorf("created at: got %v, want %v", got.CreatedAt, t0)
	}

	if err := f.proc.MarkReverted(ctx, sortedID, t0.Add(time.Hour)); err != nil {
		t.Fatalf("mark reverted: %v", err)
	}
	if err := f.proc.MarkReverted(ctx, sortedID, t0.Add(2*time.Hour)); !errors.Is(err, ErrConflict) {
		t.Errorf("second revert: got %v, want ErrConflict", err)
	}
	if err := f.proc.MarkReverted(ctx, 9999, t0); !errors.Is(err, ErrNotFound) {
		t.Errorf("revert missing: got %v, want ErrNotFound", err)
	}
	if _, err := f.proc.GetLog(ctx, 9999); !errors.Is(err, ErrNotFound) {
		t.Errorf("get missing: got %v, want ErrNotFound", err)
	}
}

func TestInboxProcessingRepo_PruneLog(t *testing.T) {
	f := newInboxProcFixture(t)
	ctx := context.Background()
	old := time.Now().Add(-100 * 24 * time.Hour)
	for _, at := range []time.Time{old, time.Now()} {
		if _, err := f.proc.AppendLog(ctx, model.InboxProcessingLogEntry{
			TaskTitle: "t", Outcome: model.InboxOutcomeKept, Model: "m", CreatedAt: at,
		}); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	n, err := f.proc.PruneLog(ctx, time.Now().Add(-90*24*time.Hour))
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if n != 1 {
		t.Errorf("pruned: got %d, want 1", n)
	}
	_, total, err := f.proc.ListLog(ctx, Page{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 {
		t.Errorf("remaining: got %d, want 1", total)
	}
}

func TestInboxProcessingRepo_PruneStaleStates(t *testing.T) {
	f := newInboxProcFixture(t)
	ctx := context.Background()
	stay := f.inboxTask(t, "stay", time.Now())
	gone := f.inboxTask(t, "gone", time.Now())
	for _, task := range []*model.Task{stay, gone} {
		if err := f.proc.UpsertState(ctx, model.InboxProcessingState{TaskID: task.ID, Fingerprint: "f",
			Status: model.InboxStateKept, UpdatedAt: time.Now()}); err != nil {
			t.Fatalf("upsert: %v", err)
		}
	}
	if err := f.tasks.Move(ctx, gone.ID, Placement{ContextID: &f.contextID}); err != nil {
		t.Fatalf("move: %v", err)
	}
	n, err := f.proc.PruneStaleStates(ctx)
	if err != nil {
		t.Fatalf("prune states: %v", err)
	}
	if n != 1 {
		t.Errorf("pruned: got %d, want 1", n)
	}
	if _, err := f.proc.GetState(ctx, stay.ID); err != nil {
		t.Errorf("inbox task state: got %v, want kept", err)
	}
}

func TestInboxProcessingRepo_OldestInboxTask(t *testing.T) {
	f := newInboxProcFixture(t)
	ctx := context.Background()
	if _, err := f.proc.OldestInboxTask(ctx); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty inbox: got %v, want ErrNotFound", err)
	}
	now := time.Now()
	f.inboxTask(t, "newer", now.Add(-time.Minute))
	older := f.inboxTask(t, "older", now.Add(-time.Hour))
	got, err := f.proc.OldestInboxTask(ctx)
	if err != nil {
		t.Fatalf("oldest: %v", err)
	}
	if got.ID != older.ID {
		t.Errorf("oldest: got %d, want %d", got.ID, older.ID)
	}
}

func TestInboxProcessingRepo_UndecidedTasksAreNotPending(t *testing.T) {
	f := newInboxProcFixture(t)
	ctx := context.Background()
	now := time.Now()
	fresh := f.inboxTask(t, "fresh", now.Add(-2*time.Minute))
	undecided := f.inboxTask(t, "undecided", now.Add(-time.Minute))
	at := now
	if _, err := f.tasks.Update(ctx, undecided.ID, TaskUpdate{AutoSortUndecidedAt: &at}); err != nil {
		t.Fatalf("mark: %v", err)
	}

	got, err := f.proc.ListPending(ctx, now, 10)
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if want := []int64{fresh.ID}; !equalIDs(pendingIDs(got), want) {
		t.Errorf("pending: got %v, want %v", pendingIDs(got), want)
	}
	n, err := f.proc.UndecidedCount(ctx)
	if err != nil {
		t.Fatalf("undecided count: %v", err)
	}
	if n != 1 {
		t.Errorf("undecided count: got %d, want 1", n)
	}
}

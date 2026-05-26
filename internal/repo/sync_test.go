package repo

import (
	"context"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

type syncFixture struct {
	*taskFixture
	sync *SyncRepo
}

func newSyncFixture(t *testing.T) *syncFixture {
	t.Helper()
	tf := newTaskFixture(t)
	plr := NewProjectLabelsRepo(tf.tasks.db)
	return &syncFixture{
		taskFixture: tf,
		sync:        NewSyncRepo(tf.tasks.db, tf.tlabels, plr),
	}
}

func softDelete(t *testing.T, f *syncFixture, table string, id int64, when time.Time) {
	t.Helper()
	q := "UPDATE " + table + " SET deleted_at = ?, updated_at = ? WHERE id = ?"
	if _, err := f.tasks.db.ExecContext(context.Background(), q,
		model.FormatUTC(when), model.FormatUTC(when), id); err != nil {
		t.Fatalf("soft-delete %s id=%d: %v", table, id, err)
	}
}

func stampCompleted(t *testing.T, f *syncFixture, taskID int64, completedAt time.Time) {
	t.Helper()
	if _, err := f.tasks.db.ExecContext(context.Background(),
		`UPDATE tasks SET status = 'completed', completed_at = ?, updated_at = ? WHERE id = ?`,
		model.FormatUTC(completedAt), model.FormatUTC(completedAt), taskID); err != nil {
		t.Fatalf("stamp completed: %v", err)
	}
}

func stampUpdatedAt(t *testing.T, f *syncFixture, table string, id int64, when time.Time) {
	t.Helper()
	q := "UPDATE " + table + " SET updated_at = ? WHERE id = ?"
	if _, err := f.tasks.db.ExecContext(context.Background(), q,
		model.FormatUTC(when), id); err != nil {
		t.Fatalf("stamp updated_at %s id=%d: %v", table, id, err)
	}
}

func TestSyncRepo_Tasks_InitialPull_OpenAndRecentCompletedOnly(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	now := time.Now().UTC()
	cutoff := now.Add(-30 * 24 * time.Hour)

	open, err := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "open",
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	recent, err := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "completed recent",
	})
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	stampCompleted(t, f, recent.ID, now.Add(-1*24*time.Hour))

	old, err := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "completed old",
	})
	if err != nil {
		t.Fatalf("old: %v", err)
	}
	stampCompleted(t, f, old.ID, now.Add(-40*24*time.Hour))

	dead, err := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "deleted",
	})
	if err != nil {
		t.Fatalf("dead: %v", err)
	}
	softDelete(t, f, "tasks", dead.ID, now)

	got, err := f.sync.Tasks(ctx, nil, cutoff)
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len: got %d, want 2 (%v)", len(got), got)
	}
	ids := map[int64]bool{got[0].ID: true, got[1].ID: true}
	if !ids[open.ID] || !ids[recent.ID] {
		t.Errorf("missing expected ids: open=%d recent=%d, got %v", open.ID, recent.ID, ids)
	}
	if ids[old.ID] {
		t.Errorf("old completed task should be excluded: id=%d", old.ID)
	}
	if ids[dead.ID] {
		t.Errorf("deleted task should be excluded: id=%d", dead.ID)
	}
}

func TestSyncRepo_Tasks_Incremental_IncludesTombstones(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()

	t0 := time.Now().UTC().Add(-1 * time.Hour)
	since := t0.Add(-1 * time.Second)

	alive, err := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "alive",
	})
	if err != nil {
		t.Fatalf("alive: %v", err)
	}
	stampUpdatedAt(t, f, "tasks", alive.ID, t0.Add(1*time.Minute))

	tomb, err := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "tomb",
	})
	if err != nil {
		t.Fatalf("tomb: %v", err)
	}
	softDelete(t, f, "tasks", tomb.ID, t0.Add(2*time.Minute))

	stale, err := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "stale",
	})
	if err != nil {
		t.Fatalf("stale: %v", err)
	}
	stampUpdatedAt(t, f, "tasks", stale.ID, since.Add(-1*time.Hour))

	got, err := f.sync.Tasks(ctx, &since, time.Time{})
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	ids := map[int64]bool{}
	for _, tk := range got {
		ids[tk.ID] = true
	}
	if !ids[alive.ID] {
		t.Errorf("alive must be in incremental: id=%d", alive.ID)
	}
	if !ids[tomb.ID] {
		t.Errorf("tombstone must be in incremental: id=%d", tomb.ID)
	}
	if ids[stale.ID] {
		t.Errorf("stale (updated before since) must not be in incremental: id=%d", stale.ID)
	}
	for _, tk := range got {
		if tk.ID == tomb.ID && tk.DeletedAt == nil {
			t.Errorf("tombstone deleted_at must be non-nil")
		}
	}
}

func TestSyncRepo_Tasks_CompletedCutoffBoundary(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	cutoff := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	atCutoff, err := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "at-cutoff",
	})
	if err != nil {
		t.Fatalf("at-cutoff: %v", err)
	}
	stampCompleted(t, f, atCutoff.ID, cutoff)

	justBefore, err := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "just-before",
	})
	if err != nil {
		t.Fatalf("just-before: %v", err)
	}
	stampCompleted(t, f, justBefore.ID, cutoff.Add(-1*time.Millisecond))

	got, err := f.sync.Tasks(ctx, nil, cutoff)
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	ids := map[int64]bool{}
	for _, tk := range got {
		ids[tk.ID] = true
	}
	if !ids[atCutoff.ID] {
		t.Errorf("task completed exactly at cutoff must be included (>= boundary), id=%d", atCutoff.ID)
	}
	if ids[justBefore.ID] {
		t.Errorf("task completed before cutoff must be excluded, id=%d", justBefore.ID)
	}
}

func TestSyncRepo_Tasks_HydratesLabels(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()

	lbl, err := f.labels.Create(ctx, "urgent", "red", false)
	if err != nil {
		t.Fatalf("label: %v", err)
	}
	task, err := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "with-label",
	})
	if err != nil {
		t.Fatalf("task: %v", err)
	}
	if err := f.tlabels.SetForTask(ctx, task.ID, []int64{lbl.ID}); err != nil {
		t.Fatalf("set labels: %v", err)
	}

	got, err := f.sync.Tasks(ctx, nil, time.Now().Add(-30*24*time.Hour))
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len: got %d, want 1", len(got))
	}
	if len(got[0].Labels) != 1 || got[0].Labels[0].ID != lbl.ID {
		t.Errorf("labels: got %+v, want one label id=%d", got[0].Labels, lbl.ID)
	}
}

func TestSyncRepo_Tasks_EmptyResult(t *testing.T) {
	f := newSyncFixture(t)
	got, err := f.sync.Tasks(context.Background(), nil, time.Now())
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len: got %d, want 0", len(got))
	}
	if got == nil {
		t.Errorf("expected non-nil empty slice, got nil")
	}
}

func TestSyncRepo_Projects_InitialExcludesTombstones(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	now := time.Now().UTC()

	p2, err := f.projects.Create(ctx, CreateProject{ContextID: f.contextID, Title: "beta", Color: "red"})
	if err != nil {
		t.Fatalf("p2: %v", err)
	}
	softDelete(t, f, "projects", p2.ID, now)

	got, err := f.sync.Projects(ctx, nil)
	if err != nil {
		t.Fatalf("Projects: %v", err)
	}
	for _, p := range got {
		if p.ID == p2.ID {
			t.Errorf("tombstoned project must be excluded from initial pull, id=%d", p.ID)
		}
	}
	if len(got) != 1 || got[0].ID != f.projectID {
		t.Errorf("expected only the fixture project, got %v", got)
	}
}

func TestSyncRepo_Projects_IncrementalIncludesTombstones(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	since := time.Now().UTC().Add(-1 * time.Hour)

	p, err := f.projects.Create(ctx, CreateProject{ContextID: f.contextID, Title: "gamma", Color: "green"})
	if err != nil {
		t.Fatalf("p: %v", err)
	}
	softDelete(t, f, "projects", p.ID, time.Now())

	got, err := f.sync.Projects(ctx, &since)
	if err != nil {
		t.Fatalf("Projects: %v", err)
	}
	found := false
	for _, item := range got {
		if item.ID == p.ID {
			found = true
			if item.DeletedAt == nil {
				t.Errorf("incremental project must carry deleted_at")
			}
		}
	}
	if !found {
		t.Errorf("incremental projects must include tombstone, got %v", got)
	}
}

func TestSyncRepo_Projects_HydratesLabels(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()

	lbl, err := f.labels.Create(ctx, "ops", "blue", false)
	if err != nil {
		t.Fatalf("label: %v", err)
	}
	plr := NewProjectLabelsRepo(f.tasks.db)
	if err := plr.SetForProject(ctx, f.projectID, []int64{lbl.ID}); err != nil {
		t.Fatalf("set project labels: %v", err)
	}

	got, err := f.sync.Projects(ctx, nil)
	if err != nil {
		t.Fatalf("Projects: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len: got %d", len(got))
	}
	if len(got[0].Labels) != 1 || got[0].Labels[0].ID != lbl.ID {
		t.Errorf("project labels: got %+v, want one label id=%d", got[0].Labels, lbl.ID)
	}
}

func TestSyncRepo_Sections_InitialExcludesTombstones(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	now := time.Now().UTC()

	s2, err := f.sections.Create(ctx, f.projectID, "deleted-section")
	if err != nil {
		t.Fatalf("s2: %v", err)
	}
	softDelete(t, f, "project_sections", s2.ID, now)

	got, err := f.sync.Sections(ctx, nil)
	if err != nil {
		t.Fatalf("Sections: %v", err)
	}
	for _, s := range got {
		if s.ID == s2.ID {
			t.Errorf("tombstoned section must be excluded, id=%d", s.ID)
		}
	}
}

func TestSyncRepo_Sections_IncrementalIncludesTombstones(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	since := time.Now().UTC().Add(-1 * time.Hour)
	s, err := f.sections.Create(ctx, f.projectID, "to-delete")
	if err != nil {
		t.Fatalf("section: %v", err)
	}
	softDelete(t, f, "project_sections", s.ID, time.Now())

	got, err := f.sync.Sections(ctx, &since)
	if err != nil {
		t.Fatalf("Sections: %v", err)
	}
	found := false
	for _, item := range got {
		if item.ID == s.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("incremental sections must include tombstone")
	}
}

func TestSyncRepo_Labels_InitialExcludesTombstones(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()

	live, err := f.labels.Create(ctx, "live", "blue", false)
	if err != nil {
		t.Fatalf("live: %v", err)
	}
	dead, err := f.labels.Create(ctx, "dead", "red", false)
	if err != nil {
		t.Fatalf("dead: %v", err)
	}
	softDelete(t, f, "labels", dead.ID, time.Now())

	got, err := f.sync.Labels(ctx, nil)
	if err != nil {
		t.Fatalf("Labels: %v", err)
	}
	ids := map[int64]bool{}
	for _, l := range got {
		ids[l.ID] = true
	}
	if !ids[live.ID] {
		t.Errorf("live label missing: id=%d", live.ID)
	}
	if ids[dead.ID] {
		t.Errorf("dead label must be excluded: id=%d", dead.ID)
	}
}

func TestSyncRepo_Labels_IncrementalIncludesTombstones(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	since := time.Now().UTC().Add(-1 * time.Hour)
	dead, err := f.labels.Create(ctx, "dead", "red", false)
	if err != nil {
		t.Fatalf("dead: %v", err)
	}
	softDelete(t, f, "labels", dead.ID, time.Now())

	got, err := f.sync.Labels(ctx, &since)
	if err != nil {
		t.Fatalf("Labels: %v", err)
	}
	found := false
	for _, item := range got {
		if item.ID == dead.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("incremental labels must include tombstone")
	}
}

func TestSyncRepo_Contexts_InitialExcludesTombstones(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()

	dead, err := f.contexts.Create(ctx, "tomb", "grey", false)
	if err != nil {
		t.Fatalf("dead: %v", err)
	}
	softDelete(t, f, "contexts", dead.ID, time.Now())

	got, err := f.sync.Contexts(ctx, nil)
	if err != nil {
		t.Fatalf("Contexts: %v", err)
	}
	for _, c := range got {
		if c.ID == dead.ID {
			t.Errorf("tombstoned context must be excluded, id=%d", c.ID)
		}
	}
}

func TestSyncRepo_Contexts_IncrementalIncludesTombstones(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	since := time.Now().UTC().Add(-1 * time.Hour)
	dead, err := f.contexts.Create(ctx, "tomb-inc", "grey", false)
	if err != nil {
		t.Fatalf("dead: %v", err)
	}
	softDelete(t, f, "contexts", dead.ID, time.Now())

	got, err := f.sync.Contexts(ctx, &since)
	if err != nil {
		t.Fatalf("Contexts: %v", err)
	}
	found := false
	for _, item := range got {
		if item.ID == dead.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("incremental contexts must include tombstone")
	}
}

func TestSyncRepo_Tasks_NilTaskLabelsRepo_NoHydration(t *testing.T) {
	tf := newTaskFixture(t)
	sr := NewSyncRepo(tf.tasks.db, nil, nil)
	ctx := context.Background()

	lbl, err := tf.labels.Create(ctx, "x", "blue", false)
	if err != nil {
		t.Fatalf("label: %v", err)
	}
	task, err := tf.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &tf.contextID},
		Title:     "t",
	})
	if err != nil {
		t.Fatalf("task: %v", err)
	}
	if err := tf.tlabels.SetForTask(ctx, task.ID, []int64{lbl.ID}); err != nil {
		t.Fatalf("set labels: %v", err)
	}

	got, err := sr.Tasks(ctx, nil, time.Now().Add(-30*24*time.Hour))
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len: got %d", len(got))
	}
	if len(got[0].Labels) != 0 {
		t.Errorf("labels must be empty when taskLabels==nil, got %+v", got[0].Labels)
	}
}

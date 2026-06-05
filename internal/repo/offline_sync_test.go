package repo

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

// rawCount counts physical rows for the given table regardless of the
// deleted_at tombstone, so tests can assert soft-delete keeps the row.
func rawCount(t *testing.T, f *taskFixture, table string, id int64) int {
	t.Helper()
	d := f.tasks.db
	var n int
	if err := d.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM "+table+" WHERE id = ?", id).Scan(&n); err != nil {
		t.Fatalf("raw count %s: %v", table, err)
	}
	return n
}

func deletedAtRaw(t *testing.T, f *taskFixture, table string, id int64) string {
	t.Helper()
	d := f.tasks.db
	var s *string
	if err := d.QueryRowContext(context.Background(),
		"SELECT deleted_at FROM "+table+" WHERE id = ?", id).Scan(&s); err != nil {
		t.Fatalf("raw deleted_at %s: %v", table, err)
	}
	if s == nil {
		return ""
	}
	return *s
}

// TestTaskRepo_Create_AssignsClientID asserts every newly created task carries a
// non-empty client_id (Federation v1 F0.1, US-3.7 foundation).
func TestTaskRepo_Create_AssignsClientID(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	task, err := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "x",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if task.ClientID == "" {
		t.Errorf("expected non-empty client_id, got empty")
	}
	got, err := f.tasks.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ClientID != task.ClientID {
		t.Errorf("client_id round-trip: got %q, want %q", got.ClientID, task.ClientID)
	}
}

// TestTaskRepo_Delete_SoftDeleteKeepsRow asserts Delete sets the tombstone but
// leaves the physical row in place (US-3.7 AC1 foundation), and that Get/List
// then exclude it.
func TestTaskRepo_Delete_SoftDeleteKeepsRow(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	task, err := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID, ProjectID: &f.projectID},
		Title:     "doomed",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := f.tasks.Delete(ctx, task.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// Row physically survives.
	if n := rawCount(t, f, "tasks", task.ID); n != 1 {
		t.Errorf("physical row after soft-delete: got %d, want 1", n)
	}
	// Tombstone is set.
	if deletedAtRaw(t, f, "tasks", task.ID) == "" {
		t.Errorf("expected deleted_at to be set after soft-delete")
	}
	// Get excludes the tombstone.
	if _, err := f.tasks.Get(ctx, task.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get tombstone: got %v, want ErrNotFound", err)
	}
	// List excludes the tombstone.
	items, total, err := f.tasks.ListByProject(ctx, f.projectID, TaskFilter{}, Page{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 0 || len(items) != 0 {
		t.Errorf("list after soft-delete: got total=%d len=%d, want 0/0", total, len(items))
	}
}

// TestTaskRepo_Delete_CascadesSubtaskTombstones asserts the service-layer-style
// cascade emulation: deleting a parent soft-deletes its whole subtree, since the
// DB FK cascade no longer fires for soft-deletes (US-3.7 AC3 foundation).
func TestTaskRepo_Delete_CascadesSubtaskTombstones(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	parent, _ := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "p",
	})
	child, _ := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID, ParentID: &parent.ID},
		Title:     "c",
	})
	grandchild, _ := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID, ParentID: &child.ID},
		Title:     "gc",
	})

	if err := f.tasks.Delete(ctx, parent.ID); err != nil {
		t.Fatalf("delete parent: %v", err)
	}

	for _, id := range []int64{parent.ID, child.ID, grandchild.ID} {
		// Physical row remains.
		if n := rawCount(t, f, "tasks", id); n != 1 {
			t.Errorf("physical row %d: got %d, want 1", id, n)
		}
		// But it is tombstoned (invisible to Get).
		if _, err := f.tasks.Get(ctx, id); !errors.Is(err, ErrNotFound) {
			t.Errorf("Get %d after cascade: got %v, want ErrNotFound", id, err)
		}
	}
}

// TestTaskRepo_Update_TombstoneReturnsGone asserts that re-editing a soft-deleted
// task is rejected with ErrGone, not silently resurrected (US-3.7 AC2 foundation).
func TestTaskRepo_Update_TombstoneReturnsGone(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	task, _ := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "edit me",
	})
	if err := f.tasks.Delete(ctx, task.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err := f.tasks.Update(ctx, task.ID, TaskUpdate{Title: ptr("resurrected")})
	if !errors.Is(err, ErrGone) {
		t.Fatalf("update tombstone: got %v, want ErrGone", err)
	}
}

// TestProjectRepo_Delete_CascadesTaskTombstones asserts deleting a project
// soft-deletes its sections and tasks (FK-cascade emulation).
func TestProjectRepo_Delete_CascadesTaskTombstones(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	task, _ := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID, ProjectID: &f.projectID, SectionID: &f.sectionID},
		Title:     "in project",
	})

	if err := f.projects.Delete(ctx, f.projectID); err != nil {
		t.Fatalf("delete project: %v", err)
	}

	if _, err := f.projects.Get(ctx, f.projectID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get project after delete: got %v, want ErrNotFound", err)
	}
	if _, err := f.sections.Get(ctx, f.sectionID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get section after project delete: got %v, want ErrNotFound", err)
	}
	if _, err := f.tasks.Get(ctx, task.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get task after project delete: got %v, want ErrNotFound", err)
	}
	// Physical rows survive.
	if n := rawCount(t, f, "tasks", task.ID); n != 1 {
		t.Errorf("physical task row: got %d, want 1", n)
	}
}

// TestContextRepo_Delete_SoftDeleteAndUpdateGone asserts contexts soft-delete and
// that re-editing the tombstone returns ErrGone.
func TestContextRepo_Delete_SoftDeleteAndUpdateGone(t *testing.T) {
	d := setupTestDB(t)
	r := NewContextRepo(d)
	ctx := context.Background()

	c, err := r.Create(ctx, "work", "blue", false)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if c.ClientID == "" {
		t.Errorf("expected context client_id assigned")
	}
	if err := r.Delete(ctx, c.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	var n int
	if err := d.QueryRowContext(ctx, "SELECT COUNT(*) FROM contexts WHERE id = ?", c.ID).Scan(&n); err != nil {
		t.Fatalf("raw count: %v", err)
	}
	if n != 1 {
		t.Errorf("physical context row after soft-delete: got %d, want 1", n)
	}
	if _, err := r.Update(ctx, c.ID, ContextUpdate{Color: ptr("red")}); !errors.Is(err, ErrGone) {
		t.Errorf("update tombstoned context: got %v, want ErrGone", err)
	}
}

// TestLabelRepo_Delete_SoftDeleteAndGone asserts labels soft-delete and Update
// on a tombstone returns ErrGone.
func TestLabelRepo_Delete_SoftDeleteAndGone(t *testing.T) {
	d := setupTestDB(t)
	r := NewLabelRepo(d)
	ctx := context.Background()

	l, err := r.Create(ctx, "x", "blue", false)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if l.ClientID == "" {
		t.Errorf("expected label client_id assigned")
	}
	if err := r.Delete(ctx, l.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	var n int
	if err := d.QueryRowContext(ctx, "SELECT COUNT(*) FROM labels WHERE id = ?", l.ID).Scan(&n); err != nil {
		t.Fatalf("raw count: %v", err)
	}
	if n != 1 {
		t.Errorf("physical label row after soft-delete: got %d, want 1", n)
	}
	if _, err := r.Update(ctx, l.ID, LabelUpdate{Name: ptr("y")}); !errors.Is(err, ErrGone) {
		t.Errorf("update tombstoned label: got %v, want ErrGone", err)
	}
}

// TestLabelRepo_Recreate_AfterSoftDelete_SucceedsAndKeepsTombstone asserts the
// name-uniqueness-vs-soft-delete reconciliation (Federation v1 F0.1 fix): after a
// label is soft-deleted, recreating a label with the same name must succeed (the
// live-only partial UNIQUE index frees the slot), the new row gets a fresh id and
// client_id, and the original tombstone physically survives for federation
// retention/LWW (it is NOT revived — US-3.7 AC2 keeps the tombstone final).
func TestLabelRepo_Recreate_AfterSoftDelete_SucceedsAndKeepsTombstone(t *testing.T) {
	d := setupTestDB(t)
	r := NewLabelRepo(d)
	ctx := context.Background()

	first, err := r.Create(ctx, "work", "blue", false)
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	if err := r.Delete(ctx, first.ID); err != nil {
		t.Fatalf("delete first: %v", err)
	}

	// Recreating the same name must NOT 409 against the invisible tombstone.
	second, err := r.Create(ctx, "work", "red", false)
	if err != nil {
		t.Fatalf("recreate after soft-delete: got %v, want success", err)
	}
	if second.ID == first.ID {
		t.Errorf("recreate reused tombstone id %d; want a fresh row (no revival)", first.ID)
	}
	if second.ClientID == "" || second.ClientID == first.ClientID {
		t.Errorf("recreate client_id: got %q (first %q), want a fresh non-empty id", second.ClientID, first.ClientID)
	}

	// The live label is the new one.
	got, err := r.GetByName(ctx, "work")
	if err != nil {
		t.Fatalf("get by name: %v", err)
	}
	if got.ID != second.ID || got.Color != "red" {
		t.Errorf("live label after recreate: got id=%d color=%q, want id=%d color=red", got.ID, got.Color, second.ID)
	}

	// The original tombstone physically survives (not revived, not overwritten).
	var n int
	if err := d.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM labels WHERE name = 'work' AND deleted_at IS NOT NULL").Scan(&n); err != nil {
		t.Fatalf("count tombstones: %v", err)
	}
	if n != 1 {
		t.Errorf("surviving tombstones named 'work': got %d, want 1", n)
	}
}

// TestContextRepo_Recreate_AfterSoftDelete_SucceedsAndKeepsTombstone mirrors the
// label case for contexts (Federation v1 F0.1 fix).
func TestContextRepo_Recreate_AfterSoftDelete_SucceedsAndKeepsTombstone(t *testing.T) {
	d := setupTestDB(t)
	r := NewContextRepo(d)
	ctx := context.Background()

	first, err := r.Create(ctx, "work", "blue", false)
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	if err := r.Delete(ctx, first.ID); err != nil {
		t.Fatalf("delete first: %v", err)
	}

	second, err := r.Create(ctx, "work", "red", false)
	if err != nil {
		t.Fatalf("recreate after soft-delete: got %v, want success", err)
	}
	if second.ID == first.ID {
		t.Errorf("recreate reused tombstone id %d; want a fresh row (no revival)", first.ID)
	}

	got, err := r.Get(ctx, second.ID)
	if err != nil {
		t.Fatalf("get recreated: %v", err)
	}
	if got.Color != "red" {
		t.Errorf("live context color after recreate: got %q, want red", got.Color)
	}

	var n int
	if err := d.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM contexts WHERE name = 'work' AND deleted_at IS NOT NULL").Scan(&n); err != nil {
		t.Fatalf("count tombstones: %v", err)
	}
	if n != 1 {
		t.Errorf("surviving tombstones named 'work': got %d, want 1", n)
	}
}

// TestLabelRepo_LiveDuplicateName_StillConflicts asserts the live-only partial
// unique index still rejects two *live* labels sharing a name (the soft-delete
// fix must not weaken uniqueness among live rows).
func TestLabelRepo_LiveDuplicateName_StillConflicts(t *testing.T) {
	d := setupTestDB(t)
	r := NewLabelRepo(d)
	ctx := context.Background()
	if _, err := r.Create(ctx, "dup", "red", false); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := r.Create(ctx, "dup", "blue", false); !errors.Is(err, ErrConflict) {
		t.Errorf("live duplicate name: got %v, want ErrConflict", err)
	}
}

// TestContextRepo_LiveDuplicateName_StillConflicts mirrors the label live-dup case.
func TestContextRepo_LiveDuplicateName_StillConflicts(t *testing.T) {
	d := setupTestDB(t)
	r := NewContextRepo(d)
	ctx := context.Background()
	if _, err := r.Create(ctx, "dup", "red", false); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := r.Create(ctx, "dup", "blue", false); !errors.Is(err, ErrConflict) {
		t.Errorf("live duplicate name: got %v, want ErrConflict", err)
	}
}

// TestTaskRepo_SetPinned_TombstoneReturnsGone asserts pinning a soft-deleted
// task does NOT silently succeed: a tombstone re-edit is ErrGone and is_pinned is
// not flipped (Federation v1 F0.1 follow-up — the pin write path previously
// omitted the deleted_at filter).
func TestTaskRepo_SetPinned_TombstoneReturnsGone(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	task, _ := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "pin me",
	})
	if err := f.tasks.Delete(ctx, task.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := f.tasks.SetPinned(ctx, task.ID, true); !errors.Is(err, ErrGone) {
		t.Errorf("pin tombstone: got %v, want ErrGone", err)
	}
	var pinned int
	if err := f.tasks.db.QueryRowContext(ctx,
		"SELECT is_pinned FROM tasks WHERE id = ?", task.ID).Scan(&pinned); err != nil {
		t.Fatalf("raw is_pinned: %v", err)
	}
	if pinned != 0 {
		t.Errorf("tombstoned task is_pinned: got %d, want 0 (pin must not apply)", pinned)
	}
}

// TestTaskRepo_SetPinned_MissingReturnsNotFound asserts a genuinely missing id
// still maps to ErrNotFound (not ErrGone).
func TestTaskRepo_SetPinned_MissingReturnsNotFound(t *testing.T) {
	f := newTaskFixture(t)
	if err := f.tasks.SetPinned(context.Background(), 999999, true); !errors.Is(err, ErrNotFound) {
		t.Errorf("pin missing task: got %v, want ErrNotFound", err)
	}
}

// TestProjectRepo_SetPinned_TombstoneReturnsGone mirrors the task guard for the
// project pin path so the two stay consistent (both 410 on a tombstone).
func TestProjectRepo_SetPinned_TombstoneReturnsGone(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	if err := f.projects.Delete(ctx, f.projectID); err != nil {
		t.Fatalf("delete project: %v", err)
	}
	if err := f.projects.SetPinned(ctx, f.projectID, true); !errors.Is(err, ErrGone) {
		t.Errorf("pin tombstoned project: got %v, want ErrGone", err)
	}
}

// TestTaskRepo_NotFoundOrGone_DBErrorNotMaskedAsSentinel asserts that a real DB
// failure during tombstone disambiguation surfaces as a non-sentinel error — so
// the PATCH handler returns 500, not a masked 404 (Federation v1 F0.1 follow-up).
func TestTaskRepo_NotFoundOrGone_DBErrorNotMaskedAsSentinel(t *testing.T) {
	f := newTaskFixture(t)
	// Closing the connection forces the disambiguation query to fail.
	if err := f.tasks.db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	err := f.tasks.NotFoundOrGone(context.Background(), 1)
	if err == nil {
		t.Fatal("expected an error after closing the DB, got nil")
	}
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrGone) {
		t.Errorf("DB error masked as sentinel: got %v, want a non-sentinel DB error", err)
	}
}

// TestTaskRepo_CreateRecurrenceCompletionTx_CopiesLabels asserts the tx-aware
// recurring-completion snapshot copies the parent's labels onto the LOCAL row —
// enabling federation must not silently drop labels from the completed-history
// row (labels are local; only the federated EVENT excludes them). Federation v1
// emit-wiring review H1.
func TestTaskRepo_CreateRecurrenceCompletionTx_CopiesLabels(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	d := f.tasks.db
	labels := NewLabelRepo(d)
	tl := NewTaskLabelsRepo(d)

	lbl, err := labels.Create(ctx, "urgent", "red", false)
	if err != nil {
		t.Fatalf("create label: %v", err)
	}
	task, err := f.tasks.Create(ctx, CreateTask{Placement: Placement{ContextID: &f.contextID}, Title: "recurring"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := tl.SetForTask(ctx, task.ID, []int64{lbl.ID}); err != nil {
		t.Fatalf("attach label: %v", err)
	}
	base, err := f.tasks.Get(ctx, task.ID) // hydrates base.Labels
	if err != nil {
		t.Fatalf("get base: %v", err)
	}
	if len(base.Labels) != 1 {
		t.Fatalf("setup: base labels = %d, want 1", len(base.Labels))
	}

	var snapID int64
	if err := withTx(ctx, d, func(tx *sql.Tx) error {
		id, e := f.tasks.CreateRecurrenceCompletionTx(ctx, tx, base, time.Now(), model.NewClientID())
		snapID = id
		return e
	}); err != nil {
		t.Fatalf("create recurrence completion tx: %v", err)
	}

	snap, err := f.tasks.Get(ctx, snapID)
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	if len(snap.Labels) != 1 || snap.Labels[0].ID != lbl.ID {
		t.Errorf("recurrence-completion snapshot labels: got %v, want [%d] (labels must be copied onto the local row)", snap.Labels, lbl.ID)
	}
}

// TestProjectRepo_DeleteTx_TombstoneReturnsGone asserts re-deleting an already
// soft-deleted project surfaces ErrGone (→410), consistent with UpdateTx and the
// task/section delete paths (US-3.7 AC2). Federation v1 emit-wiring review M1.
func TestProjectRepo_DeleteTx_TombstoneReturnsGone(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	if err := f.projects.Delete(ctx, f.projectID); err != nil {
		t.Fatalf("first delete: %v", err)
	}
	if err := f.projects.Delete(ctx, f.projectID); !errors.Is(err, ErrGone) {
		t.Errorf("re-delete tombstoned project: got %v, want ErrGone", err)
	}
}

var _ = model.NewClientID

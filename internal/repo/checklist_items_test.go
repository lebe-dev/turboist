package repo

import (
	"context"
	"errors"
	"testing"
)

type checklistFixture struct {
	f      *taskFixture
	items  *ChecklistItemRepo
	taskID int64
}

func newChecklistFixture(t *testing.T) *checklistFixture {
	t.Helper()
	f := newTaskFixture(t)
	task, err := f.tasks.Create(context.Background(), CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "host task",
	})
	if err != nil {
		t.Fatalf("seed task: %v", err)
	}
	return &checklistFixture{
		f:      f,
		items:  NewChecklistItemRepo(f.tasks.db),
		taskID: task.ID,
	}
}

// TestChecklistItemRepo_Create_AssignsClientIDAndPosition asserts each item
// carries a client_id and gets an appended position (Federation v1 F0.2 schema).
func TestChecklistItemRepo_Create_AssignsClientIDAndPosition(t *testing.T) {
	cf := newChecklistFixture(t)
	ctx := context.Background()
	a, err := cf.items.Create(ctx, cf.taskID, "step a")
	if err != nil {
		t.Fatalf("create a: %v", err)
	}
	b, err := cf.items.Create(ctx, cf.taskID, "step b")
	if err != nil {
		t.Fatalf("create b: %v", err)
	}
	if a.ClientID == "" || b.ClientID == "" {
		t.Errorf("expected non-empty client_id on both items")
	}
	if a.Position != 0 || b.Position != 1 {
		t.Errorf("positions: got a=%d b=%d, want 0 and 1", a.Position, b.Position)
	}
	if a.IsCompleted {
		t.Errorf("new item should default to not completed")
	}
}

// TestChecklistItemRepo_Toggle_IsolatesSiblings asserts toggling one item's
// completion does not affect its siblings (US-3.6 AC1 precursor).
func TestChecklistItemRepo_Toggle_IsolatesSiblings(t *testing.T) {
	cf := newChecklistFixture(t)
	ctx := context.Background()
	a, _ := cf.items.Create(ctx, cf.taskID, "a")
	b, _ := cf.items.Create(ctx, cf.taskID, "b")

	done := true
	updated, err := cf.items.Update(ctx, cf.taskID, a.ID, ChecklistItemUpdate{IsCompleted: &done})
	if err != nil {
		t.Fatalf("toggle a: %v", err)
	}
	if !updated.IsCompleted {
		t.Errorf("item a should be completed after toggle")
	}

	gotB, err := cf.items.Get(ctx, b.ID)
	if err != nil {
		t.Fatalf("get b: %v", err)
	}
	if gotB.IsCompleted {
		t.Errorf("sibling b must remain uncompleted, got completed")
	}
}

// TestChecklistItemRepo_Update_Title asserts the title is mutable (unlike a
// comment body): checklist items are editable sub-todos.
func TestChecklistItemRepo_Update_Title(t *testing.T) {
	cf := newChecklistFixture(t)
	ctx := context.Background()
	it, _ := cf.items.Create(ctx, cf.taskID, "old")
	title := "new"
	updated, err := cf.items.Update(ctx, cf.taskID, it.ID, ChecklistItemUpdate{Title: &title})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Title != "new" {
		t.Errorf("title: got %q, want %q", updated.Title, "new")
	}
}

// TestChecklistItemRepo_Delete_SoftDeleteHidesRow asserts soft-delete keeps the
// physical row and hides it from list/get, and a re-edit of the tombstone is
// ErrGone.
func TestChecklistItemRepo_Delete_SoftDeleteHidesRow(t *testing.T) {
	cf := newChecklistFixture(t)
	ctx := context.Background()
	it, _ := cf.items.Create(ctx, cf.taskID, "doomed")
	if err := cf.items.Delete(ctx, cf.taskID, it.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	var n int
	if err := cf.f.tasks.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM checklist_items WHERE id = ?", it.ID).Scan(&n); err != nil {
		t.Fatalf("raw count: %v", err)
	}
	if n != 1 {
		t.Errorf("physical row after soft-delete: got %d, want 1", n)
	}
	if _, err := cf.items.Get(ctx, it.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get tombstone: got %v, want ErrNotFound", err)
	}
	done := true
	if _, err := cf.items.Update(ctx, cf.taskID, it.ID, ChecklistItemUpdate{IsCompleted: &done}); !errors.Is(err, ErrGone) {
		t.Errorf("update tombstone: got %v, want ErrGone", err)
	}
}

// TestChecklistItemRepo_ListByTask_Ordered asserts items list by position.
func TestChecklistItemRepo_ListByTask_Ordered(t *testing.T) {
	cf := newChecklistFixture(t)
	ctx := context.Background()
	a, _ := cf.items.Create(ctx, cf.taskID, "a")
	b, _ := cf.items.Create(ctx, cf.taskID, "b")

	items, total, err := cf.items.ListByTask(ctx, cf.taskID, Page{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("list count: got total=%d len=%d, want 2/2", total, len(items))
	}
	if items[0].ID != a.ID || items[1].ID != b.ID {
		t.Errorf("order: got [%d,%d], want [%d,%d]", items[0].ID, items[1].ID, a.ID, b.ID)
	}
}

// seedOtherTask creates a second task in the fixture's context so cross-task
// scoping can be exercised.
func (cf *checklistFixture) seedOtherTask(t *testing.T) int64 {
	t.Helper()
	other, err := cf.f.tasks.Create(context.Background(), CreateTask{
		Placement: Placement{ContextID: &cf.f.contextID},
		Title:     "other task",
	})
	if err != nil {
		t.Fatalf("seed other task: %v", err)
	}
	return other.ID
}

// TestChecklistItemRepo_Update_WrongTaskNotModified asserts an item can only be
// patched through its own task's id; a sibling task's id matches no rows and the
// item is left unchanged (Federation v1 F0.2 follow-up).
func TestChecklistItemRepo_Update_WrongTaskNotModified(t *testing.T) {
	cf := newChecklistFixture(t)
	ctx := context.Background()
	otherID := cf.seedOtherTask(t)
	it, _ := cf.items.Create(ctx, cf.taskID, "mine")
	title := "hacked"
	if _, err := cf.items.Update(ctx, otherID, it.ID, ChecklistItemUpdate{Title: &title}); !errors.Is(err, ErrNotFound) {
		t.Errorf("update via wrong task: got %v, want ErrNotFound", err)
	}
	got, err := cf.items.Get(ctx, it.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "mine" {
		t.Errorf("title changed via wrong task: got %q, want %q", got.Title, "mine")
	}
}

// TestChecklistItemRepo_Delete_WrongTaskNotDeleted mirrors the update guard for
// delete: a sibling task's id must not soft-delete the item.
func TestChecklistItemRepo_Delete_WrongTaskNotDeleted(t *testing.T) {
	cf := newChecklistFixture(t)
	ctx := context.Background()
	otherID := cf.seedOtherTask(t)
	it, _ := cf.items.Create(ctx, cf.taskID, "mine")
	if err := cf.items.Delete(ctx, otherID, it.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete via wrong task: got %v, want ErrNotFound", err)
	}
	if _, err := cf.items.Get(ctx, it.ID); err != nil {
		t.Errorf("item must remain live after cross-task delete attempt: %v", err)
	}
}

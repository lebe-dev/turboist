package repo

import (
	"context"
	"errors"
	"testing"
)

// commentFixture provides a CommentRepo wired to a fresh DB with one seeded task.
type commentFixture struct {
	f        *taskFixture
	comments *CommentRepo
	taskID   int64
}

func newCommentFixture(t *testing.T) *commentFixture {
	t.Helper()
	f := newTaskFixture(t)
	task, err := f.tasks.Create(context.Background(), CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "host task",
	})
	if err != nil {
		t.Fatalf("seed task: %v", err)
	}
	return &commentFixture{
		f:        f,
		comments: NewCommentRepo(f.tasks.db),
		taskID:   task.ID,
	}
}

// TestCommentRepo_Create_AssignsClientID asserts every comment carries a
// non-empty client_id and round-trips (Federation v1 F0.2 schema).
func TestCommentRepo_Create_AssignsClientID(t *testing.T) {
	cf := newCommentFixture(t)
	ctx := context.Background()
	c, err := cf.comments.Create(ctx, cf.taskID, "first note")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if c.ClientID == "" {
		t.Errorf("expected non-empty client_id")
	}
	if c.Body != "first note" {
		t.Errorf("body: got %q, want %q", c.Body, "first note")
	}
	got, err := cf.comments.Get(ctx, c.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ClientID != c.ClientID {
		t.Errorf("client_id round-trip: got %q, want %q", got.ClientID, c.ClientID)
	}
}

// TestCommentRepo_NoUpdateMethod_Immutable asserts the comment body cannot be
// updated through the repo — comments are immutable, so federation never merges
// a comment body (US-3.5 AC2). The guard is the absence of an Update method;
// this test documents that contract by exercising the available surface and
// confirming a re-read returns the original body.
func TestCommentRepo_NoUpdateMethod_Immutable(t *testing.T) {
	cf := newCommentFixture(t)
	ctx := context.Background()
	c, err := cf.comments.Create(ctx, cf.taskID, "immutable")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := cf.comments.Get(ctx, c.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Body != "immutable" {
		t.Errorf("body changed unexpectedly: got %q, want %q", got.Body, "immutable")
	}
}

// TestCommentRepo_ListByTask_Ordered asserts two comments are returned in
// creation order (oldest first), the precursor to US-3.5 AC3.
func TestCommentRepo_ListByTask_Ordered(t *testing.T) {
	cf := newCommentFixture(t)
	ctx := context.Background()
	first, err := cf.comments.Create(ctx, cf.taskID, "older")
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := cf.comments.Create(ctx, cf.taskID, "newer")
	if err != nil {
		t.Fatalf("create second: %v", err)
	}

	items, total, err := cf.comments.ListByTask(ctx, cf.taskID, Page{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("list count: got total=%d len=%d, want 2/2", total, len(items))
	}
	if items[0].ID != first.ID || items[1].ID != second.ID {
		t.Errorf("order: got [%d,%d], want [%d,%d]", items[0].ID, items[1].ID, first.ID, second.ID)
	}
}

// TestCommentRepo_Delete_SoftDeleteHidesRow asserts Delete sets the tombstone
// but keeps the physical row and excludes it from list/get.
func TestCommentRepo_Delete_SoftDeleteHidesRow(t *testing.T) {
	cf := newCommentFixture(t)
	ctx := context.Background()
	c, err := cf.comments.Create(ctx, cf.taskID, "doomed")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := cf.comments.Delete(ctx, cf.taskID, c.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	var n int
	if err := cf.f.tasks.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM comments WHERE id = ?", c.ID).Scan(&n); err != nil {
		t.Fatalf("raw count: %v", err)
	}
	if n != 1 {
		t.Errorf("physical comment row after soft-delete: got %d, want 1", n)
	}
	if _, err := cf.comments.Get(ctx, c.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get tombstone: got %v, want ErrNotFound", err)
	}
	_, total, err := cf.comments.ListByTask(ctx, cf.taskID, Page{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 0 {
		t.Errorf("list after soft-delete: got total=%d, want 0", total)
	}
}

// TestCommentRepo_Delete_DoubleDeleteGone asserts deleting an already-tombstoned
// comment surfaces ErrGone (the tombstone is final).
func TestCommentRepo_Delete_DoubleDeleteGone(t *testing.T) {
	cf := newCommentFixture(t)
	ctx := context.Background()
	c, err := cf.comments.Create(ctx, cf.taskID, "x")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := cf.comments.Delete(ctx, cf.taskID, c.ID); err != nil {
		t.Fatalf("first delete: %v", err)
	}
	if err := cf.comments.Delete(ctx, cf.taskID, c.ID); !errors.Is(err, ErrGone) {
		t.Errorf("second delete: got %v, want ErrGone", err)
	}
}

// TestCommentRepo_Delete_WrongTaskNotDeleted asserts a comment can only be
// deleted through its OWN task's id: deleting it via a sibling task's id matches
// no rows (ErrNotFound) and leaves the comment live (Federation v1 F0.2
// follow-up — sub-resource mutations are scoped by parent task).
func TestCommentRepo_Delete_WrongTaskNotDeleted(t *testing.T) {
	cf := newCommentFixture(t)
	ctx := context.Background()
	other, err := cf.f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &cf.f.contextID},
		Title:     "other task",
	})
	if err != nil {
		t.Fatalf("seed other task: %v", err)
	}
	c, err := cf.comments.Create(ctx, cf.taskID, "mine")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := cf.comments.Delete(ctx, other.ID, c.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete via wrong task: got %v, want ErrNotFound", err)
	}
	if _, err := cf.comments.Get(ctx, c.ID); err != nil {
		t.Errorf("comment must remain live after cross-task delete attempt: %v", err)
	}
}

package repo

import (
	"context"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

// TestOfflineSync_TaskClientIDRoundTrip ensures the client_id column is
// persisted on insert and read back by scanTask. Without round-trip support the
// frontend can't bind its locally-minted ulid to the server-assigned PK during
// outbox flush.
func TestOfflineSync_TaskClientIDRoundTrip(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	cid := "01HXYZTASK000000000000000"
	tk, err := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "with-cid",
		ClientID:  &cid,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if tk.ClientID == nil || *tk.ClientID != cid {
		t.Errorf("client_id round-trip: got %v, want %q", tk.ClientID, cid)
	}
}

// TestOfflineSync_TaskClientIDUnique asserts the partial unique index prevents
// duplicate client_ids — a retry of the same outbox row must not produce two
// server rows.
func TestOfflineSync_TaskClientIDUnique(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	cid := "01HXYZTASKDUPLICATE000000"
	if _, err := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "first",
		ClientID:  &cid,
	}); err != nil {
		t.Fatalf("first: %v", err)
	}
	_, err := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "second",
		ClientID:  &cid,
	})
	if err == nil {
		t.Fatalf("expected unique violation, got nil")
	}
}

// TestOfflineSync_TaskListsHideTombstones simulates a soft-deleted task and
// verifies that listing endpoints exclude it. Task 2 wires DELETE to soft-
// delete; this test pins the filter in place ahead of that change so the
// invariant ("lists hide tombstones") survives the wiring.
func TestOfflineSync_TaskListsHideTombstones(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	live, err := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "live",
	})
	if err != nil {
		t.Fatalf("live: %v", err)
	}
	dead, err := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "dead",
	})
	if err != nil {
		t.Fatalf("dead: %v", err)
	}
	if _, err := f.tasks.db.ExecContext(ctx,
		`UPDATE tasks SET deleted_at = ? WHERE id = ?`,
		model.FormatUTC(time.Now()), dead.ID); err != nil {
		t.Fatalf("tombstone: %v", err)
	}

	items, total, err := f.tasks.ListByContext(ctx, f.contextID, true, TaskFilter{}, Page{Limit: 100})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 {
		t.Errorf("total: got %d, want 1", total)
	}
	if len(items) != 1 || items[0].ID != live.ID {
		t.Errorf("items: got %+v, want only live id=%d", items, live.ID)
	}

	if n, err := f.tasks.CountInbox(ctx); err != nil {
		t.Fatalf("count inbox: %v", err)
	} else if n != 0 {
		t.Errorf("count inbox: got %d, want 0", n)
	}
}

// TestOfflineSync_TaskGetReturnsTombstone keeps Get visible past tombstoning
// because Task 2's PATCH-on-tombstone → 410 Gone path needs to read the row.
func TestOfflineSync_TaskGetReturnsTombstone(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	tk, err := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "ghost",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := f.tasks.db.ExecContext(ctx,
		`UPDATE tasks SET deleted_at = ? WHERE id = ?`,
		model.FormatUTC(time.Now()), tk.ID); err != nil {
		t.Fatalf("tombstone: %v", err)
	}
	got, err := f.tasks.Get(ctx, tk.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.DeletedAt == nil {
		t.Errorf("expected DeletedAt set, got nil")
	}
}

// TestOfflineSync_ProjectListsHideTombstones covers projects.
func TestOfflineSync_ProjectListsHideTombstones(t *testing.T) {
	d := setupTestDB(t)
	cr := NewContextRepo(d)
	pr := NewProjectRepo(d, NewProjectLabelsRepo(d))
	ctx := context.Background()
	c, err := cr.Create(ctx, "work", "blue", false)
	if err != nil {
		t.Fatalf("ctx: %v", err)
	}
	live, err := pr.Create(ctx, CreateProject{ContextID: c.ID, Title: "live", Color: "blue"})
	if err != nil {
		t.Fatalf("live: %v", err)
	}
	dead, err := pr.Create(ctx, CreateProject{ContextID: c.ID, Title: "dead", Color: "red"})
	if err != nil {
		t.Fatalf("dead: %v", err)
	}
	if _, err := d.ExecContext(ctx,
		`UPDATE projects SET deleted_at = ? WHERE id = ?`,
		model.FormatUTC(time.Now()), dead.ID); err != nil {
		t.Fatalf("tombstone: %v", err)
	}
	items, total, err := pr.List(ctx, ProjectListFilter{}, Page{Limit: 100})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].ID != live.ID {
		t.Errorf("expected only live (id=%d), got total=%d items=%+v", live.ID, total, items)
	}
}

// TestOfflineSync_LabelListsHideTombstones covers labels.
func TestOfflineSync_LabelListsHideTombstones(t *testing.T) {
	d := setupTestDB(t)
	lr := NewLabelRepo(d)
	ctx := context.Background()
	live, err := lr.Create(ctx, "live", "blue", false)
	if err != nil {
		t.Fatalf("live: %v", err)
	}
	dead, err := lr.Create(ctx, "dead", "red", false)
	if err != nil {
		t.Fatalf("dead: %v", err)
	}
	if _, err := d.ExecContext(ctx,
		`UPDATE labels SET deleted_at = ? WHERE id = ?`,
		model.FormatUTC(time.Now()), dead.ID); err != nil {
		t.Fatalf("tombstone: %v", err)
	}
	items, total, err := lr.List(ctx, LabelListFilter{}, Page{Limit: 100})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].ID != live.ID {
		t.Errorf("expected only live (id=%d), got total=%d items=%+v", live.ID, total, items)
	}
}

// TestOfflineSync_ContextListsHideTombstones covers contexts.
func TestOfflineSync_ContextListsHideTombstones(t *testing.T) {
	d := setupTestDB(t)
	cr := NewContextRepo(d)
	ctx := context.Background()
	live, err := cr.Create(ctx, "live", "blue", false)
	if err != nil {
		t.Fatalf("live: %v", err)
	}
	dead, err := cr.Create(ctx, "dead", "red", false)
	if err != nil {
		t.Fatalf("dead: %v", err)
	}
	if _, err := d.ExecContext(ctx,
		`UPDATE contexts SET deleted_at = ? WHERE id = ?`,
		model.FormatUTC(time.Now()), dead.ID); err != nil {
		t.Fatalf("tombstone: %v", err)
	}
	items, total, err := cr.List(ctx, Page{Limit: 100})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].ID != live.ID {
		t.Errorf("expected only live (id=%d), got total=%d items=%+v", live.ID, total, items)
	}
}

// TestOfflineSync_SectionListsHideTombstones covers project_sections.
func TestOfflineSync_SectionListsHideTombstones(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	dead, err := f.sections.Create(ctx, f.projectID, "dead-section")
	if err != nil {
		t.Fatalf("dead: %v", err)
	}
	if _, err := f.tasks.db.ExecContext(ctx,
		`UPDATE project_sections SET deleted_at = ? WHERE id = ?`,
		model.FormatUTC(time.Now()), dead.ID); err != nil {
		t.Fatalf("tombstone: %v", err)
	}
	items, total, err := f.sections.ListByProject(ctx, f.projectID, Page{Limit: 100})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// fixture creates one live section already
	if total != 1 || len(items) != 1 {
		t.Errorf("expected only live section, got total=%d items=%+v", total, items)
	}
	if items[0].ID == dead.ID {
		t.Errorf("dead section returned by list")
	}
}

// TestOfflineSync_SectionClientIDRoundTrip ensures section CreateWithClientID
// stores and reads the client id.
func TestOfflineSync_SectionClientIDRoundTrip(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()
	cid := "01HXYZSECTION000000000000"
	s, err := f.sections.CreateWithClientID(ctx, f.projectID, "with-cid", &cid)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if s.ClientID == nil || *s.ClientID != cid {
		t.Errorf("client_id: got %v, want %q", s.ClientID, cid)
	}
}

// TestOfflineSync_LabelClientIDRoundTrip covers labels.
func TestOfflineSync_LabelClientIDRoundTrip(t *testing.T) {
	d := setupTestDB(t)
	lr := NewLabelRepo(d)
	cid := "01HXYZLABEL00000000000000"
	l, err := lr.CreateWithClientID(context.Background(), "tag", "blue", false, &cid)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if l.ClientID == nil || *l.ClientID != cid {
		t.Errorf("client_id: got %v, want %q", l.ClientID, cid)
	}
}

// TestOfflineSync_ContextClientIDRoundTrip covers contexts.
func TestOfflineSync_ContextClientIDRoundTrip(t *testing.T) {
	d := setupTestDB(t)
	cr := NewContextRepo(d)
	cid := "01HXYZCONTEXT0000000000000"
	c, err := cr.CreateWithClientID(context.Background(), "ctx", "blue", false, &cid)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if c.ClientID == nil || *c.ClientID != cid {
		t.Errorf("client_id: got %v, want %q", c.ClientID, cid)
	}
}

// TestOfflineSync_ProjectClientIDRoundTrip covers projects.
func TestOfflineSync_ProjectClientIDRoundTrip(t *testing.T) {
	d := setupTestDB(t)
	cr := NewContextRepo(d)
	pr := NewProjectRepo(d, NewProjectLabelsRepo(d))
	ctx := context.Background()
	c, err := cr.Create(ctx, "work", "blue", false)
	if err != nil {
		t.Fatalf("ctx: %v", err)
	}
	cid := "01HXYZPROJECT0000000000000"
	p, err := pr.Create(ctx, CreateProject{ContextID: c.ID, Title: "p", Color: "blue", ClientID: &cid})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if p.ClientID == nil || *p.ClientID != cid {
		t.Errorf("client_id: got %v, want %q", p.ClientID, cid)
	}
}

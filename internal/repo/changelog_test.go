package repo

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/lebe-dev/turboist/internal/model"
)

func maxChangeSeq(t *testing.T, d *sql.DB) int64 {
	t.Helper()
	var seq sql.NullInt64
	if err := d.QueryRow(`SELECT MAX(seq) FROM change_log`).Scan(&seq); err != nil {
		t.Fatalf("read max seq: %v", err)
	}
	return seq.Int64
}

func TestChangeLogChanges_EmptyLogOnFreshDatabase(t *testing.T) {
	d := setupTestDB(t)
	page, err := NewChangeLogRepo(d).Changes(context.Background(), SyncChangesQuery{})
	if err != nil {
		t.Fatalf("changes: %v", err)
	}
	if len(page.Changes) != 0 {
		t.Errorf("changes: got %d, want 0", len(page.Changes))
	}
	if page.Cursor != 0 {
		t.Errorf("cursor: got %d, want 0", page.Cursor)
	}
	if page.HasMore {
		t.Error("hasMore is true on an empty log")
	}
	if page.Epoch != 1 {
		t.Errorf("epoch: got %d, want 1", page.Epoch)
	}
}

// With nothing new to report the cursor must stay where the caller left it,
// rather than resetting and making the next call re-read the whole window.
func TestChangeLogChanges_CursorHoldsWhenNothingChanged(t *testing.T) {
	d := setupTestDB(t)
	ctx := context.Background()
	if _, err := NewLabelRepo(d).Create(ctx, "urgent", "red", false); err != nil {
		t.Fatalf("create label: %v", err)
	}
	head := maxChangeSeq(t, d)

	page, err := NewChangeLogRepo(d).Changes(ctx, SyncChangesQuery{Since: head})
	if err != nil {
		t.Fatalf("changes: %v", err)
	}
	if len(page.Changes) != 0 {
		t.Errorf("changes: got %d, want 0", len(page.Changes))
	}
	if page.Cursor != head {
		t.Errorf("cursor: got %d, want %d", page.Cursor, head)
	}
}

// The op that survives dedupe is the one from the highest seq, not the first or
// the loudest: a row deleted and then recreated under the same id is present.
func TestChangeLogChanges_KeepsOpOfHighestSeq(t *testing.T) {
	d := setupTestDB(t)
	ctx := context.Background()
	labels := NewLabelRepo(d)
	l, err := labels.Create(ctx, "urgent", "red", false)
	if err != nil {
		t.Fatalf("create label: %v", err)
	}
	if err := labels.Delete(ctx, l.ID); err != nil {
		t.Fatalf("delete label: %v", err)
	}
	if _, err := d.Exec(
		`INSERT INTO labels (id, name, color, is_favourite, is_private, created_at, updated_at)
		 VALUES (?, 'urgent', 'red', 0, 0, ?, ?)`,
		l.ID, model.FormatUTC(l.CreatedAt), model.FormatUTC(l.UpdatedAt)); err != nil {
		t.Fatalf("recreate label: %v", err)
	}

	page, err := NewChangeLogRepo(d).Changes(ctx, SyncChangesQuery{})
	if err != nil {
		t.Fatalf("changes: %v", err)
	}
	var seen int
	for _, ch := range page.Changes {
		if ch.Entity != SyncEntityLabel || ch.EntityID != l.ID {
			continue
		}
		seen++
		if ch.Op != SyncOpUpsert {
			t.Errorf("op: got %q, want %q", ch.Op, SyncOpUpsert)
		}
		if ch.Payload == nil {
			t.Error("upsert carried no payload")
		}
	}
	if seen != 1 {
		t.Errorf("label changes: got %d, want 1", seen)
	}
}

func TestChangeLogChanges_SectionAndTemplatePayloads(t *testing.T) {
	d := setupTestDB(t)
	ctx := context.Background()
	ctxRepo := NewContextRepo(d)
	c, err := ctxRepo.Create(ctx, "Work", "blue", false)
	if err != nil {
		t.Fatalf("create context: %v", err)
	}
	p, err := NewProjectRepo(d, NewProjectLabelsRepo(d)).Create(ctx, CreateProject{ContextID: c.ID, Title: "Website", Color: "blue"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	s, err := NewProjectSectionRepo(d).Create(ctx, p.ID, "Backlog")
	if err != nil {
		t.Fatalf("create section: %v", err)
	}
	tpl, err := NewTemplateRepo(d).Create(ctx, TemplateInput{
		Name:     "Weekly review",
		Subtasks: []TemplateSubtaskInput{{Title: "Clear inbox"}},
	})
	if err != nil {
		t.Fatalf("create template: %v", err)
	}

	page, err := NewChangeLogRepo(d).Changes(ctx, SyncChangesQuery{})
	if err != nil {
		t.Fatalf("changes: %v", err)
	}
	payloads := map[string]any{}
	for _, ch := range page.Changes {
		payloads[ch.Entity] = ch.Payload
	}

	section, ok := payloads[SyncEntitySection].(*model.ProjectSection)
	if !ok {
		t.Fatalf("section payload: got %T", payloads[SyncEntitySection])
	}
	if section.ID != s.ID || section.Title != "Backlog" {
		t.Errorf("section: got %d/%q, want %d/Backlog", section.ID, section.Title, s.ID)
	}

	template, ok := payloads[SyncEntityTaskTemplate].(*model.TaskTemplate)
	if !ok {
		t.Fatalf("template payload: got %T", payloads[SyncEntityTaskTemplate])
	}
	if template.ID != tpl.ID {
		t.Errorf("template id: got %d, want %d", template.ID, tpl.ID)
	}
	// The subtasks come from the same read, so a template never arrives as a
	// bare header the client would have to fetch the body for.
	if len(template.Subtasks) != 1 || template.Subtasks[0].Title != "Clear inbox" {
		t.Errorf("template subtasks: got %+v, want one titled 'Clear inbox'", template.Subtasks)
	}
}

// Pruning the log down to nothing still leaves a boundary: a caller that had
// already seen everything is up to date, an older one has lost changes it can
// never be handed again.
func TestChangeLogChanges_EmptiedLogKeepsExpiryBoundary(t *testing.T) {
	d := setupTestDB(t)
	ctx := context.Background()
	if _, err := NewLabelRepo(d).Create(ctx, "urgent", "red", false); err != nil {
		t.Fatalf("create label: %v", err)
	}
	head := maxChangeSeq(t, d)
	if _, err := d.Exec(`DELETE FROM change_log`); err != nil {
		t.Fatalf("prune log: %v", err)
	}
	r := NewChangeLogRepo(d)

	if _, err := r.Changes(ctx, SyncChangesQuery{Since: head}); err != nil {
		t.Errorf("cursor at the head should still be valid: %v", err)
	}

	var expired *SyncCursorExpiredError
	_, err := r.Changes(ctx, SyncChangesQuery{Since: head - 1})
	if !errors.As(err, &expired) {
		t.Fatalf("older cursor: got %v, want a cursor-expired error", err)
	}
	if expired.OldestRetained != head+1 {
		t.Errorf("oldestRetained: got %d, want %d", expired.OldestRetained, head+1)
	}
}

func TestChangeLogChanges_EpochMismatchCarriesCurrentEpoch(t *testing.T) {
	d := setupTestDB(t)
	ctx := context.Background()
	current, err := NewAppSettingsRepo(d).BumpSyncEpoch(ctx)
	if err != nil {
		t.Fatalf("bump epoch: %v", err)
	}
	stale := current - 1

	var mismatch *SyncEpochMismatchError
	_, err = NewChangeLogRepo(d).Changes(ctx, SyncChangesQuery{Epoch: &stale})
	if !errors.As(err, &mismatch) {
		t.Fatalf("got %v, want an epoch-mismatch error", err)
	}
	if mismatch.Current != current {
		t.Errorf("current epoch: got %d, want %d", mismatch.Current, current)
	}
}

// A limit past the cap is clamped rather than honoured, so no caller can ask the
// server to hydrate an unbounded page.
func TestChangeLogChanges_ClampsLimitToMaximum(t *testing.T) {
	d := setupTestDB(t)
	ctx := context.Background()
	labels := NewLabelRepo(d)
	for i := range 3 {
		if _, err := labels.Create(ctx, string(rune('a'+i)), "red", false); err != nil {
			t.Fatalf("create label: %v", err)
		}
	}
	page, err := NewChangeLogRepo(d).Changes(ctx, SyncChangesQuery{Limit: MaxSyncChangeLimit * 10})
	if err != nil {
		t.Fatalf("changes: %v", err)
	}
	if len(page.Changes) != 3 {
		t.Errorf("changes: got %d, want 3", len(page.Changes))
	}
	if page.HasMore {
		t.Error("hasMore is true although every change fit")
	}
}

// An entity the reader does not know how to serve must fail the page. Hydrating
// it as "no such row" would turn it into a tombstone and tell the replica to
// delete data that is perfectly alive.
func TestChangeLogChanges_RejectsUnknownEntity(t *testing.T) {
	d := setupTestDB(t)
	if _, err := d.Exec(
		`INSERT INTO change_log (entity, entity_id, op, changed_at) VALUES ('gizmo', 1, 'upsert', '2026-01-01T00:00:00.000Z')`); err != nil {
		t.Fatalf("insert change: %v", err)
	}
	if _, err := NewChangeLogRepo(d).Changes(context.Background(), SyncChangesQuery{}); err == nil {
		t.Fatal("expected an error for an unknown entity")
	}
}

// The log stores pointers, never payloads, so a pointer can outlive the row it
// names: an edge trigger logs an upsert of its owner, and the owner can be gone
// by the time the page is read — a cascade removes rows without logging a
// delete for each one. An upsert with nothing behind it would tell a replica to
// keep a row the server no longer has, so it must be served as a tombstone.
func TestChangeLogChanges_DanglingUpsertCollapsesToDelete(t *testing.T) {
	d := setupTestDB(t)
	ctx := context.Background()
	labels := NewLabelRepo(d)
	l, err := labels.Create(ctx, "urgent", "red", false)
	if err != nil {
		t.Fatalf("create label: %v", err)
	}
	if _, err := d.Exec(`DELETE FROM labels WHERE id = ?`, l.ID); err != nil {
		t.Fatalf("delete label row: %v", err)
	}
	// Drop the tombstone the delete trigger wrote, leaving only the upsert that
	// preceded it — the state a cascade or a missing trigger guard produces.
	if _, err := d.Exec(
		`DELETE FROM change_log WHERE entity = ? AND entity_id = ? AND op = ?`,
		SyncEntityLabel, l.ID, SyncOpDelete); err != nil {
		t.Fatalf("drop tombstone: %v", err)
	}

	page, err := NewChangeLogRepo(d).Changes(ctx, SyncChangesQuery{})
	if err != nil {
		t.Fatalf("changes: %v", err)
	}
	var seen int
	for _, ch := range page.Changes {
		if ch.Entity != SyncEntityLabel || ch.EntityID != l.ID {
			continue
		}
		seen++
		if ch.Op != SyncOpDelete {
			t.Errorf("op: got %q, want %q", ch.Op, SyncOpDelete)
		}
		if ch.Payload != nil {
			t.Errorf("payload: got %v, want nil", ch.Payload)
		}
	}
	if seen != 1 {
		t.Errorf("label changes: got %d, want 1", seen)
	}
}

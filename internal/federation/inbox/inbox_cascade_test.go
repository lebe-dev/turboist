package inbox_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/lebe-dev/turboist/internal/federation/events"
)

// seedChild inserts a live comment + checklist item on the env's task and returns
// their ids, so a cascade-delete test can assert they become tombstones.
func seedChildComment(t *testing.T, env *applyEnv, body, clientID string) int64 {
	t.Helper()
	now := now()
	res, err := env.db.Exec(
		`INSERT INTO comments (task_id, body, client_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		env.localTaskID, body, clientID, now, now)
	if err != nil {
		t.Fatalf("seed comment: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func seedChildChecklist(t *testing.T, env *applyEnv, title, clientID string) int64 {
	t.Helper()
	now := now()
	res, err := env.db.Exec(
		`INSERT INTO checklist_items (task_id, title, is_completed, position, client_id, created_at, updated_at) VALUES (?, ?, 0, 0, ?, ?, ?)`,
		env.localTaskID, title, clientID, now, now)
	if err != nil {
		t.Fatalf("seed checklist: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func now() string {
	return "2026-06-01T10:00:00.000Z"
}

func childDeletedAt(t *testing.T, env *applyEnv, table string, id int64) sql.NullString {
	t.Helper()
	var del sql.NullString
	if err := env.db.QueryRow(`SELECT deleted_at FROM `+table+` WHERE id = ?`, id).Scan(&del); err != nil {
		t.Fatalf("scan %s deleted_at: %v", table, id)
	}
	return del
}

// TestApply_DeleteCascadesChildrenWithoutReEmit asserts that applying an op=delete
// for a TASK soft-deletes the task's local comments + checklist_items as well, so
// a receiver never shows orphan children (§8.4, US-3.7 AC3). Crucially, the
// cascade is LOCAL ONLY — the receiver writes NO new outbox events for the children
// (the origin emits those explicitly; re-emitting here would create echo loops).
func TestApply_DeleteCascadesChildrenWithoutReEmit(t *testing.T) {
	env := newApplyEnv(t)
	ctx := context.Background()

	commentID := seedChildComment(t, env, "note", "comment-1")
	checklistID := seedChildChecklist(t, env, "step", "checklist-1")

	del := events.Event{
		EventID:         "ev-delete-cascade",
		Op:              events.OpDelete,
		EntityType:      events.EntityTask,
		EntityID:        env.taskClientID,
		ProjectClientID: env.projectClient,
		Author:          "https://alice.example",
		OriginInstance:  "https://alice.example",
		CreatedAt:       "2026-06-01T10:00:00.000Z",
		Fields: map[string]events.Field{
			"_deleted": {Value: true, HLC: "00000000000800-0000-nodeA"},
		},
	}
	if _, err := env.applier.Apply(ctx, del, "https://alice.example"); err != nil {
		t.Fatalf("apply delete: %v", err)
	}

	if !childDeletedAt(t, env, "comments", commentID).Valid {
		t.Errorf("task delete must cascade-tombstone its comments (US-3.7 AC3)")
	}
	if !childDeletedAt(t, env, "checklist_items", checklistID).Valid {
		t.Errorf("task delete must cascade-tombstone its checklist items (US-3.7 AC3)")
	}

	// Receiver MUST NOT re-emit child-delete events (no outbox rows written by apply).
	var outboxCount int
	if err := env.db.QueryRow(`SELECT COUNT(1) FROM federation_outbox`).Scan(&outboxCount); err != nil {
		t.Fatalf("count outbox: %v", err)
	}
	if outboxCount != 0 {
		t.Errorf("receiver cascade must not write outbox events: got %d", outboxCount)
	}
}

// TestApply_StaleUpdateOnTombstoneIgnored asserts resurrection prevention: once a
// task is soft-deleted (tombstone + _deleted field HLC), a LATER op=update at a
// LOWER (stale) HLC must NOT bring it back (US-3.7 AC2). The row stays deleted and
// the stale field is not written.
func TestApply_StaleUpdateOnTombstoneIgnored(t *testing.T) {
	env := newApplyEnv(t)
	ctx := context.Background()

	del := events.Event{
		EventID:         "ev-del",
		Op:              events.OpDelete,
		EntityType:      events.EntityTask,
		EntityID:        env.taskClientID,
		ProjectClientID: env.projectClient,
		Author:          "https://alice.example",
		OriginInstance:  "https://alice.example",
		CreatedAt:       "2026-06-01T10:00:00.000Z",
		Fields:          map[string]events.Field{"_deleted": {Value: true, HLC: "00000000000800-0000-nodeA"}},
	}
	if _, err := env.applier.Apply(ctx, del, "https://alice.example"); err != nil {
		t.Fatalf("apply delete: %v", err)
	}

	// A stale update (lower HLC than the delete) arrives late.
	stale := updateEvent(env, map[string]events.Field{
		"title": {Value: "resurrected?", HLC: "00000000000500-0000-nodeA"},
	})
	stale.EventID = eventID("stale-after-delete")
	if _, err := env.applier.Apply(ctx, stale, "https://alice.example"); err != nil {
		t.Fatalf("apply stale update: %v", err)
	}

	var del2 sql.NullString
	if err := env.db.QueryRow(`SELECT deleted_at FROM tasks WHERE id = ?`, env.localTaskID).Scan(&del2); err != nil {
		t.Fatalf("scan deleted_at: %v", err)
	}
	if !del2.Valid {
		t.Errorf("stale update must not resurrect a tombstoned task (US-3.7 AC2)")
	}
	var title string
	if err := env.db.QueryRow(`SELECT title FROM tasks WHERE id = ?`, env.localTaskID).Scan(&title); err != nil {
		t.Fatalf("scan title: %v", err)
	}
	if title == "resurrected?" {
		t.Errorf("stale field must not be written on a tombstone: got title %q", title)
	}
}

// TestApply_OrphanDeleteWritesProtectiveTombstone asserts the §10.4 orphan case:
// an op=delete arriving for an entity the receiver has NEVER seen still records a
// protective _deleted field HLC, so a LATER (lower-HLC) op=create cannot
// materialise the entity (the tombstone wins per-field LWW). The orphan-event
// ghost stays deleted (F3.3 "orphan-event ghost stays deleted").
func TestApply_OrphanDeleteWritesProtectiveTombstone(t *testing.T) {
	env := newApplyEnv(t)
	ctx := context.Background()

	orphan := "task-never-seen"
	del := events.Event{
		EventID:         "ev-orphan-del",
		Op:              events.OpDelete,
		EntityType:      events.EntityTask,
		EntityID:        orphan,
		ProjectClientID: env.projectClient,
		Author:          "https://alice.example",
		OriginInstance:  "https://alice.example",
		CreatedAt:       "2026-06-01T10:00:00.000Z",
		Fields:          map[string]events.Field{"_deleted": {Value: true, HLC: "00000000000800-0000-nodeA"}},
	}
	if _, err := env.applier.Apply(ctx, del, "https://alice.example"); err != nil {
		t.Fatalf("apply orphan delete: %v", err)
	}

	// The protective tombstone HLC is recorded even though no local row existed.
	tomb, err := env.store.GetFieldHLC(ctx, "task", orphan, "_deleted")
	if err != nil {
		t.Fatalf("get _deleted hlc: %v", err)
	}
	if tomb != "00000000000800-0000-nodeA" {
		t.Errorf("orphan delete must record a protective _deleted HLC: got %q", tomb)
	}

	// A LATER op=create at a LOWER HLC must lose to the tombstone (no resurrection).
	create := events.Event{
		EventID:         "ev-orphan-create",
		Op:              events.OpCreate,
		EntityType:      events.EntityTask,
		EntityID:        orphan,
		ProjectClientID: env.projectClient,
		Author:          "https://alice.example",
		OriginInstance:  "https://alice.example",
		CreatedAt:       "2026-06-01T10:00:00.000Z",
		Fields:          map[string]events.Field{"title": {Value: "late create", HLC: "00000000000500-0000-nodeA"}},
	}
	if _, err := env.applier.Apply(ctx, create, "https://alice.example"); err != nil {
		t.Fatalf("apply orphan create: %v", err)
	}

	// The ghost row exists (created by op=create) but stays deleted because the
	// protective tombstone HLC is higher than the create's title HLC — its
	// deleted_at must be set by the cascade-aware ghost path.
	var del2 sql.NullString
	err = env.db.QueryRow(`SELECT deleted_at FROM tasks WHERE client_id = ?`, orphan).Scan(&del2)
	if err == sql.ErrNoRows {
		// Acceptable: no row materialised at all (the create lost entirely). Either
		// "no row" or "row but deleted" satisfies no-resurrection.
		return
	}
	if err != nil {
		t.Fatalf("scan orphan deleted_at: %v", err)
	}
	if !del2.Valid {
		t.Errorf("orphan ghost must stay deleted after a lower-HLC create (no resurrection)")
	}
}

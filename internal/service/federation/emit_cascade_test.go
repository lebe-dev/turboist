package federation_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/lebe-dev/turboist/internal/federation/events"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

// seedComment / seedChecklist add a live child to a task so the cascade emit has
// children to tombstone (the origin side of §8.4).
func seedComment(t *testing.T, d *sql.DB, taskID int64, clientID string) {
	t.Helper()
	if _, err := d.Exec(
		`INSERT INTO comments (task_id, body, client_id, created_at, updated_at) VALUES (?, 'note', ?, '2026-06-01T00:00:00.000Z', '2026-06-01T00:00:00.000Z')`,
		taskID, clientID); err != nil {
		t.Fatalf("seed comment: %v", err)
	}
}

func seedChecklist(t *testing.T, d *sql.DB, taskID int64, clientID string) {
	t.Helper()
	if _, err := d.Exec(
		`INSERT INTO checklist_items (task_id, title, is_completed, position, client_id, created_at, updated_at) VALUES (?, 'step', 0, 0, ?, '2026-06-01T00:00:00.000Z', '2026-06-01T00:00:00.000Z')`,
		taskID, clientID); err != nil {
		t.Fatalf("seed checklist: %v", err)
	}
}

// TestEmitDeleteCascade_TaskTombstonesChildrenInOneTx asserts the §8.4 origin
// cascade: deleting a federated task emits the task's op=delete tombstone AND one
// op=delete tombstone per child comment / checklist item, ALL in the same outbox
// transaction (US-3.7 AC3 emit side; crash between parent/child mitigated by the
// single tx). The children are also soft-deleted locally by the write closure.
func TestEmitDeleteCascade_TaskTombstonesChildrenInOneTx(t *testing.T) {
	env := newEmitEnv(t)
	ctx := context.Background()

	seedComment(t, env.db, env.fedTaskID, "comment-A")
	seedChecklist(t, env.db, env.fedTaskID, "checklist-A")

	taskClient := taskClientID(t, env.db, env.fedTaskID)
	children := []fedsvc.ChildTombstone{
		{EntityType: events.EntityComment, EntityID: "comment-A"},
		{EntityType: events.EntityChecklistItem, EntityID: "checklist-A"},
	}
	err := env.emitter.EmitDeleteCascade(ctx, fedsvc.MutationSpec{
		LocalProjectID: env.fedProject,
		EntityType:     events.EntityTask,
		EntityID:       taskClient,
		Op:             events.OpDelete,
	}, children, func(tx *sql.Tx) error {
		now := "2026-06-02T00:00:00.000Z"
		if _, err := tx.ExecContext(ctx, `UPDATE tasks SET deleted_at = ?, updated_at = ? WHERE id = ?`, now, now, env.fedTaskID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE comments SET deleted_at = ?, updated_at = ? WHERE task_id = ?`, now, now, env.fedTaskID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE checklist_items SET deleted_at = ?, updated_at = ? WHERE task_id = ?`, now, now, env.fedTaskID)
		return err
	})
	if err != nil {
		t.Fatalf("emit delete cascade: %v", err)
	}

	// One event per (task + 2 children) = 3 outbox rows, all op=delete.
	if got := outboxCount(t, env.db, env.fedProject); got != 3 {
		t.Errorf("cascade outbox count: got %d, want 3 (task + comment + checklist)", got)
	}

	rows, err := env.db.Query(`SELECT payload FROM federation_outbox WHERE local_project_id = ? ORDER BY id ASC`, env.fedProject)
	if err != nil {
		t.Fatalf("read outbox: %v", err)
	}
	defer func() { _ = rows.Close() }()
	seen := map[string]string{} // entity_id -> op
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			t.Fatalf("scan: %v", err)
		}
		var e events.Event
		if err := events.Unmarshal([]byte(payload), &e); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if e.Signature == "" {
			t.Errorf("cascade event %s must be signed", e.EntityID)
		}
		if f, ok := e.Fields[events.FieldDeleted]; !ok || f.HLC == "" {
			t.Errorf("cascade event %s must carry a _deleted field HLC", e.EntityID)
		}
		seen[e.EntityID] = string(e.Op)
	}
	for _, id := range []string{taskClient, "comment-A", "checklist-A"} {
		if seen[id] != string(events.OpDelete) {
			t.Errorf("missing op=delete event for %s: got %q", id, seen[id])
		}
	}

	// The children carry their own _deleted field HLC in entity_field_hlc so a
	// receiver resolves the tombstone per-field LWW.
	for _, c := range []struct{ typ, id string }{{"comment", "comment-A"}, {"checklist_item", "checklist-A"}} {
		var n int
		if err := env.db.QueryRow(
			`SELECT COUNT(1) FROM entity_field_hlc WHERE entity_type = ? AND entity_id = ? AND field_name = '_deleted'`,
			c.typ, c.id).Scan(&n); err != nil {
			t.Fatalf("count child field_hlc: %v", err)
		}
		if n != 1 {
			t.Errorf("child %s/%s must have a _deleted field HLC: got %d", c.typ, c.id, n)
		}
	}
}

// TestEmitDeleteCascade_NonFederatedNoEvents asserts a delete-cascade on a task in
// a NON-federated project still runs the domain write (children soft-deleted) but
// writes ZERO outbox events — federation stays a scoped overlay (US-3.2 AC1).
func TestEmitDeleteCascade_NonFederatedNoEvents(t *testing.T) {
	env := newEmitEnv(t)
	ctx := context.Background()

	seedComment(t, env.db, env.plainTaskID, "comment-P")
	taskClient := taskClientID(t, env.db, env.plainTaskID)

	err := env.emitter.EmitDeleteCascade(ctx, fedsvc.MutationSpec{
		LocalProjectID: env.plainProj,
		EntityType:     events.EntityTask,
		EntityID:       taskClient,
		Op:             events.OpDelete,
	}, []fedsvc.ChildTombstone{{EntityType: events.EntityComment, EntityID: "comment-P"}},
		func(tx *sql.Tx) error {
			now := "2026-06-02T00:00:00.000Z"
			_, err := tx.ExecContext(ctx, `UPDATE tasks SET deleted_at = ?, updated_at = ? WHERE id = ?`, now, now, env.plainTaskID)
			return err
		})
	if err != nil {
		t.Fatalf("emit cascade plain: %v", err)
	}

	if got := outboxCount(t, env.db, env.plainProj); got != 0 {
		t.Errorf("non-federated cascade outbox: got %d, want 0", got)
	}
	var del sql.NullString
	if err := env.db.QueryRow(`SELECT deleted_at FROM tasks WHERE id = ?`, env.plainTaskID).Scan(&del); err != nil {
		t.Fatalf("scan deleted_at: %v", err)
	}
	if !del.Valid {
		t.Errorf("domain delete must still run for a non-federated task")
	}
}

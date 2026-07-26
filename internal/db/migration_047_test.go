package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// openAt046 opens a fresh database migrated to version 46 — the state right
// before task_labels grew its created_at column — so a test can seed rows on the
// old schema and then assert what 047 makes of them. Partial migration control
// comes from `migrateTo` (migration_048_test.go).
func openAt046(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "m047.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	migrateTo(t, d, 46)
	return d
}

const (
	taskCreatedAt  = "2026-01-05T08:30:00.000Z"
	otherCreatedAt = "2026-02-11T19:45:00.000Z"
)

// Migration 047 adds task_labels.created_at and backfills existing rows from the
// task's own creation time — the only approximation available for tagging events
// that were never recorded. A wrong backfill would silently distort every usage
// window, so it is asserted against data seeded on the pre-047 schema.
func TestMigration047_BackfillsTaskLabelCreatedAt(t *testing.T) {
	d := openAt046(t)
	ctx := context.Background()

	var col int
	if err := d.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('task_labels') WHERE name = 'created_at'`,
	).Scan(&col); err != nil {
		t.Fatalf("probe column: %v", err)
	}
	if col != 0 {
		t.Fatalf("task_labels.created_at at version 46: got %d columns, want 0", col)
	}

	if _, err := d.Exec(
		`INSERT INTO contexts (id, name, color, created_at, updated_at) VALUES (1, 'work', 'blue', ?, ?)`,
		taskCreatedAt, taskCreatedAt); err != nil {
		t.Fatalf("insert context: %v", err)
	}
	if _, err := d.Exec(
		`INSERT INTO labels (id, name, color, created_at, updated_at) VALUES (1, 'bug', 'red', ?, ?)`,
		taskCreatedAt, taskCreatedAt); err != nil {
		t.Fatalf("insert label: %v", err)
	}
	for id, createdAt := range map[int]string{1: taskCreatedAt, 2: otherCreatedAt} {
		if _, err := d.Exec(
			`INSERT INTO tasks (id, title, context_id, created_at, updated_at) VALUES (?, 't', 1, ?, ?)`,
			id, createdAt, createdAt); err != nil {
			t.Fatalf("insert task %d: %v", id, err)
		}
		if _, err := d.Exec(`INSERT INTO task_labels (task_id, label_id) VALUES (?, 1)`, id); err != nil {
			t.Fatalf("insert task_label %d: %v", id, err)
		}
	}

	if err := RunMigrations(ctx, d); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	for taskID, want := range map[int]string{1: taskCreatedAt, 2: otherCreatedAt} {
		var got sql.NullString
		if err := d.QueryRow(`SELECT created_at FROM task_labels WHERE task_id = ?`, taskID).Scan(&got); err != nil {
			t.Fatalf("read backfilled created_at for task %d: %v", taskID, err)
		}
		if !got.Valid {
			t.Errorf("task %d: created_at is NULL, want backfill %s", taskID, want)
			continue
		}
		if got.String != want {
			t.Errorf("task %d created_at: got %s, want %s (the task's own creation time)", taskID, got.String, want)
		}
	}

	var idx int
	if err := d.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_task_labels_created_at'`,
	).Scan(&idx); err != nil {
		t.Fatalf("probe index: %v", err)
	}
	if idx != 1 {
		t.Errorf("idx_task_labels_created_at: got %d, want 1", idx)
	}
}

// New rows may be inserted without the column (older code paths, raw SQL), and
// the stats query must still work — so the column stays nullable rather than
// carrying an empty-string sentinel that would sort before every real timestamp.
func TestMigration047_CreatedAtIsNullable(t *testing.T) {
	d := mustOpenMigrated(t)

	if _, err := d.Exec(
		`INSERT INTO contexts (id, name, color, created_at, updated_at) VALUES (1, 'work', 'blue', ?, ?)`,
		taskCreatedAt, taskCreatedAt); err != nil {
		t.Fatalf("insert context: %v", err)
	}
	if _, err := d.Exec(
		`INSERT INTO labels (id, name, color, created_at, updated_at) VALUES (1, 'bug', 'red', ?, ?)`,
		taskCreatedAt, taskCreatedAt); err != nil {
		t.Fatalf("insert label: %v", err)
	}
	if _, err := d.Exec(
		`INSERT INTO tasks (id, title, context_id, created_at, updated_at) VALUES (1, 't', 1, ?, ?)`,
		taskCreatedAt, taskCreatedAt); err != nil {
		t.Fatalf("insert task: %v", err)
	}
	if _, err := d.Exec(`INSERT INTO task_labels (task_id, label_id) VALUES (1, 1)`); err != nil {
		t.Fatalf("insert task_label without created_at: %v", err)
	}

	var got sql.NullString
	if err := d.QueryRow(`SELECT created_at FROM task_labels WHERE task_id = 1`).Scan(&got); err != nil {
		t.Fatalf("read created_at: %v", err)
	}
	if got.Valid {
		t.Errorf("created_at: got %q, want NULL", got.String)
	}
}

package db

import (
	"context"
	"path/filepath"
	"testing"
)

func TestMigration051_InboxProcessingTablesAreNotLogged(t *testing.T) {
	d := mustOpenMigrated(t)

	for _, table := range []string{"inbox_processing_state", "inbox_processing_log"} {
		var exists int
		if err := d.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&exists); err != nil {
			t.Fatalf("probe %s: %v", table, err)
		}
		if exists != 1 {
			t.Errorf("table %s: got %d, want 1", table, exists)
		}
	}
	var col int
	if err := d.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('tasks') WHERE name='auto_sorted_at'`).Scan(&col); err != nil {
		t.Fatalf("probe column: %v", err)
	}
	if col != 1 {
		t.Errorf("tasks.auto_sorted_at: got %d columns, want 1", col)
	}
}

func TestMigration051_MarkerChangeIsLoggedAsTaskUpsert(t *testing.T) {
	d := mustOpenMigrated(t)
	seedTaskFixture(t, d)
	if _, err := d.Exec(`DELETE FROM change_log`); err != nil {
		t.Fatalf("clear log: %v", err)
	}
	if _, err := d.Exec(`UPDATE tasks SET auto_sorted_at = '2026-09-13T10:00:00.000Z' WHERE id = 1`); err != nil {
		t.Fatalf("set marker: %v", err)
	}
	if !hasOp(changeLog(t, d), "task", 1, "upsert") {
		t.Errorf("marker change: missing a task upsert in %v", changeLog(t, d))
	}
}

func TestMigration051_TaskDeleteCascadesStateAndDetachesLog(t *testing.T) {
	d := mustOpenMigrated(t)
	seedTaskFixture(t, d)
	const ts = "2026-09-13T10:00:00.000Z"
	if _, err := d.Exec(`INSERT INTO inbox_processing_state (task_id, fingerprint, status, updated_at) VALUES (1, 'f', 'kept', ?)`, ts); err != nil {
		t.Fatalf("insert state: %v", err)
	}
	if _, err := d.Exec(`INSERT INTO inbox_processing_log (task_id, task_title, outcome, model, before_state, created_at)
		VALUES (1, 'root', 'kept', 'm', '{}', ?)`, ts); err != nil {
		t.Fatalf("insert log: %v", err)
	}
	if _, err := d.Exec(`DELETE FROM tasks WHERE id = 1`); err != nil {
		t.Fatalf("delete task: %v", err)
	}
	var states int
	if err := d.QueryRow(`SELECT COUNT(*) FROM inbox_processing_state`).Scan(&states); err != nil {
		t.Fatalf("count state: %v", err)
	}
	if states != 0 {
		t.Errorf("state rows after delete: got %d, want 0", states)
	}
	var taskID *int64
	var title string
	if err := d.QueryRow(`SELECT task_id, task_title FROM inbox_processing_log`).Scan(&taskID, &title); err != nil {
		t.Fatalf("read log: %v", err)
	}
	if taskID != nil || title != "root" {
		t.Errorf("log row after delete: got task_id=%v title=%q, want nil and the kept title", taskID, title)
	}
}

func TestMigration051_Down(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "m051down.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := RunMigrations(context.Background(), d); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	migrateTo(t, d, 50)

	var tables, col int
	if err := d.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name LIKE 'inbox_processing_%'`).Scan(&tables); err != nil {
		t.Fatalf("probe tables: %v", err)
	}
	if tables != 0 {
		t.Errorf("inbox processing tables after down: got %d, want 0", tables)
	}
	if err := d.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('tasks') WHERE name='auto_sorted_at'`).Scan(&col); err != nil {
		t.Fatalf("probe column: %v", err)
	}
	if col != 0 {
		t.Errorf("tasks.auto_sorted_at after down: got %d columns, want 0", col)
	}
}

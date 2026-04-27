package db

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func mustOpenMigrated(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := RunMigrations(context.Background(), d); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d
}

func TestOpenSetsForeignKeysPragma(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "fk.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = d.Close() }()

	var fk int
	if err := d.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("query pragma: %v", err)
	}
	if fk != 1 {
		t.Fatalf("expected foreign_keys=1, got %d", fk)
	}
}

func TestRunMigrationsCreatesTablesAndInbox(t *testing.T) {
	d := mustOpenMigrated(t)

	var n int
	if err := d.QueryRow("SELECT COUNT(*) FROM inbox WHERE id = 1").Scan(&n); err != nil {
		t.Fatalf("query inbox: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 inbox row, got %d", n)
	}

	wantTables := []string{"contexts", "labels", "projects", "project_sections", "tasks", "task_labels", "project_labels", "users", "sessions"}
	for _, table := range wantTables {
		var one int
		err := d.QueryRow("SELECT 1 FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&one)
		if err != nil {
			t.Fatalf("table %s missing: %v", table, err)
		}
	}
}

func TestInboxIDTwoRejected(t *testing.T) {
	d := mustOpenMigrated(t)
	_, err := d.Exec("INSERT INTO inbox (id, created_at) VALUES (2, '2024-01-01T00:00:00.000Z')")
	if err == nil {
		t.Fatalf("expected error inserting inbox id=2")
	}
}

func TestUsersIDTwoRejected(t *testing.T) {
	d := mustOpenMigrated(t)

	_, err := d.Exec("INSERT INTO users (id, username, password_hash, created_at, updated_at) VALUES (1, 'u', 'h', '2024-01-01T00:00:00.000Z', '2024-01-01T00:00:00.000Z')")
	if err != nil {
		t.Fatalf("first insert id=1 failed: %v", err)
	}
	_, err = d.Exec("INSERT INTO users (id, username, password_hash, created_at, updated_at) VALUES (2, 'u2', 'h', '2024-01-01T00:00:00.000Z', '2024-01-01T00:00:00.000Z')")
	if err == nil {
		t.Fatalf("expected error inserting users id=2")
	}
}

func TestMigrationsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "rt.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = d.Close() }()

	ctx := context.Background()
	if err := RunMigrations(ctx, d); err != nil {
		t.Fatalf("up: %v", err)
	}
	if err := RollbackMigrations(ctx, d); err != nil {
		t.Fatalf("down: %v", err)
	}

	var n int
	if err := d.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='tasks'").Scan(&n); err != nil && !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("query: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected tasks table dropped after down, got count %d", n)
	}

	if err := RunMigrations(ctx, d); err != nil {
		t.Fatalf("re-up: %v", err)
	}
	var taskCount int
	if err := d.QueryRow("SELECT COUNT(*) FROM tasks").Scan(&taskCount); err != nil {
		t.Fatalf("re-query tasks: %v", err)
	}
}

func TestWithTxCommit(t *testing.T) {
	d := mustOpenMigrated(t)

	err := WithTx(context.Background(), d, func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO contexts (name, color, created_at, updated_at) VALUES (?, ?, ?, ?)",
			"work", "blue", "2024-01-01T00:00:00.000Z", "2024-01-01T00:00:00.000Z")
		return err
	})
	if err != nil {
		t.Fatalf("WithTx commit: %v", err)
	}

	var n int
	if err := d.QueryRow("SELECT COUNT(*) FROM contexts WHERE name='work'").Scan(&n); err != nil {
		t.Fatalf("query: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 row committed, got %d", n)
	}
}

func TestWithTxRollback(t *testing.T) {
	d := mustOpenMigrated(t)

	sentinel := errors.New("boom")
	err := WithTx(context.Background(), d, func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO contexts (name, color, created_at, updated_at) VALUES (?, ?, ?, ?)",
			"home", "green", "2024-01-01T00:00:00.000Z", "2024-01-01T00:00:00.000Z")
		if err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}

	var n int
	if err := d.QueryRow("SELECT COUNT(*) FROM contexts WHERE name='home'").Scan(&n); err != nil {
		t.Fatalf("query: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected rollback, got %d rows", n)
	}
}

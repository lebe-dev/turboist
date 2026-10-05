package db

import (
	"context"
	"path/filepath"
	"testing"
)

func TestMigration052_UpAndDown(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "m052.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := RunMigrations(context.Background(), d); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	count := func() int {
		t.Helper()
		var n int
		if err := d.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('tasks') WHERE name='auto_sort_undecided_at'`).Scan(&n); err != nil {
			t.Fatalf("probe column: %v", err)
		}
		return n
	}
	if got := count(); got != 1 {
		t.Fatalf("after up: got %d columns, want 1", got)
	}
	migrateTo(t, d, 51)
	if got := count(); got != 0 {
		t.Errorf("after down: got %d columns, want 0", got)
	}
}

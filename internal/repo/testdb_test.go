package repo

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/lebe-dev/turboist/internal/db"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.RunMigrations(context.Background(), d); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d
}

func ptr[T any](v T) *T {
	return &v
}

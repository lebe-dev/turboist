package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
)

// migrateTo applies (or rolls back) the migration set up to and including
// version v. Every other test in this package migrates all the way up, which
// means the users row is always created *after* the migrations ran — so the
// upgrade path that migration 048 actually cares about (a live database with an
// existing user) needs this partial control.
func migrateTo(t *testing.T, d *sql.DB, v int64) {
	t.Helper()
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("set dialect: %v", err)
	}
	current, err := goose.GetDBVersionContext(context.Background(), d)
	if err != nil {
		t.Fatalf("db version: %v", err)
	}
	if v >= current {
		if err := goose.UpToContext(context.Background(), d, "migrations", v); err != nil {
			t.Fatalf("up to %d: %v", v, err)
		}
		return
	}
	if err := goose.DownToContext(context.Background(), d, "migrations", v); err != nil {
		t.Fatalf("down to %d: %v", v, err)
	}
}

func openAt047(t *testing.T, settings string) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "m048.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	migrateTo(t, d, 47)

	const ts = "2024-01-01T00:00:00.000Z"
	if _, err := d.Exec(
		`INSERT INTO users (id, username, password_hash, settings, created_at, updated_at)
		 VALUES (1, 'admin', 'h', ?, ?, ?)`, settings, ts, ts); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return d
}

func readUserSettings(t *testing.T, d *sql.DB) map[string]any {
	t.Helper()
	var raw string
	if err := d.QueryRow(`SELECT settings FROM users WHERE id = 1`).Scan(&raw); err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("decode settings %q: %v", raw, err)
	}
	return out
}

// TestMigration048_SeedsDefaultsForExistingUser asserts an upgraded database
// gets both caps set to 10 without losing any other preference.
func TestMigration048_SeedsDefaultsForExistingUser(t *testing.T) {
	d := openAt047(t, `{"locale":"ru","publicView":true,"bugLabelIds":[3,7]}`)
	migrateTo(t, d, 48)

	got := readUserSettings(t, d)
	if got["maxPinnedTasks"] != float64(10) {
		t.Errorf("maxPinnedTasks: got %v, want 10", got["maxPinnedTasks"])
	}
	if got["maxPinnedProjects"] != float64(10) {
		t.Errorf("maxPinnedProjects: got %v, want 10", got["maxPinnedProjects"])
	}
	if got["locale"] != "ru" {
		t.Errorf("locale: got %v, want ru", got["locale"])
	}
	if got["publicView"] != true {
		t.Errorf("publicView: got %v, want true", got["publicView"])
	}
	ids, ok := got["bugLabelIds"].([]any)
	if !ok || len(ids) != 2 {
		t.Errorf("bugLabelIds: got %v, want two ids", got["bugLabelIds"])
	}
}

// TestMigration048_EmptyAndBrokenBlobs asserts the json_valid guard: a default
// '{}' blob and a non-JSON leftover both end up as a valid object carrying the
// two defaults, instead of failing the migration or producing NULL.
func TestMigration048_EmptyAndBrokenBlobs(t *testing.T) {
	cases := []struct {
		name string
		seed string
	}{
		{"default empty object", `{}`},
		// Names must stay path-safe: t.TempDir() embeds them in the SQLite file
		// path, and characters like '#' would be parsed as a URI fragment.
		{"empty string", ``},
		{"non-json leftover", `not json at all`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := openAt047(t, tc.seed)
			migrateTo(t, d, 48)

			got := readUserSettings(t, d)
			if len(got) != 2 {
				t.Errorf("keys: got %v, want only the two caps", got)
			}
			if got["maxPinnedTasks"] != float64(10) || got["maxPinnedProjects"] != float64(10) {
				t.Errorf("caps: got %v, want 10/10", got)
			}
		})
	}
}

// TestMigration048_DownRemovesOnlyTheCaps asserts the Down step is a clean
// inverse — the two keys go away and everything else is left alone.
func TestMigration048_DownRemovesOnlyTheCaps(t *testing.T) {
	d := openAt047(t, `{"locale":"en","troikiEnabled":true}`)
	migrateTo(t, d, 48)
	migrateTo(t, d, 47)

	got := readUserSettings(t, d)
	if _, ok := got["maxPinnedTasks"]; ok {
		t.Error("maxPinnedTasks: still present after down")
	}
	if _, ok := got["maxPinnedProjects"]; ok {
		t.Error("maxPinnedProjects: still present after down")
	}
	if got["locale"] != "en" {
		t.Errorf("locale: got %v, want en", got["locale"])
	}
	if got["troikiEnabled"] != true {
		t.Errorf("troikiEnabled: got %v, want true", got["troikiEnabled"])
	}
}

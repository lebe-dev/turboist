package db

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
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

func TestMigration010_ProjectsTroikiCategory(t *testing.T) {
	d := mustOpenMigrated(t)

	var col int
	err := d.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('projects') WHERE name = 'troiki_category'`,
	).Scan(&col)
	if err != nil {
		t.Fatalf("query column: %v", err)
	}
	if col != 1 {
		t.Errorf("projects.troiki_category column: got %d, want 1", col)
	}

	var idx int
	err = d.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_projects_troiki'`,
	).Scan(&idx)
	if err != nil {
		t.Fatalf("query index: %v", err)
	}
	if idx != 1 {
		t.Errorf("idx_projects_troiki: got %d, want 1", idx)
	}

	if _, err := d.Exec(
		`INSERT INTO contexts (id, name, color, created_at, updated_at) VALUES (1, 'c', 'blue', '2024-01-01T00:00:00.000Z', '2024-01-01T00:00:00.000Z')`,
	); err != nil {
		t.Fatalf("insert context: %v", err)
	}
	if _, err := d.Exec(
		`INSERT INTO projects (context_id, title, description, color, status, is_pinned, troiki_category, created_at, updated_at)
		 VALUES (1, 't', '', 'blue', 'open', 0, 'bogus', '2024-01-01T00:00:00.000Z', '2024-01-01T00:00:00.000Z')`,
	); err == nil {
		t.Errorf("expected CHECK constraint to reject 'bogus' troiki_category")
	}
}

// TestMigration024_OfflineSyncColumns asserts the offline-sync overlay
// (Federation v1 F0.1) adds client_id + deleted_at to all five synced tables.
func TestMigration024_OfflineSyncColumns(t *testing.T) {
	d := mustOpenMigrated(t)

	tables := []string{"tasks", "projects", "project_sections", "labels", "contexts"}
	for _, table := range tables {
		for _, col := range []string{"client_id", "deleted_at"} {
			var n int
			if err := d.QueryRow(
				`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, col,
			).Scan(&n); err != nil {
				t.Fatalf("query %s.%s: %v", table, col, err)
			}
			if n != 1 {
				t.Errorf("%s.%s column: got %d, want 1", table, col, n)
			}
		}
	}
}

// TestMigration024_BackfillsUniqueClientID asserts that existing rows present
// before migration 024 are backfilled with non-NULL, unique client ids.
func TestMigration024_BackfillsUniqueClientID(t *testing.T) {
	// Migrate only up to 023, seed rows, then migrate to 024 and assert backfill.
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "backfill.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	ctx := context.Background()
	if err := migrateTo(ctx, d, 23); err != nil {
		t.Fatalf("migrate to 23: %v", err)
	}

	// Seed a context + two projects + a task before the offline-sync columns exist.
	if _, err := d.Exec(
		`INSERT INTO contexts (id, name, color, created_at, updated_at) VALUES (1, 'c', 'blue', '2024-01-01T00:00:00.000Z', '2024-01-01T00:00:00.000Z')`,
	); err != nil {
		t.Fatalf("seed context: %v", err)
	}
	for _, title := range []string{"p1", "p2"} {
		if _, err := d.Exec(
			`INSERT INTO projects (context_id, title, description, color, status, is_pinned, created_at, updated_at)
			 VALUES (1, ?, '', 'blue', 'open', 0, '2024-01-01T00:00:00.000Z', '2024-01-01T00:00:00.000Z')`,
			title); err != nil {
			t.Fatalf("seed project %s: %v", title, err)
		}
	}
	if _, err := d.Exec(
		`INSERT INTO tasks (title, description, inbox_id, priority, status, created_at, updated_at)
		 VALUES ('t', '', 1, 'no-priority', 'open', '2024-01-01T00:00:00.000Z', '2024-01-01T00:00:00.000Z')`,
	); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	if err := RunMigrations(ctx, d); err != nil {
		t.Fatalf("migrate to head: %v", err)
	}

	// Every seeded project has a non-NULL client_id.
	var nullCount int
	if err := d.QueryRow(`SELECT COUNT(*) FROM projects WHERE client_id IS NULL`).Scan(&nullCount); err != nil {
		t.Fatalf("count null project client_id: %v", err)
	}
	if nullCount != 0 {
		t.Errorf("projects with NULL client_id after backfill: got %d, want 0", nullCount)
	}

	// client_ids are distinct (partial unique index holds).
	var distinct, total int
	if err := d.QueryRow(`SELECT COUNT(DISTINCT client_id), COUNT(*) FROM projects`).Scan(&distinct, &total); err != nil {
		t.Fatalf("count distinct: %v", err)
	}
	if distinct != total {
		t.Errorf("project client_ids not unique: %d distinct of %d rows", distinct, total)
	}

	// The partial unique index rejects a duplicate client_id.
	var dup string
	if err := d.QueryRow(`SELECT client_id FROM projects LIMIT 1`).Scan(&dup); err != nil {
		t.Fatalf("read client_id: %v", err)
	}
	if _, err := d.Exec(
		`INSERT INTO projects (context_id, title, description, color, status, is_pinned, client_id, created_at, updated_at)
		 VALUES (1, 'dupe', '', 'blue', 'open', 0, ?, '2024-01-01T00:00:00.000Z', '2024-01-01T00:00:00.000Z')`,
		dup); err == nil {
		t.Errorf("expected UNIQUE violation inserting duplicate client_id %q", dup)
	}

	// A second row with NULL client_id is allowed (partial index ignores NULLs).
	if _, err := d.Exec(
		`INSERT INTO projects (context_id, title, description, color, status, is_pinned, created_at, updated_at)
		 VALUES (1, 'nullcid', '', 'blue', 'open', 0, '2024-01-01T00:00:00.000Z', '2024-01-01T00:00:00.000Z')`,
	); err != nil {
		t.Errorf("expected NULL client_id insert to succeed, got %v", err)
	}
}

// TestMigration025_CommentsTable asserts migration 025 creates the immutable
// comments table with the offline-sync overlay columns and that it coexists
// with the pre-existing live schema (Federation v1 F0.2, US-3.5 schema).
func TestMigration025_CommentsTable(t *testing.T) {
	d := mustOpenMigrated(t)

	var one int
	if err := d.QueryRow(
		"SELECT 1 FROM sqlite_master WHERE type='table' AND name='comments'",
	).Scan(&one); err != nil {
		t.Fatalf("comments table missing: %v", err)
	}

	for _, col := range []string{"task_id", "body", "client_id", "deleted_at", "created_at", "updated_at"} {
		var n int
		if err := d.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info('comments') WHERE name = ?`, col,
		).Scan(&n); err != nil {
			t.Fatalf("query comments.%s: %v", col, err)
		}
		if n != 1 {
			t.Errorf("comments.%s column: got %d, want 1", col, n)
		}
	}

	// Seed a task and assert the FK + a comment insert/read round-trip works.
	seedTaskForComment(t, d)
	if _, err := d.Exec(
		`INSERT INTO comments (task_id, body, client_id, created_at, updated_at)
		 VALUES (1, 'hi', 'cid-1', '2024-01-01T00:00:00.000Z', '2024-01-01T00:00:00.000Z')`,
	); err != nil {
		t.Fatalf("insert comment: %v", err)
	}

	// The partial unique index rejects a duplicate client_id.
	if _, err := d.Exec(
		`INSERT INTO comments (task_id, body, client_id, created_at, updated_at)
		 VALUES (1, 'dupe', 'cid-1', '2024-01-01T00:00:00.000Z', '2024-01-01T00:00:00.000Z')`,
	); err == nil {
		t.Errorf("expected UNIQUE violation inserting duplicate comment client_id")
	}
}

// TestMigration026_ChecklistItemsTable asserts migration 026 creates the
// checklist_items table with title/is_completed/position/frac_position plus the
// offline-sync overlay columns (Federation v1 F0.2, US-3.6 schema).
func TestMigration026_ChecklistItemsTable(t *testing.T) {
	d := mustOpenMigrated(t)

	var one int
	if err := d.QueryRow(
		"SELECT 1 FROM sqlite_master WHERE type='table' AND name='checklist_items'",
	).Scan(&one); err != nil {
		t.Fatalf("checklist_items table missing: %v", err)
	}

	for _, col := range []string{"task_id", "title", "is_completed", "position", "frac_position", "client_id", "deleted_at", "created_at", "updated_at"} {
		var n int
		if err := d.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info('checklist_items') WHERE name = ?`, col,
		).Scan(&n); err != nil {
			t.Fatalf("query checklist_items.%s: %v", col, err)
		}
		if n != 1 {
			t.Errorf("checklist_items.%s column: got %d, want 1", col, n)
		}
	}

	seedTaskForComment(t, d)
	// is_completed CHECK rejects values outside {0,1}.
	if _, err := d.Exec(
		`INSERT INTO checklist_items (task_id, title, is_completed, created_at, updated_at)
		 VALUES (1, 'x', 2, '2024-01-01T00:00:00.000Z', '2024-01-01T00:00:00.000Z')`,
	); err == nil {
		t.Errorf("expected CHECK constraint to reject is_completed=2")
	}
}

// TestMigration027_FederationKeysTable asserts migration 027 creates the
// single-row federation_keys table (Federation v1 F0.3) carrying the instance
// Ed25519 keypair + identity, that id is pinned to 1, and that it coexists with
// the pre-existing live schema (incl. the GTD `inbox` table).
func TestMigration027_FederationKeysTable(t *testing.T) {
	d := mustOpenMigrated(t)

	var one int
	if err := d.QueryRow(
		"SELECT 1 FROM sqlite_master WHERE type='table' AND name='federation_keys'",
	).Scan(&one); err != nil {
		t.Fatalf("federation_keys table missing: %v", err)
	}

	for _, col := range []string{"id", "public_key", "private_seed_enc", "node_id", "display_name", "created_at"} {
		var n int
		if err := d.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info('federation_keys') WHERE name = ?`, col,
		).Scan(&n); err != nil {
			t.Fatalf("query federation_keys.%s: %v", col, err)
		}
		if n != 1 {
			t.Errorf("federation_keys.%s column: got %d, want 1", col, n)
		}
	}

	// The GTD `inbox` table still exists — the collision-avoidance prefix held.
	var inboxN int
	if err := d.QueryRow("SELECT COUNT(*) FROM inbox WHERE id = 1").Scan(&inboxN); err != nil {
		t.Fatalf("inbox coexistence: %v", err)
	}
	if inboxN != 1 {
		t.Errorf("inbox row after 027: got %d, want 1", inboxN)
	}

	// id=1 inserts; id=2 is rejected by the CHECK (single-row singleton).
	if _, err := d.Exec(
		`INSERT INTO federation_keys (id, public_key, private_seed_enc, node_id, display_name, created_at)
		 VALUES (1, 'pub', 'enc', 'node', 'host', '2024-01-01T00:00:00.000Z')`,
	); err != nil {
		t.Fatalf("insert federation_keys id=1: %v", err)
	}
	if _, err := d.Exec(
		`INSERT INTO federation_keys (id, public_key, private_seed_enc, node_id, display_name, created_at)
		 VALUES (2, 'pub', 'enc', 'node', 'host', '2024-01-01T00:00:00.000Z')`,
	); err == nil {
		t.Errorf("expected CHECK constraint to reject federation_keys id=2")
	}
}

// TestMigration028_NameUniquenessIsLiveOnly asserts migration 028 reconciles the
// 001 full-table UNIQUE(name) with soft-delete (Federation v1 F0.1 fix): a
// soft-deleted name no longer occupies the uniqueness slot, so the name can be
// recreated, while two *live* rows still cannot share a name, and the tombstone
// physically survives. Verified at the schema level for both labels and contexts.
func TestMigration028_NameUniquenessIsLiveOnly(t *testing.T) {
	d := mustOpenMigrated(t)

	for _, tbl := range []string{"labels", "contexts"} {
		// The live-only partial unique index exists and the table-level
		// UNIQUE(name) is gone (no auto-index on a NOT NULL UNIQUE column).
		var liveIdx int
		if err := d.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`,
			"idx_"+tbl+"_name_live",
		).Scan(&liveIdx); err != nil {
			t.Fatalf("%s: query live index: %v", tbl, err)
		}
		if liveIdx != 1 {
			t.Errorf("%s: idx_%s_name_live: got %d, want 1", tbl, tbl, liveIdx)
		}

		// Insert a live row, soft-delete it, then recreate the same name — must
		// succeed because the tombstone is excluded from the partial index.
		if _, err := d.Exec(
			`INSERT INTO `+tbl+` (name, color, client_id, created_at, updated_at)
			 VALUES ('work', 'blue', ?, '2024-01-01T00:00:00.000Z', '2024-01-01T00:00:00.000Z')`,
			tbl+"-cid-1"); err != nil {
			t.Fatalf("%s: insert first: %v", tbl, err)
		}
		if _, err := d.Exec(
			`UPDATE ` + tbl + ` SET deleted_at = '2024-01-02T00:00:00.000Z' WHERE name = 'work'`,
		); err != nil {
			t.Fatalf("%s: soft-delete: %v", tbl, err)
		}
		if _, err := d.Exec(
			`INSERT INTO `+tbl+` (name, color, client_id, created_at, updated_at)
			 VALUES ('work', 'red', ?, '2024-01-03T00:00:00.000Z', '2024-01-03T00:00:00.000Z')`,
			tbl+"-cid-2"); err != nil {
			t.Errorf("%s: recreate after soft-delete: got %v, want success", tbl, err)
		}

		// Two live rows with the same name are still rejected.
		if _, err := d.Exec(
			`INSERT INTO `+tbl+` (name, color, client_id, created_at, updated_at)
			 VALUES ('work', 'green', ?, '2024-01-04T00:00:00.000Z', '2024-01-04T00:00:00.000Z')`,
			tbl+"-cid-3"); err == nil {
			t.Errorf("%s: expected UNIQUE violation for a second live 'work'", tbl)
		}

		// The tombstone physically survives alongside the live recreated row.
		var live, dead int
		if err := d.QueryRow(
			`SELECT
			   SUM(CASE WHEN deleted_at IS NULL THEN 1 ELSE 0 END),
			   SUM(CASE WHEN deleted_at IS NOT NULL THEN 1 ELSE 0 END)
			 FROM `+tbl+` WHERE name = 'work'`,
		).Scan(&live, &dead); err != nil {
			t.Fatalf("%s: count live/dead: %v", tbl, err)
		}
		if live != 1 || dead != 1 {
			t.Errorf("%s: rows named 'work': got live=%d dead=%d, want 1/1", tbl, live, dead)
		}
	}
}

// TestMigration029_FederationCoreTables asserts migration 029 creates the
// federation bookkeeping tables (Federation v1 F1.1) and — critically — that
// the federation outbox/inbox use the federation_ prefix so they do NOT collide
// with the pre-existing GTD `inbox` table (001_schema.sql:6, R21). The migration
// must apply cleanly to a DB that already contains `inbox`.
func TestMigration029_FederationCoreTables(t *testing.T) {
	d := mustOpenMigrated(t)

	// All federation core tables exist under the federation_/federated_ prefix.
	wantTables := []string{
		"federated_instances",
		"federated_projects",
		"federation_invites",
		"federation_outbox",
		"federation_inbox",
	}
	for _, table := range wantTables {
		var one int
		if err := d.QueryRow(
			"SELECT 1 FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&one); err != nil {
			t.Fatalf("federation table %s missing: %v", table, err)
		}
	}

	// Regression guard for R21: the GTD `inbox` table still exists and is intact
	// (the federation migration did NOT clobber it or fail on a name collision).
	var inboxN int
	if err := d.QueryRow("SELECT COUNT(*) FROM inbox WHERE id = 1").Scan(&inboxN); err != nil {
		t.Fatalf("GTD inbox coexistence after 029: %v", err)
	}
	if inboxN != 1 {
		t.Errorf("GTD inbox row after 029: got %d, want 1", inboxN)
	}
	// And the federation inbox is a distinct, empty table.
	var fedInboxN int
	if err := d.QueryRow("SELECT COUNT(*) FROM federation_inbox").Scan(&fedInboxN); err != nil {
		t.Fatalf("federation_inbox query: %v", err)
	}
	if fedInboxN != 0 {
		t.Errorf("federation_inbox initial rows: got %d, want 0", fedInboxN)
	}

	// federated_projects carries the columns F1.1 / F0.4 promise.
	for _, col := range []string{
		"local_project_id", "peer_instance_url", "remote_project_id", "is_owner",
		"origin_instance_url", "permissions", "paused", "revoked", "protocol_version",
		"last_sent_hlc", "last_received_hlc", "joined_at",
	} {
		var n int
		if err := d.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info('federated_projects') WHERE name = ?`, col,
		).Scan(&n); err != nil {
			t.Fatalf("query federated_projects.%s: %v", col, err)
		}
		if n != 1 {
			t.Errorf("federated_projects.%s column: got %d, want 1", col, n)
		}
	}

	// federated_instances carries display_name + the peer public key + last_contact_at.
	for _, col := range []string{"instance_url", "public_key", "display_name", "last_contact_at"} {
		var n int
		if err := d.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info('federated_instances') WHERE name = ?`, col,
		).Scan(&n); err != nil {
			t.Fatalf("query federated_instances.%s: %v", col, err)
		}
		if n != 1 {
			t.Errorf("federated_instances.%s column: got %d, want 1", col, n)
		}
	}

	// federation_invites carries the hashed secret + lifecycle columns; there is
	// NO plaintext secret column (US-1.2 AC2, asserted at the schema level).
	for _, col := range []string{
		"invite_id", "local_project_id", "secret_hash", "permissions",
		"max_uses", "used_count", "expires_at", "revoked_at", "consumed_at",
	} {
		var n int
		if err := d.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info('federation_invites') WHERE name = ?`, col,
		).Scan(&n); err != nil {
			t.Fatalf("query federation_invites.%s: %v", col, err)
		}
		if n != 1 {
			t.Errorf("federation_invites.%s column: got %d, want 1", col, n)
		}
	}
	var secretCol int
	if err := d.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('federation_invites') WHERE name = 'secret'`,
	).Scan(&secretCol); err != nil {
		t.Fatalf("query federation_invites.secret: %v", err)
	}
	if secretCol != 0 {
		t.Errorf("federation_invites must not store a plaintext secret column (US-1.2 AC2)")
	}
}

// TestMigration029_PermissionsCheckConstraint asserts the permissions column on
// federated_projects only accepts read|write|admin (Federation v1 F1.1).
func TestMigration029_PermissionsCheckConstraint(t *testing.T) {
	d := mustOpenMigrated(t)
	seedFederatedProjectFixture(t, d)

	if _, err := d.Exec(
		`INSERT INTO federated_projects
		   (local_project_id, peer_instance_url, remote_project_id, is_owner, origin_instance_url, permissions, protocol_version, joined_at)
		 VALUES (1, '', '', 1, 'https://me.example', 'bogus', 1, '2024-01-01T00:00:00.000Z')`,
	); err == nil {
		t.Errorf("expected CHECK constraint to reject permissions='bogus'")
	}
	if _, err := d.Exec(
		`INSERT INTO federated_projects
		   (local_project_id, peer_instance_url, remote_project_id, is_owner, origin_instance_url, permissions, protocol_version, joined_at)
		 VALUES (1, '', '', 1, 'https://me.example', 'admin', 1, '2024-01-01T00:00:00.000Z')`,
	); err != nil {
		t.Errorf("expected permissions='admin' to insert, got %v", err)
	}
}

// TestMigration029_FederatedProjectsCompositePK asserts the PK is
// (local_project_id, peer_instance_url): the same project may have many peer
// rows (one per peer) but not two rows for the same (project, peer).
func TestMigration029_FederatedProjectsCompositePK(t *testing.T) {
	d := mustOpenMigrated(t)
	seedFederatedProjectFixture(t, d)

	insert := func(peer string) error {
		_, err := d.Exec(
			`INSERT INTO federated_projects
			   (local_project_id, peer_instance_url, remote_project_id, is_owner, origin_instance_url, permissions, protocol_version, joined_at)
			 VALUES (1, ?, '', 1, 'https://me.example', 'admin', 1, '2024-01-01T00:00:00.000Z')`,
			peer)
		return err
	}
	if err := insert("https://me.example"); err != nil {
		t.Fatalf("self-row insert: %v", err)
	}
	if err := insert("https://peer.example"); err != nil {
		t.Fatalf("second peer-row insert: %v", err)
	}
	if err := insert("https://me.example"); err == nil {
		t.Errorf("expected composite PK to reject a duplicate (project, peer) row")
	}
}

// TestMigration030_ProjectsIsFederated asserts migration 030 adds the
// is_federated flag to projects, mirroring 012_projects_is_private.
func TestMigration030_ProjectsIsFederated(t *testing.T) {
	d := mustOpenMigrated(t)

	var col int
	if err := d.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('projects') WHERE name = 'is_federated'`,
	).Scan(&col); err != nil {
		t.Fatalf("query column: %v", err)
	}
	if col != 1 {
		t.Errorf("projects.is_federated column: got %d, want 1", col)
	}

	// Default is 0 and the CHECK rejects anything outside {0,1}.
	if _, err := d.Exec(
		`INSERT INTO contexts (id, name, color, created_at, updated_at) VALUES (1, 'c', 'blue', '2024-01-01T00:00:00.000Z', '2024-01-01T00:00:00.000Z')`,
	); err != nil {
		t.Fatalf("seed context: %v", err)
	}
	if _, err := d.Exec(
		`INSERT INTO projects (id, context_id, title, description, color, status, is_pinned, client_id, created_at, updated_at)
		 VALUES (1, 1, 'p', '', 'blue', 'open', 0, 'fed-p1', '2024-01-01T00:00:00.000Z', '2024-01-01T00:00:00.000Z')`,
	); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	var isFed int
	if err := d.QueryRow(`SELECT is_federated FROM projects WHERE id = 1`).Scan(&isFed); err != nil {
		t.Fatalf("read is_federated: %v", err)
	}
	if isFed != 0 {
		t.Errorf("projects.is_federated default: got %d, want 0", isFed)
	}
	if _, err := d.Exec(`UPDATE projects SET is_federated = 2 WHERE id = 1`); err == nil {
		t.Errorf("expected CHECK constraint to reject is_federated=2")
	}
}

// TestMigration031_FederationHLCTables asserts migration 031 creates the
// entity_field_hlc (WITHOUT ROWID) + hlc_state (singleton id=1) sidecar tables
// (Federation v1 F2.3) and that it applies cleanly on top of the full live
// schema (incl. the GTD `inbox` table and the earlier federation tables).
func TestMigration031_FederationHLCTables(t *testing.T) {
	d := mustOpenMigrated(t)

	for _, table := range []string{"entity_field_hlc", "hlc_state"} {
		var one int
		if err := d.QueryRow(
			"SELECT 1 FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&one); err != nil {
			t.Fatalf("%s table missing: %v", table, err)
		}
	}

	for _, col := range []string{"entity_type", "entity_id", "field_name", "hlc"} {
		var n int
		if err := d.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info('entity_field_hlc') WHERE name = ?`, col,
		).Scan(&n); err != nil {
			t.Fatalf("query entity_field_hlc.%s: %v", col, err)
		}
		if n != 1 {
			t.Errorf("entity_field_hlc.%s column: got %d, want 1", col, n)
		}
	}

	// entity_field_hlc is WITHOUT ROWID (no rowid column reported).
	var hasRowid int
	if err := d.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('entity_field_hlc') WHERE name = 'rowid'`,
	).Scan(&hasRowid); err != nil {
		t.Fatalf("query rowid: %v", err)
	}
	if hasRowid != 0 {
		t.Errorf("entity_field_hlc reports a rowid column; expected WITHOUT ROWID")
	}

	// The composite PK (entity_type, entity_id, field_name) rejects a duplicate.
	if _, err := d.Exec(
		`INSERT INTO entity_field_hlc (entity_type, entity_id, field_name, hlc)
		 VALUES ('task', 'cid-1', 'title', '00000000000001-0000-node')`,
	); err != nil {
		t.Fatalf("insert field_hlc: %v", err)
	}
	if _, err := d.Exec(
		`INSERT INTO entity_field_hlc (entity_type, entity_id, field_name, hlc)
		 VALUES ('task', 'cid-1', 'title', '00000000000002-0000-node')`,
	); err == nil {
		t.Errorf("expected PK violation inserting duplicate (entity_type, entity_id, field_name)")
	}

	// hlc_state id is pinned to 1 (single-row singleton); id=2 is rejected.
	if _, err := d.Exec(`INSERT INTO hlc_state (id) VALUES (1)`); err != nil {
		t.Fatalf("insert hlc_state id=1: %v", err)
	}
	if _, err := d.Exec(`INSERT INTO hlc_state (id) VALUES (2)`); err == nil {
		t.Errorf("expected CHECK constraint to reject hlc_state id=2")
	}

	// The GTD `inbox` table still exists — collision-avoidance prefix held.
	var inboxN int
	if err := d.QueryRow("SELECT COUNT(*) FROM inbox WHERE id = 1").Scan(&inboxN); err != nil {
		t.Fatalf("inbox coexistence: %v", err)
	}
	if inboxN != 1 {
		t.Errorf("inbox row after 031: got %d, want 1", inboxN)
	}
}

// TestMigration033_FederationReBootstrapMarker asserts migration 033 adds the
// re-bootstrap marker columns to federated_projects (Federation v1 F4.2, US-4.2
// AC4) and that it applies cleanly on top of the full live schema (incl. the
// existing federated_projects rows it ALTERs). The Down leg must preserve
// existing rows (it recreates the table without the two columns).
func TestMigration033_FederationReBootstrapMarker(t *testing.T) {
	d := mustOpenMigrated(t)

	for _, col := range []string{"rebootstrap_cutoff_hlc", "rebootstrapped_at"} {
		var n int
		if err := d.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info('federated_projects') WHERE name = ?`, col,
		).Scan(&n); err != nil {
			t.Fatalf("query federated_projects.%s: %v", col, err)
		}
		if n != 1 {
			t.Errorf("federated_projects.%s column: got %d, want 1", col, n)
		}
	}

	// A row can carry the marker; it round-trips.
	seedFederatedProjectFixture(t, d)
	if _, err := d.Exec(
		`INSERT INTO federated_projects
		   (local_project_id, peer_instance_url, remote_project_id, is_owner, origin_instance_url, permissions, protocol_version, last_received_hlc, rebootstrap_cutoff_hlc, rebootstrapped_at, joined_at)
		 VALUES (1, 'https://owner.example', '', 0, 'https://owner.example', 'write', 1, '00000000050000-0000-o', '00000000050000-0000-o', '2026-06-03T09:30:00.000Z', '2024-01-01T00:00:00.000Z')`,
	); err != nil {
		t.Fatalf("insert federated_projects with marker: %v", err)
	}
	var cutoff, at string
	if err := d.QueryRow(
		`SELECT rebootstrap_cutoff_hlc, rebootstrapped_at FROM federated_projects WHERE local_project_id = 1 AND is_owner = 0`,
	).Scan(&cutoff, &at); err != nil {
		t.Fatalf("read marker: %v", err)
	}
	if cutoff != "00000000050000-0000-o" || at != "2026-06-03T09:30:00.000Z" {
		t.Errorf("marker round-trip: got (%q, %q)", cutoff, at)
	}
}

// TestMigration033_DownPreservesRows asserts the Down leg of migration 033
// preserves existing federated_projects rows (it rebuilds the table without the
// marker columns rather than dropping data).
func TestMigration033_DownPreservesRows(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "down033.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ctx := context.Background()
	if err := RunMigrations(ctx, d); err != nil {
		t.Fatalf("migrate to head: %v", err)
	}
	seedFederatedProjectFixture(t, d)
	if _, err := d.Exec(
		`INSERT INTO federated_projects
		   (local_project_id, peer_instance_url, remote_project_id, is_owner, origin_instance_url, permissions, protocol_version, joined_at)
		 VALUES (1, 'https://peer.example', '', 0, 'https://peer.example', 'write', 1, '2024-01-01T00:00:00.000Z')`,
	); err != nil {
		t.Fatalf("seed peer row: %v", err)
	}

	// Roll back the head migrations one at a time down to 033, so this test
	// asserts the 033 Down specifically regardless of how many federation
	// migrations stack above it (036 lost_status, 035 backpressure, 034
	// key_mismatch_at, 033).
	for _, ver := range []string{"040", "039", "038", "037", "036", "035", "034", "033"} {
		if err := goose.DownContext(ctx, d, "migrations"); err != nil {
			t.Fatalf("down %s: %v", ver, err)
		}
	}

	// The peer row survives, and the marker columns are gone.
	var rows int
	if err := d.QueryRow(`SELECT COUNT(*) FROM federated_projects WHERE peer_instance_url = 'https://peer.example'`).Scan(&rows); err != nil {
		t.Fatalf("count after down: %v", err)
	}
	if rows != 1 {
		t.Errorf("federated_projects rows after down: got %d, want 1 (Down must preserve data)", rows)
	}
	var hasCol int
	if err := d.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('federated_projects') WHERE name = 'rebootstrapped_at'`).Scan(&hasCol); err != nil {
		t.Fatalf("query column after down: %v", err)
	}
	if hasCol != 0 {
		t.Errorf("rebootstrapped_at column still present after down: got %d, want 0", hasCol)
	}
}

// TestMigration034_FederationPeerHealthMarker asserts migration 034 adds the
// sticky key_mismatch_at health column to federated_projects (Federation v1
// F4.3, US-4.3 AC4) and that it applies cleanly on top of the full live schema.
// The marker round-trips and is NULL by default (no key mismatch).
func TestMigration034_FederationPeerHealthMarker(t *testing.T) {
	d := mustOpenMigrated(t)

	var n int
	if err := d.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('federated_projects') WHERE name = 'key_mismatch_at'`,
	).Scan(&n); err != nil {
		t.Fatalf("query federated_projects.key_mismatch_at: %v", err)
	}
	if n != 1 {
		t.Errorf("federated_projects.key_mismatch_at column: got %d, want 1", n)
	}

	// A row defaults to NULL key_mismatch_at, and the marker round-trips when set.
	seedFederatedProjectFixture(t, d)
	if _, err := d.Exec(
		`INSERT INTO federated_projects
		   (local_project_id, peer_instance_url, remote_project_id, is_owner, origin_instance_url, permissions, protocol_version, joined_at)
		 VALUES (1, 'https://peer.example', '', 0, 'https://peer.example', 'write', 1, '2024-01-01T00:00:00.000Z')`,
	); err != nil {
		t.Fatalf("insert federated_projects: %v", err)
	}
	var marker sql.NullString
	if err := d.QueryRow(
		`SELECT key_mismatch_at FROM federated_projects WHERE local_project_id = 1 AND peer_instance_url = 'https://peer.example'`,
	).Scan(&marker); err != nil {
		t.Fatalf("read marker default: %v", err)
	}
	if marker.Valid {
		t.Errorf("key_mismatch_at default: got %q, want NULL", marker.String)
	}
	if _, err := d.Exec(
		`UPDATE federated_projects SET key_mismatch_at = '2026-06-03T10:00:00.000Z' WHERE local_project_id = 1 AND peer_instance_url = 'https://peer.example'`,
	); err != nil {
		t.Fatalf("set marker: %v", err)
	}
	if err := d.QueryRow(
		`SELECT key_mismatch_at FROM federated_projects WHERE local_project_id = 1 AND peer_instance_url = 'https://peer.example'`,
	).Scan(&marker); err != nil {
		t.Fatalf("read marker: %v", err)
	}
	if !marker.Valid || marker.String != "2026-06-03T10:00:00.000Z" {
		t.Errorf("key_mismatch_at round-trip: got %+v, want 2026-06-03T10:00:00.000Z", marker)
	}
}

// TestMigration034_DownPreservesRows asserts the Down leg of migration 034
// preserves existing federated_projects rows (incl. the 033 marker columns) and
// removes only key_mismatch_at.
func TestMigration034_DownPreservesRows(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "down034.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ctx := context.Background()
	if err := RunMigrations(ctx, d); err != nil {
		t.Fatalf("migrate to head: %v", err)
	}
	seedFederatedProjectFixture(t, d)
	if _, err := d.Exec(
		`INSERT INTO federated_projects
		   (local_project_id, peer_instance_url, remote_project_id, is_owner, origin_instance_url, permissions, protocol_version, rebootstrapped_at, key_mismatch_at, joined_at)
		 VALUES (1, 'https://peer.example', '', 0, 'https://peer.example', 'write', 1, '2026-06-03T09:30:00.000Z', '2026-06-03T10:00:00.000Z', '2024-01-01T00:00:00.000Z')`,
	); err != nil {
		t.Fatalf("seed peer row: %v", err)
	}

	// Roll back the head (036 lost_status, then 035 backpressure) first, then 034,
	// so this test asserts the 034 Down specifically regardless of how many
	// federation migrations stack above it.
	for _, ver := range []string{"040", "039", "038", "037", "036", "035", "034"} {
		if err := goose.DownContext(ctx, d, "migrations"); err != nil {
			t.Fatalf("down %s: %v", ver, err)
		}
	}

	// The peer row + its 033 marker survive; key_mismatch_at is gone.
	var rebootAt string
	if err := d.QueryRow(
		`SELECT rebootstrapped_at FROM federated_projects WHERE peer_instance_url = 'https://peer.example'`,
	).Scan(&rebootAt); err != nil {
		t.Fatalf("read after down: %v", err)
	}
	if rebootAt != "2026-06-03T09:30:00.000Z" {
		t.Errorf("033 marker after 034 down: got %q, want preserved", rebootAt)
	}
	var hasCol int
	if err := d.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('federated_projects') WHERE name = 'key_mismatch_at'`).Scan(&hasCol); err != nil {
		t.Fatalf("query column after down: %v", err)
	}
	if hasCol != 0 {
		t.Errorf("key_mismatch_at column still present after down: got %d, want 0", hasCol)
	}
}

// TestMigration035_FederationBackpressureTables asserts migration 035 adds the
// dead-letter parking lot and the durable per-peer retry gate (Federation v1
// F4.4, US-4.4 / US-8.3) and that both apply cleanly on top of the full live
// schema. A dead-letter row round-trips, the (peer, event) pair is unique, and
// the peer-retry row round-trips with its permanent flag.
func TestMigration035_FederationBackpressureTables(t *testing.T) {
	d := mustOpenMigrated(t)
	seedFederatedProjectFixture(t, d)

	// federation_dead_letter round-trips and is idempotent per (peer, event).
	if _, err := d.Exec(
		`INSERT INTO federation_dead_letter (event_id, peer_instance_url, local_project_id, payload, status_code, reason, failed_at)
		 VALUES ('e1', 'https://peer.example', 1, '{"event_id":"e1"}', 403, 'federation_read_only', '2026-06-03T10:00:00.000Z')`,
	); err != nil {
		t.Fatalf("insert dead-letter: %v", err)
	}
	if _, err := d.Exec(
		`INSERT INTO federation_dead_letter (event_id, peer_instance_url, local_project_id, payload, status_code, reason, failed_at)
		 VALUES ('e1', 'https://peer.example', 1, '{"event_id":"e1"}', 403, 'federation_read_only', '2026-06-03T10:00:00.000Z')`,
	); err == nil {
		t.Errorf("expected UNIQUE violation inserting a duplicate (peer, event) dead-letter row")
	}
	var n int
	if err := d.QueryRow(`SELECT COUNT(*) FROM federation_dead_letter WHERE peer_instance_url = 'https://peer.example'`).Scan(&n); err != nil {
		t.Fatalf("count dead-letter: %v", err)
	}
	if n != 1 {
		t.Errorf("dead-letter rows: got %d, want 1", n)
	}

	// federation_peer_retry round-trips and rejects an out-of-range permanent flag.
	if _, err := d.Exec(
		`INSERT INTO federation_peer_retry (peer_instance_url, not_before, attempt, permanent, updated_at)
		 VALUES ('https://peer.example', '2026-06-03T10:00:05.000Z', 3, 1, '2026-06-03T10:00:00.000Z')`,
	); err != nil {
		t.Fatalf("insert peer-retry: %v", err)
	}
	if _, err := d.Exec(
		`INSERT INTO federation_peer_retry (peer_instance_url, not_before, attempt, permanent, updated_at)
		 VALUES ('https://bad.example', '', 0, 2, '2026-06-03T10:00:00.000Z')`,
	); err == nil {
		t.Errorf("expected CHECK constraint to reject permanent=2")
	}
	var attempt, permanent int
	var notBefore string
	if err := d.QueryRow(
		`SELECT not_before, attempt, permanent FROM federation_peer_retry WHERE peer_instance_url = 'https://peer.example'`,
	).Scan(&notBefore, &attempt, &permanent); err != nil {
		t.Fatalf("read peer-retry: %v", err)
	}
	if notBefore != "2026-06-03T10:00:05.000Z" || attempt != 3 || permanent != 1 {
		t.Errorf("peer-retry round-trip: got (%q, %d, %d), want (2026-06-03T10:00:05.000Z, 3, 1)", notBefore, attempt, permanent)
	}
}

// TestMigration035_DownDropsTables asserts the Down leg of migration 035 removes
// both backpressure tables (they are net-new, so Down is a clean DROP).
func TestMigration035_DownDropsTables(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "down035.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ctx := context.Background()
	if err := RunMigrations(ctx, d); err != nil {
		t.Fatalf("migrate to head: %v", err)
	}

	// Roll back the head (036 lost_status) first, then 035, so this test asserts
	// the 035 Down specifically regardless of how many federation migrations stack
	// above it.
	for _, ver := range []string{"040", "039", "038", "037", "036", "035"} {
		if err := goose.DownContext(ctx, d, "migrations"); err != nil {
			t.Fatalf("down %s: %v", ver, err)
		}
	}

	for _, tbl := range []string{"federation_dead_letter", "federation_peer_retry"} {
		var n int
		if err := d.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, tbl,
		).Scan(&n); err != nil {
			t.Fatalf("query %s after down: %v", tbl, err)
		}
		if n != 0 {
			t.Errorf("%s still present after down: got %d, want 0", tbl, n)
		}
	}
}

// TestMigration036_FederationLostStatus asserts migration 036 adds the lost /
// lost_reason columns to federated_projects (Federation v1 F5.4, US-6.2) and that
// they apply cleanly on top of the full live schema. A row defaults to NOT lost,
// the reason vocabulary is pinned by a CHECK, and a revoked-lost row round-trips.
func TestMigration036_FederationLostStatus(t *testing.T) {
	d := mustOpenMigrated(t)

	for _, col := range []string{"lost", "lost_reason"} {
		var n int
		if err := d.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info('federated_projects') WHERE name = ?`, col,
		).Scan(&n); err != nil {
			t.Fatalf("query federated_projects.%s: %v", col, err)
		}
		if n != 1 {
			t.Errorf("federated_projects.%s column: got %d, want 1", col, n)
		}
	}

	seedFederatedProjectFixture(t, d)
	if _, err := d.Exec(
		`INSERT INTO federated_projects
		   (local_project_id, peer_instance_url, remote_project_id, is_owner, origin_instance_url, permissions, protocol_version, joined_at)
		 VALUES (1, 'https://owner.example', '', 0, 'https://owner.example', 'read', 1, '2024-01-01T00:00:00.000Z')`,
	); err != nil {
		t.Fatalf("insert federated_projects: %v", err)
	}
	// Defaults: not lost, empty reason.
	var lost int
	var reason string
	if err := d.QueryRow(
		`SELECT lost, lost_reason FROM federated_projects WHERE local_project_id = 1 AND peer_instance_url = 'https://owner.example'`,
	).Scan(&lost, &reason); err != nil {
		t.Fatalf("read defaults: %v", err)
	}
	if lost != 0 || reason != "" {
		t.Errorf("lost defaults: got (%d, %q), want (0, \"\")", lost, reason)
	}
	// A revoked-lost row round-trips.
	if _, err := d.Exec(
		`UPDATE federated_projects SET lost = 1, lost_reason = 'revoked' WHERE local_project_id = 1 AND peer_instance_url = 'https://owner.example'`,
	); err != nil {
		t.Fatalf("set lost: %v", err)
	}
	if err := d.QueryRow(
		`SELECT lost, lost_reason FROM federated_projects WHERE local_project_id = 1 AND peer_instance_url = 'https://owner.example'`,
	).Scan(&lost, &reason); err != nil {
		t.Fatalf("read lost: %v", err)
	}
	if lost != 1 || reason != "revoked" {
		t.Errorf("lost round-trip: got (%d, %q), want (1, revoked)", lost, reason)
	}
	// The CHECK pins the reason vocabulary: an unknown reason is rejected.
	if _, err := d.Exec(
		`UPDATE federated_projects SET lost_reason = 'banished' WHERE local_project_id = 1 AND peer_instance_url = 'https://owner.example'`,
	); err == nil {
		t.Errorf("expected CHECK constraint to reject an unknown lost_reason")
	}
}

// TestMigration036_DownPreservesRows asserts the Down leg of migration 036
// preserves existing federated_projects rows (incl. the 033/034 marker columns)
// and removes only the lost / lost_reason columns.
func TestMigration036_DownPreservesRows(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "down036.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ctx := context.Background()
	if err := RunMigrations(ctx, d); err != nil {
		t.Fatalf("migrate to head: %v", err)
	}
	seedFederatedProjectFixture(t, d)
	if _, err := d.Exec(
		`INSERT INTO federated_projects
		   (local_project_id, peer_instance_url, remote_project_id, is_owner, origin_instance_url, permissions, protocol_version, rebootstrapped_at, key_mismatch_at, lost, lost_reason, joined_at)
		 VALUES (1, 'https://peer.example', '', 0, 'https://peer.example', 'read', 1, '2026-06-03T09:30:00.000Z', '2026-06-03T10:00:00.000Z', 1, 'revoked', '2024-01-01T00:00:00.000Z')`,
	); err != nil {
		t.Fatalf("seed peer row: %v", err)
	}

	// Roll back the head (037 security_incidents) then 036 (lost_status) so this
	// test asserts the 036 Down specifically regardless of how many federation
	// migrations stack above it.
	for _, ver := range []string{"040", "039", "038", "037", "036"} {
		if err := goose.DownContext(ctx, d, "migrations"); err != nil {
			t.Fatalf("down %s: %v", ver, err)
		}
	}

	// The peer row + its 033/034 markers survive; lost / lost_reason are gone.
	var rebootAt, keyMismatch string
	if err := d.QueryRow(
		`SELECT rebootstrapped_at, key_mismatch_at FROM federated_projects WHERE peer_instance_url = 'https://peer.example'`,
	).Scan(&rebootAt, &keyMismatch); err != nil {
		t.Fatalf("read after down: %v", err)
	}
	if rebootAt != "2026-06-03T09:30:00.000Z" || keyMismatch != "2026-06-03T10:00:00.000Z" {
		t.Errorf("033/034 markers after 036 down: got (%q, %q), want preserved", rebootAt, keyMismatch)
	}
	for _, col := range []string{"lost", "lost_reason"} {
		var hasCol int
		if err := d.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('federated_projects') WHERE name = ?`, col).Scan(&hasCol); err != nil {
			t.Fatalf("query column %s after down: %v", col, err)
		}
		if hasCol != 0 {
			t.Errorf("%s column still present after down: got %d, want 0", col, hasCol)
		}
	}
}

// TestMigration037_FederationSecurityIncidentsTable asserts migration 037 creates
// the key-change incident log (Federation v1 F5.6b, US-6.4 AC2/AC3) and that it
// applies cleanly on top of the full live schema. An open incident round-trips,
// the partial unique index pins at most one OPEN incident per (project, peer), and
// resolving an incident frees the partial index so a later rotation opens a fresh
// row — the append-only history the trust-key audit relies on.
func TestMigration037_FederationSecurityIncidentsTable(t *testing.T) {
	d := mustOpenMigrated(t)

	// The table exists.
	var hasTable int
	if err := d.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'federation_security_incidents'`,
	).Scan(&hasTable); err != nil {
		t.Fatalf("query incidents table: %v", err)
	}
	if hasTable != 1 {
		t.Fatalf("federation_security_incidents table: got %d, want 1", hasTable)
	}

	seedFederatedProjectFixture(t, d)

	// An open incident round-trips.
	if _, err := d.Exec(
		`INSERT INTO federation_security_incidents (local_project_id, peer_instance_url, kind, detected_at, old_public_key)
		 VALUES (1, 'https://peer.example', 'key_change', '2026-06-03T10:00:00.000Z', 'oldkey')`,
	); err != nil {
		t.Fatalf("insert incident: %v", err)
	}

	// The partial unique index allows AT MOST ONE open incident per (project, peer):
	// a second open incident for the same pair is rejected.
	if _, err := d.Exec(
		`INSERT INTO federation_security_incidents (local_project_id, peer_instance_url, kind, detected_at)
		 VALUES (1, 'https://peer.example', 'key_change', '2026-06-03T10:05:00.000Z')`,
	); err == nil {
		t.Errorf("expected partial UNIQUE index to reject a second OPEN incident for the same (project, peer)")
	}

	// An unknown kind is rejected by the CHECK.
	if _, err := d.Exec(
		`INSERT INTO federation_security_incidents (local_project_id, peer_instance_url, kind, detected_at)
		 VALUES (1, 'https://other.example', 'mystery', '2026-06-03T10:00:00.000Z')`,
	); err == nil {
		t.Errorf("expected CHECK constraint to reject an unknown incident kind")
	}

	// Resolving the incident frees the partial index, so a LATER rotation opens a
	// fresh row — the history is append-only (two rows for one peer).
	if _, err := d.Exec(
		`UPDATE federation_security_incidents SET resolved_at = '2026-06-03T11:00:00.000Z', new_public_key = 'newkey'
		   WHERE local_project_id = 1 AND peer_instance_url = 'https://peer.example' AND resolved_at IS NULL`,
	); err != nil {
		t.Fatalf("resolve incident: %v", err)
	}
	if _, err := d.Exec(
		`INSERT INTO federation_security_incidents (local_project_id, peer_instance_url, kind, detected_at, old_public_key)
		 VALUES (1, 'https://peer.example', 'key_change', '2026-06-03T12:00:00.000Z', 'newkey')`,
	); err != nil {
		t.Fatalf("insert fresh incident after resolve: %v", err)
	}
	var n int
	if err := d.QueryRow(
		`SELECT COUNT(*) FROM federation_security_incidents WHERE peer_instance_url = 'https://peer.example'`,
	).Scan(&n); err != nil {
		t.Fatalf("count incidents: %v", err)
	}
	if n != 2 {
		t.Errorf("incident history rows: got %d, want 2 (resolved + fresh)", n)
	}
}

// TestMigration037_DownDropsTable asserts the Down leg of migration 037 removes
// the incidents table (it is net-new, so Down is a clean DROP).
func TestMigration037_DownDropsTable(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "down037.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ctx := context.Background()
	if err := RunMigrations(ctx, d); err != nil {
		t.Fatalf("migrate to head: %v", err)
	}

	// Roll back the heads (039 audit log, 038 outbox protocol_version) then 037
	// so this test asserts the 037 Down specifically regardless of how many
	// federation migrations stack above it.
	for _, ver := range []string{"040", "039", "038", "037"} {
		if err := goose.DownContext(ctx, d, "migrations"); err != nil {
			t.Fatalf("down %s: %v", ver, err)
		}
	}

	var n int
	if err := d.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'federation_security_incidents'`,
	).Scan(&n); err != nil {
		t.Fatalf("query incidents table after down: %v", err)
	}
	if n != 0 {
		t.Errorf("federation_security_incidents still present after down: got %d, want 0", n)
	}
}

// TestMigration038_OutboxProtocolVersion asserts migration 038 adds the
// protocol_version dual-write seam column to federation_outbox (Federation v1
// F6.1) and that it applies cleanly on top of the full live schema. The column
// defaults to 1 (the only v1 protocol version), round-trips an explicit value,
// and pre-existing federation_outbox rows are backfilled to 1 (NOT NULL).
func TestMigration038_OutboxProtocolVersion(t *testing.T) {
	d := mustOpenMigrated(t)

	var n int
	if err := d.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('federation_outbox') WHERE name = 'protocol_version'`,
	).Scan(&n); err != nil {
		t.Fatalf("query federation_outbox.protocol_version: %v", err)
	}
	if n != 1 {
		t.Errorf("federation_outbox.protocol_version column: got %d, want 1", n)
	}

	seedFederatedProjectFixture(t, d)

	// A row inserted WITHOUT protocol_version defaults to 1.
	if _, err := d.Exec(
		`INSERT INTO federation_outbox (event_id, local_project_id, payload, delivered_to, created_at)
		 VALUES ('e-default', 1, '{"event_id":"e-default"}', '', '2026-06-03T10:00:00.000Z')`,
	); err != nil {
		t.Fatalf("insert default-version outbox row: %v", err)
	}
	var ver int
	if err := d.QueryRow(`SELECT protocol_version FROM federation_outbox WHERE event_id = 'e-default'`).Scan(&ver); err != nil {
		t.Fatalf("read default protocol_version: %v", err)
	}
	if ver != 1 {
		t.Errorf("federation_outbox.protocol_version default: got %d, want 1", ver)
	}

	// An explicit protocol_version round-trips.
	if _, err := d.Exec(
		`INSERT INTO federation_outbox (event_id, local_project_id, payload, delivered_to, protocol_version, created_at)
		 VALUES ('e-explicit', 1, '{"event_id":"e-explicit"}', '', 1, '2026-06-03T10:00:01.000Z')`,
	); err != nil {
		t.Fatalf("insert explicit-version outbox row: %v", err)
	}
	if err := d.QueryRow(`SELECT protocol_version FROM federation_outbox WHERE event_id = 'e-explicit'`).Scan(&ver); err != nil {
		t.Fatalf("read explicit protocol_version: %v", err)
	}
	if ver != 1 {
		t.Errorf("federation_outbox.protocol_version explicit: got %d, want 1", ver)
	}
}

// TestMigration038_BackfillsExistingRows asserts that federation_outbox rows
// present BEFORE migration 038 are backfilled with protocol_version = 1 (the
// column is NOT NULL, so a NULL backfill would fail the migration on a populated
// table).
func TestMigration038_BackfillsExistingRows(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "backfill038.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	ctx := context.Background()
	// Migrate up to 037 (before the outbox protocol_version column exists).
	if err := migrateTo(ctx, d, 37); err != nil {
		t.Fatalf("migrate to 37: %v", err)
	}
	seedFederatedProjectFixture(t, d)
	if _, err := d.Exec(
		`INSERT INTO federation_outbox (event_id, local_project_id, payload, delivered_to, created_at)
		 VALUES ('pre038', 1, '{"event_id":"pre038"}', '', '2026-06-03T09:00:00.000Z')`,
	); err != nil {
		t.Fatalf("seed pre-038 outbox row: %v", err)
	}

	// Migrate to head — the 038 ALTER + backfill must succeed on a populated table.
	if err := RunMigrations(ctx, d); err != nil {
		t.Fatalf("migrate to head: %v", err)
	}

	var ver int
	if err := d.QueryRow(`SELECT protocol_version FROM federation_outbox WHERE event_id = 'pre038'`).Scan(&ver); err != nil {
		t.Fatalf("read backfilled protocol_version: %v", err)
	}
	if ver != 1 {
		t.Errorf("pre-038 row protocol_version after backfill: got %d, want 1", ver)
	}
	var nullN int
	if err := d.QueryRow(`SELECT COUNT(*) FROM federation_outbox WHERE protocol_version IS NULL`).Scan(&nullN); err != nil {
		t.Fatalf("count null protocol_version: %v", err)
	}
	if nullN != 0 {
		t.Errorf("federation_outbox rows with NULL protocol_version: got %d, want 0", nullN)
	}
}

// TestMigration038_DownPreservesRows asserts the Down leg of migration 038
// removes only the protocol_version column from federation_outbox while
// preserving every existing row (DROP COLUMN, not a destructive rebuild).
func TestMigration038_DownPreservesRows(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "down038.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ctx := context.Background()
	if err := RunMigrations(ctx, d); err != nil {
		t.Fatalf("migrate to head: %v", err)
	}
	seedFederatedProjectFixture(t, d)
	if _, err := d.Exec(
		`INSERT INTO federation_outbox (event_id, local_project_id, payload, delivered_to, protocol_version, created_at)
		 VALUES ('keep-me', 1, '{"event_id":"keep-me"}', '', 1, '2026-06-03T10:00:00.000Z')`,
	); err != nil {
		t.Fatalf("seed outbox row: %v", err)
	}

	// Roll back the head (039 audit log) then 038 so this test asserts the 038
	// Down specifically regardless of how many federation migrations stack above it.
	for _, ver := range []string{"040", "039", "038"} {
		if err := goose.DownContext(ctx, d, "migrations"); err != nil {
			t.Fatalf("down %s: %v", ver, err)
		}
	}

	// The row survives, and the protocol_version column is gone.
	var n int
	if err := d.QueryRow(`SELECT COUNT(*) FROM federation_outbox WHERE event_id = 'keep-me'`).Scan(&n); err != nil {
		t.Fatalf("count after down: %v", err)
	}
	if n != 1 {
		t.Errorf("federation_outbox rows after down: got %d, want 1 (Down must preserve data)", n)
	}
	var hasCol int
	if err := d.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('federation_outbox') WHERE name = 'protocol_version'`).Scan(&hasCol); err != nil {
		t.Fatalf("query column after down: %v", err)
	}
	if hasCol != 0 {
		t.Errorf("protocol_version column still present after down: got %d, want 0", hasCol)
	}
}

// TestMigration039_FederationAuditLogTable asserts migration 039 creates the
// federation audit log (Federation v1 F6.3, US-7.4) and that it applies cleanly
// on the full live schema: a row round-trips, the kind/outcome CHECK constraints
// reject unknown values, and client_id is UNIQUE.
func TestMigration039_FederationAuditLogTable(t *testing.T) {
	d := mustOpenMigrated(t)

	// The table exists.
	var hasTable int
	if err := d.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'federation_audit_log'`,
	).Scan(&hasTable); err != nil {
		t.Fatalf("query audit table: %v", err)
	}
	if hasTable != 1 {
		t.Fatalf("federation_audit_log table: got %d, want 1", hasTable)
	}

	// A row round-trips (peer + kind + outcome + detail + timestamp).
	if _, err := d.Exec(
		`INSERT INTO federation_audit_log (kind, outcome, peer_instance_url, detail, created_at)
		 VALUES ('signature_invalid', 'rejected', 'https://peer.example', 'nonce replay', '2026-06-03T10:00:00.000Z')`,
	); err != nil {
		t.Fatalf("insert audit row: %v", err)
	}

	// An unknown kind is rejected by the CHECK.
	if _, err := d.Exec(
		`INSERT INTO federation_audit_log (kind, outcome, created_at)
		 VALUES ('mystery', 'rejected', '2026-06-03T10:00:00.000Z')`,
	); err == nil {
		t.Errorf("expected CHECK constraint to reject an unknown audit kind")
	}

	// An unknown outcome is rejected by the CHECK.
	if _, err := d.Exec(
		`INSERT INTO federation_audit_log (kind, outcome, created_at)
		 VALUES ('handshake', 'maybe', '2026-06-03T10:00:00.000Z')`,
	); err == nil {
		t.Errorf("expected CHECK constraint to reject an unknown outcome")
	}

	// client_id is UNIQUE when set (NULL is allowed, repeated NULLs are fine).
	if _, err := d.Exec(
		`INSERT INTO federation_audit_log (client_id, kind, outcome, created_at)
		 VALUES ('cid-1', 'revoke', 'accepted', '2026-06-03T10:01:00.000Z')`,
	); err != nil {
		t.Fatalf("insert audit row with client_id: %v", err)
	}
	if _, err := d.Exec(
		`INSERT INTO federation_audit_log (client_id, kind, outcome, created_at)
		 VALUES ('cid-1', 'revoke', 'accepted', '2026-06-03T10:02:00.000Z')`,
	); err == nil {
		t.Errorf("expected UNIQUE violation inserting a duplicate client_id")
	}
}

// TestMigration039_DownDropsTable asserts the Down leg of migration 039 removes
// the audit log table (it is net-new, so Down is a clean DROP).
func TestMigration039_DownDropsTable(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "down039.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ctx := context.Background()
	if err := RunMigrations(ctx, d); err != nil {
		t.Fatalf("migrate to head: %v", err)
	}

	// Roll back the head (040) then 039 so this test asserts the 039 Down
	// specifically regardless of how many federation migrations stack above it.
	for _, ver := range []string{"040", "039"} {
		if err := goose.DownContext(ctx, d, "migrations"); err != nil {
			t.Fatalf("down %s: %v", ver, err)
		}
	}

	var n int
	if err := d.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'federation_audit_log'`,
	).Scan(&n); err != nil {
		t.Fatalf("query audit table after down: %v", err)
	}
	if n != 0 {
		t.Errorf("federation_audit_log still present after down: got %d, want 0", n)
	}
}

// TestMigration040_RetentionSettingsTable asserts migration 040 creates the
// single-row federation_retention_settings table seeded empty (id=1, all NULL),
// pins the id=1 CHECK, and round-trips an updated retention value (Federation v1
// F6.5, US-8.4).
func TestMigration040_RetentionSettingsTable(t *testing.T) {
	d := mustOpenMigrated(t)

	// The table exists and is seeded with exactly the single id=1 row, all NULL.
	var id int
	var tomb, outbox, inbox sql.NullInt64
	if err := d.QueryRow(
		`SELECT id, tombstone_retention_days, outbox_retention_days, inbox_retention_days
		 FROM federation_retention_settings WHERE id = 1`,
	).Scan(&id, &tomb, &outbox, &inbox); err != nil {
		t.Fatalf("read seeded retention row: %v", err)
	}
	if id != 1 {
		t.Errorf("retention row id: got %d, want 1", id)
	}
	if tomb.Valid || outbox.Valid || inbox.Valid {
		t.Errorf("retention row seeded non-NULL: tomb=%v outbox=%v inbox=%v (want all NULL so defaults apply)", tomb, outbox, inbox)
	}

	// A second id is rejected by the id=1 CHECK.
	if _, err := d.Exec(`INSERT INTO federation_retention_settings (id) VALUES (2)`); err == nil {
		t.Errorf("expected CHECK(id=1) to reject a second row")
	}

	// An updated value round-trips.
	if _, err := d.Exec(
		`UPDATE federation_retention_settings SET tombstone_retention_days = 120, updated_at = '2026-06-04T00:00:00.000Z' WHERE id = 1`,
	); err != nil {
		t.Fatalf("update retention: %v", err)
	}
	var got int
	if err := d.QueryRow(`SELECT tombstone_retention_days FROM federation_retention_settings WHERE id = 1`).Scan(&got); err != nil {
		t.Fatalf("read updated retention: %v", err)
	}
	if got != 120 {
		t.Errorf("tombstone_retention_days: got %d, want 120", got)
	}
}

// TestMigration040_LostReasonInstanceURLChanged asserts migration 040 widens the
// federated_projects.lost_reason CHECK so the instance_url_changed reason can be
// persisted (Federation v1 F6.5, US-8.5 AC2, R27) while every prior reason still
// validates and an unknown reason is still rejected.
func TestMigration040_LostReasonInstanceURLChanged(t *testing.T) {
	d := mustOpenMigrated(t)
	seedFederatedProjectFixture(t, d)

	base := `INSERT INTO federated_projects
		(local_project_id, peer_instance_url, origin_instance_url, permissions, joined_at, lost, lost_reason)
		VALUES (1, ?, 'https://owner.example', 'read', '2026-06-04T00:00:00.000Z', 1, ?)`

	// The new reason is now accepted.
	if _, err := d.Exec(base, "https://peer-iuc.example", "instance_url_changed"); err != nil {
		t.Fatalf("insert instance_url_changed lost_reason: %v", err)
	}
	// The prior reasons still validate.
	for _, reason := range []string{"revoked", "left", "owner-dead"} {
		if _, err := d.Exec(base, "https://peer-"+reason+".example", reason); err != nil {
			t.Fatalf("insert lost_reason %q: %v", reason, err)
		}
	}
	// An unknown reason is still rejected by the (widened) CHECK.
	if _, err := d.Exec(base, "https://peer-bogus.example", "bogus"); err == nil {
		t.Errorf("expected CHECK to reject unknown lost_reason")
	}
}

// TestMigration040_DownPreservesRows asserts the Down leg of migration 040 drops
// the retention table and narrows the lost_reason vocabulary while preserving
// federated_projects rows — an instance_url_changed row is downgraded to the
// closest pre-040 read-only reason rather than failing the migration.
func TestMigration040_DownPreservesRows(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "down040.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ctx := context.Background()
	if err := RunMigrations(ctx, d); err != nil {
		t.Fatalf("migrate to head: %v", err)
	}
	seedFederatedProjectFixture(t, d)
	if _, err := d.Exec(
		`INSERT INTO federated_projects
		 (local_project_id, peer_instance_url, origin_instance_url, permissions, joined_at, lost, lost_reason)
		 VALUES (1, 'https://peer-iuc.example', 'https://owner.example', 'read', '2026-06-04T00:00:00.000Z', 1, 'instance_url_changed')`,
	); err != nil {
		t.Fatalf("seed instance_url_changed row: %v", err)
	}

	// Roll back the head (040) so this test asserts the 040 Down specifically.
	if err := goose.DownContext(ctx, d, "migrations"); err != nil {
		t.Fatalf("down 040: %v", err)
	}

	// The retention table is gone.
	var hasTable int
	if err := d.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'federation_retention_settings'`,
	).Scan(&hasTable); err != nil {
		t.Fatalf("query retention table after down: %v", err)
	}
	if hasTable != 0 {
		t.Errorf("federation_retention_settings still present after down: got %d, want 0", hasTable)
	}

	// The row survives, with its reason downgraded so the narrowed CHECK holds.
	var reason string
	if err := d.QueryRow(
		`SELECT lost_reason FROM federated_projects WHERE peer_instance_url = 'https://peer-iuc.example'`,
	).Scan(&reason); err != nil {
		t.Fatalf("read row after down: %v", err)
	}
	if reason != "owner-dead" {
		t.Errorf("lost_reason after down: got %q, want %q (instance_url_changed downgraded)", reason, "owner-dead")
	}
}

// seedFederatedProjectFixture inserts a context + project (id=1) so that
// federated_projects FK inserts have a local project target.
func seedFederatedProjectFixture(t *testing.T, d *sql.DB) {
	t.Helper()
	if _, err := d.Exec(
		`INSERT INTO contexts (id, name, color, created_at, updated_at) VALUES (1, 'c', 'blue', '2024-01-01T00:00:00.000Z', '2024-01-01T00:00:00.000Z')`,
	); err != nil {
		t.Fatalf("seed context: %v", err)
	}
	if _, err := d.Exec(
		`INSERT INTO projects (id, context_id, title, description, color, status, is_pinned, client_id, created_at, updated_at)
		 VALUES (1, 1, 'p', '', 'blue', 'open', 0, 'fed-fixture-p1', '2024-01-01T00:00:00.000Z', '2024-01-01T00:00:00.000Z')`,
	); err != nil {
		t.Fatalf("seed project: %v", err)
	}
}

// seedTaskForComment inserts a context + task (id=1) so comment/checklist FK
// inserts have a target. Tables already carry the migration-024 overlay columns.
func seedTaskForComment(t *testing.T, d *sql.DB) {
	t.Helper()
	if _, err := d.Exec(
		`INSERT INTO contexts (id, name, color, created_at, updated_at) VALUES (1, 'c', 'blue', '2024-01-01T00:00:00.000Z', '2024-01-01T00:00:00.000Z')`,
	); err != nil {
		t.Fatalf("seed context: %v", err)
	}
	if _, err := d.Exec(
		`INSERT INTO tasks (id, title, description, context_id, priority, status, created_at, updated_at)
		 VALUES (1, 't', '', 1, 'no-priority', 'open', '2024-01-01T00:00:00.000Z', '2024-01-01T00:00:00.000Z')`,
	); err != nil {
		t.Fatalf("seed task: %v", err)
	}
}

func migrateTo(ctx context.Context, d *sql.DB, version int64) error {
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return err
	}
	return goose.UpToContext(ctx, d, "migrations", version)
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

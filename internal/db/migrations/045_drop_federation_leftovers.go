// Package migrations holds the goose migration set. Most migrations are plain
// `.sql` files embedded by internal/db; this file is a Go migration because it
// has to be conditional — the objects it removes only exist on databases that
// once ran the abandoned federation/sync branch (migrations 024-040, long since
// deleted from the repo but still recorded in their goose_db_version), and
// SQLite has no `DROP COLUMN IF EXISTS`.
package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upDropFederationLeftovers, downDropFederationLeftovers)
}

// Tables created solely by the federation/sync branch. checklist_items belongs
// to the same dead branch: nothing in internal/ reads or writes it.
var federationTables = []string{
	"entity_field_hlc",
	"hlc_state",
	"federated_instances",
	"federated_projects",
	"federation_audit_log",
	"federation_dead_letter",
	"federation_inbox",
	"federation_invites",
	"federation_keys",
	"federation_outbox",
	"federation_peer_retry",
	"federation_pruned_floor",
	"federation_retention_settings",
	"federation_security_incidents",
	"checklist_items",
}

// Sync-overlay columns bolted onto the live tables. Note calendar_oauth_configs
// also has a `client_id`, but that is an OAuth client id — unrelated, keep it.
var federationColumns = []struct {
	table   string
	columns []string
}{
	{"tasks", []string{"client_id", "deleted_at"}},
	{"projects", []string{"client_id", "deleted_at", "is_federated"}},
	{"labels", []string{"client_id", "deleted_at"}},
	{"contexts", []string{"client_id", "deleted_at"}},
	{"project_sections", []string{"client_id", "deleted_at"}},
}

func upDropFederationLeftovers(ctx context.Context, tx *sql.Tx) error {
	for _, table := range federationTables {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", table)); err != nil {
			return fmt.Errorf("drop table %s: %w", table, err)
		}
	}

	for _, spec := range federationColumns {
		// A column cannot be dropped while an index references it, and the
		// branch also left partial "live row" indexes (… WHERE deleted_at IS NULL).
		if err := dropIndexesReferencing(ctx, tx, spec.table); err != nil {
			return err
		}
		for _, column := range spec.columns {
			exists, err := columnExists(ctx, tx, spec.table, column)
			if err != nil {
				return err
			}
			if !exists {
				continue
			}
			if _, err := tx.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", spec.table, column)); err != nil {
				return fmt.Errorf("drop column %s.%s: %w", spec.table, column, err)
			}
		}
	}

	// The branch rebuilt contexts/labels without the original UNIQUE(name),
	// replacing it with a partial index dropped above. Restore the constraint;
	// on a database that never ran it, this index merely restates UNIQUE(name).
	for _, stmt := range []string{
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_contexts_name ON contexts(name)",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_labels_name ON labels(name)",
	} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("restore unique name index: %w", err)
		}
	}

	return nil
}

// downDropFederationLeftovers only reverses what can be reversed: the dropped
// federation objects are gone for good, and re-creating them would resurrect a
// schema no code understands.
func downDropFederationLeftovers(ctx context.Context, tx *sql.Tx) error {
	for _, stmt := range []string{
		"DROP INDEX IF EXISTS idx_contexts_name",
		"DROP INDEX IF EXISTS idx_labels_name",
	} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("drop unique name index: %w", err)
		}
	}
	return nil
}

func columnExists(ctx context.Context, tx *sql.Tx, table, column string) (bool, error) {
	var count int
	err := tx.QueryRowContext(ctx,
		`SELECT count(*) FROM pragma_table_info(?) WHERE name = ?`, table, column).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("inspect %s.%s: %w", table, column, err)
	}
	return count > 0, nil
}

// dropIndexesReferencing removes every index on table whose definition mentions
// a sync-overlay column, so the following DROP COLUMN can proceed.
func dropIndexesReferencing(ctx context.Context, tx *sql.Tx, table string) error {
	rows, err := tx.QueryContext(ctx,
		`SELECT name FROM sqlite_master
		 WHERE type = 'index' AND tbl_name = ? AND sql IS NOT NULL
		   AND (sql LIKE '%client_id%' OR sql LIKE '%deleted_at%' OR sql LIKE '%is_federated%')`, table)
	if err != nil {
		return fmt.Errorf("list indexes on %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("scan index name on %s: %w", table, err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("list indexes on %s: %w", table, err)
	}

	for _, name := range names {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("DROP INDEX IF EXISTS %q", name)); err != nil {
			return fmt.Errorf("drop index %s: %w", name, err)
		}
	}
	return nil
}

package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

// syncableTables maps every table whose rows a replica mirrors to the entity
// name its changes are logged under. Tables that fold into an owner (join rows,
// template subtasks) log the owner's entity, which is why several tables share
// one name here.
var syncableTables = map[string]string{
	"tasks":                        "task",
	"projects":                     "project",
	"project_sections":             "section",
	"contexts":                     "context",
	"labels":                       "label",
	"task_relations":               "task_relation",
	"task_templates":               "task_template",
	"task_labels":                  "task",
	"project_labels":               "project",
	"task_template_subtasks":       "task_template",
	"task_template_labels":         "task_template",
	"task_template_subtask_labels": "task_template",
	"users":                        "user_settings",
	"app_settings":                 "app_settings",
}

// notLoggedTables hold credentials, replay caches and provider-side integration
// state. None of it belongs on a replica, so none of it may grow a changelog
// trigger — a trigger added here would leak the existence and timing of secrets.
var notLoggedTables = []string{
	"sessions", "api_tokens", "idempotency_keys",
	"webauthn_credentials", "webauthn_users", "totp_recovery_codes",
	"calendar_accounts", "calendar_sources", "calendar_oauth_configs", "calendar_oauth_states",
	"inbox",
}

type changeRow struct {
	seq      int64
	entity   string
	entityID int64
	op       string
}

func changeLog(t *testing.T, d *sql.DB) []changeRow {
	t.Helper()
	rows, err := d.Query(`SELECT seq, entity, entity_id, op FROM change_log ORDER BY seq`)
	if err != nil {
		t.Fatalf("read change_log: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var out []changeRow
	for rows.Next() {
		var r changeRow
		if err := rows.Scan(&r.seq, &r.entity, &r.entityID, &r.op); err != nil {
			t.Fatalf("scan change_log: %v", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read change_log: %v", err)
	}
	return out
}

// hasOp reports whether the log holds a row for the entity/id/op triple.
func hasOp(rows []changeRow, entity string, entityID int64, op string) bool {
	for _, r := range rows {
		if r.entity == entity && r.entityID == entityID && r.op == op {
			return true
		}
	}
	return false
}

// seedTaskFixture creates the minimum row set a task needs: a context, a label
// and one task, all with explicit ids so assertions can name them.
func seedTaskFixture(t *testing.T, d *sql.DB) {
	t.Helper()
	const ts = "2026-03-01T09:00:00.000Z"
	stmts := []string{
		`INSERT INTO contexts (id, name, color, created_at, updated_at) VALUES (1, 'work', 'blue', ?, ?)`,
		`INSERT INTO labels (id, name, color, created_at, updated_at) VALUES (1, 'bug', 'red', ?, ?)`,
		`INSERT INTO tasks (id, title, context_id, created_at, updated_at) VALUES (1, 'root', 1, ?, ?)`,
	}
	for _, q := range stmts {
		if _, err := d.Exec(q, ts, ts); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}
}

// Logging lives in triggers so that it cannot be forgotten by a write path. That
// only holds if every syncable table actually carries all three of them, so the
// coverage is asserted against the live schema rather than against the migration
// text.
func TestMigration050_EverySyncableTableHasAllThreeTriggers(t *testing.T) {
	d := mustOpenMigrated(t)

	for table := range syncableTables {
		rows, err := d.Query(
			`SELECT name, sql FROM sqlite_master
			  WHERE type = 'trigger' AND tbl_name = ? AND name LIKE 'trg_changelog_%'`, table)
		if err != nil {
			t.Fatalf("list triggers on %s: %v", table, err)
		}
		seen := map[string]bool{}
		for rows.Next() {
			var name, ddl string
			if err := rows.Scan(&name, &ddl); err != nil {
				t.Fatalf("scan trigger on %s: %v", table, err)
			}
			upper := strings.ToUpper(ddl)
			switch {
			case strings.Contains(upper, "AFTER INSERT"):
				seen["INSERT"] = true
			case strings.Contains(upper, "AFTER UPDATE"):
				seen["UPDATE"] = true
			case strings.Contains(upper, "AFTER DELETE"):
				seen["DELETE"] = true
			default:
				t.Errorf("trigger %s on %s: cannot tell which operation it fires on", name, table)
			}
		}
		_ = rows.Close()
		if err := rows.Err(); err != nil {
			t.Fatalf("list triggers on %s: %v", table, err)
		}
		for _, op := range []string{"INSERT", "UPDATE", "DELETE"} {
			if !seen[op] {
				t.Errorf("table %s: no AFTER %s changelog trigger, got %v", table, op, seen)
			}
		}
	}
}

func TestMigration050_SecretsAndCachesAreNotLogged(t *testing.T) {
	d := mustOpenMigrated(t)

	for _, table := range notLoggedTables {
		var n int
		if err := d.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master
			  WHERE type = 'trigger' AND tbl_name = ? AND name LIKE 'trg_changelog_%'`, table).Scan(&n); err != nil {
			t.Fatalf("count triggers on %s: %v", table, err)
		}
		if n != 0 {
			t.Errorf("table %s: got %d changelog triggers, want 0", table, n)
		}
	}
}

func TestMigration050_SyncEpochStartsAtOne(t *testing.T) {
	d := mustOpenMigrated(t)

	var epoch int64
	if err := d.QueryRow(`SELECT sync_epoch FROM app_settings WHERE id = 1`).Scan(&epoch); err != nil {
		t.Fatalf("read sync epoch: %v", err)
	}
	if epoch != 1 {
		t.Errorf("sync_epoch: got %d, want 1", epoch)
	}
}

// A replica reads the log, not the tables, so the three row operations must each
// produce exactly the row a replica can act on: an upsert it refetches, and a
// delete that stands in for the row that no longer exists.
func TestMigration050_TaskInsertUpdateDeleteAreLogged(t *testing.T) {
	d := mustOpenMigrated(t)
	seedTaskFixture(t, d)

	if _, err := d.Exec(`UPDATE tasks SET title = 'renamed' WHERE id = 1`); err != nil {
		t.Fatalf("update task: %v", err)
	}
	if _, err := d.Exec(`DELETE FROM tasks WHERE id = 1`); err != nil {
		t.Fatalf("delete task: %v", err)
	}

	rows := changeLog(t, d)
	var ops []string
	for _, r := range rows {
		if r.entity == "task" && r.entityID == 1 {
			ops = append(ops, r.op)
		}
	}
	want := []string{"upsert", "upsert", "delete"}
	if len(ops) != len(want) {
		t.Fatalf("task change ops: got %v, want %v", ops, want)
	}
	for i := range want {
		if ops[i] != want[i] {
			t.Errorf("task change op %d: got %s, want %s", i, ops[i], want[i])
		}
	}
	if !hasOp(rows, "context", 1, "upsert") || !hasOp(rows, "label", 1, "upsert") {
		t.Errorf("context and label inserts: missing from %v", rows)
	}
}

// The API serves a task's labels inside the task payload, so a replica has no
// edge entity to apply. Tagging must therefore surface as a change of the task
// itself, otherwise a label added to an unchanged task would never reach it.
func TestMigration050_TaskLabelEdgeLogsTheTask(t *testing.T) {
	d := mustOpenMigrated(t)
	seedTaskFixture(t, d)

	if _, err := d.Exec(`DELETE FROM change_log`); err != nil {
		t.Fatalf("clear log: %v", err)
	}
	if _, err := d.Exec(
		`INSERT INTO task_labels (task_id, label_id, created_at) VALUES (1, 1, '2026-03-01T09:00:00.000Z')`); err != nil {
		t.Fatalf("tag task: %v", err)
	}
	if _, err := d.Exec(`DELETE FROM task_labels WHERE task_id = 1`); err != nil {
		t.Fatalf("untag task: %v", err)
	}

	rows := changeLog(t, d)
	if len(rows) != 2 {
		t.Fatalf("rows after tagging and untagging: got %v, want 2", rows)
	}
	for i, r := range rows {
		if r.entity != "task" || r.entityID != 1 || r.op != "upsert" {
			t.Errorf("row %d: got %+v, want an upsert of task 1", i, r)
		}
	}
}

// Deleting a task cascades to its labels and relations. Those child rows must not
// log an upsert of the task that is on its way out: the replica would refetch an
// id the server no longer knows. The delete row is the last word.
func TestMigration050_DeletingATaskLogsNoUpsertAfterTheDelete(t *testing.T) {
	d := mustOpenMigrated(t)
	seedTaskFixture(t, d)

	if _, err := d.Exec(
		`INSERT INTO task_labels (task_id, label_id, created_at) VALUES (1, 1, '2026-03-01T09:00:00.000Z')`); err != nil {
		t.Fatalf("tag task: %v", err)
	}
	if _, err := d.Exec(`DELETE FROM change_log`); err != nil {
		t.Fatalf("clear log: %v", err)
	}
	if _, err := d.Exec(`DELETE FROM tasks WHERE id = 1`); err != nil {
		t.Fatalf("delete task: %v", err)
	}

	rows := changeLog(t, d)
	if len(rows) == 0 {
		t.Fatal("no rows logged for the delete")
	}
	last := rows[len(rows)-1]
	if last.entity != "task" || last.entityID != 1 || last.op != "delete" {
		t.Errorf("last logged row: got %+v, want a delete of task 1", last)
	}
	for _, r := range rows {
		if r.entity == "task" && r.entityID == 1 && r.op == "upsert" {
			t.Errorf("cascade logged an upsert of the deleted task: %+v", r)
		}
	}
}

// Deleting a label leaves the tagged tasks alive with one label fewer, so each of
// them is a genuine change the replica has to pick up.
func TestMigration050_DeletingALabelLogsItsTasks(t *testing.T) {
	d := mustOpenMigrated(t)
	seedTaskFixture(t, d)

	if _, err := d.Exec(
		`INSERT INTO task_labels (task_id, label_id, created_at) VALUES (1, 1, '2026-03-01T09:00:00.000Z')`); err != nil {
		t.Fatalf("tag task: %v", err)
	}
	if _, err := d.Exec(`DELETE FROM change_log`); err != nil {
		t.Fatalf("clear log: %v", err)
	}
	if _, err := d.Exec(`DELETE FROM labels WHERE id = 1`); err != nil {
		t.Fatalf("delete label: %v", err)
	}

	rows := changeLog(t, d)
	if !hasOp(rows, "label", 1, "delete") {
		t.Errorf("label delete: missing from %v", rows)
	}
	if !hasOp(rows, "task", 1, "upsert") {
		t.Errorf("task that lost its label: missing an upsert in %v", rows)
	}
}

// A template is served as one payload — root fields, subtasks and both label
// sets — so every one of those tables logs the template, and tearing a template
// down must not leave upserts of it behind.
func TestMigration050_TemplateChildRowsLogTheTemplate(t *testing.T) {
	d := mustOpenMigrated(t)
	const ts = "2026-03-01T09:00:00.000Z"

	if _, err := d.Exec(`INSERT INTO labels (id, name, color, created_at, updated_at) VALUES (1, 'bug', 'red', ?, ?)`, ts, ts); err != nil {
		t.Fatalf("seed label: %v", err)
	}
	if _, err := d.Exec(`INSERT INTO task_templates (id, name, created_at, updated_at) VALUES (1, 'weekly', ?, ?)`, ts, ts); err != nil {
		t.Fatalf("seed template: %v", err)
	}
	if _, err := d.Exec(`DELETE FROM change_log`); err != nil {
		t.Fatalf("clear log: %v", err)
	}
	if _, err := d.Exec(
		`INSERT INTO task_template_subtasks (id, template_id, title, created_at, updated_at) VALUES (1, 1, 'step', ?, ?)`, ts, ts); err != nil {
		t.Fatalf("seed subtask: %v", err)
	}
	if _, err := d.Exec(`INSERT INTO task_template_labels (template_id, label_id) VALUES (1, 1)`); err != nil {
		t.Fatalf("seed template label: %v", err)
	}
	if _, err := d.Exec(`INSERT INTO task_template_subtask_labels (subtask_id, label_id) VALUES (1, 1)`); err != nil {
		t.Fatalf("seed subtask label: %v", err)
	}

	rows := changeLog(t, d)
	if len(rows) != 3 {
		t.Fatalf("child inserts: got %v, want 3 template upserts", rows)
	}
	for i, r := range rows {
		if r.entity != "task_template" || r.entityID != 1 || r.op != "upsert" {
			t.Errorf("row %d: got %+v, want an upsert of task_template 1", i, r)
		}
	}

	if _, err := d.Exec(`DELETE FROM change_log`); err != nil {
		t.Fatalf("clear log: %v", err)
	}
	if _, err := d.Exec(`DELETE FROM task_templates WHERE id = 1`); err != nil {
		t.Fatalf("delete template: %v", err)
	}
	rows = changeLog(t, d)
	if len(rows) != 1 || rows[0].entity != "task_template" || rows[0].entityID != 1 || rows[0].op != "delete" {
		t.Errorf("template teardown: got %v, want a single task_template delete", rows)
	}
}

// The user row backs two resources the API serves separately, so a preference
// change and a UI-state change must not be indistinguishable to a replica.
func TestMigration050_UserRowLogsSettingsAndState(t *testing.T) {
	d := mustOpenMigrated(t)
	const ts = "2026-03-01T09:00:00.000Z"

	if _, err := d.Exec(
		`INSERT INTO users (id, username, password_hash, created_at, updated_at) VALUES (1, 'admin', 'h', ?, ?)`, ts, ts); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	rows := changeLog(t, d)
	if !hasOp(rows, "user_settings", 1, "upsert") || !hasOp(rows, "user_state", 1, "upsert") {
		t.Fatalf("user insert: got %v, want both user_settings and user_state upserts", rows)
	}

	if _, err := d.Exec(`DELETE FROM change_log`); err != nil {
		t.Fatalf("clear log: %v", err)
	}
	if _, err := d.Exec(`UPDATE users SET settings = '{"locale":"en"}' WHERE id = 1`); err != nil {
		t.Fatalf("update settings: %v", err)
	}
	rows = changeLog(t, d)
	if len(rows) != 1 || rows[0].entity != "user_settings" {
		t.Errorf("settings write: got %v, want a single user_settings upsert", rows)
	}

	if _, err := d.Exec(`DELETE FROM change_log`); err != nil {
		t.Fatalf("clear log: %v", err)
	}
	if _, err := d.Exec(`UPDATE users SET state = '{"sidebar":"open"}' WHERE id = 1`); err != nil {
		t.Fatalf("update state: %v", err)
	}
	if !hasOp(changeLog(t, d), "user_state", 1, "upsert") {
		t.Errorf("state write: missing a user_state upsert in %v", changeLog(t, d))
	}
}

// The epoch shares a row with the app settings but is sync bookkeeping: bumping
// it must not masquerade as a settings edit, or every replica would refetch
// settings at the exact moment it is being told to resync from scratch.
func TestMigration050_EpochBumpIsNotASettingsChange(t *testing.T) {
	d := mustOpenMigrated(t)

	if _, err := d.Exec(`UPDATE app_settings SET data = '{"autoLabels":[]}' WHERE id = 1`); err != nil {
		t.Fatalf("update app settings: %v", err)
	}
	if !hasOp(changeLog(t, d), "app_settings", 1, "upsert") {
		t.Fatalf("app settings write: missing an upsert in %v", changeLog(t, d))
	}

	if _, err := d.Exec(`DELETE FROM change_log`); err != nil {
		t.Fatalf("clear log: %v", err)
	}
	if _, err := d.Exec(`UPDATE app_settings SET sync_epoch = sync_epoch + 1 WHERE id = 1`); err != nil {
		t.Fatalf("bump epoch: %v", err)
	}
	if rows := changeLog(t, d); len(rows) != 0 {
		t.Errorf("epoch bump: got %v, want no rows", rows)
	}
}

// Pruning old rows must never let a new change reuse a cursor value an offline
// replica has already seen, which is what AUTOINCREMENT buys over a plain rowid
// primary key.
func TestMigration050_SeqIsNeverReusedAfterPruning(t *testing.T) {
	d := mustOpenMigrated(t)
	seedTaskFixture(t, d)

	before := changeLog(t, d)
	if len(before) == 0 {
		t.Fatal("expected seeded rows to be logged")
	}
	highest := before[len(before)-1].seq

	if _, err := d.Exec(`DELETE FROM change_log`); err != nil {
		t.Fatalf("prune log: %v", err)
	}
	if _, err := d.Exec(`UPDATE tasks SET title = 'after prune' WHERE id = 1`); err != nil {
		t.Fatalf("update task: %v", err)
	}

	after := changeLog(t, d)
	if len(after) != 1 {
		t.Fatalf("rows after prune: got %v, want 1", after)
	}
	if after[0].seq <= highest {
		t.Errorf("seq after prune: got %d, want greater than %d", after[0].seq, highest)
	}
}

// The upgrade path matters more than the fresh one: a live database already
// holds rows, and the migration must attach the log to it without inventing a
// history it cannot vouch for.
func TestMigration050_UpgradeFrom049LeavesExistingRowsUnlogged(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "m050.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	migrateTo(t, d, 49)
	seedTaskFixture(t, d)

	if err := RunMigrations(context.Background(), d); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	if rows := changeLog(t, d); len(rows) != 0 {
		t.Errorf("log right after upgrade: got %v, want empty", rows)
	}
	var epoch int64
	if err := d.QueryRow(`SELECT sync_epoch FROM app_settings WHERE id = 1`).Scan(&epoch); err != nil {
		t.Fatalf("read sync epoch: %v", err)
	}
	if epoch != 1 {
		t.Errorf("sync_epoch after upgrade: got %d, want 1", epoch)
	}

	if _, err := d.Exec(`UPDATE tasks SET title = 'edited after upgrade' WHERE id = 1`); err != nil {
		t.Fatalf("update task: %v", err)
	}
	if !hasOp(changeLog(t, d), "task", 1, "upsert") {
		t.Errorf("edit after upgrade: missing an upsert in %v", changeLog(t, d))
	}
}

func TestMigration050_DownRemovesTheLogAndTheEpoch(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "m050down.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := RunMigrations(context.Background(), d); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	migrateTo(t, d, 49)

	var tables, triggers, epochCol int
	if err := d.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='change_log'`).Scan(&tables); err != nil {
		t.Fatalf("probe table: %v", err)
	}
	if tables != 0 {
		t.Errorf("change_log after down: got %d tables, want 0", tables)
	}
	if err := d.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='trigger' AND name LIKE 'trg_changelog_%'`).Scan(&triggers); err != nil {
		t.Fatalf("probe triggers: %v", err)
	}
	if triggers != 0 {
		t.Errorf("changelog triggers after down: got %d, want 0", triggers)
	}
	if err := d.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('app_settings') WHERE name='sync_epoch'`).Scan(&epochCol); err != nil {
		t.Fatalf("probe column: %v", err)
	}
	if epochCol != 0 {
		t.Errorf("app_settings.sync_epoch after down: got %d columns, want 0", epochCol)
	}
}

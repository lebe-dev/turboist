package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lebe-dev/turboist/internal/model"
)

// FederationSnapshotRepo writes the entities a joiner receives in a project
// snapshot (Federation v1 F2.3). Unlike the regular Create paths it PRESERVES
// the cross-instance client_id from the owner (the entity's portable identity,
// §3) instead of minting a fresh one, so future per-field events from any peer
// resolve to the same local row. Every method takes a *sql.Tx so the whole
// snapshot apply commits or rolls back atomically (US-2.3 AC5 — no resume).
type FederationSnapshotRepo struct {
	db *sql.DB
}

func NewFederationSnapshotRepo(db *sql.DB) *FederationSnapshotRepo {
	return &FederationSnapshotRepo{db: db}
}

// SnapshotProject is the minimal project shape carried in a snapshot. Only the
// federated field set is applied; troiki/plan/local-only fields are NOT synced.
type SnapshotProject struct {
	ClientID    string
	ContextID   int64
	Title       string
	Description string
	Color       string
	Status      string
	CreatedAt   string
	UpdatedAt   string
}

// InsertProjectTx inserts the joiner's local project for a snapshot, preserving
// the owner's client_id and marking it federated. Returns the new local int64 id.
func (r *FederationSnapshotRepo) InsertProjectTx(ctx context.Context, tx *sql.Tx, p SnapshotProject) (int64, error) {
	const op = "repo.federation_snapshot.InsertProjectTx"
	logQuery(ctx, op, p.ClientID)
	status := p.Status
	if status == "" {
		status = string(model.ProjectStatusOpen)
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO projects (context_id, title, description, color, status, project_type, is_pinned, pinned_at, is_federated, client_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 'generic', 0, NULL, 1, ?, ?, ?)`,
		p.ContextID, p.Title, p.Description, p.Color, status, p.ClientID, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return 0, logErr(ctx, op, fmt.Errorf("insert snapshot project: %w", err))
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, logErr(ctx, op, err)
	}
	return id, nil
}

// FieldWonFunc reports whether the snapshot's value for a federated field should
// overwrite the live column. The caller (ReApply) implements it as a per-field
// HLC gate: the snapshot wins only when its field HLC strictly exceeds the stored
// HLC. A field the joiner advanced locally past the snapshot therefore returns
// false and KEEPS its local column value (US-4.2 AC3 — an old-HLC value loses LWW
// gracefully, instead of the column silently regressing to the snapshot's losing
// value while the HLC keeps the winning clock).
type FieldWonFunc func(field string) bool

// StoredFieldHLCTx returns the stored per-field HLC for an entity, or "" when no
// row exists yet (treated as "always older" so a first-seen field is written).
func (r *FederationSnapshotRepo) StoredFieldHLCTx(ctx context.Context, tx *sql.Tx, entityType, entityID, field string) (string, error) {
	const op = "repo.federation_snapshot.StoredFieldHLCTx"
	var hlc string
	err := tx.QueryRowContext(ctx,
		`SELECT hlc FROM entity_field_hlc WHERE entity_type = ? AND entity_id = ? AND field_name = ?`,
		entityType, entityID, field).Scan(&hlc)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", logErr(ctx, op, fmt.Errorf("read stored field_hlc: %w", err))
	}
	return hlc, nil
}

// UpsertProjectTx overwrites an EXISTING local project's federated field set from
// a re-bootstrap snapshot (Federation v1 F4.2). Unlike InsertProjectTx (initial
// bootstrap), it targets a project already mapped to the owner by its int64 id,
// updating only the synced columns (title/description/color/status) and clearing
// any local tombstone so a project deleted-then-restored on the owner reappears.
// It NEVER touches federation_outbox (R3 — unsent local edits must survive).
//
// Each federated column is GATED by won(field): the snapshot value is written only
// for a field the snapshot wins; a field the joiner advanced locally past the
// snapshot keeps its current column value (won returns false). This is the SAME
// per-field HLC gate the inbox Applier uses, so the column and its HLC never
// diverge (US-4.2 AC3 — the field that WON LWW keeps the value that WON). The HLC
// row itself is advanced higher-wins by the caller via InsertFieldHLCTx.
func (r *FederationSnapshotRepo) UpsertProjectTx(ctx context.Context, tx *sql.Tx, localProjectID int64, p SnapshotProject, won FieldWonFunc) error {
	const op = "repo.federation_snapshot.UpsertProjectTx"
	logQuery(ctx, op, localProjectID, p.ClientID)
	status := p.Status
	if status == "" {
		status = string(model.ProjectStatusOpen)
	}
	// Read the current federated columns so a field the joiner won keeps its value.
	var curTitle, curDesc, curColor, curStatus string
	if err := tx.QueryRowContext(ctx,
		`SELECT title, description, color, status FROM projects WHERE id = ?`, localProjectID).
		Scan(&curTitle, &curDesc, &curColor, &curStatus); err != nil {
		return logErr(ctx, op, fmt.Errorf("read project columns: %w", err))
	}
	title := pickStr(won, "title", p.Title, curTitle)
	desc := pickStr(won, "description", p.Description, curDesc)
	color := pickStr(won, "color", p.Color, curColor)
	st := pickStr(won, "status", status, curStatus)
	if _, err := tx.ExecContext(ctx,
		`UPDATE projects SET title = ?, description = ?, color = ?, status = ?, deleted_at = NULL, is_federated = 1, updated_at = ?
		 WHERE id = ?`,
		title, desc, color, st, p.UpdatedAt, localProjectID); err != nil {
		return logErr(ctx, op, fmt.Errorf("upsert snapshot project: %w", err))
	}
	return nil
}

// pickStr returns the snapshot value for a field the snapshot wins, else the
// current local value (the per-field HLC gate kept the local edit).
func pickStr(won FieldWonFunc, field, snapVal, curVal string) string {
	if won(field) {
		return snapVal
	}
	return curVal
}

// SnapshotSection is the minimal section shape carried in a snapshot.
type SnapshotSection struct {
	ClientID  string
	Title     string
	Position  int
	CreatedAt string
	UpdatedAt string
}

// InsertSectionTx inserts a section under the local project, preserving its
// client_id, and returns the new local int64 id.
func (r *FederationSnapshotRepo) InsertSectionTx(ctx context.Context, tx *sql.Tx, localProjectID int64, s SnapshotSection) (int64, error) {
	const op = "repo.federation_snapshot.InsertSectionTx"
	logQuery(ctx, op, localProjectID, s.ClientID)
	res, err := tx.ExecContext(ctx,
		`INSERT INTO project_sections (project_id, title, position, client_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		localProjectID, s.Title, s.Position, s.ClientID, s.CreatedAt, s.UpdatedAt)
	if err != nil {
		return 0, logErr(ctx, op, fmt.Errorf("insert snapshot section: %w", err))
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, logErr(ctx, op, err)
	}
	return id, nil
}

// UpsertSectionTx inserts a section by client_id, or — when a section with that
// cross-instance client_id already exists locally — overwrites its federated
// fields and clears any local tombstone (Federation v1 F4.2 re-bootstrap). It
// returns the local int64 id either way so the caller can re-link tasks. Sections
// are matched on client_id; a snapshot section the joiner has never seen is
// inserted under the project. The ON CONFLICT requires the partial UNIQUE on
// client_id (024_offline_sync.sql) so the upsert is a single statement.
func (r *FederationSnapshotRepo) UpsertSectionTx(ctx context.Context, tx *sql.Tx, localProjectID int64, s SnapshotSection) (int64, error) {
	const op = "repo.federation_snapshot.UpsertSectionTx"
	logQuery(ctx, op, localProjectID, s.ClientID)
	var existingID int64
	err := tx.QueryRowContext(ctx,
		`SELECT id FROM project_sections WHERE client_id = ?`, s.ClientID).Scan(&existingID)
	if err == nil {
		if _, uerr := tx.ExecContext(ctx,
			`UPDATE project_sections SET project_id = ?, title = ?, position = ?, deleted_at = NULL, updated_at = ?
			 WHERE id = ?`,
			localProjectID, s.Title, s.Position, s.UpdatedAt, existingID); uerr != nil {
			return 0, logErr(ctx, op, fmt.Errorf("update snapshot section: %w", uerr))
		}
		return existingID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, logErr(ctx, op, fmt.Errorf("lookup snapshot section: %w", err))
	}
	return r.InsertSectionTx(ctx, tx, localProjectID, s)
}

// SnapshotTask is the minimal, FEDERATED task field set carried in a snapshot.
// Troiki/plan/day-part/postpone/pin fields are opaque local-only and excluded
// (§3 DEVIATE row) so a peer without those features applies cleanly. SectionID
// is the local int64 resolved from the section client_id (nil if none). ParentID
// is the local int64 resolved from the parent task's client_id (nil for a
// top-level task), preserving the subtask hierarchy across instances.
type SnapshotTask struct {
	ClientID string
	// ContextID is the joiner's local context the federated project hangs off.
	// The tasks placement CHECK (001_schema.sql) requires a project task to carry
	// a non-NULL context_id, so it is supplied locally (it is not a synced field).
	ContextID       int64
	Title           string
	Description     string
	Priority        string
	Status          string
	DueAt           *string
	DueHasTime      bool
	DeadlineAt      *string
	DeadlineHasTime bool
	CompletedAt     *string
	SectionID       *int64
	// ParentID is the local int64 of the parent task resolved from the parent's
	// section/parent client_id (nil for a top-level task). Preserving it keeps the
	// subtask hierarchy intact across instances instead of flattening it.
	ParentID  *int64
	CreatedAt string
	UpdatedAt string
}

// InsertTaskTx inserts a task under the local project, preserving its client_id.
// Tasks placed in a section keep the section pointer; tasks with no section are
// attached directly to the project (project_id + context_id set, section_id NULL).
// Subtasks keep their parent pointer (parent_id set to the resolved local id) so
// the hierarchy is not flattened on bootstrap.
func (r *FederationSnapshotRepo) InsertTaskTx(ctx context.Context, tx *sql.Tx, localProjectID int64, t SnapshotTask) (int64, error) {
	const op = "repo.federation_snapshot.InsertTaskTx"
	logQuery(ctx, op, localProjectID, t.ClientID)
	priority := t.Priority
	if priority == "" {
		priority = string(model.PriorityNone)
	}
	status := t.Status
	if status == "" {
		status = string(model.TaskStatusOpen)
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO tasks (title, description, context_id, project_id, section_id, parent_id, priority, status,
			due_at, due_has_time, deadline_at, deadline_has_time, day_part, plan_state,
			is_pinned, completed_at, client_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'none', 'none', 0, ?, ?, ?, ?)`,
		t.Title, t.Description, t.ContextID, localProjectID, nullInt(t.SectionID), nullInt(t.ParentID), priority, status,
		nullStr(t.DueAt), boolInt(t.DueHasTime), nullStr(t.DeadlineAt), boolInt(t.DeadlineHasTime),
		nullStr(t.CompletedAt), t.ClientID, t.CreatedAt, t.UpdatedAt)
	if err != nil {
		return 0, logErr(ctx, op, fmt.Errorf("insert snapshot task: %w", err))
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, logErr(ctx, op, err)
	}
	return id, nil
}

// UpsertTaskTx inserts a task by client_id, or — when a task with that
// cross-instance client_id already exists locally — overwrites its federated
// fields and clears any local tombstone (Federation v1 F4.2 re-bootstrap). It
// returns the local int64 id either way so a subtask can resolve its parent.
//
// Each federated VALUE column is GATED by won(field): a field the joiner advanced
// locally past the snapshot keeps its current value (won returns false), so the
// column and its per-field HLC stay consistent (US-4.2 AC3). The structural
// placement (project_id/section_id/parent_id) is NOT a per-field HLC field — it
// converges to the snapshot unconditionally so a re-parented/re-sectioned task
// follows the owner. A first-seen task is inserted with all snapshot values (no
// local value to preserve). Local-only/troiki columns are left untouched.
func (r *FederationSnapshotRepo) UpsertTaskTx(ctx context.Context, tx *sql.Tx, localProjectID int64, t SnapshotTask, won FieldWonFunc) (int64, error) {
	const op = "repo.federation_snapshot.UpsertTaskTx"
	logQuery(ctx, op, localProjectID, t.ClientID)
	var existingID int64
	err := tx.QueryRowContext(ctx,
		`SELECT id FROM tasks WHERE client_id = ?`, t.ClientID).Scan(&existingID)
	if err == nil {
		priority := t.Priority
		if priority == "" {
			priority = string(model.PriorityNone)
		}
		status := t.Status
		if status == "" {
			status = string(model.TaskStatusOpen)
		}
		// Read the current federated columns so a field the joiner won keeps its
		// value rather than regressing to the snapshot's losing value.
		var cur taskFederatedCols
		if rerr := tx.QueryRowContext(ctx,
			`SELECT title, description, priority, status, due_at, due_has_time,
			        deadline_at, deadline_has_time, completed_at
			   FROM tasks WHERE id = ?`, existingID).
			Scan(&cur.title, &cur.description, &cur.priority, &cur.status, &cur.dueAt, &cur.dueHasTime,
				&cur.deadlineAt, &cur.deadlineHasTime, &cur.completedAt); rerr != nil {
			return 0, logErr(ctx, op, fmt.Errorf("read task columns: %w", rerr))
		}
		title := pickStr(won, "title", t.Title, cur.title)
		desc := pickStr(won, "description", t.Description, cur.description)
		prio := pickStr(won, "priority", priority, cur.priority)
		st := pickStr(won, "status", status, cur.status)
		dueAt := pickNullStr(won, "due_at", t.DueAt, nullStrToPtr(cur.dueAt))
		dueHas := pickBoolCol(won, "due_has_time", boolInt(t.DueHasTime), cur.dueHasTime)
		deadlineAt := pickNullStr(won, "deadline_at", t.DeadlineAt, nullStrToPtr(cur.deadlineAt))
		deadlineHas := pickBoolCol(won, "deadline_has_time", boolInt(t.DeadlineHasTime), cur.deadlineHasTime)
		completedAt := pickNullStr(won, "completed_at", t.CompletedAt, nullStrToPtr(cur.completedAt))
		if _, uerr := tx.ExecContext(ctx,
			`UPDATE tasks SET title = ?, description = ?, project_id = ?, section_id = ?, parent_id = ?,
				priority = ?, status = ?, due_at = ?, due_has_time = ?, deadline_at = ?, deadline_has_time = ?,
				completed_at = ?, deleted_at = NULL, updated_at = ?
			 WHERE id = ?`,
			title, desc, localProjectID, nullInt(t.SectionID), nullInt(t.ParentID),
			prio, st, dueAt, dueHas, deadlineAt, deadlineHas,
			completedAt, t.UpdatedAt, existingID); uerr != nil {
			return 0, logErr(ctx, op, fmt.Errorf("update snapshot task: %w", uerr))
		}
		return existingID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, logErr(ctx, op, fmt.Errorf("lookup snapshot task: %w", err))
	}
	return r.InsertTaskTx(ctx, tx, localProjectID, t)
}

// taskFederatedCols holds the current federated VALUE columns of a task, read so a
// locally-won field can keep its value during a gated re-bootstrap upsert.
type taskFederatedCols struct {
	title           string
	description     string
	priority        string
	status          string
	dueAt           sql.NullString
	dueHasTime      int
	deadlineAt      sql.NullString
	deadlineHasTime int
	completedAt     sql.NullString
}

// pickNullStr returns the snapshot nullable value for a won field, else the
// current local nullable value.
func pickNullStr(won FieldWonFunc, field string, snapVal, curVal *string) any {
	if won(field) {
		return nullStr(snapVal)
	}
	return nullStr(curVal)
}

// pickBoolCol returns the snapshot int-bool for a won field, else the current
// local int-bool.
func pickBoolCol(won FieldWonFunc, field string, snapVal, curVal int) int {
	if won(field) {
		return snapVal
	}
	return curVal
}

// nullStrToPtr converts a sql.NullString to *string (nil when not valid) so it can
// feed pickNullStr alongside the snapshot's *string value.
func nullStrToPtr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	v := ns.String
	return &v
}

// SoftDeleteByClientIDTx soft-deletes a federated entity (task/section) by its
// cross-instance client_id from a re-bootstrap snapshot tombstone (Federation v1
// F4.2). A tombstone in the snapshot means the owner deleted the entity since the
// joiner last synced; the joiner must mirror that without resurrecting it. The
// statement is a no-op when the joiner has never seen the entity (orphan
// tombstone) or it is already deleted. table must be a trusted constant.
func (r *FederationSnapshotRepo) SoftDeleteByClientIDTx(ctx context.Context, tx *sql.Tx, table, clientID, now string) error {
	const op = "repo.federation_snapshot.SoftDeleteByClientIDTx"
	if table != "tasks" && table != "project_sections" {
		return fmt.Errorf("%s: unsupported table %q", op, table)
	}
	if _, err := tx.ExecContext(ctx,
		fmt.Sprintf(`UPDATE %s SET deleted_at = ?, updated_at = ? WHERE client_id = ? AND deleted_at IS NULL`, table),
		now, now, clientID); err != nil {
		return logErr(ctx, op, fmt.Errorf("soft-delete %s by client_id: %w", table, err))
	}
	return nil
}

// SoftDeleteLiveTasksNotInTx soft-deletes every LIVE task of a re-bootstrapped
// project whose client_id is NOT in the snapshot's live set (Federation v1 F4.2).
// A live local task absent from a fresh owner snapshot has been removed upstream
// (deleted or moved out of the project) and must not linger; this converges the
// joiner's project to the snapshot. Local tasks with NO client_id (never
// federated) are left untouched — they cannot belong to the federated set. The
// keep set is the snapshot's live + tombstoned client_ids (a tombstone is handled
// by SoftDeleteByClientIDTx; passing it here too is harmless).
//
// CRITICALLY, a task the joiner created LOCALLY while offline past retention
// carries its own client_id (tasks.go mints one on every create) but is absent
// from the owner snapshot (the owner never received it) — yet its op=create still
// sits unsent in federation_outbox (R3 preserves the outbox). Soft-deleting it
// would make the user's own task vanish from their UI until the preserved outbox
// event flushes and the owner echoes it back. The sweep therefore EXCLUDES any
// task whose client_id is still referenced (as entity_id) by an undelivered
// federation_outbox row for this project: those are the joiner's own unsent
// creates/edits and must survive the convergence (US-4.2 AC2/AC3 — unsent local
// work is preserved; "the highest-impact F4.2 bug" is blowing it away).
func (r *FederationSnapshotRepo) SoftDeleteLiveTasksNotInTx(ctx context.Context, tx *sql.Tx, localProjectID int64, keepClientIDs []string, now string) error {
	const op = "repo.federation_snapshot.SoftDeleteLiveTasksNotInTx"
	logQuery(ctx, op, localProjectID, len(keepClientIDs))
	query := `UPDATE tasks SET deleted_at = ?, updated_at = ?
	          WHERE project_id = ? AND deleted_at IS NULL AND client_id IS NOT NULL AND client_id != ''
	            AND client_id NOT IN (
	              SELECT json_extract(payload, '$.entity_id') FROM federation_outbox
	               WHERE local_project_id = ? AND json_extract(payload, '$.entity_id') IS NOT NULL
	            )`
	args := []any{now, now, localProjectID, localProjectID}
	if len(keepClientIDs) > 0 {
		placeholders := make([]string, len(keepClientIDs))
		for i, cid := range keepClientIDs {
			placeholders[i] = "?"
			args = append(args, cid)
		}
		query += " AND client_id NOT IN (" + joinComma(placeholders) + ")"
	}
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return logErr(ctx, op, fmt.Errorf("converge live tasks: %w", err))
	}
	return nil
}

// SoftDeleteLiveSectionsNotInTx is the section analogue of
// SoftDeleteLiveTasksNotInTx (Federation v1 F4.2): it soft-deletes every LIVE
// project_section of a re-bootstrapped project whose client_id is NOT in the
// snapshot's live+tombstoned section set, so an owner-deleted section converges
// on the joiner instead of lingering as a ghost. Sections with no client_id are
// left untouched, and — exactly as with tasks — a section whose client_id is
// still referenced by an undelivered federation_outbox row (the joiner's own
// unsent create) is preserved (US-4.2 AC2/AC3, R3).
func (r *FederationSnapshotRepo) SoftDeleteLiveSectionsNotInTx(ctx context.Context, tx *sql.Tx, localProjectID int64, keepClientIDs []string, now string) error {
	const op = "repo.federation_snapshot.SoftDeleteLiveSectionsNotInTx"
	logQuery(ctx, op, localProjectID, len(keepClientIDs))
	query := `UPDATE project_sections SET deleted_at = ?, updated_at = ?
	          WHERE project_id = ? AND deleted_at IS NULL AND client_id IS NOT NULL AND client_id != ''
	            AND client_id NOT IN (
	              SELECT json_extract(payload, '$.entity_id') FROM federation_outbox
	               WHERE local_project_id = ? AND json_extract(payload, '$.entity_id') IS NOT NULL
	            )`
	args := []any{now, now, localProjectID, localProjectID}
	if len(keepClientIDs) > 0 {
		placeholders := make([]string, len(keepClientIDs))
		for i, cid := range keepClientIDs {
			placeholders[i] = "?"
			args = append(args, cid)
		}
		query += " AND client_id NOT IN (" + joinComma(placeholders) + ")"
	}
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return logErr(ctx, op, fmt.Errorf("converge live sections: %w", err))
	}
	return nil
}

// joinComma joins SQL placeholders with commas (kept local to avoid pulling in
// strings just for this).
func joinComma(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ","
		}
		out += p
	}
	return out
}

// InsertFieldHLCTx writes one per-field HLC row from a snapshot. entity_id is the
// entity's client_id (the cross-instance identity). On conflict it keeps the
// higher HLC so a re-bootstrap never regresses a field clock (F4.2 forward-safe).
func (r *FederationSnapshotRepo) InsertFieldHLCTx(ctx context.Context, tx *sql.Tx, entityType, entityID, field, hlc string) error {
	const op = "repo.federation_snapshot.InsertFieldHLCTx"
	_, err := tx.ExecContext(ctx,
		`INSERT INTO entity_field_hlc (entity_type, entity_id, field_name, hlc)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(entity_type, entity_id, field_name) DO UPDATE SET
		   hlc = CASE WHEN excluded.hlc > entity_field_hlc.hlc THEN excluded.hlc ELSE entity_field_hlc.hlc END`,
		entityType, entityID, field, hlc)
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("insert field_hlc: %w", err))
	}
	return nil
}

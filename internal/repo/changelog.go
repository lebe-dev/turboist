package repo

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
)

// queryer is the read half of *sql.DB and *sql.Tx. Query bodies that both the
// pool and an open transaction need are written against it, so a reader that
// wants every row of a page to come from one snapshot can pass its transaction
// without any query being duplicated.
type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Entity names written into change_log.entity by the triggers, and served back
// as the `entity` field of a delta page. They are a wire contract: a replica
// keys its local tables off these strings, so they are append-only — renaming
// one silently breaks every client that has already synced.
const (
	SyncEntityTask         = "task"
	SyncEntityProject      = "project"
	SyncEntitySection      = "section"
	SyncEntityContext      = "context"
	SyncEntityLabel        = "label"
	SyncEntityTaskRelation = "task_relation"
	SyncEntityTaskTemplate = "task_template"
	SyncEntityUserSettings = "user_settings"
	SyncEntityUserState    = "user_state"
	SyncEntityAppSettings  = "app_settings"
)

// The two operations a change can carry. A delete is the tombstone for a row
// that is gone from its table; there are no soft deletes to read instead.
const (
	SyncOpUpsert = "upsert"
	SyncOpDelete = "delete"
)

// MaxSyncChangeLimit caps one page. A replica catching up after a long offline
// stretch pages rather than asking for the whole backlog at once, which keeps
// the hydration work (and the response) bounded on a phone.
const MaxSyncChangeLimit = 500

// SyncUserState is the per-user UI blob, carried verbatim as stored so the
// delta serves byte-for-byte what GET /state serves.
type SyncUserState string

// SyncEpochMismatchError says the caller's cursor was stamped under a different
// epoch than the one the log is now keeping. The history it was reading was
// replaced underneath it, so resuming would silently diverge; the remedy is a
// full snapshot.
type SyncEpochMismatchError struct {
	Current int64
}

func (e *SyncEpochMismatchError) Error() string {
	return fmt.Sprintf("sync epoch mismatch: current epoch is %d", e.Current)
}

// SyncCursorExpiredError says the changes the caller still needs have already
// been pruned: the oldest row the log still holds is newer than the next one it
// asked for. Same remedy — a full snapshot.
type SyncCursorExpiredError struct {
	Since          int64
	OldestRetained int64
	Epoch          int64
}

func (e *SyncCursorExpiredError) Error() string {
	return fmt.Sprintf("sync cursor %d expired: oldest retained change is %d", e.Since, e.OldestRetained)
}

// SyncChange is one entity's net change since the caller's cursor. Seq is the
// highest changelog row seen for that entity in the window, which is what the
// caller stores as its new cursor once the change is applied.
//
// Payload is the entity's current row, hydrated from the live tables — the log
// records pointers, never payloads. It is nil for a delete, and otherwise holds
// one of *model.Task, *model.Project, *model.ProjectSection, *model.Context,
// *model.Label, *model.TaskRelation, *model.TaskTemplate, *model.UserSettings,
// *model.AppSettings or SyncUserState.
type SyncChange struct {
	Seq      int64
	Entity   string
	EntityID int64
	Op       string
	Payload  any
}

// SyncChangePage is one page of changes plus the bookkeeping the caller needs
// to ask for the next one.
type SyncChangePage struct {
	Epoch   int64
	Cursor  int64
	HasMore bool
	Changes []SyncChange
}

// SyncChangesQuery asks for everything that happened after Since. Epoch, when
// set, is the epoch the caller believes it is synced against; leaving it nil
// skips the check, which is how a caller learns the current epoch on its first
// call.
type SyncChangesQuery struct {
	Since int64
	Epoch *int64
	Limit int
}

// ChangeLogRepo reads the append-only change log and turns it into the current
// state of everything that moved.
type ChangeLogRepo struct {
	db *sql.DB
}

func NewChangeLogRepo(db *sql.DB) *ChangeLogRepo {
	return &ChangeLogRepo{db: db}
}

// Changes returns one page of net changes after q.Since.
//
// The whole page is read inside a single transaction: the epoch, the log window
// and every hydrated row come from one snapshot, so a page can never claim a
// cursor that is newer than the rows it carries. Callers loop with
// Since = Cursor until HasMore is false.
func (r *ChangeLogRepo) Changes(ctx context.Context, q SyncChangesQuery) (*SyncChangePage, error) {
	const op = "repo.change_log.Changes"
	logQuery(ctx, op, q.Since, q.Limit)

	limit := q.Limit
	if limit <= 0 || limit > MaxSyncChangeLimit {
		limit = MaxSyncChangeLimit
	}
	since := q.Since
	if since < 0 {
		since = 0
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("begin read tx: %w", err))
	}
	// Read-only: rolled back on every path, including the happy one.
	defer func() { _ = tx.Rollback() }()

	epoch, err := txSyncEpoch(ctx, tx)
	if err != nil {
		return nil, logErr(ctx, op, err)
	}
	if q.Epoch != nil && *q.Epoch != epoch {
		return nil, &SyncEpochMismatchError{Current: epoch}
	}

	oldest, err := txOldestRetainedSeq(ctx, tx)
	if err != nil {
		return nil, logErr(ctx, op, err)
	}
	// The caller holds everything up to `since`, so the next change it needs is
	// since+1. If the log no longer goes back that far, the gap was pruned and a
	// delta cannot close it.
	if since+1 < oldest {
		return nil, &SyncCursorExpiredError{Since: since, OldestRetained: oldest, Epoch: epoch}
	}

	rows, err := txDedupedChanges(ctx, tx, since, limit+1)
	if err != nil {
		return nil, logErr(ctx, op, err)
	}

	page := &SyncChangePage{Epoch: epoch, Cursor: since}
	if len(rows) > limit {
		page.HasMore = true
		rows = rows[:limit]
	}
	if len(rows) > 0 {
		page.Cursor = rows[len(rows)-1].Seq
	}

	if err := txHydrate(ctx, tx, rows); err != nil {
		return nil, logErr(ctx, op, err)
	}
	page.Changes = rows
	return page, nil
}

// txSyncEpoch reads the epoch inside the caller's transaction so it cannot
// disagree with the log window read next to it.
func txSyncEpoch(ctx context.Context, src queryer) (int64, error) {
	var epoch int64
	err := src.QueryRowContext(ctx, `SELECT sync_epoch FROM app_settings WHERE id = 1`).Scan(&epoch)
	if err == sql.ErrNoRows {
		return defaultSyncEpoch, nil
	}
	if err != nil {
		return 0, fmt.Errorf("get sync epoch: %w", err)
	}
	return epoch, nil
}

// txOldestRetainedSeq is the lowest seq the log still holds — the boundary a
// cursor must not have fallen behind.
//
// An empty log is not the same as "nothing was ever logged": pruning can empty
// it. sqlite_sequence remembers the highest seq ever handed out even when no row
// survives, so the boundary is that value plus one — a cursor equal to it lost
// nothing, an older one did.
func txOldestRetainedSeq(ctx context.Context, src queryer) (int64, error) {
	var min sql.NullInt64
	if err := src.QueryRowContext(ctx, `SELECT MIN(seq) FROM change_log`).Scan(&min); err != nil {
		return 0, fmt.Errorf("get oldest change seq: %w", err)
	}
	if min.Valid {
		return min.Int64, nil
	}
	var highest sql.NullInt64
	err := src.QueryRowContext(ctx,
		`SELECT seq FROM sqlite_sequence WHERE name = 'change_log'`).Scan(&highest)
	if err != nil && err != sql.ErrNoRows {
		return 0, fmt.Errorf("get highest change seq: %w", err)
	}
	if !highest.Valid {
		return 1, nil
	}
	return highest.Int64 + 1, nil
}

// txDedupedChanges collapses the window down to one row per entity — the one
// with the highest seq, which is the only one whose op still describes reality.
// A task edited twelve times is one change to apply, and an entity created and
// then deleted is just the delete.
func txDedupedChanges(ctx context.Context, src queryer, since int64, limit int) ([]SyncChange, error) {
	const q = `SELECT c.entity, c.entity_id, c.seq, c.op
	             FROM change_log c
	             JOIN (SELECT entity, entity_id, MAX(seq) AS max_seq
	                     FROM change_log WHERE seq > ?
	                    GROUP BY entity, entity_id) m
	               ON m.entity = c.entity AND m.entity_id = c.entity_id AND m.max_seq = c.seq
	            ORDER BY c.seq ASC
	            LIMIT ?`
	rows, err := src.QueryContext(ctx, q, since, limit)
	if err != nil {
		return nil, fmt.Errorf("list changes: %w", err)
	}
	defer logging.LogClose(ctx, "repo.change_log.Changes.rows", rows)

	out := make([]SyncChange, 0)
	for rows.Next() {
		var ch SyncChange
		if err := rows.Scan(&ch.Entity, &ch.EntityID, &ch.Seq, &ch.Op); err != nil {
			return nil, err
		}
		out = append(out, ch)
	}
	return out, rows.Err()
}

// txHydrate fills in the current row behind every upsert, and rewrites an
// upsert whose row is gone into a delete.
//
// The rewrite is what makes a page self-consistent. The log window may end
// before the delete that removed a row, or the row may have been removed by a
// cascade the log attributes to its owner; either way an upsert with nothing to
// serve would tell the replica to keep a row the server no longer has.
func txHydrate(ctx context.Context, src queryer, changes []SyncChange) error {
	byEntity := map[string][]int64{}
	for i := range changes {
		if changes[i].Op != SyncOpUpsert {
			continue
		}
		byEntity[changes[i].Entity] = append(byEntity[changes[i].Entity], changes[i].EntityID)
	}

	payloads := map[string]map[int64]any{}
	for entity, ids := range byEntity {
		loaded, err := txLoadEntity(ctx, src, entity, ids)
		if err != nil {
			return err
		}
		payloads[entity] = loaded
	}

	for i := range changes {
		ch := &changes[i]
		if ch.Op != SyncOpUpsert {
			continue
		}
		payload, ok := payloads[ch.Entity][ch.EntityID]
		if !ok {
			ch.Op = SyncOpDelete
			continue
		}
		ch.Payload = payload
	}
	return nil
}

// txLoadEntity loads the current rows for one entity kind, keyed by id. A
// missing key means the row is gone.
func txLoadEntity(ctx context.Context, src queryer, entity string, ids []int64) (map[int64]any, error) {
	switch entity {
	case SyncEntityTask:
		return txLoadTasks(ctx, src, ids)
	case SyncEntityProject:
		return txLoadProjects(ctx, src, ids)
	case SyncEntitySection:
		return txLoadSections(ctx, src, ids)
	case SyncEntityContext:
		return txLoadContexts(ctx, src, ids)
	case SyncEntityLabel:
		return txLoadLabels(ctx, src, ids)
	case SyncEntityTaskRelation:
		return txLoadTaskRelations(ctx, src, ids)
	case SyncEntityTaskTemplate:
		return txLoadTaskTemplates(ctx, src, ids)
	case SyncEntityUserSettings:
		return txLoadUserSettings(ctx, src, ids)
	case SyncEntityUserState:
		return txLoadUserState(ctx, src, ids)
	case SyncEntityAppSettings:
		return txLoadAppSettings(ctx, src, ids)
	}
	// The triggers and this switch are written together, so an entity that lands
	// here is a bug rather than a client's mistake. Answering "no such row" would
	// hydrate into a delete and tell the replica to throw away live data, so the
	// page fails loudly instead.
	return nil, fmt.Errorf("unknown sync entity %q", entity)
}

// idPlaceholders renders "?, ?, ?" plus the matching argument slice.
func idPlaceholders(ids []int64) (string, []any) {
	marks := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		marks[i] = "?"
		args[i] = id
	}
	return strings.Join(marks, ","), args
}

func txLoadTasks(ctx context.Context, src queryer, ids []int64) (map[int64]any, error) {
	marks, args := idPlaceholders(ids)
	rows, err := src.QueryContext(ctx, `SELECT `+taskColumns+` FROM tasks WHERE id IN (`+marks+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("load sync tasks: %w", err)
	}
	defer logging.LogClose(ctx, "repo.change_log.tasks.rows", rows)

	tasks := make([]*model.Task, 0, len(ids))
	found := make([]int64, 0, len(ids))
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
		found = append(found, t.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// The same two batch loaders every other read path uses, so a task served in
	// a delta is indistinguishable from the same task served by GET /tasks/:id.
	labels, err := labelsByTaskIDs(ctx, src, found)
	if err != nil {
		return nil, err
	}
	summaries, err := summaryByTaskIDs(ctx, src, found)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]any, len(tasks))
	for _, t := range tasks {
		t.Labels = labels[t.ID]
		t.RelationSummary = summaries[t.ID]
		out[t.ID] = t
	}
	return out, nil
}

func txLoadProjects(ctx context.Context, src queryer, ids []int64) (map[int64]any, error) {
	marks, args := idPlaceholders(ids)
	rows, err := src.QueryContext(ctx, `SELECT `+projectColumns+` FROM projects WHERE id IN (`+marks+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("load sync projects: %w", err)
	}
	defer logging.LogClose(ctx, "repo.change_log.projects.rows", rows)

	projects := make([]*model.Project, 0, len(ids))
	found := make([]int64, 0, len(ids))
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		projects = append(projects, p)
		found = append(found, p.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	labels, err := labelsByProjectIDs(ctx, src, found)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]any, len(projects))
	for _, p := range projects {
		p.Labels = labels[p.ID]
		out[p.ID] = p
	}
	return out, nil
}

func txLoadSections(ctx context.Context, src queryer, ids []int64) (map[int64]any, error) {
	marks, args := idPlaceholders(ids)
	rows, err := src.QueryContext(ctx,
		`SELECT id, project_id, title, position, created_at, updated_at
		   FROM project_sections WHERE id IN (`+marks+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("load sync sections: %w", err)
	}
	defer logging.LogClose(ctx, "repo.change_log.sections.rows", rows)

	out := make(map[int64]any, len(ids))
	for rows.Next() {
		s, err := scanSection(rows)
		if err != nil {
			return nil, err
		}
		out[s.ID] = s
	}
	return out, rows.Err()
}

func txLoadContexts(ctx context.Context, src queryer, ids []int64) (map[int64]any, error) {
	marks, args := idPlaceholders(ids)
	rows, err := src.QueryContext(ctx,
		`SELECT id, name, color, is_favourite, created_at, updated_at
		   FROM contexts WHERE id IN (`+marks+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("load sync contexts: %w", err)
	}
	defer logging.LogClose(ctx, "repo.change_log.contexts.rows", rows)

	out := make(map[int64]any, len(ids))
	for rows.Next() {
		c, err := scanContext(rows)
		if err != nil {
			return nil, err
		}
		out[c.ID] = c
	}
	return out, rows.Err()
}

func txLoadLabels(ctx context.Context, src queryer, ids []int64) (map[int64]any, error) {
	marks, args := idPlaceholders(ids)
	rows, err := src.QueryContext(ctx,
		`SELECT id, name, color, is_favourite, is_private, created_at, updated_at
		   FROM labels WHERE id IN (`+marks+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("load sync labels: %w", err)
	}
	defer logging.LogClose(ctx, "repo.change_log.labels.rows", rows)

	out := make(map[int64]any, len(ids))
	for rows.Next() {
		l, err := scanLabel(rows)
		if err != nil {
			return nil, err
		}
		out[l.ID] = l
	}
	return out, rows.Err()
}

// txLoadTaskRelations serves the stored edge itself — source, target, type —
// rather than the direction-relative view a task detail page gets. A replica
// keeps the edge table and derives direction locally for whichever end it is
// rendering, so shipping one endpoint's perspective would be the wrong half.
func txLoadTaskRelations(ctx context.Context, src queryer, ids []int64) (map[int64]any, error) {
	marks, args := idPlaceholders(ids)
	rows, err := src.QueryContext(ctx,
		`SELECT id, source_task_id, target_task_id, type, created_at
		   FROM task_relations WHERE id IN (`+marks+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("load sync task relations: %w", err)
	}
	defer logging.LogClose(ctx, "repo.change_log.task_relations.rows", rows)

	out := make(map[int64]any, len(ids))
	for rows.Next() {
		rel, err := scanTaskRelationEdge(rows)
		if err != nil {
			return nil, err
		}
		out[rel.ID] = rel
	}
	return out, rows.Err()
}

// scanTaskRelationEdge reads one stored edge — source, target, type — in the
// column order both the delta feed and the snapshot select it in.
func scanTaskRelationEdge(row interface{ Scan(...any) error }) (*model.TaskRelation, error) {
	var rel model.TaskRelation
	var relType, createdAt string
	if err := row.Scan(&rel.ID, &rel.SourceTaskID, &rel.TargetTaskID, &relType, &createdAt); err != nil {
		return nil, err
	}
	rel.Type = model.RelationType(relType)
	ts, err := model.ParseUTC(createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	rel.CreatedAt = ts
	return &rel, nil
}

func txLoadTaskTemplates(ctx context.Context, src queryer, ids []int64) (map[int64]any, error) {
	marks, args := idPlaceholders(ids)
	rows, err := src.QueryContext(ctx,
		`SELECT id, name, description, priority, day_part, position, created_at, updated_at
		   FROM task_templates WHERE id IN (`+marks+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("load sync task templates: %w", err)
	}
	defer logging.LogClose(ctx, "repo.change_log.task_templates.rows", rows)

	templates := make([]*model.TaskTemplate, 0, len(ids))
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			return nil, err
		}
		templates = append(templates, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make(map[int64]any, len(templates))
	for _, t := range templates {
		if err := hydrateTemplate(ctx, src, t); err != nil {
			return nil, err
		}
		out[t.ID] = t
	}
	return out, nil
}

func txLoadUserSettings(ctx context.Context, src queryer, ids []int64) (map[int64]any, error) {
	return txLoadUserColumn(ctx, src, ids, `SELECT id, settings FROM users WHERE id IN `,
		func(raw string) any { return decodeUserSettings(raw) })
}

func txLoadUserState(ctx context.Context, src queryer, ids []int64) (map[int64]any, error) {
	return txLoadUserColumn(ctx, src, ids, `SELECT id, state FROM users WHERE id IN `,
		func(raw string) any {
			if raw == "" {
				raw = "{}"
			}
			return SyncUserState(raw)
		})
}

// txLoadUserColumn reads one blob column off the user rows. The single user row
// carries both the preference blob and the UI-state blob, and the API serves
// them as two resources, so they arrive here as two entities off one table.
func txLoadUserColumn(ctx context.Context, src queryer, ids []int64, query string, decode func(string) any) (map[int64]any, error) {
	marks, args := idPlaceholders(ids)
	rows, err := src.QueryContext(ctx, query+`(`+marks+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("load sync user blob: %w", err)
	}
	defer logging.LogClose(ctx, "repo.change_log.users.rows", rows)

	out := make(map[int64]any, len(ids))
	for rows.Next() {
		var id int64
		var raw string
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		out[id] = decode(raw)
	}
	return out, rows.Err()
}

func txLoadAppSettings(ctx context.Context, src queryer, ids []int64) (map[int64]any, error) {
	marks, args := idPlaceholders(ids)
	rows, err := src.QueryContext(ctx, `SELECT id, data FROM app_settings WHERE id IN (`+marks+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("load sync app settings: %w", err)
	}
	defer logging.LogClose(ctx, "repo.change_log.app_settings.rows", rows)

	out := make(map[int64]any, len(ids))
	for rows.Next() {
		var id int64
		var raw string
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		out[id] = decodeAppSettings(raw)
	}
	return out, rows.Err()
}

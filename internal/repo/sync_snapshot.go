package repo

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
)

// SyncHistoryWindow is how far back a replica is seeded with, and how long the
// change log keeps history for. One number governs both on purpose: a client is
// handed completed tasks for exactly the stretch it can still catch up over, so
// the two never disagree about what "old enough to drop" means. Anything older
// stays on the server and is read online, on demand.
const SyncHistoryWindow = 90 * 24 * time.Hour

// appSettingsSingletonID is the one row app_settings ever holds.
const appSettingsSingletonID int64 = 1

// SyncSnapshot is a complete replica seed: every syncable entity plus the
// {Epoch, Cursor} pair the caller resumes the delta feed from.
//
// Cursor is read inside the same transaction as the rows, so it can never run
// ahead of them. A change committed while the snapshot was being read is either
// already reflected in the rows and at or below the cursor, or lands above the
// cursor and arrives with the next delta page; the worst case is that a caller
// re-applies a row it already has, which is free.
type SyncSnapshot struct {
	Epoch  int64
	Cursor int64

	// CompletedSince is the cutoff the task window was taken at: completed tasks
	// older than this are absent, deliberately, and there is no delta that will
	// ever deliver them.
	CompletedSince time.Time

	Tasks         []*model.Task
	Projects      []*model.Project
	Sections      []*model.ProjectSection
	Contexts      []*model.Context
	Labels        []*model.Label
	TaskRelations []*model.TaskRelation
	TaskTemplates []*model.TaskTemplate

	UserSettings *model.UserSettings
	UserState    SyncUserState
	AppSettings  *model.AppSettings
}

// SyncSnapshotQuery selects whose singleton blobs (settings, UI state) the
// snapshot carries. Now overrides the clock the completed-task window is
// measured back from; the zero value means "right now" and is what the handler
// passes.
type SyncSnapshotQuery struct {
	UserID int64
	Now    time.Time
}

// syncWindowTaskIDs collects the tasks a snapshot carries: every task that is
// not completed, every task completed on or after the cutoff, and — walking up
// through the recursive arm — the ancestors of those, however old.
//
// The ancestor closure is what keeps the seed referentially whole. An open
// subtask whose parent was completed two years ago would otherwise arrive with a
// parent_id pointing at a row the replica has never seen, and it would render as
// a detached top-level task rather than as the subtask it is.
//
// UNION rather than UNION ALL: it deduplicates siblings that share an ancestor,
// and it terminates even if a parent chain ever managed to form a cycle.
const syncWindowTaskIDs = `WITH RECURSIVE kept(id, parent_id) AS (
	    SELECT id, parent_id FROM tasks WHERE completed_at IS NULL OR completed_at >= ?
	     UNION
	    SELECT t.id, t.parent_id FROM tasks t JOIN kept k ON t.id = k.parent_id
	)`

// Snapshot reads the whole workspace as of one point in the change log.
//
// Everything comes out of a single read transaction — epoch, cursor and every
// entity — so the caller receives a state that actually existed, not a mosaic of
// several. Callers continue with Changes(Since: Cursor).
func (r *ChangeLogRepo) Snapshot(ctx context.Context, q SyncSnapshotQuery) (*SyncSnapshot, error) {
	const op = "repo.change_log.Snapshot"
	logQuery(ctx, op, q.UserID)

	now := q.Now
	if now.IsZero() {
		now = time.Now()
	}
	cutoff := now.Add(-SyncHistoryWindow)

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("begin read tx: %w", err))
	}
	// Read-only: rolled back on every path, including the happy one.
	defer func() { _ = tx.Rollback() }()

	snap := &SyncSnapshot{CompletedSince: cutoff}

	if snap.Epoch, err = txSyncEpoch(ctx, tx); err != nil {
		return nil, logErr(ctx, op, err)
	}
	// Read before the rows rather than after. Both orders are consistent inside
	// one transaction; taking it first states the intent — the cursor describes a
	// log position the rows are guaranteed to be at least as new as.
	if snap.Cursor, err = txHighestSeq(ctx, tx); err != nil {
		return nil, logErr(ctx, op, err)
	}

	if err := r.loadSnapshotEntities(ctx, tx, q, cutoff, snap); err != nil {
		return nil, logErr(ctx, op, err)
	}
	return snap, nil
}

// loadSnapshotEntities fills every collection of the seed, one entity kind at a
// time, so no single intermediate structure holds the workspace twice.
func (r *ChangeLogRepo) loadSnapshotEntities(
	ctx context.Context, tx *sql.Tx, q SyncSnapshotQuery, cutoff time.Time, snap *SyncSnapshot,
) error {
	cutoffArg := model.FormatUTC(cutoff)

	var err error
	if snap.Tasks, err = txSnapshotTasks(ctx, tx, cutoffArg); err != nil {
		return err
	}
	if snap.Projects, err = txSnapshotProjects(ctx, tx); err != nil {
		return err
	}
	if snap.Sections, err = txSnapshotSections(ctx, tx); err != nil {
		return err
	}
	if snap.Contexts, err = txSnapshotContexts(ctx, tx); err != nil {
		return err
	}
	if snap.Labels, err = txSnapshotLabels(ctx, tx); err != nil {
		return err
	}
	if snap.TaskRelations, err = txSnapshotTaskRelations(ctx, tx, cutoffArg); err != nil {
		return err
	}
	if snap.TaskTemplates, err = txSnapshotTaskTemplates(ctx, tx); err != nil {
		return err
	}
	return txSnapshotUserBlobs(ctx, tx, q.UserID, snap)
}

// txHighestSeq is the log position a snapshot is taken at.
//
// MIN/MAX disagree about an emptied log: MAX(seq) is NULL once pruning has
// removed every row, but the history is not gone — it is merely all consumed.
// sqlite_sequence remembers the highest seq ever handed out, and that is the
// right cursor, otherwise a snapshot taken against a fully pruned log would hand
// back 0 and the very next delta call would reject it as expired.
func txHighestSeq(ctx context.Context, src queryer) (int64, error) {
	var latest sql.NullInt64
	if err := src.QueryRowContext(ctx, `SELECT MAX(seq) FROM change_log`).Scan(&latest); err != nil {
		return 0, fmt.Errorf("get latest change seq: %w", err)
	}
	if latest.Valid {
		return latest.Int64, nil
	}
	var allocated sql.NullInt64
	err := src.QueryRowContext(ctx,
		`SELECT seq FROM sqlite_sequence WHERE name = 'change_log'`).Scan(&allocated)
	if err != nil && err != sql.ErrNoRows {
		return 0, fmt.Errorf("get highest allocated change seq: %w", err)
	}
	if !allocated.Valid {
		return 0, nil
	}
	return allocated.Int64, nil
}

func txSnapshotTasks(ctx context.Context, src queryer, cutoff string) ([]*model.Task, error) {
	rows, err := src.QueryContext(ctx,
		syncWindowTaskIDs+` SELECT `+taskColumns+`
		     FROM tasks WHERE id IN (SELECT id FROM kept) ORDER BY id ASC`, cutoff)
	if err != nil {
		return nil, fmt.Errorf("list snapshot tasks: %w", err)
	}
	defer logging.LogClose(ctx, "repo.change_log.snapshot.tasks.rows", rows)

	tasks := make([]*model.Task, 0)
	ids := make([]int64, 0)
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
		ids = append(ids, t.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// The same batch loaders every other read path uses, so a task in a snapshot
	// is indistinguishable from the same task served by GET /tasks/:id — the
	// blocker rollup especially, because the padlock a replica draws and the
	// guard that refuses a completion have to agree.
	labels, err := labelsByTaskIDs(ctx, src, ids)
	if err != nil {
		return nil, err
	}
	summaries, err := summaryByTaskIDs(ctx, src, ids)
	if err != nil {
		return nil, err
	}
	for _, t := range tasks {
		t.Labels = labels[t.ID]
		t.RelationSummary = summaries[t.ID]
	}
	return tasks, nil
}

func txSnapshotProjects(ctx context.Context, src queryer) ([]*model.Project, error) {
	rows, err := src.QueryContext(ctx, `SELECT `+projectColumns+` FROM projects ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list snapshot projects: %w", err)
	}
	defer logging.LogClose(ctx, "repo.change_log.snapshot.projects.rows", rows)

	projects := make([]*model.Project, 0)
	ids := make([]int64, 0)
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		projects = append(projects, p)
		ids = append(ids, p.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	labels, err := labelsByProjectIDs(ctx, src, ids)
	if err != nil {
		return nil, err
	}
	for _, p := range projects {
		p.Labels = labels[p.ID]
	}
	return projects, nil
}

func txSnapshotSections(ctx context.Context, src queryer) ([]*model.ProjectSection, error) {
	rows, err := src.QueryContext(ctx,
		`SELECT id, project_id, title, position, created_at, updated_at
		   FROM project_sections ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list snapshot sections: %w", err)
	}
	defer logging.LogClose(ctx, "repo.change_log.snapshot.sections.rows", rows)

	out := make([]*model.ProjectSection, 0)
	for rows.Next() {
		s, err := scanSection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func txSnapshotContexts(ctx context.Context, src queryer) ([]*model.Context, error) {
	rows, err := src.QueryContext(ctx,
		`SELECT id, name, color, is_favourite, created_at, updated_at
		   FROM contexts ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list snapshot contexts: %w", err)
	}
	defer logging.LogClose(ctx, "repo.change_log.snapshot.contexts.rows", rows)

	out := make([]*model.Context, 0)
	for rows.Next() {
		c, err := scanContext(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func txSnapshotLabels(ctx context.Context, src queryer) ([]*model.Label, error) {
	rows, err := src.QueryContext(ctx,
		`SELECT id, name, color, is_favourite, is_private, created_at, updated_at
		   FROM labels ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list snapshot labels: %w", err)
	}
	defer logging.LogClose(ctx, "repo.change_log.snapshot.labels.rows", rows)

	out := make([]*model.Label, 0)
	for rows.Next() {
		l, err := scanLabel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// txSnapshotTaskRelations serves only edges whose both endpoints are inside the
// task window. An edge naming a task the seed does not carry would be an
// instruction to store a dangling reference, and the replica has nothing to
// resolve it against.
func txSnapshotTaskRelations(ctx context.Context, src queryer, cutoff string) ([]*model.TaskRelation, error) {
	rows, err := src.QueryContext(ctx,
		syncWindowTaskIDs+` SELECT id, source_task_id, target_task_id, type, created_at
		     FROM task_relations
		    WHERE source_task_id IN (SELECT id FROM kept)
		      AND target_task_id IN (SELECT id FROM kept)
		    ORDER BY id ASC`, cutoff)
	if err != nil {
		return nil, fmt.Errorf("list snapshot task relations: %w", err)
	}
	defer logging.LogClose(ctx, "repo.change_log.snapshot.task_relations.rows", rows)

	out := make([]*model.TaskRelation, 0)
	for rows.Next() {
		rel, err := scanTaskRelationEdge(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rel)
	}
	return out, rows.Err()
}

func txSnapshotTaskTemplates(ctx context.Context, src queryer) ([]*model.TaskTemplate, error) {
	rows, err := src.QueryContext(ctx,
		`SELECT id, name, description, priority, day_part, position, created_at, updated_at
		   FROM task_templates ORDER BY position ASC, name ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list snapshot task templates: %w", err)
	}
	defer logging.LogClose(ctx, "repo.change_log.snapshot.task_templates.rows", rows)

	out := make([]*model.TaskTemplate, 0)
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, t := range out {
		if err := hydrateTemplate(ctx, src, t); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// txSnapshotUserBlobs reads the two singleton blobs off the user row plus the
// server-wide settings row, reusing the same loaders the delta feed hydrates
// them with so both endpoints serve the identical bytes.
func txSnapshotUserBlobs(ctx context.Context, src queryer, userID int64, snap *SyncSnapshot) error {
	settings, err := txLoadUserSettings(ctx, src, []int64{userID})
	if err != nil {
		return err
	}
	// A missing row degrades to the defaults rather than to a null blob: a
	// replica seeded with nothing where its preferences should be would render
	// with no locale and a zero pin cap.
	snap.UserSettings = decodeUserSettings("")
	if s, ok := settings[userID].(*model.UserSettings); ok {
		snap.UserSettings = s
	}

	state, err := txLoadUserState(ctx, src, []int64{userID})
	if err != nil {
		return err
	}
	snap.UserState = SyncUserState("{}")
	if s, ok := state[userID].(SyncUserState); ok {
		snap.UserState = s
	}

	appSettings, err := txLoadAppSettings(ctx, src, []int64{appSettingsSingletonID})
	if err != nil {
		return err
	}
	snap.AppSettings = decodeAppSettings("")
	if s, ok := appSettings[appSettingsSingletonID].(*model.AppSettings); ok {
		snap.AppSettings = s
	}
	return nil
}

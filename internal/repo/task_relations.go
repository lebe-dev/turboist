package repo

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
)

// TaskRelationsRepo owns the task_relations join table (migration 046). Rows are
// directed: for `blocks` the source blocks the target; for `related` the caller is
// expected to have normalised the pair (see RelationService) so the UNIQUE
// constraint dedupes A↔B.
type TaskRelationsRepo struct {
	db *sql.DB
}

func NewTaskRelationsRepo(db *sql.DB) *TaskRelationsRepo {
	return &TaskRelationsRepo{db: db}
}

// Create inserts one relation and bumps updated_at on both endpoints.
//
// The bump is load-bearing, not cosmetic: adding or removing a relation changes
// what both task detail pages render, and the SPA's hydrate() short-circuits on an
// unchanged updatedAt — without it the peer's page (and its cached copy) would
// keep showing the stale relation set.
func (r *TaskRelationsRepo) Create(ctx context.Context, sourceID, targetID int64, relType model.RelationType) (*model.TaskRelation, error) {
	const op = "repo.task_relations.Create"
	logQuery(ctx, op, sourceID, targetID, relType)
	// Truncated to the millisecond the wire format carries, so the CreatedAt on the
	// returned model is exactly what was persisted rather than a more precise value
	// that no subsequent read will ever reproduce.
	now := time.Now().UTC().Truncate(time.Millisecond)
	nowStr := model.FormatUTC(now)

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("begin tx: %w", err))
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx,
		`INSERT INTO task_relations (source_task_id, target_task_id, type, created_at)
		 VALUES (?, ?, ?, ?)`,
		sourceID, targetID, string(relType), nowStr)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, logErr(ctx, op, ErrConflict)
		}
		return nil, logErr(ctx, op, fmt.Errorf("insert task_relation: %w", err))
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("last insert id: %w", err))
	}
	if err := touchTasks(ctx, tx, nowStr, sourceID, targetID); err != nil {
		return nil, logErr(ctx, op, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("commit: %w", err))
	}
	return &model.TaskRelation{
		ID:           id,
		SourceTaskID: sourceID,
		TargetTaskID: targetID,
		Type:         relType,
		CreatedAt:    now,
	}, nil
}

// Delete removes a relation, scoped to a task it actually touches so a relation
// cannot be deleted through an unrelated task's endpoint. Returns ErrNotFound when
// no such row exists for that task.
func (r *TaskRelationsRepo) Delete(ctx context.Context, relationID, taskID int64) error {
	const op = "repo.task_relations.Delete"
	logQuery(ctx, op, relationID, taskID)
	nowStr := model.FormatUTC(time.Now().UTC())

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("begin tx: %w", err))
	}
	defer func() { _ = tx.Rollback() }()

	var sourceID, targetID int64
	err = tx.QueryRowContext(ctx,
		`SELECT source_task_id, target_task_id FROM task_relations
		 WHERE id = ? AND (source_task_id = ? OR target_task_id = ?)`,
		relationID, taskID, taskID).Scan(&sourceID, &targetID)
	if err == sql.ErrNoRows {
		return logErr(ctx, op, ErrNotFound)
	}
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("select task_relation: %w", err))
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM task_relations WHERE id = ?`, relationID); err != nil {
		return logErr(ctx, op, fmt.Errorf("delete task_relation: %w", err))
	}
	if err := touchTasks(ctx, tx, nowStr, sourceID, targetID); err != nil {
		return logErr(ctx, op, err)
	}
	if err := tx.Commit(); err != nil {
		return logErr(ctx, op, fmt.Errorf("commit: %w", err))
	}
	return nil
}

func touchTasks(ctx context.Context, tx *sql.Tx, nowStr string, ids ...int64) error {
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE tasks SET updated_at = ? WHERE id = ?`, nowStr, id); err != nil {
			return fmt.Errorf("touch task %d: %w", id, err)
		}
	}
	return nil
}

// relationPeerColumns are the peer task's fields, aliased so scanTask can read
// them. Deliberately not taskColumns: the peer is rendered as a link (title,
// status, placement) and hydrating its own labels/relations would recurse.
const relationPeerColumns = `p.id, p.title, p.description, p.inbox_id, p.context_id, p.project_id, p.section_id, p.parent_id,
		p.priority, p.status, p.due_at, p.due_has_time, p.deadline_at, p.deadline_has_time,
		p.day_part, p.plan_state, p.is_pinned, p.pinned_at, p.is_private, p.is_complex, p.recurrence_rule,
		p.completed_at, p.postpone_count, p.troiki_category, p.source_task_id, p.created_at, p.updated_at`

// ListForTask returns every relation touching taskID, in both directions, with the
// peer task hydrated into Other and Direction resolved relative to taskID.
func (r *TaskRelationsRepo) ListForTask(ctx context.Context, taskID int64) ([]model.TaskRelation, error) {
	const op = "repo.task_relations.ListForTask"
	logQuery(ctx, op, taskID)
	// Two arms rather than an OR-join: each arm knows which end is the peer, which
	// is what makes `direction` computable in SQL instead of in a second pass.
	//
	// The relation's id/created_at are aliased because the peer task contributes
	// columns of the same name (tasks.created_at, and tasks.source_task_id vs
	// tr.source_task_id). In a compound SELECT, ORDER BY resolves against result
	// column names, so an unaliased `created_at` is ambiguous and SQLite rejects it.
	const relationCols = `tr.id AS relation_id, tr.source_task_id AS rel_source_id,
		tr.target_task_id AS rel_target_id, tr.type AS rel_type, tr.created_at AS relation_created_at`
	q := `SELECT ` + relationCols + `, 'outgoing' AS direction, ` + relationPeerColumns + `
	      FROM task_relations tr JOIN tasks p ON p.id = tr.target_task_id
	      WHERE tr.source_task_id = ?
	      UNION ALL
	      SELECT ` + relationCols + `, 'incoming' AS direction, ` + relationPeerColumns + `
	      FROM task_relations tr JOIN tasks p ON p.id = tr.source_task_id
	      WHERE tr.target_task_id = ?
	      ORDER BY relation_created_at ASC, relation_id ASC`
	rows, err := r.db.QueryContext(ctx, q, taskID, taskID)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("list task relations: %w", err))
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	out := make([]model.TaskRelation, 0)
	for rows.Next() {
		rel, err := scanRelationWithPeer(rows)
		if err != nil {
			return nil, logErr(ctx, op, err)
		}
		out = append(out, *rel)
	}
	return out, rows.Err()
}

// prefixScanner adapts *sql.Rows to the single-row Scan signature scanTask expects,
// letting the relation columns be consumed first and the peer columns fall through
// to the shared task scanner. Without it the peer would need a duplicate scan body.
type prefixScanner struct {
	rows   *sql.Rows
	prefix []any
}

func (p prefixScanner) Scan(dest ...any) error {
	return p.rows.Scan(append(p.prefix, dest...)...)
}

func scanRelationWithPeer(rows *sql.Rows) (*model.TaskRelation, error) {
	var rel model.TaskRelation
	var relType, direction, createdAt string
	peer, err := scanTask(prefixScanner{
		rows:   rows,
		prefix: []any{&rel.ID, &rel.SourceTaskID, &rel.TargetTaskID, &relType, &createdAt, &direction},
	})
	if err != nil {
		return nil, err
	}
	rel.Type = model.RelationType(relType)
	rel.Direction = model.RelationDirection(direction)
	ts, err := model.ParseUTC(createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	rel.CreatedAt = ts
	rel.Other = peer
	return &rel, nil
}

// SummaryByTaskIDs batch-loads the per-task rollup for a whole page of tasks —
// the anti-N+1 loader that TaskRepo.Get and every list view call once.
func (r *TaskRelationsRepo) SummaryByTaskIDs(ctx context.Context, taskIDs []int64) (map[int64]model.TaskRelationSummary, error) {
	const op = "repo.task_relations.SummaryByTaskIDs"
	logQuery(ctx, op, taskIDs)
	if len(taskIDs) == 0 {
		return map[int64]model.TaskRelationSummary{}, nil
	}
	placeholders := make([]string, len(taskIDs))
	for i := range taskIDs {
		placeholders[i] = "?"
	}
	in := strings.Join(placeholders, ",")
	// `total` counts the task's own relations — both endpoints of every row — because
	// that is what the detail page lists. `blocked` walks the ancestor chain instead,
	// so a subtask of a blocked parent reads as blocked too: the badge has to agree
	// with the completion guard in OpenBlockerIDs, which inherits the same way (and
	// drops inherited blockers sitting inside the task's own subtree for the same
	// reason). Counted per distinct blocker, so a blocker attached to both a parent
	// and its child is not counted twice for the child.
	q := `WITH RECURSIVE ancestors(task_id, anc_id) AS (
	          SELECT id, id FROM tasks WHERE id IN (` + in + `)
	          UNION
	          SELECT a.task_id, t.parent_id FROM ancestors a JOIN tasks t ON t.id = a.anc_id
	          WHERE t.parent_id IS NOT NULL
	      ),
	      subtree(task_id, desc_id) AS (
	          SELECT id, id FROM tasks WHERE id IN (` + in + `)
	          UNION
	          SELECT s.task_id, t.id FROM subtree s JOIN tasks t ON t.parent_id = s.desc_id
	      ),
	      blockers(task_id, blocker_id) AS (
	          SELECT DISTINCT a.task_id, tr.source_task_id
	          FROM ancestors a
	          JOIN task_relations tr ON tr.target_task_id = a.anc_id AND tr.type = 'blocks'
	          JOIN tasks src ON src.id = tr.source_task_id AND src.status = 'open'
	          WHERE a.anc_id = a.task_id
	             OR NOT EXISTS (
	                 SELECT 1 FROM subtree s
	                 WHERE s.task_id = a.task_id AND s.desc_id = tr.source_task_id
	             )
	      )
	      SELECT task_id, SUM(total) AS total, SUM(blocked) AS blocked FROM (
	          SELECT tr.source_task_id AS task_id, 1 AS total, 0 AS blocked
	          FROM task_relations tr WHERE tr.source_task_id IN (` + in + `)
	          UNION ALL
	          SELECT tr.target_task_id AS task_id, 1 AS total, 0 AS blocked
	          FROM task_relations tr WHERE tr.target_task_id IN (` + in + `)
	          UNION ALL
	          SELECT task_id, 0 AS total, COUNT(*) AS blocked FROM blockers GROUP BY task_id
	      ) GROUP BY task_id`
	args := make([]any, 0, len(taskIDs)*4)
	for range 4 {
		for _, id := range taskIDs {
			args = append(args, id)
		}
	}
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("summarise task relations: %w", err))
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	out := make(map[int64]model.TaskRelationSummary, len(taskIDs))
	for rows.Next() {
		var taskID int64
		var total, blocked int
		if err := rows.Scan(&taskID, &total, &blocked); err != nil {
			return nil, logErr(ctx, op, err)
		}
		out[taskID] = model.TaskRelationSummary{BlockedByOpen: blocked, Total: total}
	}
	return out, rows.Err()
}

// OpenBlockerIDs returns the still-open tasks blocking taskID, including the ones
// blocking any of its ancestors: a subtask of a task that cannot be finished yet
// cannot be finished either, otherwise the work would start out of order.
// Completed and cancelled blockers are excluded — a cancelled task would otherwise
// deadlock its dependents permanently.
//
// Inherited blockers inside taskID's own subtree are dropped: "child blocks
// parent" is a legal pair, and inheriting it downwards would leave the child
// waiting for itself, unfinishable from either end. A blocker attached to taskID
// directly always counts, wherever it sits in the tree.
func (r *TaskRelationsRepo) OpenBlockerIDs(ctx context.Context, taskID int64) ([]int64, error) {
	const op = "repo.task_relations.OpenBlockerIDs"
	logQuery(ctx, op, taskID)
	rows, err := r.db.QueryContext(ctx,
		`WITH RECURSIVE ancestors(id) AS (
		     SELECT parent_id FROM tasks WHERE id = ? AND parent_id IS NOT NULL
		     UNION
		     SELECT t.parent_id FROM tasks t JOIN ancestors a ON t.id = a.id
		     WHERE t.parent_id IS NOT NULL
		 ),
		 subtree(id) AS (
		     SELECT ?
		     UNION
		     SELECT t.id FROM tasks t JOIN subtree s ON t.parent_id = s.id
		 )
		 SELECT DISTINCT tr.source_task_id FROM task_relations tr
		 JOIN tasks src ON src.id = tr.source_task_id
		 WHERE tr.type = 'blocks' AND src.status = 'open'
		   AND (
		     tr.target_task_id = ?
		     OR (tr.target_task_id IN (SELECT id FROM ancestors)
		         AND tr.source_task_id NOT IN (SELECT id FROM subtree))
		   )
		 ORDER BY tr.source_task_id ASC`, taskID, taskID, taskID)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("list open blockers: %w", err))
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	out := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, logErr(ctx, op, err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// WouldCycle reports whether adding "sourceID blocks targetID" closes a loop in
// the blocking graph — i.e. whether sourceID is already reachable from targetID by
// following `blocks` edges forward. Such a pair would leave both tasks permanently
// uncompletable, so the service rejects it.
func (r *TaskRelationsRepo) WouldCycle(ctx context.Context, sourceID, targetID int64) (bool, error) {
	const op = "repo.task_relations.WouldCycle"
	logQuery(ctx, op, sourceID, targetID)
	if sourceID == targetID {
		return true, nil
	}
	var exists int
	err := r.db.QueryRowContext(ctx,
		`WITH RECURSIVE reachable(id) AS (
		     SELECT ?
		     UNION
		     SELECT tr.target_task_id FROM task_relations tr
		     JOIN reachable rr ON rr.id = tr.source_task_id
		     WHERE tr.type = 'blocks'
		 )
		 SELECT EXISTS (SELECT 1 FROM reachable WHERE id = ?)`,
		targetID, sourceID).Scan(&exists)
	if err != nil {
		return false, logErr(ctx, op, fmt.Errorf("check blocking cycle: %w", err))
	}
	return exists == 1, nil
}

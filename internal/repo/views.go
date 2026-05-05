package repo

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

// TaskFilter is the shared per-list filter for view queries.
type TaskFilter struct {
	ContextID *int64
	ProjectID *int64
	LabelID   *int64
	Priority  *model.Priority
	Status    *model.TaskStatus
	Query     string
}

func (f TaskFilter) where() (string, []any) {
	conds := []string{}
	args := []any{}
	if f.ContextID != nil {
		conds = append(conds, "t.context_id = ?")
		args = append(args, *f.ContextID)
	}
	if f.ProjectID != nil {
		conds = append(conds, "t.project_id = ?")
		args = append(args, *f.ProjectID)
	}
	if f.LabelID != nil {
		conds = append(conds, "EXISTS (SELECT 1 FROM task_labels tl WHERE tl.task_id = t.id AND tl.label_id = ?)")
		args = append(args, *f.LabelID)
	}
	if f.Priority != nil {
		conds = append(conds, "t.priority = ?")
		args = append(args, string(*f.Priority))
	}
	if f.Status != nil {
		conds = append(conds, "t.status = ?")
		args = append(args, string(*f.Status))
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		conds = append(conds, "(t.title LIKE ? ESCAPE '\\' OR t.description LIKE ? ESCAPE '\\')")
		escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)
		like := "%" + escaped + "%"
		args = append(args, like, like)
	}
	if len(conds) == 0 {
		return "", args
	}
	return " AND " + strings.Join(conds, " AND "), args
}

// ListInbox returns open inbox tasks; subtasks are forbidden in inbox so all
// rows here are root-level.
func (r *TaskRepo) ListInbox(ctx context.Context, filter TaskFilter, page Page) ([]model.Task, int, error) {
	base := "FROM tasks t WHERE t.inbox_id IS NOT NULL AND t.status = 'open'"
	return r.listWithBase(ctx, base, filter, page, true)
}

// ListByContext lists tasks attached directly to a context (without project).
func (r *TaskRepo) ListByContext(ctx context.Context, contextID int64, withinProject bool, filter TaskFilter, page Page) ([]model.Task, int, error) {
	base := "FROM tasks t WHERE t.context_id = ?"
	args := []any{contextID}
	if !withinProject {
		base += " AND t.project_id IS NULL"
	}
	return r.listWithBaseArgs(ctx, base, args, filter, page, true)
}

func (r *TaskRepo) ListByProject(ctx context.Context, projectID int64, filter TaskFilter, page Page) ([]model.Task, int, error) {
	base := "FROM tasks t WHERE t.project_id = ?"
	return r.listWithBaseArgs(ctx, base, []any{projectID}, filter, page, true)
}

func (r *TaskRepo) ListBySection(ctx context.Context, sectionID int64, filter TaskFilter, page Page) ([]model.Task, int, error) {
	base := "FROM tasks t WHERE t.section_id = ?"
	return r.listWithBaseArgs(ctx, base, []any{sectionID}, filter, page, true)
}

func (r *TaskRepo) ListByLabel(ctx context.Context, labelID int64, filter TaskFilter, page Page) ([]model.Task, int, error) {
	base := "FROM tasks t WHERE EXISTS (SELECT 1 FROM task_labels tl WHERE tl.task_id = t.id AND tl.label_id = ?)"
	return r.listWithBaseArgs(ctx, base, []any{labelID}, filter, page, true)
}

// ListByProjectIDs batch-loads root + subtasks for the given project ids and
// groups them by project_id. Used by the Troiki view to render every task tree
// of every category-bound project in one query. Completed tasks stay visible
// so they don't appear to vanish from the slot the moment a user ticks them
// off; cancelled tasks and their entire descendant subtree are excluded — the
// Troiki UI only understands open vs completed, and rendering cancelled rows
// there would show them unchecked and turn the checkbox into a "complete"
// action against the user's intent. Excluding descendants prevents orphaned
// children of a cancelled parent from being rendered as detached top-level
// items by the tree builder when their parent row is filtered out.
func (r *TaskRepo) ListByProjectIDs(ctx context.Context, ids []int64) (map[int64][]model.Task, error) {
	if len(ids) == 0 {
		return map[int64][]model.Task{}, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, v := range ids {
		placeholders[i] = "?"
		args[i] = v
	}
	rows, err := r.db.QueryContext(ctx,
		`WITH RECURSIVE cancelled_subtree(id) AS (
		     SELECT id FROM tasks WHERE status = 'cancelled'
		     UNION ALL
		     SELECT t.id FROM tasks t JOIN cancelled_subtree cs ON t.parent_id = cs.id
		 )
		 SELECT `+taskColumns+` FROM tasks
		 WHERE project_id IN (`+strings.Join(placeholders, ",")+`)
		   AND id NOT IN (SELECT id FROM cancelled_subtree)
		 ORDER BY `+taskOrderBy, args...)
	if err != nil {
		return nil, fmt.Errorf("list tasks by project ids: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[int64][]model.Task{}
	allIDs := make([]int64, 0)
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		if t.ProjectID != nil {
			out[*t.ProjectID] = append(out[*t.ProjectID], *t)
			allIDs = append(allIDs, t.ID)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if r.labels != nil && len(allIDs) > 0 {
		hydrated, err := r.labels.LabelsByTaskIDs(ctx, allIDs)
		if err != nil {
			return nil, err
		}
		for pid, list := range out {
			for i := range list {
				list[i].Labels = hydrated[list[i].ID]
			}
			out[pid] = list
		}
	}
	return out, nil
}

func (r *TaskRepo) ListSubtasks(ctx context.Context, parentID int64) ([]model.Task, error) {
	base := "FROM tasks t WHERE t.parent_id = ?"
	out, _, err := r.listWithBaseArgs(ctx, base, []any{parentID}, TaskFilter{}, Page{Limit: 200}, false)
	return out, err
}

// --- views ---

// ListToday returns open tasks with due_at within [start, start+24h).
func (r *TaskRepo) ListToday(ctx context.Context, start time.Time, filter TaskFilter, page Page) ([]model.Task, int, error) {
	end := start.Add(24 * time.Hour)
	base := "FROM tasks t WHERE t.status = 'open' AND t.due_at >= ? AND t.due_at < ?"
	return r.listWithBaseArgs(ctx, base, []any{model.FormatUTC(start), model.FormatUTC(end)}, filter, page, true)
}

func (r *TaskRepo) ListTomorrow(ctx context.Context, todayStart time.Time, filter TaskFilter, page Page) ([]model.Task, int, error) {
	start := todayStart.Add(24 * time.Hour)
	end := start.Add(24 * time.Hour)
	base := "FROM tasks t WHERE t.status = 'open' AND t.due_at >= ? AND t.due_at < ?"
	return r.listWithBaseArgs(ctx, base, []any{model.FormatUTC(start), model.FormatUTC(end)}, filter, page, true)
}

// ListCompletedInRange returns tasks marked completed within [start, end).
func (r *TaskRepo) ListCompletedInRange(ctx context.Context, start, end time.Time, filter TaskFilter, page Page) ([]model.Task, int, error) {
	base := "FROM tasks t WHERE t.status = 'completed' AND t.completed_at >= ? AND t.completed_at < ?"
	return r.listWithBaseArgs(ctx, base, []any{model.FormatUTC(start), model.FormatUTC(end)}, filter, page, true)
}

func (r *TaskRepo) ListOverdue(ctx context.Context, todayStart time.Time, filter TaskFilter, page Page) ([]model.Task, int, error) {
	base := "FROM tasks t WHERE t.status = 'open' AND t.due_at IS NOT NULL AND t.due_at < ?"
	return r.listWithBaseArgs(ctx, base, []any{model.FormatUTC(todayStart)}, filter, page, true)
}

func (r *TaskRepo) ListWeek(ctx context.Context, filter TaskFilter) ([]model.Task, int, error) {
	base := "FROM tasks t WHERE t.plan_state = 'week' AND t.status = 'open'"
	return r.listWithBase(ctx, base, filter, Page{Limit: 200}, true)
}

func (r *TaskRepo) ListBacklog(ctx context.Context, filter TaskFilter) ([]model.Task, int, error) {
	base := "FROM tasks t WHERE t.plan_state = 'backlog' AND t.status = 'open'"
	return r.listWithBase(ctx, base, filter, Page{Limit: 200}, true)
}

func (r *TaskRepo) ListPinned(ctx context.Context, filter TaskFilter) ([]model.Task, int, error) {
	base := "FROM tasks t WHERE t.is_pinned = 1 AND t.status = 'open'"
	return r.listWithBase(ctx, base, filter, Page{Limit: 200}, true)
}

// ListByTroikiCategory returns all open tasks for the given Troiki category along
// with their total count. Troiki capacity for medium/rest is unbounded (grows
// with completions), so this listing is intentionally not paginated — slot
// counts and the rendered list must agree, otherwise the UI shows phantom
// "empty slot" placeholders.
func (r *TaskRepo) ListByTroikiCategory(ctx context.Context, cat model.TroikiCategory) ([]model.Task, int, error) {
	base := "FROM tasks t WHERE t.troiki_category = ? AND t.status = 'open'"

	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) `+base, string(cat)).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count troiki tasks: %w", err)
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT `+taskColumns+` `+base+` ORDER BY `+taskOrderBy, string(cat))
	if err != nil {
		return nil, 0, fmt.Errorf("list troiki tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]model.Task, 0, total)
	ids := make([]int64, 0, total)
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *t)
		ids = append(ids, t.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if r.labels != nil && len(ids) > 0 {
		hydrated, err := r.labels.LabelsByTaskIDs(ctx, ids)
		if err != nil {
			return nil, 0, err
		}
		for i := range out {
			out[i].Labels = hydrated[out[i].ID]
		}
	}
	return out, total, nil
}

func (r *TaskRepo) CountOpenByTroikiCategory(ctx context.Context, cat model.TroikiCategory) (int, error) {
	return r.scalarCount(ctx,
		`SELECT COUNT(*) FROM tasks WHERE troiki_category = ? AND status = 'open'`,
		string(cat))
}

func (r *TaskRepo) listWithBase(ctx context.Context, base string, filter TaskFilter, page Page, hydrate bool) ([]model.Task, int, error) {
	return r.listWithBaseArgs(ctx, base, nil, filter, page, hydrate)
}

func (r *TaskRepo) listWithBaseArgs(ctx context.Context, base string, baseArgs []any, filter TaskFilter, page Page, hydrate bool) ([]model.Task, int, error) {
	page = page.Normalize()
	whereExtra, extraArgs := filter.where()
	allArgs := append([]any{}, baseArgs...)
	allArgs = append(allArgs, extraArgs...)

	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) `+base+whereExtra, allArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count tasks: %w", err)
	}

	listArgs := append([]any{}, allArgs...)
	listArgs = append(listArgs, page.Limit, page.Offset)
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+taskColumns+` `+base+whereExtra+
			` ORDER BY `+taskOrderBy+` LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]model.Task, 0)
	ids := make([]int64, 0)
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *t)
		ids = append(ids, t.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if hydrate && r.labels != nil && len(ids) > 0 {
		hydrated, err := r.labels.LabelsByTaskIDs(ctx, ids)
		if err != nil {
			return nil, 0, err
		}
		for i := range out {
			out[i].Labels = hydrated[out[i].ID]
		}
	}
	return out, total, nil
}

// SubtreeIDs returns the task plus all descendants (BFS) — useful for
// validations beyond the move path.
func (r *TaskRepo) SubtreeIDs(ctx context.Context, root int64) ([]int64, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	descendants, err := collectDescendants(ctx, tx, root)
	if err != nil {
		return nil, err
	}
	return append([]int64{root}, descendants...), nil
}

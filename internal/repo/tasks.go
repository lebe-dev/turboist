package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

var (
	ErrInvalidPlacement = errors.New("repo: invalid task placement")
	ErrCycle            = errors.New("repo: parent cycle")
)

type TaskRepo struct {
	db     *sql.DB
	labels *TaskLabelsRepo
}

func NewTaskRepo(db *sql.DB, labels *TaskLabelsRepo) *TaskRepo {
	return &TaskRepo{db: db, labels: labels}
}

const taskColumns = `id, title, description, inbox_id, context_id, project_id, section_id, parent_id,
		priority, status, due_at, due_has_time, deadline_at, deadline_has_time,
		day_part, plan_state, is_pinned, pinned_at, recurrence_rule, completed_at, created_at, updated_at`

// taskOrderBy is the unified sort for all task listings (see business-rules.md).
const taskOrderBy = `is_pinned DESC,
	CASE priority
		WHEN 'high' THEN 4
		WHEN 'medium' THEN 3
		WHEN 'low' THEN 2
		WHEN 'no-priority' THEN 1
	END DESC,
	pinned_at DESC,
	created_at DESC`

func scanTask(row interface{ Scan(...any) error }) (*model.Task, error) {
	var t model.Task
	var inboxID, contextID, projectID, sectionID, parentID sql.NullInt64
	var dueAt, deadlineAt, pinnedAt, completedAt sql.NullString
	var recurrenceRule sql.NullString
	var dueHasTime, deadlineHasTime, isPinned int
	var createdAt, updatedAt string
	if err := row.Scan(
		&t.ID, &t.Title, &t.Description,
		&inboxID, &contextID, &projectID, &sectionID, &parentID,
		&t.Priority, &t.Status,
		&dueAt, &dueHasTime, &deadlineAt, &deadlineHasTime,
		&t.DayPart, &t.PlanState,
		&isPinned, &pinnedAt, &recurrenceRule, &completedAt,
		&createdAt, &updatedAt,
	); err != nil {
		return nil, err
	}
	if inboxID.Valid {
		v := inboxID.Int64
		t.InboxID = &v
	}
	if contextID.Valid {
		v := contextID.Int64
		t.ContextID = &v
	}
	if projectID.Valid {
		v := projectID.Int64
		t.ProjectID = &v
	}
	if sectionID.Valid {
		v := sectionID.Int64
		t.SectionID = &v
	}
	if parentID.Valid {
		v := parentID.Int64
		t.ParentID = &v
	}
	t.DueHasTime = dueHasTime == 1
	t.DeadlineHasTime = deadlineHasTime == 1
	t.IsPinned = isPinned == 1
	if dueAt.Valid {
		ts, err := model.ParseUTC(dueAt.String)
		if err != nil {
			return nil, fmt.Errorf("parse due_at: %w", err)
		}
		t.DueAt = &ts
	}
	if deadlineAt.Valid {
		ts, err := model.ParseUTC(deadlineAt.String)
		if err != nil {
			return nil, fmt.Errorf("parse deadline_at: %w", err)
		}
		t.DeadlineAt = &ts
	}
	if pinnedAt.Valid {
		ts, err := model.ParseUTC(pinnedAt.String)
		if err != nil {
			return nil, fmt.Errorf("parse pinned_at: %w", err)
		}
		t.PinnedAt = &ts
	}
	if completedAt.Valid {
		ts, err := model.ParseUTC(completedAt.String)
		if err != nil {
			return nil, fmt.Errorf("parse completed_at: %w", err)
		}
		t.CompletedAt = &ts
	}
	if recurrenceRule.Valid {
		v := recurrenceRule.String
		t.RecurrenceRule = &v
	}
	ts, err := model.ParseUTC(createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	t.CreatedAt = ts
	ts, err = model.ParseUTC(updatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}
	t.UpdatedAt = ts
	return &t, nil
}

// Placement carries the optional ownership pointers of a task.
type Placement struct {
	InboxID   *int64
	ContextID *int64
	ProjectID *int64
	SectionID *int64
	ParentID  *int64
}

// Validate mirrors the CHECK constraints in 001_schema.sql.
func (p Placement) Validate() error {
	inboxSet := p.InboxID != nil
	ctxSet := p.ContextID != nil
	if inboxSet == ctxSet {
		return ErrInvalidPlacement
	}
	if inboxSet && (p.ProjectID != nil || p.SectionID != nil || p.ParentID != nil) {
		return ErrInvalidPlacement
	}
	if p.SectionID != nil && p.ProjectID == nil {
		return ErrInvalidPlacement
	}
	return nil
}

type CreateTask struct {
	Placement
	Title           string
	Description     string
	Priority        model.Priority
	DueAt           *time.Time
	DueHasTime      bool
	DeadlineAt      *time.Time
	DeadlineHasTime bool
	DayPart         model.DayPart
	PlanState       model.PlanState
	RecurrenceRule  *string
}

func (r *TaskRepo) Create(ctx context.Context, in CreateTask) (*model.Task, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	if in.Priority == "" {
		in.Priority = model.PriorityNone
	}
	if in.DayPart == "" {
		in.DayPart = model.DayPartNone
	}
	if in.PlanState == "" {
		in.PlanState = model.PlanStateNone
	}
	now := model.FormatUTC(time.Now())
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO tasks (title, description, inbox_id, context_id, project_id, section_id, parent_id,
			priority, status, due_at, due_has_time, deadline_at, deadline_has_time,
			day_part, plan_state, is_pinned, pinned_at, recurrence_rule, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'open', ?, ?, ?, ?, ?, ?, 0, NULL, ?, ?, ?)`,
		in.Title, in.Description,
		nullInt(in.InboxID), nullInt(in.ContextID), nullInt(in.ProjectID), nullInt(in.SectionID), nullInt(in.ParentID),
		string(in.Priority),
		nullTime(in.DueAt), boolInt(in.DueHasTime), nullTime(in.DeadlineAt), boolInt(in.DeadlineHasTime),
		string(in.DayPart), string(in.PlanState),
		nullStr(in.RecurrenceRule),
		now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert task: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, id)
}

func (r *TaskRepo) Get(ctx context.Context, id int64) (*model.Task, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+taskColumns+` FROM tasks WHERE id = ?`, id)
	t, err := scanTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if r.labels != nil {
		hydrated, err := r.labels.LabelsByTaskIDs(ctx, []int64{t.ID})
		if err != nil {
			return nil, err
		}
		t.Labels = hydrated[t.ID]
	}
	return t, nil
}

type TaskUpdate struct {
	Title           *string
	Description     *string
	Priority        *model.Priority
	DueAt           *time.Time
	DueAtClear      bool
	DueHasTime      *bool
	DeadlineAt      *time.Time
	DeadlineAtClear bool
	DeadlineHasTime *bool
	DayPart         *model.DayPart
	PlanState       *model.PlanState
	RecurrenceRule  *string
	RecurrenceClear bool
	Status          *model.TaskStatus
}

func (r *TaskRepo) Update(ctx context.Context, id int64, u TaskUpdate) (*model.Task, error) {
	sets := make([]string, 0, 8)
	args := make([]any, 0, 12)
	if u.Title != nil {
		sets = append(sets, "title = ?")
		args = append(args, *u.Title)
	}
	if u.Description != nil {
		sets = append(sets, "description = ?")
		args = append(args, *u.Description)
	}
	if u.Priority != nil {
		sets = append(sets, "priority = ?")
		args = append(args, string(*u.Priority))
	}
	if u.DueAtClear {
		sets = append(sets, "due_at = NULL", "due_has_time = 0")
	} else {
		if u.DueAt != nil {
			sets = append(sets, "due_at = ?")
			args = append(args, model.FormatUTC(*u.DueAt))
		}
		if u.DueHasTime != nil {
			sets = append(sets, "due_has_time = ?")
			args = append(args, boolInt(*u.DueHasTime))
		}
	}
	if u.DeadlineAtClear {
		sets = append(sets, "deadline_at = NULL", "deadline_has_time = 0")
	} else {
		if u.DeadlineAt != nil {
			sets = append(sets, "deadline_at = ?")
			args = append(args, model.FormatUTC(*u.DeadlineAt))
		}
		if u.DeadlineHasTime != nil {
			sets = append(sets, "deadline_has_time = ?")
			args = append(args, boolInt(*u.DeadlineHasTime))
		}
	}
	if u.DayPart != nil {
		sets = append(sets, "day_part = ?")
		args = append(args, string(*u.DayPart))
	}
	if u.PlanState != nil {
		sets = append(sets, "plan_state = ?")
		args = append(args, string(*u.PlanState))
	}
	if u.RecurrenceClear {
		sets = append(sets, "recurrence_rule = NULL")
	} else if u.RecurrenceRule != nil {
		sets = append(sets, "recurrence_rule = ?")
		args = append(args, *u.RecurrenceRule)
	}
	if u.Status != nil {
		sets = append(sets, "status = ?")
		args = append(args, string(*u.Status))
		if *u.Status == model.TaskStatusCompleted {
			sets = append(sets, "completed_at = ?")
			args = append(args, model.FormatUTC(time.Now()))
		} else {
			sets = append(sets, "completed_at = NULL")
		}
	}
	if len(sets) == 0 {
		return r.Get(ctx, id)
	}
	sets = append(sets, "updated_at = ?")
	args = append(args, model.FormatUTC(time.Now()))
	args = append(args, id)

	res, err := r.db.ExecContext(ctx, `UPDATE tasks SET `+joinSets(sets)+` WHERE id = ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("update task: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, ErrNotFound
	}
	return r.Get(ctx, id)
}

func (r *TaskRepo) SetPinned(ctx context.Context, id int64, pinned bool) error {
	now := model.FormatUTC(time.Now())
	var res sql.Result
	var err error
	if pinned {
		res, err = r.db.ExecContext(ctx,
			`UPDATE tasks SET is_pinned = 1, pinned_at = ?, updated_at = ? WHERE id = ?`, now, now, id)
	} else {
		res, err = r.db.ExecContext(ctx,
			`UPDATE tasks SET is_pinned = 0, pinned_at = NULL, updated_at = ? WHERE id = ?`, now, id)
	}
	if err != nil {
		return fmt.Errorf("set pinned: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *TaskRepo) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Move relocates a task and all its descendants atomically. Cycles (target ∈
// subtree of taskID) are rejected with ErrCycle. Subtasks in inbox are
// rejected by Placement.Validate (parent_id forbidden alongside inbox_id).
func (r *TaskRepo) Move(ctx context.Context, taskID int64, target Placement) error {
	if err := target.Validate(); err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if target.ParentID != nil {
		if err := assertNoCycle(ctx, tx, taskID, *target.ParentID); err != nil {
			return err
		}
	}

	now := model.FormatUTC(time.Now())
	// Move the task itself: it adopts new inbox/context/project/section/parent.
	if _, err := tx.ExecContext(ctx,
		`UPDATE tasks SET inbox_id = ?, context_id = ?, project_id = ?, section_id = ?, parent_id = ?, updated_at = ?
		 WHERE id = ?`,
		nullInt(target.InboxID), nullInt(target.ContextID), nullInt(target.ProjectID), nullInt(target.SectionID),
		nullInt(target.ParentID), now, taskID,
	); err != nil {
		return fmt.Errorf("move task: %w", err)
	}

	// Cascade: descendants inherit context/project/section but keep their parent links.
	descendants, err := collectDescendants(ctx, tx, taskID)
	if err != nil {
		return err
	}
	for _, did := range descendants {
		if _, err := tx.ExecContext(ctx,
			`UPDATE tasks SET inbox_id = NULL, context_id = ?, project_id = ?, section_id = ?, updated_at = ?
			 WHERE id = ?`,
			nullInt(target.ContextID), nullInt(target.ProjectID), nullInt(target.SectionID), now, did,
		); err != nil {
			return fmt.Errorf("cascade move: %w", err)
		}
	}
	return tx.Commit()
}

// assertNoCycle walks parent_id from candidateParent upward; if it encounters
// taskID, the move would create a cycle.
func assertNoCycle(ctx context.Context, tx *sql.Tx, taskID, candidateParent int64) error {
	cur := candidateParent
	for i := 0; i < 1000; i++ {
		if cur == taskID {
			return ErrCycle
		}
		var pid sql.NullInt64
		err := tx.QueryRowContext(ctx, `SELECT parent_id FROM tasks WHERE id = ?`, cur).Scan(&pid)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("walk parents: %w", err)
		}
		if !pid.Valid {
			return nil
		}
		cur = pid.Int64
	}
	return ErrCycle
}

func collectDescendants(ctx context.Context, tx *sql.Tx, root int64) ([]int64, error) {
	frontier := []int64{root}
	out := []int64{}
	for len(frontier) > 0 {
		placeholders := make([]string, len(frontier))
		args := make([]any, len(frontier))
		for i, v := range frontier {
			placeholders[i] = "?"
			args[i] = v
		}
		rows, err := tx.QueryContext(ctx,
			`SELECT id FROM tasks WHERE parent_id IN (`+strings.Join(placeholders, ",")+`)`, args...)
		if err != nil {
			return nil, fmt.Errorf("collect descendants: %w", err)
		}
		next := []int64{}
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				_ = rows.Close()
				return nil, err
			}
			next = append(next, id)
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
		out = append(out, next...)
		frontier = next
	}
	return out, nil
}

// --- counters (limit checks) ---

func (r *TaskRepo) CountWeek(ctx context.Context) (int, error) {
	return r.scalarCount(ctx, `SELECT COUNT(*) FROM tasks WHERE plan_state = 'week' AND status = 'open'`)
}

func (r *TaskRepo) CountBacklog(ctx context.Context) (int, error) {
	return r.scalarCount(ctx, `SELECT COUNT(*) FROM tasks WHERE plan_state = 'backlog' AND status = 'open'`)
}

func (r *TaskRepo) CountInbox(ctx context.Context) (int, error) {
	return r.scalarCount(ctx, `SELECT COUNT(*) FROM tasks WHERE inbox_id IS NOT NULL AND status = 'open'`)
}

func (r *TaskRepo) CountPinnedTasks(ctx context.Context) (int, error) {
	return r.scalarCount(ctx, `SELECT COUNT(*) FROM tasks WHERE is_pinned = 1`)
}

func (r *TaskRepo) CountPinnedProjects(ctx context.Context) (int, error) {
	return r.scalarCount(ctx, `SELECT COUNT(*) FROM projects WHERE is_pinned = 1`)
}

func (r *TaskRepo) scalarCount(ctx context.Context, q string, args ...any) (int, error) {
	var n int
	if err := r.db.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// --- helpers ---

func nullInt(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}

func nullTime(p *time.Time) any {
	if p == nil {
		return nil
	}
	return model.FormatUTC(*p)
}

func nullStr(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

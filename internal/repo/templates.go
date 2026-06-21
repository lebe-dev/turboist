package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
)

// TemplateRepo is the raw-SQL store for task templates and their subtasks.
type TemplateRepo struct {
	db *sql.DB
}

func NewTemplateRepo(db *sql.DB) *TemplateRepo {
	return &TemplateRepo{db: db}
}

// TemplateSubtaskInput captures one subtask's editable fields plus its labels.
type TemplateSubtaskInput struct {
	Title       string
	Description string
	Priority    model.Priority
	DayPart     model.DayPart
	LabelIDs    []int64
}

// TemplateInput is the full payload for creating or replacing a template.
// Update replaces subtasks and labels wholesale (there is no granular subtask
// API): the editor always submits the complete structure.
type TemplateInput struct {
	Name        string
	Description string
	Priority    model.Priority
	DayPart     model.DayPart
	LabelIDs    []int64
	Subtasks    []TemplateSubtaskInput
}

const templateLabelCols = `l.id, l.name, l.color, l.is_favourite, l.is_private, l.created_at, l.updated_at`

func (r *TemplateRepo) List(ctx context.Context) ([]model.TaskTemplate, error) {
	const op = "repo.templates.List"
	logQuery(ctx, op)
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, description, priority, day_part, position, created_at, updated_at
		   FROM task_templates ORDER BY position ASC, name ASC, id ASC`)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("list templates: %w", err))
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	out := make([]model.TaskTemplate, 0)
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			return nil, logErr(ctx, op, err)
		}
		out = append(out, *t)
	}
	if err := rows.Err(); err != nil {
		return nil, logErr(ctx, op, err)
	}
	for i := range out {
		if err := r.hydrate(ctx, &out[i]); err != nil {
			return nil, logErr(ctx, op, err)
		}
	}
	return out, nil
}

func (r *TemplateRepo) Get(ctx context.Context, id int64) (*model.TaskTemplate, error) {
	const op = "repo.templates.Get"
	logQuery(ctx, op, id)
	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, description, priority, day_part, position, created_at, updated_at
		   FROM task_templates WHERE id = ?`, id)
	t, err := scanTemplate(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, logErr(ctx, op, ErrNotFound)
	}
	if err != nil {
		return nil, logErr(ctx, op, err)
	}
	if err := r.hydrate(ctx, t); err != nil {
		return nil, logErr(ctx, op, err)
	}
	return t, nil
}

func (r *TemplateRepo) Create(ctx context.Context, in TemplateInput) (*model.TaskTemplate, error) {
	const op = "repo.templates.Create"
	logQuery(ctx, op)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("begin tx: %w", err))
	}
	defer func() { _ = tx.Rollback() }()

	now := model.FormatUTC(time.Now())
	var nextPos int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(position)+1, 0) FROM task_templates`).Scan(&nextPos); err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("next position: %w", err))
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO task_templates (name, description, priority, day_part, position, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		in.Name, in.Description, normPriority(in.Priority), normDayPart(in.DayPart), nextPos, now, now)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("insert template: %w", err))
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, logErr(ctx, op, err)
	}
	if err := writeTemplateChildren(ctx, tx, id, in, now); err != nil {
		return nil, logErr(ctx, op, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, logErr(ctx, op, err)
	}
	return r.Get(ctx, id)
}

func (r *TemplateRepo) Update(ctx context.Context, id int64, in TemplateInput) (*model.TaskTemplate, error) {
	const op = "repo.templates.Update"
	logQuery(ctx, op, id)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("begin tx: %w", err))
	}
	defer func() { _ = tx.Rollback() }()

	now := model.FormatUTC(time.Now())
	res, err := tx.ExecContext(ctx,
		`UPDATE task_templates SET name = ?, description = ?, priority = ?, day_part = ?, updated_at = ?
		   WHERE id = ?`,
		in.Name, in.Description, normPriority(in.Priority), normDayPart(in.DayPart), now, id)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("update template: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, logErr(ctx, op, err)
	}
	if n == 0 {
		return nil, logErr(ctx, op, ErrNotFound)
	}
	// Full replace: drop root labels and all subtasks (cascades subtask labels),
	// then rewrite from the input.
	if _, err := tx.ExecContext(ctx, `DELETE FROM task_template_labels WHERE template_id = ?`, id); err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("clear template labels: %w", err))
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM task_template_subtasks WHERE template_id = ?`, id); err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("clear template subtasks: %w", err))
	}
	if err := writeTemplateChildren(ctx, tx, id, in, now); err != nil {
		return nil, logErr(ctx, op, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, logErr(ctx, op, err)
	}
	return r.Get(ctx, id)
}

func (r *TemplateRepo) Delete(ctx context.Context, id int64) error {
	const op = "repo.templates.Delete"
	logQuery(ctx, op, id)
	res, err := r.db.ExecContext(ctx, `DELETE FROM task_templates WHERE id = ?`, id)
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("delete template: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return logErr(ctx, op, err)
	}
	if n == 0 {
		return logErr(ctx, op, ErrNotFound)
	}
	return nil
}

// writeTemplateChildren inserts the root labels and the subtasks (with their
// labels) for a freshly-inserted or just-cleared template, inside tx.
func writeTemplateChildren(ctx context.Context, tx *sql.Tx, templateID int64, in TemplateInput, now string) error {
	for _, lid := range in.LabelIDs {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO task_template_labels (template_id, label_id) VALUES (?, ?)`, templateID, lid); err != nil {
			return fmt.Errorf("insert template label: %w", err)
		}
	}
	for pos, st := range in.Subtasks {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO task_template_subtasks (template_id, position, title, description, priority, day_part, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			templateID, pos, st.Title, st.Description, normPriority(st.Priority), normDayPart(st.DayPart), now, now)
		if err != nil {
			return fmt.Errorf("insert subtask: %w", err)
		}
		stID, err := res.LastInsertId()
		if err != nil {
			return err
		}
		for _, lid := range st.LabelIDs {
			if _, err := tx.ExecContext(ctx,
				`INSERT OR IGNORE INTO task_template_subtask_labels (subtask_id, label_id) VALUES (?, ?)`, stID, lid); err != nil {
				return fmt.Errorf("insert subtask label: %w", err)
			}
		}
	}
	return nil
}

func (r *TemplateRepo) hydrate(ctx context.Context, t *model.TaskTemplate) error {
	labels, err := r.labelsFor(ctx,
		`SELECT `+templateLabelCols+` FROM task_template_labels tl
		   JOIN labels l ON l.id = tl.label_id WHERE tl.template_id = ? ORDER BY l.name ASC`, t.ID)
	if err != nil {
		return err
	}
	t.Labels = labels

	rows, err := r.db.QueryContext(ctx,
		`SELECT id, position, title, description, priority, day_part, created_at, updated_at
		   FROM task_template_subtasks WHERE template_id = ? ORDER BY position ASC, id ASC`, t.ID)
	if err != nil {
		return fmt.Errorf("list subtasks: %w", err)
	}
	defer logging.LogClose(ctx, "repo.templates.hydrate.rows", rows)

	subtasks := make([]model.TaskTemplateSubtask, 0)
	for rows.Next() {
		st, err := scanSubtask(rows)
		if err != nil {
			return err
		}
		st.TemplateID = t.ID
		subtasks = append(subtasks, *st)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range subtasks {
		labels, err := r.labelsFor(ctx,
			`SELECT `+templateLabelCols+` FROM task_template_subtask_labels sl
			   JOIN labels l ON l.id = sl.label_id WHERE sl.subtask_id = ? ORDER BY l.name ASC`, subtasks[i].ID)
		if err != nil {
			return err
		}
		subtasks[i].Labels = labels
	}
	t.Subtasks = subtasks
	return nil
}

func (r *TemplateRepo) labelsFor(ctx context.Context, query string, id int64) ([]model.Label, error) {
	rows, err := r.db.QueryContext(ctx, query, id)
	if err != nil {
		return nil, fmt.Errorf("hydrate labels: %w", err)
	}
	defer logging.LogClose(ctx, "repo.templates.labelsFor.rows", rows)
	out := make([]model.Label, 0)
	for rows.Next() {
		l, err := scanLabel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *l)
	}
	return out, rows.Err()
}

func scanTemplate(row interface{ Scan(...any) error }) (*model.TaskTemplate, error) {
	var t model.TaskTemplate
	var priority, dayPart, createdAt, updatedAt string
	if err := row.Scan(&t.ID, &t.Name, &t.Description, &priority, &dayPart, &t.Position, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	t.Priority = model.Priority(priority)
	t.DayPart = model.DayPart(dayPart)
	var err error
	if t.CreatedAt, err = model.ParseUTC(createdAt); err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	if t.UpdatedAt, err = model.ParseUTC(updatedAt); err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}
	return &t, nil
}

func scanSubtask(row interface{ Scan(...any) error }) (*model.TaskTemplateSubtask, error) {
	var st model.TaskTemplateSubtask
	var priority, dayPart, createdAt, updatedAt string
	if err := row.Scan(&st.ID, &st.Position, &st.Title, &st.Description, &priority, &dayPart, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	st.Priority = model.Priority(priority)
	st.DayPart = model.DayPart(dayPart)
	var err error
	if st.CreatedAt, err = model.ParseUTC(createdAt); err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	if st.UpdatedAt, err = model.ParseUTC(updatedAt); err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}
	return &st, nil
}

func normPriority(p model.Priority) model.Priority {
	if p == "" {
		return model.PriorityNone
	}
	return p
}

func normDayPart(d model.DayPart) model.DayPart {
	if d == "" {
		return model.DayPartNone
	}
	return d
}

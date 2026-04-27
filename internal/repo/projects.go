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

type ProjectRepo struct {
	db     *sql.DB
	labels *ProjectLabelsRepo
}

func NewProjectRepo(db *sql.DB, labels *ProjectLabelsRepo) *ProjectRepo {
	return &ProjectRepo{db: db, labels: labels}
}

func scanProject(row interface{ Scan(...any) error }) (*model.Project, error) {
	var p model.Project
	var pinned int
	var pinnedAt sql.NullString
	var createdAt, updatedAt string
	if err := row.Scan(&p.ID, &p.ContextID, &p.Title, &p.Description, &p.Color, &p.Status, &pinned, &pinnedAt, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	p.IsPinned = pinned == 1
	if pinnedAt.Valid {
		t, err := model.ParseUTC(pinnedAt.String)
		if err != nil {
			return nil, fmt.Errorf("parse pinned_at: %w", err)
		}
		p.PinnedAt = &t
	}
	t, err := model.ParseUTC(createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	p.CreatedAt = t
	t, err = model.ParseUTC(updatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}
	p.UpdatedAt = t
	return &p, nil
}

const projectColumns = `id, context_id, title, description, color, status, is_pinned, pinned_at, created_at, updated_at`

type CreateProject struct {
	ContextID   int64
	Title       string
	Description string
	Color       string
}

func (r *ProjectRepo) Create(ctx context.Context, in CreateProject) (*model.Project, error) {
	now := model.FormatUTC(time.Now())
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO projects (context_id, title, description, color, status, is_pinned, pinned_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 'open', 0, NULL, ?, ?)`,
		in.ContextID, in.Title, in.Description, in.Color, now, now)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("insert project: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, id)
}

func (r *ProjectRepo) Get(ctx context.Context, id int64) (*model.Project, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+projectColumns+` FROM projects WHERE id = ?`, id)
	p, err := scanProject(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if r.labels != nil {
		hydrated, err := r.labels.LabelsByProjectIDs(ctx, []int64{p.ID})
		if err != nil {
			return nil, err
		}
		p.Labels = hydrated[p.ID]
	}
	return p, nil
}

type ProjectListFilter struct {
	ContextID *int64
	Status    *model.ProjectStatus
}

func (r *ProjectRepo) List(ctx context.Context, filter ProjectListFilter, page Page) ([]model.Project, int, error) {
	page = page.Normalize()
	conds := []string{}
	args := []any{}
	if filter.ContextID != nil {
		conds = append(conds, "context_id = ?")
		args = append(args, *filter.ContextID)
	}
	if filter.Status != nil {
		conds = append(conds, "status = ?")
		args = append(args, string(*filter.Status))
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM projects`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count projects: %w", err)
	}

	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, page.Limit, page.Offset)
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+projectColumns+` FROM projects`+where+
			` ORDER BY is_pinned DESC, pinned_at DESC, created_at DESC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list projects: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]model.Project, 0)
	ids := make([]int64, 0)
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *p)
		ids = append(ids, p.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if r.labels != nil && len(ids) > 0 {
		hydrated, err := r.labels.LabelsByProjectIDs(ctx, ids)
		if err != nil {
			return nil, 0, err
		}
		for i := range out {
			out[i].Labels = hydrated[out[i].ID]
		}
	}
	return out, total, nil
}

type ProjectUpdate struct {
	Title       *string
	Description *string
	Color       *string
	ContextID   *int64
}

func (r *ProjectRepo) Update(ctx context.Context, id int64, u ProjectUpdate) (*model.Project, error) {
	sets := make([]string, 0, 4)
	args := make([]any, 0, 5)
	if u.Title != nil {
		sets = append(sets, "title = ?")
		args = append(args, *u.Title)
	}
	if u.Description != nil {
		sets = append(sets, "description = ?")
		args = append(args, *u.Description)
	}
	if u.Color != nil {
		sets = append(sets, "color = ?")
		args = append(args, *u.Color)
	}
	if u.ContextID != nil {
		sets = append(sets, "context_id = ?")
		args = append(args, *u.ContextID)
	}
	if len(sets) == 0 {
		return r.Get(ctx, id)
	}
	sets = append(sets, "updated_at = ?")
	args = append(args, model.FormatUTC(time.Now()))
	args = append(args, id)

	res, err := r.db.ExecContext(ctx, `UPDATE projects SET `+joinSets(sets)+` WHERE id = ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("update project: %w", err)
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

func (r *ProjectRepo) UpdateStatus(ctx context.Context, id int64, status model.ProjectStatus) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE projects SET status = ?, updated_at = ? WHERE id = ?`,
		string(status), model.FormatUTC(time.Now()), id)
	if err != nil {
		return fmt.Errorf("update project status: %w", err)
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

func (r *ProjectRepo) SetPinned(ctx context.Context, id int64, pinned bool) error {
	now := model.FormatUTC(time.Now())
	var res sql.Result
	var err error
	if pinned {
		res, err = r.db.ExecContext(ctx,
			`UPDATE projects SET is_pinned = 1, pinned_at = ?, updated_at = ? WHERE id = ?`, now, now, id)
	} else {
		res, err = r.db.ExecContext(ctx,
			`UPDATE projects SET is_pinned = 0, pinned_at = NULL, updated_at = ? WHERE id = ?`, now, id)
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

func (r *ProjectRepo) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
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

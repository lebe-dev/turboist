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

type LabelRepo struct {
	db *sql.DB
}

func NewLabelRepo(db *sql.DB) *LabelRepo {
	return &LabelRepo{db: db}
}

func scanLabel(row interface{ Scan(...any) error }) (*model.Label, error) {
	var l model.Label
	var fav int
	var createdAt, updatedAt string
	if err := row.Scan(&l.ID, &l.Name, &l.Color, &fav, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	l.IsFavourite = fav == 1
	t, err := model.ParseUTC(createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	l.CreatedAt = t
	t, err = model.ParseUTC(updatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}
	l.UpdatedAt = t
	return &l, nil
}

func (r *LabelRepo) Create(ctx context.Context, name, color string, isFavourite bool) (*model.Label, error) {
	now := model.FormatUTC(time.Now())
	favInt := 0
	if isFavourite {
		favInt = 1
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO labels (name, color, is_favourite, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		name, color, favInt, now, now)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("insert label: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, id)
}

func (r *LabelRepo) Get(ctx context.Context, id int64) (*model.Label, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, color, is_favourite, created_at, updated_at FROM labels WHERE id = ?`, id)
	l, err := scanLabel(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return l, nil
}

func (r *LabelRepo) GetByName(ctx context.Context, name string) (*model.Label, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, color, is_favourite, created_at, updated_at FROM labels WHERE name = ?`, name)
	l, err := scanLabel(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return l, nil
}

type LabelListFilter struct {
	Query string
}

func (r *LabelRepo) List(ctx context.Context, filter LabelListFilter, page Page) ([]model.Label, int, error) {
	page = page.Normalize()
	where := ""
	args := []any{}
	if q := strings.TrimSpace(filter.Query); q != "" {
		where = " WHERE name LIKE ?"
		args = append(args, "%"+q+"%")
	}

	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM labels`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count labels: %w", err)
	}

	args = append(args, page.Limit, page.Offset)
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, color, is_favourite, created_at, updated_at FROM labels`+where+
			` ORDER BY is_favourite DESC, name ASC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list labels: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]model.Label, 0)
	for rows.Next() {
		l, err := scanLabel(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *l)
	}
	return out, total, rows.Err()
}

type LabelUpdate struct {
	Name        *string
	Color       *string
	IsFavourite *bool
}

func (r *LabelRepo) Update(ctx context.Context, id int64, u LabelUpdate) (*model.Label, error) {
	sets := make([]string, 0, 4)
	args := make([]any, 0, 4)
	if u.Name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *u.Name)
	}
	if u.Color != nil {
		sets = append(sets, "color = ?")
		args = append(args, *u.Color)
	}
	if u.IsFavourite != nil {
		sets = append(sets, "is_favourite = ?")
		fv := 0
		if *u.IsFavourite {
			fv = 1
		}
		args = append(args, fv)
	}
	if len(sets) == 0 {
		return r.Get(ctx, id)
	}
	sets = append(sets, "updated_at = ?")
	args = append(args, model.FormatUTC(time.Now()))
	args = append(args, id)

	res, err := r.db.ExecContext(ctx, `UPDATE labels SET `+joinSets(sets)+` WHERE id = ?`, args...)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("update label: %w", err)
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

func (r *LabelRepo) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM labels WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete label: %w", err)
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

func (r *LabelRepo) GetByIDs(ctx context.Context, ids []int64) ([]model.Label, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	q := `SELECT id, name, color, is_favourite, created_at, updated_at FROM labels WHERE id IN (` +
		strings.Join(placeholders, ",") + `)`
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("get labels by ids: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]model.Label, 0, len(ids))
	for rows.Next() {
		l, err := scanLabel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *l)
	}
	return out, rows.Err()
}

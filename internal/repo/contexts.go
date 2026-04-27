package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

type ContextRepo struct {
	db *sql.DB
}

func NewContextRepo(db *sql.DB) *ContextRepo {
	return &ContextRepo{db: db}
}

func scanContext(row interface{ Scan(...any) error }) (*model.Context, error) {
	var c model.Context
	var fav int
	var createdAt, updatedAt string
	if err := row.Scan(&c.ID, &c.Name, &c.Color, &fav, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	c.IsFavourite = fav == 1
	t, err := model.ParseUTC(createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	c.CreatedAt = t
	t, err = model.ParseUTC(updatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}
	c.UpdatedAt = t
	return &c, nil
}

func (r *ContextRepo) Create(ctx context.Context, name, color string, isFavourite bool) (*model.Context, error) {
	now := model.FormatUTC(time.Now())
	favInt := 0
	if isFavourite {
		favInt = 1
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO contexts (name, color, is_favourite, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		name, color, favInt, now, now)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("insert context: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("last insert id: %w", err)
	}
	return r.Get(ctx, id)
}

func (r *ContextRepo) Get(ctx context.Context, id int64) (*model.Context, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, color, is_favourite, created_at, updated_at FROM contexts WHERE id = ?`, id)
	c, err := scanContext(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (r *ContextRepo) List(ctx context.Context, page Page) ([]model.Context, int, error) {
	page = page.Normalize()
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM contexts`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count contexts: %w", err)
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, color, is_favourite, created_at, updated_at FROM contexts
		 ORDER BY is_favourite DESC, name ASC LIMIT ? OFFSET ?`, page.Limit, page.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list contexts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]model.Context, 0)
	for rows.Next() {
		c, err := scanContext(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *c)
	}
	return out, total, rows.Err()
}

type ContextUpdate struct {
	Name        *string
	Color       *string
	IsFavourite *bool
}

func (r *ContextRepo) Update(ctx context.Context, id int64, u ContextUpdate) (*model.Context, error) {
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

	q := `UPDATE contexts SET ` + joinSets(sets) + ` WHERE id = ?`
	res, err := r.db.ExecContext(ctx, q, args...)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("update context: %w", err)
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

func (r *ContextRepo) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM contexts WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete context: %w", err)
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

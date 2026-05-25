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

type ContextRepo struct {
	db *sql.DB
}

func NewContextRepo(db *sql.DB) *ContextRepo {
	return &ContextRepo{db: db}
}

const contextColumns = `id, name, color, is_favourite, client_id, deleted_at, created_at, updated_at`

func scanContext(row interface{ Scan(...any) error }) (*model.Context, error) {
	var c model.Context
	var fav int
	var clientID, deletedAt sql.NullString
	var createdAt, updatedAt string
	if err := row.Scan(&c.ID, &c.Name, &c.Color, &fav, &clientID, &deletedAt, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	c.IsFavourite = fav == 1
	if clientID.Valid {
		v := clientID.String
		c.ClientID = &v
	}
	if deletedAt.Valid {
		ts, err := model.ParseUTC(deletedAt.String)
		if err != nil {
			return nil, fmt.Errorf("parse deleted_at: %w", err)
		}
		c.DeletedAt = &ts
	}
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
	return r.CreateWithClientID(ctx, name, color, isFavourite, nil)
}

func (r *ContextRepo) CreateWithClientID(ctx context.Context, name, color string, isFavourite bool, clientID *string) (*model.Context, error) {
	const op = "repo.contexts.Create"
	logQuery(ctx, op, color, isFavourite)
	now := model.FormatUTC(time.Now())
	favInt := 0
	if isFavourite {
		favInt = 1
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO contexts (name, color, is_favourite, client_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		name, color, favInt, nullStr(clientID), now, now)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, logErr(ctx, op, ErrConflict)
		}
		return nil, logErr(ctx, op, fmt.Errorf("insert context: %w", err))
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("last insert id: %w", err))
	}
	return r.Get(ctx, id)
}

func (r *ContextRepo) Get(ctx context.Context, id int64) (*model.Context, error) {
	const op = "repo.contexts.Get"
	logQuery(ctx, op, id)
	row := r.db.QueryRowContext(ctx,
		`SELECT `+contextColumns+` FROM contexts WHERE id = ?`, id)
	c, err := scanContext(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, logErr(ctx, op, ErrNotFound)
	}
	if err != nil {
		return nil, logErr(ctx, op, err)
	}
	return c, nil
}

func (r *ContextRepo) List(ctx context.Context, page Page) ([]model.Context, int, error) {
	const op = "repo.contexts.List"
	logQuery(ctx, op, page)
	page = page.Normalize()
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM contexts WHERE deleted_at IS NULL`).Scan(&total); err != nil {
		return nil, 0, logErr(ctx, op, fmt.Errorf("count contexts: %w", err))
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+contextColumns+` FROM contexts
		 WHERE deleted_at IS NULL
		 ORDER BY is_favourite DESC, name ASC LIMIT ? OFFSET ?`, page.Limit, page.Offset)
	if err != nil {
		return nil, 0, logErr(ctx, op, fmt.Errorf("list contexts: %w", err))
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	out := make([]model.Context, 0)
	for rows.Next() {
		c, err := scanContext(rows)
		if err != nil {
			return nil, 0, logErr(ctx, op, err)
		}
		out = append(out, *c)
	}
	return out, total, rows.Err()
}

type ContextUpdate struct {
	Name        *string
	Color       *string
	IsFavourite *bool
	ClientID    *string
}

func (r *ContextRepo) Update(ctx context.Context, id int64, u ContextUpdate) (*model.Context, error) {
	const op = "repo.contexts.Update"
	logQuery(ctx, op, id)
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
	if u.ClientID != nil {
		sets = append(sets, "client_id = ?")
		args = append(args, *u.ClientID)
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
			return nil, logErr(ctx, op, ErrConflict)
		}
		return nil, logErr(ctx, op, fmt.Errorf("update context: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, logErr(ctx, op, err)
	}
	if n == 0 {
		return nil, logErr(ctx, op, ErrNotFound)
	}
	return r.Get(ctx, id)
}

// Delete soft-deletes the context; ErrGone signals already-tombstoned.
func (r *ContextRepo) Delete(ctx context.Context, id int64) error {
	const op = "repo.contexts.Delete"
	logQuery(ctx, op, id)
	now := model.FormatUTC(time.Now())
	res, err := r.db.ExecContext(ctx,
		`UPDATE contexts SET deleted_at = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL`,
		now, now, id)
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("delete context: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return logErr(ctx, op, err)
	}
	if n > 0 {
		return nil
	}
	var exists int
	if err := r.db.QueryRowContext(ctx, `SELECT 1 FROM contexts WHERE id = ?`, id).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return logErr(ctx, op, ErrNotFound)
		}
		return logErr(ctx, op, fmt.Errorf("delete context: %w", err))
	}
	return logErr(ctx, op, ErrGone)
}

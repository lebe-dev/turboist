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
	c.ClientID = clientID.String
	if deletedAt.Valid {
		t, err := model.ParseUTC(deletedAt.String)
		if err != nil {
			return nil, fmt.Errorf("parse deleted_at: %w", err)
		}
		c.DeletedAt = &t
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
	const op = "repo.contexts.Create"
	logQuery(ctx, op, color, isFavourite)
	now := model.FormatUTC(time.Now())
	favInt := 0
	if isFavourite {
		favInt = 1
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO contexts (name, color, is_favourite, client_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		name, color, favInt, model.NewClientID(), now, now)
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
		`SELECT `+contextColumns+` FROM contexts WHERE id = ? AND deleted_at IS NULL`, id)
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
		`SELECT `+contextColumns+` FROM contexts WHERE deleted_at IS NULL
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
	if len(sets) == 0 {
		return r.Get(ctx, id)
	}
	sets = append(sets, "updated_at = ?")
	args = append(args, model.FormatUTC(time.Now()))
	args = append(args, id)

	q := `UPDATE contexts SET ` + joinSets(sets) + ` WHERE id = ? AND deleted_at IS NULL`
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
		return nil, logErr(ctx, op, notFoundOrGone(ctx, r.db, "contexts", id))
	}
	return r.Get(ctx, id)
}

// Delete soft-deletes the context and emulates the former FK cascade
// (contexts → projects → project_sections → tasks; tasks attached directly to
// the context). The DB ON DELETE CASCADE no longer fires for soft-deletes, so
// the child tombstones are written here in one transaction (Federation v1 F0.1).
func (r *ContextRepo) Delete(ctx context.Context, id int64) error {
	const op = "repo.contexts.Delete"
	logQuery(ctx, op, id)
	now := model.FormatUTC(time.Now())
	err := withTx(ctx, r.db, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE contexts SET deleted_at = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL`, now, now, id)
		if err != nil {
			return fmt.Errorf("soft-delete context: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		// Cascade: tasks bound directly to the context, and all projects of
		// the context together with their sections and tasks.
		if _, err := tx.ExecContext(ctx,
			`UPDATE tasks SET deleted_at = ?, updated_at = ?
			 WHERE deleted_at IS NULL AND (context_id = ? OR project_id IN (SELECT id FROM projects WHERE context_id = ?))`,
			now, now, id, id); err != nil {
			return fmt.Errorf("cascade tasks: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE project_sections SET deleted_at = ?, updated_at = ?
			 WHERE deleted_at IS NULL AND project_id IN (SELECT id FROM projects WHERE context_id = ?)`,
			now, now, id); err != nil {
			return fmt.Errorf("cascade sections: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE projects SET deleted_at = ?, updated_at = ? WHERE deleted_at IS NULL AND context_id = ?`,
			now, now, id); err != nil {
			return fmt.Errorf("cascade projects: %w", err)
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return logErr(ctx, op, ErrNotFound)
		}
		return logErr(ctx, op, fmt.Errorf("delete context: %w", err))
	}
	return nil
}

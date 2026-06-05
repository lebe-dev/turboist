package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
)

type LabelRepo struct {
	db *sql.DB
}

func NewLabelRepo(db *sql.DB) *LabelRepo {
	return &LabelRepo{db: db}
}

const labelColumns = `id, name, color, is_favourite, is_private, client_id, deleted_at, created_at, updated_at`

func scanLabel(row interface{ Scan(...any) error }) (*model.Label, error) {
	var l model.Label
	var fav, priv int
	var clientID, deletedAt sql.NullString
	var createdAt, updatedAt string
	if err := row.Scan(&l.ID, &l.Name, &l.Color, &fav, &priv, &clientID, &deletedAt, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	l.IsFavourite = fav == 1
	l.IsPrivate = priv == 1
	l.ClientID = clientID.String
	if deletedAt.Valid {
		t, err := model.ParseUTC(deletedAt.String)
		if err != nil {
			return nil, fmt.Errorf("parse deleted_at: %w", err)
		}
		l.DeletedAt = &t
	}
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
	const op = "repo.labels.Create"
	logQuery(ctx, op, color, isFavourite)
	now := model.FormatUTC(time.Now())
	favInt := 0
	if isFavourite {
		favInt = 1
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO labels (name, color, is_favourite, client_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		name, color, favInt, model.NewClientID(), now, now)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, logErr(ctx, op, ErrConflict)
		}
		return nil, logErr(ctx, op, fmt.Errorf("insert label: %w", err))
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, logErr(ctx, op, err)
	}
	return r.Get(ctx, id)
}

func (r *LabelRepo) Get(ctx context.Context, id int64) (*model.Label, error) {
	const op = "repo.labels.Get"
	logQuery(ctx, op, id)
	row := r.db.QueryRowContext(ctx,
		`SELECT `+labelColumns+` FROM labels WHERE id = ? AND deleted_at IS NULL`, id)
	l, err := scanLabel(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, logErr(ctx, op, ErrNotFound)
	}
	if err != nil {
		return nil, logErr(ctx, op, err)
	}
	return l, nil
}

func (r *LabelRepo) GetByName(ctx context.Context, name string) (*model.Label, error) {
	const op = "repo.labels.GetByName"
	logQuery(ctx, op)
	row := r.db.QueryRowContext(ctx,
		`SELECT `+labelColumns+` FROM labels WHERE name = ? AND deleted_at IS NULL`, name)
	l, err := scanLabel(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, logErr(ctx, op, ErrNotFound)
	}
	if err != nil {
		return nil, logErr(ctx, op, err)
	}
	return l, nil
}

type LabelListFilter struct {
	Query string
}

func (r *LabelRepo) List(ctx context.Context, filter LabelListFilter, page Page) ([]model.Label, int, error) {
	const op = "repo.labels.List"
	logQuery(ctx, op, filter, page)
	page = page.Normalize()
	where := " WHERE deleted_at IS NULL"
	args := []any{}
	if q := strings.TrimSpace(filter.Query); q != "" {
		where += " AND name LIKE ?"
		args = append(args, "%"+q+"%")
	}

	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM labels`+where, args...).Scan(&total); err != nil {
		return nil, 0, logErr(ctx, op, fmt.Errorf("count labels: %w", err))
	}

	args = append(args, page.Limit, page.Offset)
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+labelColumns+` FROM labels`+where+
			` ORDER BY is_favourite DESC, name ASC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, logErr(ctx, op, fmt.Errorf("list labels: %w", err))
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	out := make([]model.Label, 0)
	for rows.Next() {
		l, err := scanLabel(rows)
		if err != nil {
			return nil, 0, logErr(ctx, op, err)
		}
		out = append(out, *l)
	}
	return out, total, rows.Err()
}

type LabelUpdate struct {
	Name        *string
	Color       *string
	IsFavourite *bool
	IsPrivate   *bool
}

func (r *LabelRepo) Update(ctx context.Context, id int64, u LabelUpdate) (*model.Label, error) {
	const op = "repo.labels.Update"
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
	if u.IsPrivate != nil {
		sets = append(sets, "is_private = ?")
		pv := 0
		if *u.IsPrivate {
			pv = 1
		}
		args = append(args, pv)
	}
	if len(sets) == 0 {
		return r.Get(ctx, id)
	}
	sets = append(sets, "updated_at = ?")
	args = append(args, model.FormatUTC(time.Now()))
	args = append(args, id)

	res, err := r.db.ExecContext(ctx, `UPDATE labels SET `+joinSets(sets)+` WHERE id = ? AND deleted_at IS NULL`, args...)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, logErr(ctx, op, ErrConflict)
		}
		return nil, logErr(ctx, op, fmt.Errorf("update label: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, logErr(ctx, op, err)
	}
	if n == 0 {
		return nil, logErr(ctx, op, notFoundOrGone(ctx, r.db, "labels", id))
	}
	return r.Get(ctx, id)
}

// Delete soft-deletes the label. The task_labels/project_labels junctions
// previously cleaned up via FK CASCADE; since the label row now survives, the
// junction rows are hard-deleted so the label disappears from every task and
// project it was attached to (Federation v1 F0.1).
func (r *LabelRepo) Delete(ctx context.Context, id int64) error {
	const op = "repo.labels.Delete"
	logQuery(ctx, op, id)
	now := model.FormatUTC(time.Now())
	err := withTx(ctx, r.db, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE labels SET deleted_at = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL`, now, now, id)
		if err != nil {
			return fmt.Errorf("soft-delete label: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM task_labels WHERE label_id = ?`, id); err != nil {
			return fmt.Errorf("detach task labels: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM project_labels WHERE label_id = ?`, id); err != nil {
			return fmt.Errorf("detach project labels: %w", err)
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return logErr(ctx, op, ErrNotFound)
		}
		return logErr(ctx, op, fmt.Errorf("delete label: %w", err))
	}
	return nil
}

func (r *LabelRepo) GetByIDs(ctx context.Context, ids []int64) ([]model.Label, error) {
	const op = "repo.labels.GetByIDs"
	logQuery(ctx, op, ids)
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	q := `SELECT ` + labelColumns + ` FROM labels WHERE deleted_at IS NULL AND id IN (` +
		strings.Join(placeholders, ",") + `)`
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("get labels by ids: %w", err))
	}
	defer logging.LogClose(ctx, op+".rows", rows)
	out := make([]model.Label, 0, len(ids))
	for rows.Next() {
		l, err := scanLabel(rows)
		if err != nil {
			return nil, logErr(ctx, op, err)
		}
		out = append(out, *l)
	}
	return out, rows.Err()
}

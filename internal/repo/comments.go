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

// CommentRepo provides CRUD for task comments (Federation v1 F0.2). Comments are
// immutable: there is deliberately no Update method, so a comment body can never
// change after creation — only create and (soft-)delete participate in
// federation sync (US-3.5 AC2).
type CommentRepo struct {
	db *sql.DB
}

func NewCommentRepo(db *sql.DB) *CommentRepo {
	return &CommentRepo{db: db}
}

const commentColumns = `id, task_id, body, client_id, deleted_at, created_at, updated_at`

func scanComment(row interface{ Scan(...any) error }) (*model.Comment, error) {
	var c model.Comment
	var clientID, deletedAt sql.NullString
	var createdAt, updatedAt string
	if err := row.Scan(&c.ID, &c.TaskID, &c.Body, &clientID, &deletedAt, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
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

// Create inserts a comment on the given task and returns it.
func (r *CommentRepo) Create(ctx context.Context, taskID int64, body string) (*model.Comment, error) {
	const op = "repo.comments.Create"
	logQuery(ctx, op, taskID)
	now := model.FormatUTC(time.Now())
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO comments (task_id, body, client_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		taskID, body, model.NewClientID(), now, now)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("insert comment: %w", err))
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("last insert id: %w", err))
	}
	return r.Get(ctx, id)
}

// Get returns a live (non-tombstoned) comment by id.
func (r *CommentRepo) Get(ctx context.Context, id int64) (*model.Comment, error) {
	const op = "repo.comments.Get"
	logQuery(ctx, op, id)
	row := r.db.QueryRowContext(ctx,
		`SELECT `+commentColumns+` FROM comments WHERE id = ? AND deleted_at IS NULL`, id)
	c, err := scanComment(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, logErr(ctx, op, ErrNotFound)
	}
	if err != nil {
		return nil, logErr(ctx, op, err)
	}
	return c, nil
}

// ListByTask returns the task's live comments oldest-first (creation order).
func (r *CommentRepo) ListByTask(ctx context.Context, taskID int64, page Page) ([]model.Comment, int, error) {
	const op = "repo.comments.ListByTask"
	logQuery(ctx, op, taskID, page)
	page = page.Normalize()
	var total int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM comments WHERE task_id = ? AND deleted_at IS NULL`, taskID).Scan(&total); err != nil {
		return nil, 0, logErr(ctx, op, fmt.Errorf("count comments: %w", err))
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+commentColumns+` FROM comments
		 WHERE task_id = ? AND deleted_at IS NULL ORDER BY created_at ASC, id ASC LIMIT ? OFFSET ?`,
		taskID, page.Limit, page.Offset)
	if err != nil {
		return nil, 0, logErr(ctx, op, fmt.Errorf("list comments: %w", err))
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	out := make([]model.Comment, 0)
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			return nil, 0, logErr(ctx, op, err)
		}
		out = append(out, *c)
	}
	return out, total, rows.Err()
}

// Delete soft-deletes a comment that belongs to taskID. The mutation is scoped
// by task_id so a comment can never be deleted through a sibling task's URL; a
// mismatched (taskID, id) pair affects no rows and surfaces as ErrNotFound.
// There is no hard-delete on this path; the tombstone is final (re-delete
// returns ErrGone) so deletion participates in federation sync (Federation v1
// F0.2).
func (r *CommentRepo) Delete(ctx context.Context, taskID, id int64) error {
	const op = "repo.comments.Delete"
	logQuery(ctx, op, taskID, id)
	now := model.FormatUTC(time.Now())
	res, err := r.db.ExecContext(ctx,
		`UPDATE comments SET deleted_at = ?, updated_at = ? WHERE id = ? AND task_id = ? AND deleted_at IS NULL`, now, now, id, taskID)
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("soft-delete comment: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return logErr(ctx, op, err)
	}
	if n == 0 {
		return logErr(ctx, op, notFoundOrGone(ctx, r.db, "comments", id))
	}
	return nil
}

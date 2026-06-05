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

// ChecklistItemRepo provides CRUD for task checklist items (Federation v1 F0.2).
// Unlike comments, checklist items are mutable (title + completion toggle).
type ChecklistItemRepo struct {
	db *sql.DB
}

func NewChecklistItemRepo(db *sql.DB) *ChecklistItemRepo {
	return &ChecklistItemRepo{db: db}
}

const checklistItemColumns = `id, task_id, title, is_completed, position, frac_position, client_id, deleted_at, created_at, updated_at`

func scanChecklistItem(row interface{ Scan(...any) error }) (*model.ChecklistItem, error) {
	var it model.ChecklistItem
	var completed int
	var fracPosition, clientID, deletedAt sql.NullString
	var createdAt, updatedAt string
	if err := row.Scan(&it.ID, &it.TaskID, &it.Title, &completed, &it.Position, &fracPosition, &clientID, &deletedAt, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	it.IsCompleted = completed == 1
	it.FracPosition = fracPosition.String
	it.ClientID = clientID.String
	if deletedAt.Valid {
		t, err := model.ParseUTC(deletedAt.String)
		if err != nil {
			return nil, fmt.Errorf("parse deleted_at: %w", err)
		}
		it.DeletedAt = &t
	}
	t, err := model.ParseUTC(createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	it.CreatedAt = t
	t, err = model.ParseUTC(updatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}
	it.UpdatedAt = t
	return &it, nil
}

// Create appends a checklist item to the task at the next integer position.
func (r *ChecklistItemRepo) Create(ctx context.Context, taskID int64, title string) (*model.ChecklistItem, error) {
	const op = "repo.checklist_items.Create"
	logQuery(ctx, op, taskID)
	now := model.FormatUTC(time.Now())
	var nextPos int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(position), -1) + 1 FROM checklist_items WHERE task_id = ? AND deleted_at IS NULL`,
		taskID).Scan(&nextPos); err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("next checklist position: %w", err))
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO checklist_items (task_id, title, is_completed, position, client_id, created_at, updated_at)
		 VALUES (?, ?, 0, ?, ?, ?, ?)`,
		taskID, title, nextPos, model.NewClientID(), now, now)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("insert checklist item: %w", err))
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("last insert id: %w", err))
	}
	return r.Get(ctx, id)
}

// Get returns a live (non-tombstoned) checklist item by id.
func (r *ChecklistItemRepo) Get(ctx context.Context, id int64) (*model.ChecklistItem, error) {
	const op = "repo.checklist_items.Get"
	logQuery(ctx, op, id)
	row := r.db.QueryRowContext(ctx,
		`SELECT `+checklistItemColumns+` FROM checklist_items WHERE id = ? AND deleted_at IS NULL`, id)
	it, err := scanChecklistItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, logErr(ctx, op, ErrNotFound)
	}
	if err != nil {
		return nil, logErr(ctx, op, err)
	}
	return it, nil
}

// ListByTask returns the task's live checklist items ordered by position.
func (r *ChecklistItemRepo) ListByTask(ctx context.Context, taskID int64, page Page) ([]model.ChecklistItem, int, error) {
	const op = "repo.checklist_items.ListByTask"
	logQuery(ctx, op, taskID, page)
	page = page.Normalize()
	var total int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM checklist_items WHERE task_id = ? AND deleted_at IS NULL`, taskID).Scan(&total); err != nil {
		return nil, 0, logErr(ctx, op, fmt.Errorf("count checklist items: %w", err))
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+checklistItemColumns+` FROM checklist_items
		 WHERE task_id = ? AND deleted_at IS NULL ORDER BY position ASC, id ASC LIMIT ? OFFSET ?`,
		taskID, page.Limit, page.Offset)
	if err != nil {
		return nil, 0, logErr(ctx, op, fmt.Errorf("list checklist items: %w", err))
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	out := make([]model.ChecklistItem, 0)
	for rows.Next() {
		it, err := scanChecklistItem(rows)
		if err != nil {
			return nil, 0, logErr(ctx, op, err)
		}
		out = append(out, *it)
	}
	return out, total, rows.Err()
}

// ChecklistItemUpdate is the patch shape for a checklist item. Nil fields are
// left unchanged. Toggling IsCompleted on one item leaves siblings untouched.
type ChecklistItemUpdate struct {
	Title       *string
	IsCompleted *bool
}

// Update patches the title and/or completion of a checklist item that belongs to
// taskID. The mutation is scoped by task_id so an item can never be edited via a
// sibling task's URL; a mismatched (taskID, id) pair affects no rows and surfaces
// as ErrNotFound. A re-edit of a soft-deleted item returns ErrGone (the tombstone
// is final).
func (r *ChecklistItemRepo) Update(ctx context.Context, taskID, id int64, u ChecklistItemUpdate) (*model.ChecklistItem, error) {
	const op = "repo.checklist_items.Update"
	logQuery(ctx, op, taskID, id)
	sets := make([]string, 0, 3)
	args := make([]any, 0, 3)
	if u.Title != nil {
		sets = append(sets, "title = ?")
		args = append(args, *u.Title)
	}
	if u.IsCompleted != nil {
		sets = append(sets, "is_completed = ?")
		completed := 0
		if *u.IsCompleted {
			completed = 1
		}
		args = append(args, completed)
	}
	if len(sets) == 0 {
		return r.Get(ctx, id)
	}
	sets = append(sets, "updated_at = ?")
	args = append(args, model.FormatUTC(time.Now()))
	args = append(args, id, taskID)

	q := `UPDATE checklist_items SET ` + joinSets(sets) + ` WHERE id = ? AND task_id = ? AND deleted_at IS NULL`
	res, err := r.db.ExecContext(ctx, q, args...)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("update checklist item: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, logErr(ctx, op, err)
	}
	if n == 0 {
		return nil, logErr(ctx, op, notFoundOrGone(ctx, r.db, "checklist_items", id))
	}
	return r.Get(ctx, id)
}

// Delete soft-deletes a checklist item that belongs to taskID. The mutation is
// scoped by task_id so an item can never be deleted via a sibling task's URL; a
// mismatched (taskID, id) pair affects no rows and surfaces as ErrNotFound. The
// tombstone is final (re-delete and re-edit return ErrGone) so deletion
// participates in federation sync.
func (r *ChecklistItemRepo) Delete(ctx context.Context, taskID, id int64) error {
	const op = "repo.checklist_items.Delete"
	logQuery(ctx, op, taskID, id)
	now := model.FormatUTC(time.Now())
	res, err := r.db.ExecContext(ctx,
		`UPDATE checklist_items SET deleted_at = ?, updated_at = ? WHERE id = ? AND task_id = ? AND deleted_at IS NULL`, now, now, id, taskID)
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("soft-delete checklist item: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return logErr(ctx, op, err)
	}
	if n == 0 {
		return logErr(ctx, op, notFoundOrGone(ctx, r.db, "checklist_items", id))
	}
	return nil
}

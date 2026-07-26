package repo

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
)

type TaskLabelsRepo struct {
	db *sql.DB
}

func NewTaskLabelsRepo(db *sql.DB) *TaskLabelsRepo {
	return &TaskLabelsRepo{db: db}
}

// SetForTask replaces the task's label set with labelIDs.
//
// It is a diff, not a delete-and-reinsert: rows that survive keep their
// `created_at` (migration 047), so editing an unrelated field on a task does not
// move all of its tagging events into the current week and skew the usage stats.
func (r *TaskLabelsRepo) SetForTask(ctx context.Context, taskID int64, labelIDs []int64) error {
	const op = "repo.task_labels.SetForTask"
	logQuery(ctx, op, taskID, labelIDs)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("begin tx: %w", err))
	}
	defer func() { _ = tx.Rollback() }()

	del := `DELETE FROM task_labels WHERE task_id = ?`
	delArgs := []any{taskID}
	if len(labelIDs) > 0 {
		placeholders := make([]string, len(labelIDs))
		for i, lid := range labelIDs {
			placeholders[i] = "?"
			delArgs = append(delArgs, lid)
		}
		del += ` AND label_id NOT IN (` + strings.Join(placeholders, ",") + `)`
	}
	if _, err := tx.ExecContext(ctx, del, delArgs...); err != nil {
		return logErr(ctx, op, fmt.Errorf("prune task_labels: %w", err))
	}

	now := model.FormatUTC(time.Now())
	for _, lid := range labelIDs {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO task_labels (task_id, label_id, created_at) VALUES (?, ?, ?)`,
			taskID, lid, now); err != nil {
			return logErr(ctx, op, fmt.Errorf("insert task_label: %w", err))
		}
	}
	return tx.Commit()
}

func (r *TaskLabelsRepo) LabelsByTaskIDs(ctx context.Context, taskIDs []int64) (map[int64][]model.Label, error) {
	const op = "repo.task_labels.LabelsByTaskIDs"
	logQuery(ctx, op, taskIDs)
	if len(taskIDs) == 0 {
		return map[int64][]model.Label{}, nil
	}
	placeholders := make([]string, len(taskIDs))
	args := make([]any, len(taskIDs))
	for i, id := range taskIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	q := `SELECT tl.task_id, l.id, l.name, l.color, l.is_favourite, l.created_at, l.updated_at
	      FROM task_labels tl
	      JOIN labels l ON l.id = tl.label_id
	      WHERE tl.task_id IN (` + strings.Join(placeholders, ",") + `)
	      ORDER BY l.name ASC`
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("hydrate task labels: %w", err))
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	out := make(map[int64][]model.Label, len(taskIDs))
	for rows.Next() {
		var taskID int64
		var l model.Label
		var fav int
		var createdAt, updatedAt string
		if err := rows.Scan(&taskID, &l.ID, &l.Name, &l.Color, &fav, &createdAt, &updatedAt); err != nil {
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
		out[taskID] = append(out[taskID], l)
	}
	return out, rows.Err()
}

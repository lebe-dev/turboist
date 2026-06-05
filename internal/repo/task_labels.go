package repo

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
)

type TaskLabelsRepo struct {
	db *sql.DB
}

func NewTaskLabelsRepo(db *sql.DB) *TaskLabelsRepo {
	return &TaskLabelsRepo{db: db}
}

func (r *TaskLabelsRepo) SetForTask(ctx context.Context, taskID int64, labelIDs []int64) error {
	const op = "repo.task_labels.SetForTask"
	logQuery(ctx, op, taskID, labelIDs)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("begin tx: %w", err))
	}
	defer func() { _ = tx.Rollback() }()

	if err := r.SetForTaskTx(ctx, tx, taskID, labelIDs); err != nil {
		return err
	}
	return tx.Commit()
}

// SetForTaskTx replaces a task's label set inside a caller's transaction, so a
// federated mutation can copy labels onto a newly-inserted local row (e.g. a
// recurrence-completion snapshot) atomically with the insert. Labels are a LOCAL
// concern — they are excluded only from the federated event field set (§3), not
// from the local row.
func (r *TaskLabelsRepo) SetForTaskTx(ctx context.Context, tx *sql.Tx, taskID int64, labelIDs []int64) error {
	const op = "repo.task_labels.SetForTaskTx"
	logQuery(ctx, op, taskID, labelIDs)
	if _, err := tx.ExecContext(ctx, `DELETE FROM task_labels WHERE task_id = ?`, taskID); err != nil {
		return logErr(ctx, op, fmt.Errorf("clear task_labels: %w", err))
	}
	for _, lid := range labelIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO task_labels (task_id, label_id) VALUES (?, ?)`, taskID, lid); err != nil {
			return logErr(ctx, op, fmt.Errorf("insert task_label: %w", err))
		}
	}
	return nil
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
	      WHERE l.deleted_at IS NULL AND tl.task_id IN (` + strings.Join(placeholders, ",") + `)
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

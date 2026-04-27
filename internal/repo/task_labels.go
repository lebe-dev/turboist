package repo

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/lebe-dev/turboist/internal/model"
)

type TaskLabelsRepo struct {
	db *sql.DB
}

func NewTaskLabelsRepo(db *sql.DB) *TaskLabelsRepo {
	return &TaskLabelsRepo{db: db}
}

func (r *TaskLabelsRepo) SetForTask(ctx context.Context, taskID int64, labelIDs []int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM task_labels WHERE task_id = ?`, taskID); err != nil {
		return fmt.Errorf("clear task_labels: %w", err)
	}
	for _, lid := range labelIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO task_labels (task_id, label_id) VALUES (?, ?)`, taskID, lid); err != nil {
			return fmt.Errorf("insert task_label: %w", err)
		}
	}
	return tx.Commit()
}

func (r *TaskLabelsRepo) LabelsByTaskIDs(ctx context.Context, taskIDs []int64) (map[int64][]model.Label, error) {
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
		return nil, fmt.Errorf("hydrate task labels: %w", err)
	}
	defer func() { _ = rows.Close() }()

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

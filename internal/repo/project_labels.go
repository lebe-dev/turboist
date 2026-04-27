package repo

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/lebe-dev/turboist/internal/model"
)

type ProjectLabelsRepo struct {
	db *sql.DB
}

func NewProjectLabelsRepo(db *sql.DB) *ProjectLabelsRepo {
	return &ProjectLabelsRepo{db: db}
}

// SetForProject replaces the set of labels attached to projectID. Should be
// called within a transaction by callers that need atomicity with project
// inserts/updates. The standalone form opens its own transaction.
func (r *ProjectLabelsRepo) SetForProject(ctx context.Context, projectID int64, labelIDs []int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM project_labels WHERE project_id = ?`, projectID); err != nil {
		return fmt.Errorf("clear project_labels: %w", err)
	}
	for _, lid := range labelIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO project_labels (project_id, label_id) VALUES (?, ?)`, projectID, lid); err != nil {
			return fmt.Errorf("insert project_label: %w", err)
		}
	}
	return tx.Commit()
}

// LabelsByProjectIDs returns labels grouped by project_id for the given
// projectIDs. Used to hydrate listings without GROUP_CONCAT.
func (r *ProjectLabelsRepo) LabelsByProjectIDs(ctx context.Context, projectIDs []int64) (map[int64][]model.Label, error) {
	if len(projectIDs) == 0 {
		return map[int64][]model.Label{}, nil
	}
	placeholders := make([]string, len(projectIDs))
	args := make([]any, len(projectIDs))
	for i, id := range projectIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	q := `SELECT pl.project_id, l.id, l.name, l.color, l.is_favourite, l.created_at, l.updated_at
	      FROM project_labels pl
	      JOIN labels l ON l.id = pl.label_id
	      WHERE pl.project_id IN (` + strings.Join(placeholders, ",") + `)
	      ORDER BY l.name ASC`
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("hydrate project labels: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[int64][]model.Label, len(projectIDs))
	for rows.Next() {
		var projectID int64
		var l model.Label
		var fav int
		var createdAt, updatedAt string
		if err := rows.Scan(&projectID, &l.ID, &l.Name, &l.Color, &fav, &createdAt, &updatedAt); err != nil {
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
		out[projectID] = append(out[projectID], l)
	}
	return out, rows.Err()
}

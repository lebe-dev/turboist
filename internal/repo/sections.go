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

type ProjectSectionRepo struct {
	db *sql.DB
}

func NewProjectSectionRepo(db *sql.DB) *ProjectSectionRepo {
	return &ProjectSectionRepo{db: db}
}

func scanSection(row interface{ Scan(...any) error }) (*model.ProjectSection, error) {
	var s model.ProjectSection
	var createdAt, updatedAt string
	if err := row.Scan(&s.ID, &s.ProjectID, &s.Title, &s.Position, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	t, err := model.ParseUTC(createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	s.CreatedAt = t
	t, err = model.ParseUTC(updatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}
	s.UpdatedAt = t
	return &s, nil
}

func (r *ProjectSectionRepo) Create(ctx context.Context, projectID int64, title string) (*model.ProjectSection, error) {
	const op = "repo.project_sections.Create"
	logQuery(ctx, op, projectID, title)
	now := model.FormatUTC(time.Now())
	var nextPos int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(position), -1) + 1 FROM project_sections WHERE project_id = ?`,
		projectID).Scan(&nextPos); err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("next section position: %w", err))
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO project_sections (project_id, title, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		projectID, title, nextPos, now, now)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("insert section: %w", err))
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, id)
}

func (r *ProjectSectionRepo) Get(ctx context.Context, id int64) (*model.ProjectSection, error) {
	const op = "repo.project_sections.Get"
	logQuery(ctx, op, id)
	row := r.db.QueryRowContext(ctx,
		`SELECT id, project_id, title, position, created_at, updated_at FROM project_sections WHERE id = ?`, id)
	s, err := scanSection(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, logErr(ctx, op, ErrNotFound)
	}
	if err != nil {
		return nil, logErr(ctx, op, err)
	}
	return s, nil
}

func (r *ProjectSectionRepo) ListByProject(ctx context.Context, projectID int64, page Page) ([]model.ProjectSection, int, error) {
	const op = "repo.project_sections.ListByProject"
	logQuery(ctx, op, projectID, page)
	page = page.Normalize()
	var total int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM project_sections WHERE project_id = ?`, projectID).Scan(&total); err != nil {
		return nil, 0, logErr(ctx, op, fmt.Errorf("count sections: %w", err))
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, project_id, title, position, created_at, updated_at FROM project_sections
		 WHERE project_id = ? ORDER BY position ASC, id ASC LIMIT ? OFFSET ?`,
		projectID, page.Limit, page.Offset)
	if err != nil {
		return nil, 0, logErr(ctx, op, fmt.Errorf("list sections: %w", err))
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	out := make([]model.ProjectSection, 0)
	for rows.Next() {
		s, err := scanSection(rows)
		if err != nil {
			return nil, 0, logErr(ctx, op, err)
		}
		out = append(out, *s)
	}
	return out, total, rows.Err()
}

type SectionUpdate struct {
	Title *string
}

func (r *ProjectSectionRepo) Update(ctx context.Context, id int64, u SectionUpdate) (*model.ProjectSection, error) {
	const op = "repo.project_sections.Update"
	logQuery(ctx, op, id)
	if u.Title == nil {
		return r.Get(ctx, id)
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE project_sections SET title = ?, updated_at = ? WHERE id = ?`,
		*u.Title, model.FormatUTC(time.Now()), id)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("update section: %w", err))
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

// Reorder moves the section to newPos within its project. The whole project's
// sections are renormalised to contiguous positions [0..N-1] in the new order.
// newPos is clamped to [0, N-1].
func (r *ProjectSectionRepo) Reorder(ctx context.Context, id int64, newPos int) (*model.ProjectSection, error) {
	const op = "repo.project_sections.Reorder"
	logQuery(ctx, op, id, newPos)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("begin reorder tx: %w", err))
	}
	defer func() { _ = tx.Rollback() }()

	var projectID int64
	if err := tx.QueryRowContext(ctx,
		`SELECT project_id FROM project_sections WHERE id = ?`, id).Scan(&projectID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, logErr(ctx, op, ErrNotFound)
		}
		return nil, logErr(ctx, op, fmt.Errorf("load section: %w", err))
	}

	rows, err := tx.QueryContext(ctx,
		`SELECT id FROM project_sections WHERE project_id = ?
		 ORDER BY position ASC, id ASC`, projectID)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("load siblings: %w", err))
	}
	ids := make([]int64, 0)
	for rows.Next() {
		var sid int64
		if err := rows.Scan(&sid); err != nil {
			logging.LogClose(ctx, op+".rows", rows)
			return nil, logErr(ctx, op, fmt.Errorf("scan sibling: %w", err))
		}
		ids = append(ids, sid)
	}
	if err := rows.Close(); err != nil {
		return nil, logErr(ctx, op, err)
	}

	if newPos < 0 {
		newPos = 0
	}
	if newPos >= len(ids) {
		newPos = len(ids) - 1
	}

	// Remove id from its current slot, insert at newPos.
	out := make([]int64, 0, len(ids))
	for _, sid := range ids {
		if sid != id {
			out = append(out, sid)
		}
	}
	if newPos > len(out) {
		newPos = len(out)
	}
	out = append(out[:newPos], append([]int64{id}, out[newPos:]...)...)

	now := model.FormatUTC(time.Now())
	for i, sid := range out {
		if _, err := tx.ExecContext(ctx,
			`UPDATE project_sections SET position = ?, updated_at = ? WHERE id = ?`,
			i, now, sid); err != nil {
			return nil, logErr(ctx, op, fmt.Errorf("renumber section %d: %w", sid, err))
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("commit reorder: %w", err))
	}
	return r.Get(ctx, id)
}

func (r *ProjectSectionRepo) Delete(ctx context.Context, id int64) error {
	const op = "repo.project_sections.Delete"
	logQuery(ctx, op, id)
	res, err := r.db.ExecContext(ctx, `DELETE FROM project_sections WHERE id = ?`, id)
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("delete section: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return logErr(ctx, op, err)
	}
	if n == 0 {
		return logErr(ctx, op, ErrNotFound)
	}
	return nil
}

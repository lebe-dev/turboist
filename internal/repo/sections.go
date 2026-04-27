package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

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
	if err := row.Scan(&s.ID, &s.ProjectID, &s.Title, &createdAt, &updatedAt); err != nil {
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
	now := model.FormatUTC(time.Now())
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO project_sections (project_id, title, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		projectID, title, now, now)
	if err != nil {
		return nil, fmt.Errorf("insert section: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, id)
}

func (r *ProjectSectionRepo) Get(ctx context.Context, id int64) (*model.ProjectSection, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, project_id, title, created_at, updated_at FROM project_sections WHERE id = ?`, id)
	s, err := scanSection(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (r *ProjectSectionRepo) ListByProject(ctx context.Context, projectID int64, page Page) ([]model.ProjectSection, int, error) {
	page = page.Normalize()
	var total int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM project_sections WHERE project_id = ?`, projectID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count sections: %w", err)
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, project_id, title, created_at, updated_at FROM project_sections
		 WHERE project_id = ? ORDER BY created_at ASC LIMIT ? OFFSET ?`,
		projectID, page.Limit, page.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list sections: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]model.ProjectSection, 0)
	for rows.Next() {
		s, err := scanSection(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *s)
	}
	return out, total, rows.Err()
}

type SectionUpdate struct {
	Title *string
}

func (r *ProjectSectionRepo) Update(ctx context.Context, id int64, u SectionUpdate) (*model.ProjectSection, error) {
	if u.Title == nil {
		return r.Get(ctx, id)
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE project_sections SET title = ?, updated_at = ? WHERE id = ?`,
		*u.Title, model.FormatUTC(time.Now()), id)
	if err != nil {
		return nil, fmt.Errorf("update section: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, ErrNotFound
	}
	return r.Get(ctx, id)
}

func (r *ProjectSectionRepo) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM project_sections WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete section: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

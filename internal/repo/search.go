package repo

import (
	"context"
	"fmt"
	"strings"

	"github.com/lebe-dev/turboist/internal/model"
)

type SearchRepo struct {
	tasks    *TaskRepo
	projects *ProjectRepo
}

func NewSearchRepo(tasks *TaskRepo, projects *ProjectRepo) *SearchRepo {
	return &SearchRepo{tasks: tasks, projects: projects}
}

// SearchTasks runs a LIKE search over title/description. The caller must
// validate q (min 2 chars) — repo treats empty q as no-op.
func (r *SearchRepo) SearchTasks(ctx context.Context, q string, page Page) ([]model.Task, int, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return []model.Task{}, 0, nil
	}
	page = page.Normalize()
	like := "%" + q + "%"
	args := []any{like, like}

	var total int
	if err := r.tasks.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tasks t WHERE t.title LIKE ? OR t.description LIKE ?`, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count search tasks: %w", err)
	}
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, page.Limit, page.Offset)
	rows, err := r.tasks.db.QueryContext(ctx,
		`SELECT `+taskColumns+
			` FROM tasks t WHERE t.title LIKE ? OR t.description LIKE ?
			  ORDER BY `+taskOrderBy+` LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("search tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]model.Task, 0)
	ids := make([]int64, 0)
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *t)
		ids = append(ids, t.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if r.tasks.labels != nil && len(ids) > 0 {
		hydrated, err := r.tasks.labels.LabelsByTaskIDs(ctx, ids)
		if err != nil {
			return nil, 0, err
		}
		for i := range out {
			out[i].Labels = hydrated[out[i].ID]
		}
	}
	return out, total, nil
}

func (r *SearchRepo) SearchProjects(ctx context.Context, q string, page Page) ([]model.Project, int, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return []model.Project{}, 0, nil
	}
	page = page.Normalize()
	like := "%" + q + "%"

	var total int
	if err := r.projects.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM projects WHERE title LIKE ? OR description LIKE ?`, like, like).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count search projects: %w", err)
	}
	rows, err := r.projects.db.QueryContext(ctx,
		`SELECT `+projectColumns+` FROM projects
		 WHERE title LIKE ? OR description LIKE ?
		 ORDER BY is_pinned DESC, pinned_at DESC, created_at DESC LIMIT ? OFFSET ?`,
		like, like, page.Limit, page.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("search projects: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]model.Project, 0)
	ids := make([]int64, 0)
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *p)
		ids = append(ids, p.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if r.projects.labels != nil && len(ids) > 0 {
		hydrated, err := r.projects.labels.LabelsByProjectIDs(ctx, ids)
		if err != nil {
			return nil, 0, err
		}
		for i := range out {
			out[i].Labels = hydrated[out[i].ID]
		}
	}
	return out, total, nil
}

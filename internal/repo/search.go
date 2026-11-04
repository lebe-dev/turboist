package repo

import (
	"context"
	"fmt"
	"strings"

	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
)

type SearchRepo struct {
	tasks    *TaskRepo
	projects *ProjectRepo
}

func NewSearchRepo(tasks *TaskRepo, projects *ProjectRepo) *SearchRepo {
	return &SearchRepo{tasks: tasks, projects: projects}
}

// Search matches a substring of the title, of a label name or of the
// description, compared through the casefold SQL function so the match ignores
// case in every script (SQLite's LIKE only folds ASCII). Results are ranked by
// where the words hit — title first, then a label, then the description — then
// open work ahead of finished work, then the shared list sort.

// searchPattern turns a query into a LIKE pattern for casefold(column).
func searchPattern(q string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(db.Casefold(q))
	return "%" + escaped + "%"
}

const (
	taskTitleHit       = `casefold(t.title) LIKE ? ESCAPE '\'`
	taskDescriptionHit = `casefold(t.description) LIKE ? ESCAPE '\'`
	taskLabelHit       = `EXISTS (SELECT 1 FROM task_labels tl JOIN labels l ON l.id = tl.label_id
		WHERE tl.task_id = t.id AND casefold(l.name) LIKE ? ESCAPE '\')`
	taskSearchWhere = `(` + taskTitleHit + ` OR ` + taskLabelHit + ` OR ` + taskDescriptionHit + `)`
	taskSearchRank  = `CASE WHEN ` + taskTitleHit + ` THEN 0 WHEN ` + taskLabelHit + ` THEN 1 ELSE 2 END,
		(t.status = 'open') DESC`

	projectTitleHit       = `casefold(p.title) LIKE ? ESCAPE '\'`
	projectDescriptionHit = `casefold(p.description) LIKE ? ESCAPE '\'`
	projectLabelHit       = `EXISTS (SELECT 1 FROM project_labels pl JOIN labels l ON l.id = pl.label_id
		WHERE pl.project_id = p.id AND casefold(l.name) LIKE ? ESCAPE '\')`
	projectSearchWhere = `(` + projectTitleHit + ` OR ` + projectLabelHit + ` OR ` + projectDescriptionHit + `)`
	projectSearchRank  = `CASE WHEN ` + projectTitleHit + ` THEN 0 WHEN ` + projectLabelHit + ` THEN 1 ELSE 2 END,
		(p.status = 'open') DESC`
)

// SearchTasks searches tasks (see Search above). The caller must validate q
// (min 2 chars) — repo treats empty q as no-op.
func (r *SearchRepo) SearchTasks(ctx context.Context, q string, page Page) ([]model.Task, int, error) {
	const op = "repo.search.SearchTasks"
	logQuery(ctx, op, len(q), page)
	q = strings.TrimSpace(q)
	if q == "" {
		return []model.Task{}, 0, nil
	}
	page = page.Normalize()
	like := searchPattern(q)

	var total int
	if err := r.tasks.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tasks t WHERE `+taskSearchWhere, like, like, like).Scan(&total); err != nil {
		return nil, 0, logErr(ctx, op, fmt.Errorf("count search tasks: %w", err))
	}
	rows, err := r.tasks.db.QueryContext(ctx,
		`SELECT `+taskColumns+
			` FROM tasks t WHERE `+taskSearchWhere+`
			  ORDER BY `+taskSearchRank+`, `+taskOrderBy+` LIMIT ? OFFSET ?`,
		like, like, like, like, like, page.Limit, page.Offset)
	if err != nil {
		return nil, 0, logErr(ctx, op, fmt.Errorf("search tasks: %w", err))
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	out := make([]model.Task, 0)
	ids := make([]int64, 0)
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, 0, logErr(ctx, op, err)
		}
		out = append(out, *t)
		ids = append(ids, t.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, logErr(ctx, op, err)
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
	const op = "repo.search.SearchProjects"
	logQuery(ctx, op, len(q), page)
	q = strings.TrimSpace(q)
	if q == "" {
		return []model.Project{}, 0, nil
	}
	page = page.Normalize()
	like := searchPattern(q)

	var total int
	if err := r.projects.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM projects p WHERE `+projectSearchWhere, like, like, like).Scan(&total); err != nil {
		return nil, 0, logErr(ctx, op, fmt.Errorf("count search projects: %w", err))
	}
	rows, err := r.projects.db.QueryContext(ctx,
		`SELECT `+projectColumns+` FROM projects p
		 WHERE `+projectSearchWhere+`
		 ORDER BY `+projectSearchRank+`, p.is_pinned DESC, p.pinned_at DESC, p.created_at DESC LIMIT ? OFFSET ?`,
		like, like, like, like, like, page.Limit, page.Offset)
	if err != nil {
		return nil, 0, logErr(ctx, op, fmt.Errorf("search projects: %w", err))
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	out := make([]model.Project, 0)
	ids := make([]int64, 0)
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, 0, logErr(ctx, op, err)
		}
		out = append(out, *p)
		ids = append(ids, p.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, logErr(ctx, op, err)
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

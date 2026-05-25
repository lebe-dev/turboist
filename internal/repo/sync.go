package repo

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
)

// SyncRepo aggregates listings of syncable entities for the /sync/pull endpoint.
//
// Two modes:
//   - Initial pull (since == nil): alive entities only. For tasks: open + recently
//     completed (within completedCutoff). For other entities: deleted_at IS NULL.
//   - Incremental pull (since != nil): ALL rows with updated_at > since,
//     including tombstones (deleted_at != NULL). The client uses tombstones to
//     drop locally-cached rows.
type SyncRepo struct {
	db            *sql.DB
	taskLabels    *TaskLabelsRepo
	projectLabels *ProjectLabelsRepo
}

func NewSyncRepo(db *sql.DB, taskLabels *TaskLabelsRepo, projectLabels *ProjectLabelsRepo) *SyncRepo {
	return &SyncRepo{db: db, taskLabels: taskLabels, projectLabels: projectLabels}
}

func (r *SyncRepo) Tasks(ctx context.Context, since *time.Time, completedCutoff time.Time) ([]model.Task, error) {
	const op = "repo.sync.Tasks"
	logQuery(ctx, op, since)
	var (
		rows *sql.Rows
		err  error
	)
	if since != nil {
		rows, err = r.db.QueryContext(ctx,
			`SELECT `+taskColumns+` FROM tasks WHERE updated_at > ? ORDER BY updated_at ASC`,
			model.FormatUTC(*since))
	} else {
		rows, err = r.db.QueryContext(ctx,
			`SELECT `+taskColumns+` FROM tasks
			 WHERE deleted_at IS NULL
			   AND (status = 'open' OR (status = 'completed' AND completed_at >= ?))
			 ORDER BY updated_at ASC`,
			model.FormatUTC(completedCutoff))
	}
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("query tasks: %w", err))
	}
	defer logging.LogClose(ctx, op+".rows", rows)
	out := make([]model.Task, 0)
	ids := make([]int64, 0)
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, logErr(ctx, op, err)
		}
		out = append(out, *t)
		ids = append(ids, t.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, logErr(ctx, op, err)
	}
	if r.taskLabels != nil && len(ids) > 0 {
		hydrated, err := r.taskLabels.LabelsByTaskIDs(ctx, ids)
		if err != nil {
			return nil, logErr(ctx, op, err)
		}
		for i := range out {
			out[i].Labels = hydrated[out[i].ID]
		}
	}
	return out, nil
}

func (r *SyncRepo) Projects(ctx context.Context, since *time.Time) ([]model.Project, error) {
	const op = "repo.sync.Projects"
	logQuery(ctx, op, since)
	var (
		rows *sql.Rows
		err  error
	)
	if since != nil {
		rows, err = r.db.QueryContext(ctx,
			`SELECT `+projectColumns+` FROM projects WHERE updated_at > ? ORDER BY updated_at ASC`,
			model.FormatUTC(*since))
	} else {
		rows, err = r.db.QueryContext(ctx,
			`SELECT `+projectColumns+` FROM projects WHERE deleted_at IS NULL ORDER BY updated_at ASC`)
	}
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("query projects: %w", err))
	}
	defer logging.LogClose(ctx, op+".rows", rows)
	out := make([]model.Project, 0)
	ids := make([]int64, 0)
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, logErr(ctx, op, err)
		}
		out = append(out, *p)
		ids = append(ids, p.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, logErr(ctx, op, err)
	}
	if r.projectLabels != nil && len(ids) > 0 {
		hydrated, err := r.projectLabels.LabelsByProjectIDs(ctx, ids)
		if err != nil {
			return nil, logErr(ctx, op, err)
		}
		for i := range out {
			out[i].Labels = hydrated[out[i].ID]
		}
	}
	return out, nil
}

func (r *SyncRepo) Sections(ctx context.Context, since *time.Time) ([]model.ProjectSection, error) {
	const op = "repo.sync.Sections"
	logQuery(ctx, op, since)
	var (
		rows *sql.Rows
		err  error
	)
	if since != nil {
		rows, err = r.db.QueryContext(ctx,
			`SELECT `+sectionColumns+` FROM project_sections WHERE updated_at > ? ORDER BY updated_at ASC`,
			model.FormatUTC(*since))
	} else {
		rows, err = r.db.QueryContext(ctx,
			`SELECT `+sectionColumns+` FROM project_sections WHERE deleted_at IS NULL ORDER BY updated_at ASC`)
	}
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("query sections: %w", err))
	}
	defer logging.LogClose(ctx, op+".rows", rows)
	out := make([]model.ProjectSection, 0)
	for rows.Next() {
		s, err := scanSection(rows)
		if err != nil {
			return nil, logErr(ctx, op, err)
		}
		out = append(out, *s)
	}
	if err := rows.Err(); err != nil {
		return nil, logErr(ctx, op, err)
	}
	return out, nil
}

func (r *SyncRepo) Labels(ctx context.Context, since *time.Time) ([]model.Label, error) {
	const op = "repo.sync.Labels"
	logQuery(ctx, op, since)
	var (
		rows *sql.Rows
		err  error
	)
	if since != nil {
		rows, err = r.db.QueryContext(ctx,
			`SELECT `+labelColumns+` FROM labels WHERE updated_at > ? ORDER BY updated_at ASC`,
			model.FormatUTC(*since))
	} else {
		rows, err = r.db.QueryContext(ctx,
			`SELECT `+labelColumns+` FROM labels WHERE deleted_at IS NULL ORDER BY updated_at ASC`)
	}
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("query labels: %w", err))
	}
	defer logging.LogClose(ctx, op+".rows", rows)
	out := make([]model.Label, 0)
	for rows.Next() {
		l, err := scanLabel(rows)
		if err != nil {
			return nil, logErr(ctx, op, err)
		}
		out = append(out, *l)
	}
	if err := rows.Err(); err != nil {
		return nil, logErr(ctx, op, err)
	}
	return out, nil
}

func (r *SyncRepo) Contexts(ctx context.Context, since *time.Time) ([]model.Context, error) {
	const op = "repo.sync.Contexts"
	logQuery(ctx, op, since)
	var (
		rows *sql.Rows
		err  error
	)
	if since != nil {
		rows, err = r.db.QueryContext(ctx,
			`SELECT `+contextColumns+` FROM contexts WHERE updated_at > ? ORDER BY updated_at ASC`,
			model.FormatUTC(*since))
	} else {
		rows, err = r.db.QueryContext(ctx,
			`SELECT `+contextColumns+` FROM contexts WHERE deleted_at IS NULL ORDER BY updated_at ASC`)
	}
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("query contexts: %w", err))
	}
	defer logging.LogClose(ctx, op+".rows", rows)
	out := make([]model.Context, 0)
	for rows.Next() {
		c, err := scanContext(rows)
		if err != nil {
			return nil, logErr(ctx, op, err)
		}
		out = append(out, *c)
	}
	if err := rows.Err(); err != nil {
		return nil, logErr(ctx, op, err)
	}
	return out, nil
}

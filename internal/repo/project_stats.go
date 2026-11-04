package repo

import (
	"context"
	"fmt"
	"strings"

	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
)

// ProjectActivityPeriod holds one window's counters for a single project.
// Completed counts the project's tasks (subtasks included) finished inside the
// window; PreviousCompleted is the same count over the equally long window
// right before it.
type ProjectActivityPeriod struct {
	Completed         int
	PreviousCompleted int
}

// ProjectActivity is the per-project aggregate returned by ActivityStats.
// Periods is positionally parallel to the ranges passed in.
type ProjectActivity struct {
	ProjectID int64
	Periods   []ProjectActivityPeriod
}

// ActivityStats counts completed tasks per project for every range and for the
// equally long window before it, in one pass over tasks.
//
// Only projects with at least one completion in the widest window pair are
// returned: the caller joins the result to the project list it already holds,
// and a project that did nothing contributes nothing to a chart. Cancelled
// tasks do not count — abandoning work is not finishing it.
func (r *ProjectRepo) ActivityStats(ctx context.Context, ranges []LabelUsageRange) ([]ProjectActivity, error) {
	const op = "repo.projects.ActivityStats"
	logQuery(ctx, op, len(ranges))
	if len(ranges) == 0 {
		return []ProjectActivity{}, nil
	}

	var sb strings.Builder
	var args []any
	sb.WriteString(`SELECT project_id`)
	earliest := ranges[0].Start.Add(-ranges[0].End.Sub(ranges[0].Start))
	for _, rng := range ranges {
		start := model.FormatUTC(rng.Start)
		end := model.FormatUTC(rng.End)
		prevStart := rng.Start.Add(-rng.End.Sub(rng.Start))
		if prevStart.Before(earliest) {
			earliest = prevStart
		}
		sb.WriteString(`,
	       COALESCE(SUM(CASE WHEN completed_at >= ? AND completed_at < ? THEN 1 ELSE 0 END), 0),
	       COALESCE(SUM(CASE WHEN completed_at >= ? AND completed_at < ? THEN 1 ELSE 0 END), 0)`)
		args = append(args, start, end, model.FormatUTC(prevStart), start)
	}
	sb.WriteString(`
	  FROM tasks
	 WHERE project_id IS NOT NULL
	   AND status = ?
	   AND completed_at IS NOT NULL
	   AND completed_at >= ?
	 GROUP BY project_id
	 ORDER BY project_id ASC`)
	args = append(args, string(model.TaskStatusCompleted), model.FormatUTC(earliest))

	rows, err := r.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("query project activity: %w", err))
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	out := make([]ProjectActivity, 0)
	for rows.Next() {
		a := ProjectActivity{Periods: make([]ProjectActivityPeriod, len(ranges))}
		dest := []any{&a.ProjectID}
		for i := range a.Periods {
			dest = append(dest, &a.Periods[i].Completed, &a.Periods[i].PreviousCompleted)
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, logErr(ctx, op, err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

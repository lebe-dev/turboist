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

// LabelUsageRange is one half-open [Start, End) window the usage stats are
// bucketed by. The comparison window that produces the trend arrow is derived
// from it — the equally long span that ends where this one starts.
type LabelUsageRange struct {
	Start time.Time
	End   time.Time
}

// LabelUsagePeriod holds one window's counters for a single label.
//
// Applied counts tagging events (task_labels rows created in the window), which
// is the "how often do I reach for this label" signal. Completed counts tasks
// carrying the label that were completed in the window — the two are
// deliberately independent: a task tagged months ago but finished this week
// contributes only to Completed.
type LabelUsagePeriod struct {
	Applied         int
	PreviousApplied int
	Completed       int
}

// LabelUsage is the per-label aggregate returned by UsageStats. Periods is
// positionally parallel to the ranges passed in.
type LabelUsage struct {
	Label      model.Label
	TotalTasks int
	OpenTasks  int
	Overdue    int
	// Projects is the number of distinct projects the label appears in. Inbox
	// tasks (NULL project_id) do not count.
	Projects   int
	LastUsedAt *time.Time
	Periods    []LabelUsagePeriod
}

// UsageStats aggregates label activity in one pass over task_labels.
//
// Every label is returned, including never-used ones — the stats page needs the
// zeros to offer a cleanup list, and the frontend sorts/filters client-side so
// switching the displayed period costs no round-trip.
//
// todayStart is the start of the current day (in the server timezone) used for
// the overdue check. Times are compared as the ISO-8601 UTC strings stored in
// SQLite, where lexicographic order equals chronological order.
func (r *LabelRepo) UsageStats(ctx context.Context, ranges []LabelUsageRange, todayStart time.Time, limit int) ([]LabelUsage, error) {
	const op = "repo.labels.UsageStats"
	logQuery(ctx, op, len(ranges), limit)

	var sb strings.Builder
	sb.WriteString(`SELECT l.id, l.name, l.color, l.is_favourite, l.is_private, l.created_at, l.updated_at,
	       COUNT(tl.task_id),
	       COALESCE(SUM(CASE WHEN t.status = 'open' THEN 1 ELSE 0 END), 0),
	       COALESCE(SUM(CASE WHEN t.status = 'open' AND t.due_at IS NOT NULL AND t.due_at < ? THEN 1 ELSE 0 END), 0),
	       COUNT(DISTINCT t.project_id),
	       MAX(tl.created_at)`)
	args := []any{model.FormatUTC(todayStart)}

	for _, rng := range ranges {
		start := model.FormatUTC(rng.Start)
		end := model.FormatUTC(rng.End)
		prevStart := model.FormatUTC(rng.Start.Add(-rng.End.Sub(rng.Start)))
		sb.WriteString(`,
	       COALESCE(SUM(CASE WHEN tl.created_at >= ? AND tl.created_at < ? THEN 1 ELSE 0 END), 0),
	       COALESCE(SUM(CASE WHEN tl.created_at >= ? AND tl.created_at < ? THEN 1 ELSE 0 END), 0),
	       COALESCE(SUM(CASE WHEN t.completed_at IS NOT NULL AND t.completed_at >= ? AND t.completed_at < ? THEN 1 ELSE 0 END), 0)`)
		args = append(args, start, end, prevStart, start, start, end)
	}

	sb.WriteString(`
	  FROM labels l
	  LEFT JOIN task_labels tl ON tl.label_id = l.id
	  LEFT JOIN tasks t ON t.id = tl.task_id
	 GROUP BY l.id
	 ORDER BY l.name ASC
	 LIMIT ?`)
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("query label usage: %w", err))
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	out := make([]LabelUsage, 0)
	for rows.Next() {
		u, err := scanLabelUsage(rows, len(ranges))
		if err != nil {
			return nil, logErr(ctx, op, err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func scanLabelUsage(row interface{ Scan(...any) error }, periods int) (LabelUsage, error) {
	var (
		u                    LabelUsage
		fav, priv            int
		createdAt, updatedAt string
		lastUsedAt           sql.NullString
	)
	dest := []any{
		&u.Label.ID, &u.Label.Name, &u.Label.Color, &fav, &priv, &createdAt, &updatedAt,
		&u.TotalTasks, &u.OpenTasks, &u.Overdue, &u.Projects, &lastUsedAt,
	}
	u.Periods = make([]LabelUsagePeriod, periods)
	for i := range u.Periods {
		dest = append(dest, &u.Periods[i].Applied, &u.Periods[i].PreviousApplied, &u.Periods[i].Completed)
	}
	if err := row.Scan(dest...); err != nil {
		return LabelUsage{}, err
	}

	u.Label.IsFavourite = fav == 1
	u.Label.IsPrivate = priv == 1
	t, err := model.ParseUTC(createdAt)
	if err != nil {
		return LabelUsage{}, fmt.Errorf("parse created_at: %w", err)
	}
	u.Label.CreatedAt = t
	t, err = model.ParseUTC(updatedAt)
	if err != nil {
		return LabelUsage{}, fmt.Errorf("parse updated_at: %w", err)
	}
	u.Label.UpdatedAt = t
	if lastUsedAt.Valid {
		t, err := model.ParseUTC(lastUsedAt.String)
		if err != nil {
			return LabelUsage{}, fmt.Errorf("parse last_used_at: %w", err)
		}
		u.LastUsedAt = &t
	}
	return u, nil
}

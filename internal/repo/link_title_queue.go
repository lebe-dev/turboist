package repo

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
)

// LinkTitleJob is one task waiting for the link-title worker.
type LinkTitleJob struct {
	TaskID        int64
	URL           string
	Attempts      int
	NextAttemptAt time.Time
	LastError     *string
	CreatedAt     time.Time
}

// LinkTitleQueueRepo stores the tasks whose bare-URL title still waits to be
// replaced by the page's own. Server-side bookkeeping: the table carries no
// change_log triggers, a replica sees the outcome through the task row.
type LinkTitleQueueRepo struct {
	db *sql.DB
}

func NewLinkTitleQueueRepo(db *sql.DB) *LinkTitleQueueRepo {
	return &LinkTitleQueueRepo{db: db}
}

// Enqueue schedules a task for its first attempt at now. A task already queued
// keeps its row: the job is the same, and resetting attempts would let a
// re-created duplicate dodge the retry budget.
func (r *LinkTitleQueueRepo) Enqueue(ctx context.Context, taskID int64, url string, now time.Time) error {
	const op = "repo.link_title_queue.Enqueue"
	logQuery(ctx, op, taskID)
	ts := model.FormatUTC(now)
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO link_title_queue (task_id, url, attempts, next_attempt_at, created_at)
		 VALUES (?, ?, 0, ?, ?)
		 ON CONFLICT(task_id) DO NOTHING`, taskID, url, ts, ts)
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("enqueue link title: %w", err))
	}
	return nil
}

// Due returns up to limit jobs whose next attempt has come, oldest first.
func (r *LinkTitleQueueRepo) Due(ctx context.Context, now time.Time, limit int) ([]LinkTitleJob, error) {
	const op = "repo.link_title_queue.Due"
	logQuery(ctx, op, limit)
	rows, err := r.db.QueryContext(ctx,
		`SELECT task_id, url, attempts, next_attempt_at, last_error, created_at
		   FROM link_title_queue
		  WHERE next_attempt_at <= ?
		  ORDER BY next_attempt_at ASC, task_id ASC
		  LIMIT ?`, model.FormatUTC(now), limit)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("list due link titles: %w", err))
	}
	defer logging.LogClose(ctx, op+".rows", rows)
	var out []LinkTitleJob
	for rows.Next() {
		var (
			j            LinkTitleJob
			next, create string
		)
		if err := rows.Scan(&j.TaskID, &j.URL, &j.Attempts, &next, &j.LastError, &create); err != nil {
			return nil, logErr(ctx, op, fmt.Errorf("scan link title job: %w", err))
		}
		if j.NextAttemptAt, err = model.ParseUTC(next); err != nil {
			return nil, logErr(ctx, op, fmt.Errorf("parse next_attempt_at: %w", err))
		}
		if j.CreatedAt, err = model.ParseUTC(create); err != nil {
			return nil, logErr(ctx, op, fmt.Errorf("parse created_at: %w", err))
		}
		out = append(out, j)
	}
	if err := rows.Err(); err != nil {
		return nil, logErr(ctx, op, err)
	}
	return out, nil
}

// NextDue is the earliest next_attempt_at in the queue, or nil when it is empty.
func (r *LinkTitleQueueRepo) NextDue(ctx context.Context) (*time.Time, error) {
	const op = "repo.link_title_queue.NextDue"
	logQuery(ctx, op)
	var raw sql.NullString
	if err := r.db.QueryRowContext(ctx, `SELECT MIN(next_attempt_at) FROM link_title_queue`).Scan(&raw); err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("next due link title: %w", err))
	}
	if !raw.Valid {
		return nil, nil
	}
	t, err := model.ParseUTC(raw.String)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("parse next_attempt_at: %w", err))
	}
	return &t, nil
}

// Reschedule records a failed attempt and when to try again.
func (r *LinkTitleQueueRepo) Reschedule(ctx context.Context, taskID int64, attempts int, next time.Time, lastErr string) error {
	const op = "repo.link_title_queue.Reschedule"
	logQuery(ctx, op, taskID, attempts)
	_, err := r.db.ExecContext(ctx,
		`UPDATE link_title_queue SET attempts = ?, next_attempt_at = ?, last_error = ? WHERE task_id = ?`,
		attempts, model.FormatUTC(next), lastErr, taskID)
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("reschedule link title: %w", err))
	}
	return nil
}

// Delete removes a job: the task was rewritten, given up on, or changed.
func (r *LinkTitleQueueRepo) Delete(ctx context.Context, taskID int64) error {
	const op = "repo.link_title_queue.Delete"
	logQuery(ctx, op, taskID)
	if _, err := r.db.ExecContext(ctx, `DELETE FROM link_title_queue WHERE task_id = ?`, taskID); err != nil {
		return logErr(ctx, op, fmt.Errorf("delete link title job: %w", err))
	}
	return nil
}

// Clear drops every queued job, used when the feature is switched off.
func (r *LinkTitleQueueRepo) Clear(ctx context.Context) error {
	const op = "repo.link_title_queue.Clear"
	logQuery(ctx, op)
	if _, err := r.db.ExecContext(ctx, `DELETE FROM link_title_queue`); err != nil {
		return logErr(ctx, op, fmt.Errorf("clear link title queue: %w", err))
	}
	return nil
}

// queuedLinkTitleTaskIDs is the set of tasks still waiting for a title. The
// Inbox processor leaves them alone: deciding where a bare URL belongs is a
// wasted model call when its real title is a few seconds away.
func queuedLinkTitleTaskIDs(ctx context.Context, db *sql.DB) (map[int64]struct{}, error) {
	const op = "repo.link_title_queue.ids"
	rows, err := db.QueryContext(ctx, `SELECT task_id FROM link_title_queue`)
	if err != nil {
		return nil, fmt.Errorf("list queued link titles: %w", err)
	}
	defer logging.LogClose(ctx, op+".rows", rows)
	out := map[int64]struct{}{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = struct{}{}
	}
	return out, rows.Err()
}

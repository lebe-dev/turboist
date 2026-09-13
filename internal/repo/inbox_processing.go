package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
)

// InboxProcessingRepo stores the LLM Inbox processor's bookkeeping: the
// per-task state of tasks still in the Inbox and the decision journal. Neither
// table is replicated (no change_log triggers) — replicas learn the outcome from
// the task row itself.
type InboxProcessingRepo struct {
	db     *sql.DB
	labels *TaskLabelsRepo
}

func NewInboxProcessingRepo(db *sql.DB, labels *TaskLabelsRepo) *InboxProcessingRepo {
	return &InboxProcessingRepo{db: db, labels: labels}
}

// PendingInboxTask is an open Inbox task the processor should look at, with the
// state row left by an earlier attempt (nil when it was never looked at).
type PendingInboxTask struct {
	Task  model.Task
	State *model.InboxProcessingState
}

// ListPending returns up to limit open Inbox tasks that need a decision, oldest
// first: tasks never looked at, tasks edited since the last decision, and failed
// tasks whose retry time has come. The Inbox is small by design, so the filter
// runs in Go over the whole of it.
func (r *InboxProcessingRepo) ListPending(ctx context.Context, now time.Time, limit int) ([]PendingInboxTask, error) {
	const op = "repo.inbox_processing.ListPending"
	logQuery(ctx, op, limit)
	pending, err := r.pending(ctx, now)
	if err != nil {
		return nil, logErr(ctx, op, err)
	}
	if limit > 0 && len(pending) > limit {
		pending = pending[:limit]
	}
	if r.labels == nil || len(pending) == 0 {
		return pending, nil
	}
	ids := make([]int64, len(pending))
	for i, p := range pending {
		ids[i] = p.Task.ID
	}
	hydrated, err := r.labels.LabelsByTaskIDs(ctx, ids)
	if err != nil {
		return nil, logErr(ctx, op, err)
	}
	for i := range pending {
		pending[i].Task.Labels = hydrated[pending[i].Task.ID]
	}
	return pending, nil
}

// PendingCount is how many Inbox tasks ListPending would eventually hand out.
func (r *InboxProcessingRepo) PendingCount(ctx context.Context, now time.Time) (int, error) {
	const op = "repo.inbox_processing.PendingCount"
	logQuery(ctx, op)
	pending, err := r.pending(ctx, now)
	if err != nil {
		return 0, logErr(ctx, op, err)
	}
	return len(pending), nil
}

func (r *InboxProcessingRepo) pending(ctx context.Context, now time.Time) ([]PendingInboxTask, error) {
	const op = "repo.inbox_processing.pending"
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+taskColumns+` FROM tasks
		 WHERE inbox_id IS NOT NULL AND status = 'open'
		 ORDER BY created_at ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list inbox tasks: %w", err)
	}
	defer logging.LogClose(ctx, op+".rows", rows)
	var tasks []model.Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, *t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		return nil, nil
	}

	states, err := r.inboxStates(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]PendingInboxTask, 0, len(tasks))
	for _, t := range tasks {
		st, ok := states[t.ID]
		if !ok {
			out = append(out, PendingInboxTask{Task: t})
			continue
		}
		if st.Fingerprint != model.InboxFingerprint(t.Title, t.Description) {
			out = append(out, PendingInboxTask{Task: t, State: st})
			continue
		}
		if st.Status == model.InboxStateFailed && st.NextAttemptAt != nil && !st.NextAttemptAt.After(now) {
			out = append(out, PendingInboxTask{Task: t, State: st})
		}
	}
	return out, nil
}

const inboxStateColumns = `task_id, fingerprint, status, attempts, next_attempt_at, last_error, updated_at`

func (r *InboxProcessingRepo) inboxStates(ctx context.Context) (map[int64]*model.InboxProcessingState, error) {
	const op = "repo.inbox_processing.inboxStates"
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+inboxStateColumns+` FROM inbox_processing_state
		 WHERE task_id IN (SELECT id FROM tasks WHERE inbox_id IS NOT NULL AND status = 'open')`)
	if err != nil {
		return nil, fmt.Errorf("list inbox states: %w", err)
	}
	defer logging.LogClose(ctx, op+".rows", rows)
	out := make(map[int64]*model.InboxProcessingState)
	for rows.Next() {
		st, err := scanInboxState(rows)
		if err != nil {
			return nil, err
		}
		out[st.TaskID] = st
	}
	return out, rows.Err()
}

func scanInboxState(row interface{ Scan(...any) error }) (*model.InboxProcessingState, error) {
	var st model.InboxProcessingState
	var status, updatedAt string
	var nextAttemptAt, lastError sql.NullString
	if err := row.Scan(&st.TaskID, &st.Fingerprint, &status, &st.Attempts, &nextAttemptAt, &lastError, &updatedAt); err != nil {
		return nil, err
	}
	st.Status = model.InboxStateStatus(status)
	if nextAttemptAt.Valid {
		ts, err := model.ParseUTC(nextAttemptAt.String)
		if err != nil {
			return nil, fmt.Errorf("parse next_attempt_at: %w", err)
		}
		st.NextAttemptAt = &ts
	}
	if lastError.Valid {
		v := lastError.String
		st.LastError = &v
	}
	ts, err := model.ParseUTC(updatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}
	st.UpdatedAt = ts
	return &st, nil
}

// GetState reads the state row of one task.
func (r *InboxProcessingRepo) GetState(ctx context.Context, taskID int64) (*model.InboxProcessingState, error) {
	const op = "repo.inbox_processing.GetState"
	logQuery(ctx, op, taskID)
	st, err := scanInboxState(r.db.QueryRowContext(ctx,
		`SELECT `+inboxStateColumns+` FROM inbox_processing_state WHERE task_id = ?`, taskID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, logErr(ctx, op, err)
	}
	return st, nil
}

// UpsertState writes the full state row of a task, replacing any earlier one.
func (r *InboxProcessingRepo) UpsertState(ctx context.Context, st model.InboxProcessingState) error {
	const op = "repo.inbox_processing.UpsertState"
	logQuery(ctx, op, st.TaskID, string(st.Status))
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO inbox_processing_state (`+inboxStateColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(task_id) DO UPDATE SET
		   fingerprint = excluded.fingerprint,
		   status = excluded.status,
		   attempts = excluded.attempts,
		   next_attempt_at = excluded.next_attempt_at,
		   last_error = excluded.last_error,
		   updated_at = excluded.updated_at`,
		st.TaskID, st.Fingerprint, string(st.Status), st.Attempts,
		nullTime(st.NextAttemptAt), nullStr(st.LastError), model.FormatUTC(st.UpdatedAt))
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("upsert inbox state: %w", err))
	}
	return nil
}

// DeleteState forgets a task, e.g. once it has been filed out of the Inbox.
func (r *InboxProcessingRepo) DeleteState(ctx context.Context, taskID int64) error {
	const op = "repo.inbox_processing.DeleteState"
	logQuery(ctx, op, taskID)
	if _, err := r.db.ExecContext(ctx, `DELETE FROM inbox_processing_state WHERE task_id = ?`, taskID); err != nil {
		return logErr(ctx, op, fmt.Errorf("delete inbox state: %w", err))
	}
	return nil
}

// PruneStaleStates drops state rows of tasks that are no longer open in the
// Inbox — moved by hand, completed, or filed after a partial failure.
func (r *InboxProcessingRepo) PruneStaleStates(ctx context.Context) (int64, error) {
	const op = "repo.inbox_processing.PruneStaleStates"
	logQuery(ctx, op)
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM inbox_processing_state
		 WHERE task_id NOT IN (SELECT id FROM tasks WHERE inbox_id IS NOT NULL AND status = 'open')`)
	if err != nil {
		return 0, logErr(ctx, op, fmt.Errorf("prune inbox states: %w", err))
	}
	return res.RowsAffected()
}

const inboxLogColumns = `id, task_id, task_title, outcome, model, reason, confidence, before_state, after_state,
	error, prompt_tokens, completion_tokens, reverted_at, created_at`

// AppendLog records one decision and returns its id.
func (r *InboxProcessingRepo) AppendLog(ctx context.Context, e model.InboxProcessingLogEntry) (int64, error) {
	const op = "repo.inbox_processing.AppendLog"
	logQuery(ctx, op, e.TaskID, string(e.Outcome))
	if e.Before.LabelIDs == nil {
		e.Before.LabelIDs = []int64{}
	}
	before, err := json.Marshal(e.Before)
	if err != nil {
		return 0, fmt.Errorf("encode before state: %w", err)
	}
	var after any
	if e.After != nil {
		if e.After.LabelIDs == nil {
			e.After.LabelIDs = []int64{}
		}
		raw, err := json.Marshal(e.After)
		if err != nil {
			return 0, fmt.Errorf("encode after state: %w", err)
		}
		after = string(raw)
	}
	createdAt := e.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO inbox_processing_log (task_id, task_title, outcome, model, reason, confidence, before_state,
			after_state, error, prompt_tokens, completion_tokens, reverted_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?)`,
		nullInt(e.TaskID), e.TaskTitle, string(e.Outcome), e.Model, e.Reason, nullFloat(e.Confidence), string(before),
		after, nullStr(e.Error), nullIntValue(e.PromptTokens), nullIntValue(e.CompletionTokens),
		model.FormatUTC(createdAt))
	if err != nil {
		return 0, logErr(ctx, op, fmt.Errorf("insert inbox log: %w", err))
	}
	return res.LastInsertId()
}

// ListLog returns the journal newest first.
func (r *InboxProcessingRepo) ListLog(ctx context.Context, page Page) ([]model.InboxProcessingLogEntry, int, error) {
	const op = "repo.inbox_processing.ListLog"
	logQuery(ctx, op, page)
	page = page.Normalize()
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM inbox_processing_log`).Scan(&total); err != nil {
		return nil, 0, logErr(ctx, op, fmt.Errorf("count inbox log: %w", err))
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+inboxLogColumns+` FROM inbox_processing_log
		 ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`, page.Limit, page.Offset)
	if err != nil {
		return nil, 0, logErr(ctx, op, fmt.Errorf("list inbox log: %w", err))
	}
	defer logging.LogClose(ctx, op+".rows", rows)
	out := make([]model.InboxProcessingLogEntry, 0)
	for rows.Next() {
		e, err := scanInboxLog(rows)
		if err != nil {
			return nil, 0, logErr(ctx, op, err)
		}
		out = append(out, *e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, logErr(ctx, op, err)
	}
	return out, total, nil
}

// GetLog reads one journal row.
func (r *InboxProcessingRepo) GetLog(ctx context.Context, id int64) (*model.InboxProcessingLogEntry, error) {
	const op = "repo.inbox_processing.GetLog"
	logQuery(ctx, op, id)
	e, err := scanInboxLog(r.db.QueryRowContext(ctx,
		`SELECT `+inboxLogColumns+` FROM inbox_processing_log WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, logErr(ctx, op, err)
	}
	return e, nil
}

// MarkReverted stamps a journal row as undone. A row that is already reverted
// is ErrConflict, a missing one ErrNotFound.
func (r *InboxProcessingRepo) MarkReverted(ctx context.Context, id int64, at time.Time) error {
	const op = "repo.inbox_processing.MarkReverted"
	logQuery(ctx, op, id)
	res, err := r.db.ExecContext(ctx,
		`UPDATE inbox_processing_log SET reverted_at = ? WHERE id = ? AND reverted_at IS NULL`,
		model.FormatUTC(at), id)
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("mark reverted: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return logErr(ctx, op, err)
	}
	if n > 0 {
		return nil
	}
	var exists int
	err = r.db.QueryRowContext(ctx, `SELECT 1 FROM inbox_processing_log WHERE id = ?`, id).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return logErr(ctx, op, err)
	}
	return ErrConflict
}

// PruneLog deletes journal rows created before the cutoff.
func (r *InboxProcessingRepo) PruneLog(ctx context.Context, before time.Time) (int64, error) {
	const op = "repo.inbox_processing.PruneLog"
	logQuery(ctx, op, before)
	res, err := r.db.ExecContext(ctx, `DELETE FROM inbox_processing_log WHERE created_at < ?`, model.FormatUTC(before))
	if err != nil {
		return 0, logErr(ctx, op, fmt.Errorf("prune inbox log: %w", err))
	}
	return res.RowsAffected()
}

func scanInboxLog(row interface{ Scan(...any) error }) (*model.InboxProcessingLogEntry, error) {
	var e model.InboxProcessingLogEntry
	var taskID, promptTokens, completionTokens sql.NullInt64
	var confidence sql.NullFloat64
	var outcome, before, createdAt string
	var after, errText, revertedAt sql.NullString
	if err := row.Scan(&e.ID, &taskID, &e.TaskTitle, &outcome, &e.Model, &e.Reason, &confidence, &before, &after,
		&errText, &promptTokens, &completionTokens, &revertedAt, &createdAt); err != nil {
		return nil, err
	}
	e.Outcome = model.InboxOutcome(outcome)
	if taskID.Valid {
		v := taskID.Int64
		e.TaskID = &v
	}
	if confidence.Valid {
		v := confidence.Float64
		e.Confidence = &v
	}
	if promptTokens.Valid {
		v := int(promptTokens.Int64)
		e.PromptTokens = &v
	}
	if completionTokens.Valid {
		v := int(completionTokens.Int64)
		e.CompletionTokens = &v
	}
	if before != "" {
		if err := json.Unmarshal([]byte(before), &e.Before); err != nil {
			return nil, fmt.Errorf("decode before state: %w", err)
		}
	}
	if e.Before.LabelIDs == nil {
		e.Before.LabelIDs = []int64{}
	}
	if after.Valid {
		var a model.InboxTaskAfter
		if err := json.Unmarshal([]byte(after.String), &a); err != nil {
			return nil, fmt.Errorf("decode after state: %w", err)
		}
		if a.LabelIDs == nil {
			a.LabelIDs = []int64{}
		}
		e.After = &a
	}
	if errText.Valid {
		v := errText.String
		e.Error = &v
	}
	if revertedAt.Valid {
		ts, err := model.ParseUTC(revertedAt.String)
		if err != nil {
			return nil, fmt.Errorf("parse reverted_at: %w", err)
		}
		e.RevertedAt = &ts
	}
	ts, err := model.ParseUTC(createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	e.CreatedAt = ts
	return &e, nil
}

func nullFloat(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

func nullIntValue(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

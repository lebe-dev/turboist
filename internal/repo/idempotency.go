package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

// IdempotencyRecord is the cached response for a previously seen
// (user_id, Idempotency-Key) pair. Replayed verbatim by the middleware so a
// retried mutation never creates a duplicate row.
type IdempotencyRecord struct {
	UserID       int64
	Key          string
	StatusCode   int
	ContentType  string
	ResponseBody []byte
	CreatedAt    time.Time
}

type IdempotencyRepo struct {
	db *sql.DB
}

func NewIdempotencyRepo(db *sql.DB) *IdempotencyRepo {
	return &IdempotencyRepo{db: db}
}

// Get returns the cached record for (userID, key) only when CreatedAt is at or
// after notOlderThan; an expired or missing row yields ErrNotFound so the
// middleware re-runs the handler and overwrites the stale entry.
func (r *IdempotencyRepo) Get(ctx context.Context, userID int64, key string, notOlderThan time.Time) (*IdempotencyRecord, error) {
	const op = "repo.idempotency.Get"
	logQuery(ctx, op, userID, key)
	row := r.db.QueryRowContext(ctx,
		`SELECT user_id, key, status_code, content_type, response_body, created_at
		   FROM idempotency_keys WHERE user_id = ? AND key = ?`,
		userID, key)
	var rec IdempotencyRecord
	var createdAt string
	if err := row.Scan(&rec.UserID, &rec.Key, &rec.StatusCode, &rec.ContentType, &rec.ResponseBody, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, logErr(ctx, op, ErrNotFound)
		}
		return nil, logErr(ctx, op, err)
	}
	t, err := model.ParseUTC(createdAt)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("parse created_at: %w", err))
	}
	if t.Before(notOlderThan) {
		return nil, logErr(ctx, op, ErrNotFound)
	}
	rec.CreatedAt = t
	return &rec, nil
}

// Put writes (or overwrites) the cached response. INSERT OR REPLACE makes the
// middleware safe to call after an expired-row miss: the next successful
// attempt cleanly takes over the old (user_id, key) slot.
func (r *IdempotencyRepo) Put(ctx context.Context, rec IdempotencyRecord) error {
	const op = "repo.idempotency.Put"
	logQuery(ctx, op, rec.UserID, rec.Key)
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now()
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO idempotency_keys
		     (user_id, key, status_code, content_type, response_body, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		rec.UserID, rec.Key, rec.StatusCode, rec.ContentType, rec.ResponseBody, model.FormatUTC(rec.CreatedAt))
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("insert idempotency_keys: %w", err))
	}
	return nil
}

// DeleteExpired removes rows whose created_at is strictly older than
// olderThan. Intended for a periodic cleanup; the middleware already ignores
// expired rows on read so deletion is a housekeeping concern, not a
// correctness one.
func (r *IdempotencyRepo) DeleteExpired(ctx context.Context, olderThan time.Time) (int64, error) {
	const op = "repo.idempotency.DeleteExpired"
	logQuery(ctx, op, olderThan)
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM idempotency_keys WHERE created_at < ?`,
		model.FormatUTC(olderThan))
	if err != nil {
		return 0, logErr(ctx, op, fmt.Errorf("delete idempotency_keys: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, logErr(ctx, op, err)
	}
	return n, nil
}

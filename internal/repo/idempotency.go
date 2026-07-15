package repo

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

type IdempotencyRepo struct {
	db *sql.DB
}

func NewIdempotencyRepo(db *sql.DB) *IdempotencyRepo {
	return &IdempotencyRepo{db: db}
}

func scanIdempotency(row interface{ Scan(...any) error }) (model.IdempotencyRecord, error) {
	var rec model.IdempotencyRecord
	var createdAt string
	if err := row.Scan(&rec.Key, &rec.UserID, &rec.Method, &rec.Path, &rec.Status, &rec.Response, &createdAt); err != nil {
		return model.IdempotencyRecord{}, err
	}
	t, err := model.ParseUTC(createdAt)
	if err != nil {
		return model.IdempotencyRecord{}, fmt.Errorf("parse created_at: %w", err)
	}
	rec.CreatedAt = t
	return rec, nil
}

// Reserve inserts a pending row for key. It returns (existing, true) when the
// key is already present — either still in flight (Status 0) or completed
// (Status + Response set) — and (zero, false) when a fresh reservation was
// created. The INSERT ... ON CONFLICT DO NOTHING + SELECT pair is atomic under
// SQLite's single writer, so a concurrent duplicate reads the existing row.
func (r *IdempotencyRepo) Reserve(ctx context.Context, key string, userID int64, method, path string) (model.IdempotencyRecord, bool, error) {
	const op = "repo.idempotency.Reserve"
	logQuery(ctx, op, key, userID, method, path)
	now := model.FormatUTC(time.Now())
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO idempotency_keys (key, user_id, method, path, status, response, created_at)
		 VALUES (?, ?, ?, ?, 0, '', ?)
		 ON CONFLICT(key) DO NOTHING`,
		key, userID, method, path, now)
	if err != nil {
		return model.IdempotencyRecord{}, false, logErr(ctx, op, fmt.Errorf("insert idempotency key: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return model.IdempotencyRecord{}, false, logErr(ctx, op, fmt.Errorf("rows affected: %w", err))
	}
	if n > 0 {
		// Fresh reservation. Callers only inspect the record on the existing
		// path (for replay), so a zero record is returned here per the contract.
		return model.IdempotencyRecord{}, false, nil
	}
	row := r.db.QueryRowContext(ctx,
		`SELECT key, user_id, method, path, status, response, created_at FROM idempotency_keys WHERE key = ?`, key)
	rec, err := scanIdempotency(row)
	if err != nil {
		return model.IdempotencyRecord{}, false, logErr(ctx, op, fmt.Errorf("select existing idempotency key: %w", err))
	}
	return rec, true, nil
}

// Complete stores the final status + response body for a reserved key.
func (r *IdempotencyRepo) Complete(ctx context.Context, key string, status int, response []byte) error {
	const op = "repo.idempotency.Complete"
	logQuery(ctx, op, key, status)
	_, err := r.db.ExecContext(ctx,
		`UPDATE idempotency_keys SET status = ?, response = ? WHERE key = ?`,
		status, string(response), key)
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("complete idempotency key: %w", err))
	}
	return nil
}

// Release deletes a pending reservation so an honest retry re-runs the handler
// (used when the handler errored or returned a non-2xx response).
func (r *IdempotencyRepo) Release(ctx context.Context, key string) error {
	const op = "repo.idempotency.Release"
	logQuery(ctx, op, key)
	_, err := r.db.ExecContext(ctx, `DELETE FROM idempotency_keys WHERE key = ?`, key)
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("release idempotency key: %w", err))
	}
	return nil
}

// DeleteOlderThan prunes rows created before cutoff and returns the count deleted.
func (r *IdempotencyRepo) DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	const op = "repo.idempotency.DeleteOlderThan"
	logQuery(ctx, op, cutoff)
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM idempotency_keys WHERE created_at < ?`, model.FormatUTC(cutoff))
	if err != nil {
		return 0, logErr(ctx, op, fmt.Errorf("prune idempotency keys: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, logErr(ctx, op, fmt.Errorf("rows affected: %w", err))
	}
	return n, nil
}

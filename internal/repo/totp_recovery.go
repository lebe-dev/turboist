package repo

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

type TOTPRecoveryCode struct {
	ID        int64
	UserID    int64
	CodeHash  string
	UsedAt    *time.Time
	CreatedAt time.Time
}

type TOTPRecoveryRepo struct {
	db *sql.DB
}

func NewTOTPRecoveryRepo(db *sql.DB) *TOTPRecoveryRepo {
	return &TOTPRecoveryRepo{db: db}
}

// Replace deletes all existing recovery codes for the user and inserts the
// provided hashes atomically.
func (r *TOTPRecoveryRepo) Replace(ctx context.Context, userID int64, codeHashes []string) error {
	const op = "repo.totp_recovery.Replace"
	logQuery(ctx, op, userID)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("begin: %w", err))
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM totp_recovery_codes WHERE user_id = ?`, userID); err != nil {
		return logErr(ctx, op, fmt.Errorf("delete: %w", err))
	}
	now := model.FormatUTC(time.Now())
	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO totp_recovery_codes (user_id, code_hash, created_at) VALUES (?, ?, ?)`)
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("prepare: %w", err))
	}
	defer func() { _ = stmt.Close() }()
	for _, h := range codeHashes {
		if _, err := stmt.ExecContext(ctx, userID, h, now); err != nil {
			return logErr(ctx, op, fmt.Errorf("insert: %w", err))
		}
	}
	if err := tx.Commit(); err != nil {
		return logErr(ctx, op, fmt.Errorf("commit: %w", err))
	}
	return nil
}

// ListUnused returns all recovery codes for the user that have not been used.
func (r *TOTPRecoveryRepo) ListUnused(ctx context.Context, userID int64) ([]TOTPRecoveryCode, error) {
	const op = "repo.totp_recovery.ListUnused"
	logQuery(ctx, op, userID)
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, code_hash, used_at, created_at
		 FROM totp_recovery_codes WHERE user_id = ? AND used_at IS NULL
		 ORDER BY id`, userID)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("query: %w", err))
	}
	defer func() { _ = rows.Close() }()
	var out []TOTPRecoveryCode
	for rows.Next() {
		var c TOTPRecoveryCode
		var usedAt sql.NullString
		var createdAt string
		if err := rows.Scan(&c.ID, &c.UserID, &c.CodeHash, &usedAt, &createdAt); err != nil {
			return nil, logErr(ctx, op, fmt.Errorf("scan: %w", err))
		}
		if usedAt.Valid && usedAt.String != "" {
			t, err := model.ParseUTC(usedAt.String)
			if err != nil {
				return nil, logErr(ctx, op, fmt.Errorf("parse used_at: %w", err))
			}
			c.UsedAt = &t
		}
		t, err := model.ParseUTC(createdAt)
		if err != nil {
			return nil, logErr(ctx, op, fmt.Errorf("parse created_at: %w", err))
		}
		c.CreatedAt = t
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, logErr(ctx, op, err)
	}
	return out, nil
}

// MarkUsed marks a single recovery code as used. Returns ErrNotFound if the
// code does not exist or was already used.
func (r *TOTPRecoveryRepo) MarkUsed(ctx context.Context, id int64) error {
	const op = "repo.totp_recovery.MarkUsed"
	logQuery(ctx, op, id)
	now := model.FormatUTC(time.Now())
	res, err := r.db.ExecContext(ctx,
		`UPDATE totp_recovery_codes SET used_at = ? WHERE id = ? AND used_at IS NULL`,
		now, id)
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("mark used: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return logErr(ctx, op, ErrNotFound)
	}
	return nil
}

// DeleteAll removes every recovery code for the user. Used by Disable flow.
func (r *TOTPRecoveryRepo) DeleteAll(ctx context.Context, userID int64) error {
	const op = "repo.totp_recovery.DeleteAll"
	logQuery(ctx, op, userID)
	if _, err := r.db.ExecContext(ctx,
		`DELETE FROM totp_recovery_codes WHERE user_id = ?`, userID); err != nil {
		return logErr(ctx, op, fmt.Errorf("delete: %w", err))
	}
	return nil
}

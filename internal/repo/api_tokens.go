package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
)

type APITokenRepo struct {
	db *sql.DB
}

func NewAPITokenRepo(db *sql.DB) *APITokenRepo {
	return &APITokenRepo{db: db}
}

func scanAPIToken(row interface{ Scan(...any) error }) (*model.APIToken, error) {
	var t model.APIToken
	var createdAt string
	if err := row.Scan(&t.ID, &t.UserID, &t.Name, &t.TokenHash, &createdAt); err != nil {
		return nil, err
	}
	parsed, err := model.ParseUTC(createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	t.CreatedAt = parsed
	return &t, nil
}

func (r *APITokenRepo) Create(ctx context.Context, userID int64, name, tokenHash string) (*model.APIToken, error) {
	const op = "repo.api_tokens.Create"
	logQuery(ctx, op, userID, name)
	now := model.FormatUTC(time.Now())
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO api_tokens (user_id, name, token_hash, created_at) VALUES (?, ?, ?, ?)`,
		userID, name, tokenHash, now)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, logErr(ctx, op, ErrConflict)
		}
		return nil, logErr(ctx, op, fmt.Errorf("insert api_token: %w", err))
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("last insert id: %w", err))
	}
	return r.Get(ctx, id)
}

func (r *APITokenRepo) Get(ctx context.Context, id int64) (*model.APIToken, error) {
	const op = "repo.api_tokens.Get"
	logQuery(ctx, op, id)
	row := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, name, token_hash, created_at FROM api_tokens WHERE id = ?`, id)
	t, err := scanAPIToken(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, logErr(ctx, op, ErrNotFound)
	}
	if err != nil {
		return nil, logErr(ctx, op, err)
	}
	return t, nil
}

func (r *APITokenRepo) GetByTokenHash(ctx context.Context, tokenHash string) (*model.APIToken, error) {
	const op = "repo.api_tokens.GetByTokenHash"
	logQuery(ctx, op)
	row := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, name, token_hash, created_at FROM api_tokens WHERE token_hash = ?`, tokenHash)
	t, err := scanAPIToken(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, logErr(ctx, op, ErrNotFound)
	}
	if err != nil {
		return nil, logErr(ctx, op, err)
	}
	return t, nil
}

func (r *APITokenRepo) ListByUser(ctx context.Context, userID int64) ([]model.APIToken, error) {
	const op = "repo.api_tokens.ListByUser"
	logQuery(ctx, op, userID)
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, name, token_hash, created_at FROM api_tokens
		 WHERE user_id = ? ORDER BY created_at DESC, id DESC`, userID)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("list api_tokens: %w", err))
	}
	defer logging.LogClose(ctx, op+".rows", rows)
	out := make([]model.APIToken, 0)
	for rows.Next() {
		t, err := scanAPIToken(rows)
		if err != nil {
			return nil, logErr(ctx, op, err)
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

func (r *APITokenRepo) Delete(ctx context.Context, id, userID int64) error {
	const op = "repo.api_tokens.Delete"
	logQuery(ctx, op, id, userID)
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM api_tokens WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("delete api_token: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return logErr(ctx, op, err)
	}
	if n == 0 {
		return logErr(ctx, op, ErrNotFound)
	}
	return nil
}

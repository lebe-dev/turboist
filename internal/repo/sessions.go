package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

type SessionRepo struct {
	db *sql.DB
}

func NewSessionRepo(db *sql.DB) *SessionRepo {
	return &SessionRepo{db: db}
}

func scanSession(row interface{ Scan(...any) error }) (*model.Session, error) {
	var s model.Session
	var createdAt, lastUsedAt, expiresAt string
	var revokedAt sql.NullString
	var clientKind string
	if err := row.Scan(&s.ID, &s.UserID, &s.TokenHash, &clientKind, &s.UserAgent,
		&createdAt, &lastUsedAt, &expiresAt, &revokedAt); err != nil {
		return nil, err
	}
	s.ClientKind = model.ClientKind(clientKind)
	t, err := model.ParseUTC(createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	s.CreatedAt = t
	t, err = model.ParseUTC(lastUsedAt)
	if err != nil {
		return nil, fmt.Errorf("parse last_used_at: %w", err)
	}
	s.LastUsedAt = t
	t, err = model.ParseUTC(expiresAt)
	if err != nil {
		return nil, fmt.Errorf("parse expires_at: %w", err)
	}
	s.ExpiresAt = t
	if revokedAt.Valid {
		t, err := model.ParseUTC(revokedAt.String)
		if err != nil {
			return nil, fmt.Errorf("parse revoked_at: %w", err)
		}
		s.RevokedAt = &t
	}
	return &s, nil
}

type CreateSessionParams struct {
	UserID     int64
	TokenHash  string
	ClientKind model.ClientKind
	UserAgent  string
	ExpiresAt  time.Time
}

func (r *SessionRepo) Create(ctx context.Context, p CreateSessionParams) (*model.Session, error) {
	now := model.FormatUTC(time.Now())
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO sessions (user_id, token_hash, client_kind, user_agent, created_at, last_used_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.UserID, p.TokenHash, string(p.ClientKind), p.UserAgent, now, now, model.FormatUTC(p.ExpiresAt))
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("insert session: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("last insert id: %w", err)
	}
	return r.Get(ctx, id)
}

func (r *SessionRepo) Get(ctx context.Context, id int64) (*model.Session, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, token_hash, client_kind, user_agent, created_at, last_used_at, expires_at, revoked_at
		 FROM sessions WHERE id = ?`, id)
	s, err := scanSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (r *SessionRepo) GetByTokenHash(ctx context.Context, tokenHash string) (*model.Session, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, token_hash, client_kind, user_agent, created_at, last_used_at, expires_at, revoked_at
		 FROM sessions WHERE token_hash = ?`, tokenHash)
	s, err := scanSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (r *SessionRepo) Rotate(ctx context.Context, id int64, newTokenHash string, newExpiresAt time.Time) error {
	now := model.FormatUTC(time.Now())
	res, err := r.db.ExecContext(ctx,
		`UPDATE sessions SET token_hash = ?, expires_at = ?, last_used_at = ? WHERE id = ? AND revoked_at IS NULL`,
		newTokenHash, model.FormatUTC(newExpiresAt), now, id)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return fmt.Errorf("rotate session: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *SessionRepo) TouchLastUsed(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE sessions SET last_used_at = ? WHERE id = ?`,
		model.FormatUTC(time.Now()), id)
	if err != nil {
		return fmt.Errorf("touch session: %w", err)
	}
	return nil
}

func (r *SessionRepo) Revoke(ctx context.Context, id int64) error {
	now := model.FormatUTC(time.Now())
	res, err := r.db.ExecContext(ctx,
		`UPDATE sessions SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`,
		now, id)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *SessionRepo) RevokeAllForUser(ctx context.Context, userID int64) error {
	now := model.FormatUTC(time.Now())
	_, err := r.db.ExecContext(ctx,
		`UPDATE sessions SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL`,
		now, userID)
	if err != nil {
		return fmt.Errorf("revoke all sessions: %w", err)
	}
	return nil
}

// EnforceLimit deletes oldest sessions (by last_used_at) for a user/client_kind beyond `keep`.
func (r *SessionRepo) EnforceLimit(ctx context.Context, userID int64, clientKind model.ClientKind, keep int) error {
	if keep < 1 {
		keep = 1
	}
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM sessions
		WHERE user_id = ?
		  AND client_kind = ?
		  AND revoked_at IS NULL
		  AND id NOT IN (
		      SELECT id FROM sessions
		      WHERE user_id = ?
		        AND client_kind = ?
		        AND revoked_at IS NULL
		      ORDER BY last_used_at DESC
		      LIMIT ?
		  )`, userID, string(clientKind), userID, string(clientKind), keep)
	if err != nil {
		return fmt.Errorf("enforce session limit: %w", err)
	}
	return nil
}

// ListActiveForUser returns active (non-revoked, non-expired) sessions ordered by last_used_at DESC.
func (r *SessionRepo) ListActiveForUser(ctx context.Context, userID int64) ([]model.Session, error) {
	now := model.FormatUTC(time.Now())
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, token_hash, client_kind, user_agent, created_at, last_used_at, expires_at, revoked_at
		 FROM sessions WHERE user_id = ? AND revoked_at IS NULL AND expires_at > ?
		 ORDER BY last_used_at DESC`, userID, now)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]model.Session, 0)
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

// Cleanup removes expired sessions and revoked sessions older than 7 days.
func (r *SessionRepo) Cleanup(ctx context.Context) (int64, error) {
	now := time.Now().UTC()
	cutoff := now.Add(-7 * 24 * time.Hour)
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM sessions
		WHERE expires_at < ?
		   OR (revoked_at IS NOT NULL AND revoked_at < ?)`,
		model.FormatUTC(now), model.FormatUTC(cutoff))
	if err != nil {
		return 0, fmt.Errorf("cleanup sessions: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return n, nil
}

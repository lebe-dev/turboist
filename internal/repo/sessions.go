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
	if err := row.Scan(&s.ID, &s.UserID, &s.TokenHash, &clientKind, &s.UserAgent, &s.IPAddress,
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
	IPAddress  string
	ExpiresAt  time.Time
}

func (r *SessionRepo) Create(ctx context.Context, p CreateSessionParams) (*model.Session, error) {
	const op = "repo.sessions.Create"
	logQuery(ctx, op, p.UserID, p.ClientKind)
	now := model.FormatUTC(time.Now())
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO sessions (user_id, token_hash, client_kind, user_agent, ip_address, created_at, last_used_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.UserID, p.TokenHash, string(p.ClientKind), p.UserAgent, p.IPAddress, now, now, model.FormatUTC(p.ExpiresAt))
	if err != nil {
		if isUniqueViolation(err) {
			return nil, logErr(ctx, op, ErrConflict)
		}
		return nil, logErr(ctx, op, fmt.Errorf("insert session: %w", err))
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("last insert id: %w", err))
	}
	return r.Get(ctx, id)
}

func (r *SessionRepo) Get(ctx context.Context, id int64) (*model.Session, error) {
	const op = "repo.sessions.Get"
	logQuery(ctx, op, id)
	row := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, token_hash, client_kind, user_agent, ip_address, created_at, last_used_at, expires_at, revoked_at
		 FROM sessions WHERE id = ?`, id)
	s, err := scanSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, logErr(ctx, op, ErrNotFound)
	}
	if err != nil {
		return nil, logErr(ctx, op, err)
	}
	return s, nil
}

func (r *SessionRepo) GetByTokenHash(ctx context.Context, tokenHash string) (*model.Session, error) {
	const op = "repo.sessions.GetByTokenHash"
	logQuery(ctx, op)
	row := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, token_hash, client_kind, user_agent, ip_address, created_at, last_used_at, expires_at, revoked_at
		 FROM sessions WHERE token_hash = ?`, tokenHash)
	s, err := scanSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, logErr(ctx, op, ErrNotFound)
	}
	if err != nil {
		return nil, logErr(ctx, op, err)
	}
	return s, nil
}

func (r *SessionRepo) Rotate(ctx context.Context, id int64, newTokenHash string, newExpiresAt time.Time) error {
	const op = "repo.sessions.Rotate"
	logQuery(ctx, op, id)
	now := model.FormatUTC(time.Now())
	res, err := r.db.ExecContext(ctx,
		`UPDATE sessions SET token_hash = ?, expires_at = ?, last_used_at = ? WHERE id = ? AND revoked_at IS NULL`,
		newTokenHash, model.FormatUTC(newExpiresAt), now, id)
	if err != nil {
		if isUniqueViolation(err) {
			return logErr(ctx, op, ErrConflict)
		}
		return logErr(ctx, op, fmt.Errorf("rotate session: %w", err))
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

func (r *SessionRepo) TouchLastUsed(ctx context.Context, id int64) error {
	const op = "repo.sessions.TouchLastUsed"
	logQuery(ctx, op, id)
	_, err := r.db.ExecContext(ctx,
		`UPDATE sessions SET last_used_at = ? WHERE id = ?`,
		model.FormatUTC(time.Now()), id)
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("touch session: %w", err))
	}
	return nil
}

func (r *SessionRepo) Revoke(ctx context.Context, id int64) error {
	const op = "repo.sessions.Revoke"
	logQuery(ctx, op, id)
	now := model.FormatUTC(time.Now())
	res, err := r.db.ExecContext(ctx,
		`UPDATE sessions SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`,
		now, id)
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("revoke session: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return logErr(ctx, op, err)
	}
	if n == 0 {
		return logErr(ctx, op, ErrNotFound)
<<<<<<< HEAD
	}
	return nil
}

// RevokeForUser revokes the session only if it belongs to userID, returning
// ErrNotFound otherwise. Prevents one account from touching another's session
// via the public API even when the row id is guessed.
func (r *SessionRepo) RevokeForUser(ctx context.Context, id, userID int64) error {
	const op = "repo.sessions.RevokeForUser"
	logQuery(ctx, op, id, userID)
	now := model.FormatUTC(time.Now())
	res, err := r.db.ExecContext(ctx,
		`UPDATE sessions SET revoked_at = ? WHERE id = ? AND user_id = ? AND revoked_at IS NULL`,
		now, id, userID)
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("revoke session for user: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return logErr(ctx, op, err)
	}
	if n == 0 {
		return logErr(ctx, op, ErrNotFound)
=======
>>>>>>> 049dc85 (v1.6.0 (#32))
	}
	return nil
}

func (r *SessionRepo) RevokeAllForUser(ctx context.Context, userID int64) error {
	const op = "repo.sessions.RevokeAllForUser"
	logQuery(ctx, op, userID)
	now := model.FormatUTC(time.Now())
	_, err := r.db.ExecContext(ctx,
		`UPDATE sessions SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL`,
		now, userID)
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("revoke all sessions: %w", err))
<<<<<<< HEAD
	}
	return nil
}

// RevokeAllForUserExcept revokes every active session of userID except exceptID.
// Used by "sign out of all other sessions" so the caller stays logged in.
func (r *SessionRepo) RevokeAllForUserExcept(ctx context.Context, userID, exceptID int64) error {
	const op = "repo.sessions.RevokeAllForUserExcept"
	logQuery(ctx, op, userID, exceptID)
	now := model.FormatUTC(time.Now())
	_, err := r.db.ExecContext(ctx,
		`UPDATE sessions SET revoked_at = ? WHERE user_id = ? AND id != ? AND revoked_at IS NULL`,
		now, userID, exceptID)
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("revoke other sessions: %w", err))
=======
>>>>>>> 049dc85 (v1.6.0 (#32))
	}
	return nil
}

// EnforceLimit deletes oldest sessions (by last_used_at) for a user/client_kind beyond `keep`.
func (r *SessionRepo) EnforceLimit(ctx context.Context, userID int64, clientKind model.ClientKind, keep int) error {
	const op = "repo.sessions.EnforceLimit"
	logQuery(ctx, op, userID, clientKind, keep)
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
		return logErr(ctx, op, fmt.Errorf("enforce session limit: %w", err))
	}
	return nil
}

// ListActiveForUser returns active (non-revoked, non-expired) sessions ordered by last_used_at DESC.
func (r *SessionRepo) ListActiveForUser(ctx context.Context, userID int64) ([]model.Session, error) {
	const op = "repo.sessions.ListActiveForUser"
	logQuery(ctx, op, userID)
	now := model.FormatUTC(time.Now())
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, token_hash, client_kind, user_agent, ip_address, created_at, last_used_at, expires_at, revoked_at
		 FROM sessions WHERE user_id = ? AND revoked_at IS NULL AND expires_at > ?
		 ORDER BY last_used_at DESC`, userID, now)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("list sessions: %w", err))
	}
	defer logging.LogClose(ctx, op+".rows", rows)
	out := make([]model.Session, 0)
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, logErr(ctx, op, err)
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

// Cleanup removes expired sessions and revoked sessions older than 7 days.
func (r *SessionRepo) Cleanup(ctx context.Context) (int64, error) {
	const op = "repo.sessions.Cleanup"
	logQuery(ctx, op)
	now := time.Now().UTC()
	cutoff := now.Add(-7 * 24 * time.Hour)
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM sessions
		WHERE expires_at < ?
		   OR (revoked_at IS NOT NULL AND revoked_at < ?)`,
		model.FormatUTC(now), model.FormatUTC(cutoff))
	if err != nil {
		return 0, logErr(ctx, op, fmt.Errorf("cleanup sessions: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, logErr(ctx, op, err)
	}
	return n, nil
}

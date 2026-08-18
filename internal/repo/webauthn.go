package repo

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

// handleBytes is the length of a generated WebAuthn user handle. The spec caps
// the handle at 64 bytes and recommends filling it with randomness; 32 bytes is
// plenty and keeps the base64url form short.
const handleBytes = 32

// WebAuthnRepo stores passkey credentials plus the per-user WebAuthn handle.
type WebAuthnRepo struct {
	db *sql.DB
}

func NewWebAuthnRepo(db *sql.DB) *WebAuthnRepo {
	return &WebAuthnRepo{db: db}
}

// CreateCredentialParams is the insert payload for a freshly registered passkey.
type CreateCredentialParams struct {
	UserID       int64
	CredentialID string
	Name         string
	Credential   []byte
	SignCount    uint32
}

// EnsureHandle returns the user's WebAuthn handle, generating and persisting it
// on first use. Concurrent callers converge on the same value: the INSERT is
// idempotent (ON CONFLICT DO NOTHING) and the row is read back afterwards.
func (r *WebAuthnRepo) EnsureHandle(ctx context.Context, userID int64) (string, error) {
	const op = "repo.webauthn.EnsureHandle"
	logQuery(ctx, op, userID)

	handle, err := r.handle(ctx, userID)
	if err == nil {
		return handle, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return "", err
	}

	raw := make([]byte, handleBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", logErr(ctx, op, fmt.Errorf("generate handle: %w", err))
	}
	generated := base64.RawURLEncoding.EncodeToString(raw)
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO webauthn_users (user_id, handle, created_at) VALUES (?, ?, ?)
		 ON CONFLICT (user_id) DO NOTHING`,
		userID, generated, model.FormatUTC(time.Now())); err != nil {
		return "", logErr(ctx, op, fmt.Errorf("insert handle: %w", err))
	}
	return r.handle(ctx, userID)
}

func (r *WebAuthnRepo) handle(ctx context.Context, userID int64) (string, error) {
	const op = "repo.webauthn.handle"
	var handle string
	err := r.db.QueryRowContext(ctx,
		`SELECT handle FROM webauthn_users WHERE user_id = ?`, userID).Scan(&handle)
	if errors.Is(err, sql.ErrNoRows) {
		return "", logErr(ctx, op, ErrNotFound)
	}
	if err != nil {
		return "", logErr(ctx, op, fmt.Errorf("select handle: %w", err))
	}
	return handle, nil
}

// UserIDByHandle resolves the user behind a handle echoed back by an
// authenticator during a discoverable login. Returns ErrNotFound for an unknown
// handle.
func (r *WebAuthnRepo) UserIDByHandle(ctx context.Context, handle string) (int64, error) {
	const op = "repo.webauthn.UserIDByHandle"
	logQuery(ctx, op)
	var userID int64
	err := r.db.QueryRowContext(ctx,
		`SELECT user_id FROM webauthn_users WHERE handle = ?`, handle).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, logErr(ctx, op, ErrNotFound)
	}
	if err != nil {
		return 0, logErr(ctx, op, fmt.Errorf("select user by handle: %w", err))
	}
	return userID, nil
}

const credentialSelectCols = `id, user_id, credential_id, name, credential, sign_count, created_at, last_used_at`

func scanCredential(row interface{ Scan(...any) error }) (*model.PasskeyCredential, error) {
	var c model.PasskeyCredential
	var signCount int64
	var createdAt string
	var lastUsedAt sql.NullString
	if err := row.Scan(&c.ID, &c.UserID, &c.CredentialID, &c.Name, &c.Credential,
		&signCount, &createdAt, &lastUsedAt); err != nil {
		return nil, err
	}
	c.SignCount = uint32(signCount)
	t, err := model.ParseUTC(createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	c.CreatedAt = t
	if lastUsedAt.Valid && lastUsedAt.String != "" {
		t, err := model.ParseUTC(lastUsedAt.String)
		if err != nil {
			return nil, fmt.Errorf("parse last_used_at: %w", err)
		}
		c.LastUsedAt = &t
	}
	return &c, nil
}

// ListByUser returns every passkey registered by the user, newest first.
func (r *WebAuthnRepo) ListByUser(ctx context.Context, userID int64) ([]model.PasskeyCredential, error) {
	const op = "repo.webauthn.ListByUser"
	logQuery(ctx, op, userID)
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+credentialSelectCols+` FROM webauthn_credentials
		 WHERE user_id = ? ORDER BY id DESC`, userID)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("query: %w", err))
	}
	defer func() { _ = rows.Close() }()

	out := make([]model.PasskeyCredential, 0)
	for rows.Next() {
		c, err := scanCredential(rows)
		if err != nil {
			return nil, logErr(ctx, op, fmt.Errorf("scan: %w", err))
		}
		out = append(out, *c)
	}
	if err := rows.Err(); err != nil {
		return nil, logErr(ctx, op, err)
	}
	return out, nil
}

// GetByCredentialID looks a credential up by its base64url raw id.
func (r *WebAuthnRepo) GetByCredentialID(ctx context.Context, credentialID string) (*model.PasskeyCredential, error) {
	const op = "repo.webauthn.GetByCredentialID"
	logQuery(ctx, op)
	row := r.db.QueryRowContext(ctx,
		`SELECT `+credentialSelectCols+` FROM webauthn_credentials WHERE credential_id = ?`,
		credentialID)
	c, err := scanCredential(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, logErr(ctx, op, ErrNotFound)
	}
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("scan: %w", err))
	}
	return c, nil
}

// Create stores a newly registered credential. Re-registering the same
// authenticator returns ErrConflict (the credential_id is UNIQUE).
func (r *WebAuthnRepo) Create(ctx context.Context, p CreateCredentialParams) (*model.PasskeyCredential, error) {
	const op = "repo.webauthn.Create"
	logQuery(ctx, op, p.UserID)
	now := model.FormatUTC(time.Now())
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO webauthn_credentials (user_id, credential_id, name, credential, sign_count, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		p.UserID, p.CredentialID, p.Name, p.Credential, int64(p.SignCount), now)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, logErr(ctx, op, ErrConflict)
		}
		return nil, logErr(ctx, op, fmt.Errorf("insert: %w", err))
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("last insert id: %w", err))
	}
	return r.Get(ctx, id, p.UserID)
}

// Get returns one credential owned by the user.
func (r *WebAuthnRepo) Get(ctx context.Context, id, userID int64) (*model.PasskeyCredential, error) {
	const op = "repo.webauthn.Get"
	logQuery(ctx, op, id, userID)
	row := r.db.QueryRowContext(ctx,
		`SELECT `+credentialSelectCols+` FROM webauthn_credentials WHERE id = ? AND user_id = ?`,
		id, userID)
	c, err := scanCredential(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, logErr(ctx, op, ErrNotFound)
	}
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("scan: %w", err))
	}
	return c, nil
}

// TouchAfterLogin rewrites the credential record after a successful assertion.
// The blob carries the updated signature counter and backup-state flags, so
// skipping this write would defeat clone detection.
func (r *WebAuthnRepo) TouchAfterLogin(ctx context.Context, credentialID string, credential []byte, signCount uint32) error {
	const op = "repo.webauthn.TouchAfterLogin"
	logQuery(ctx, op)
	res, err := r.db.ExecContext(ctx,
		`UPDATE webauthn_credentials
		 SET credential = ?, sign_count = ?, last_used_at = ?
		 WHERE credential_id = ?`,
		credential, int64(signCount), model.FormatUTC(time.Now()), credentialID)
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("update: %w", err))
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

// Rename changes the user-facing label of a credential.
func (r *WebAuthnRepo) Rename(ctx context.Context, id, userID int64, name string) error {
	const op = "repo.webauthn.Rename"
	logQuery(ctx, op, id, userID)
	res, err := r.db.ExecContext(ctx,
		`UPDATE webauthn_credentials SET name = ? WHERE id = ? AND user_id = ?`,
		name, id, userID)
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("update: %w", err))
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

// Delete removes a credential owned by the user.
func (r *WebAuthnRepo) Delete(ctx context.Context, id, userID int64) error {
	const op = "repo.webauthn.Delete"
	logQuery(ctx, op, id, userID)
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM webauthn_credentials WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("delete: %w", err))
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

// CountAll reports how many passkeys exist across the instance. The login page
// uses it to decide whether offering a passkey button makes sense at all.
func (r *WebAuthnRepo) CountAll(ctx context.Context) (int, error) {
	const op = "repo.webauthn.CountAll"
	logQuery(ctx, op)
	var n int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM webauthn_credentials`).Scan(&n); err != nil {
		return 0, logErr(ctx, op, fmt.Errorf("count: %w", err))
	}
	return n, nil
}

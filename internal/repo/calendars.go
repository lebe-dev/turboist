package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

type CalendarRepo struct {
	db *sql.DB
}

func NewCalendarRepo(db *sql.DB) *CalendarRepo {
	return &CalendarRepo{db: db}
}

func scanCalendarAccount(row interface{ Scan(...any) error }) (*model.CalendarAccount, error) {
	var a model.CalendarAccount
	var provider, expiry, createdAt, updatedAt string
	if err := row.Scan(&a.ID, &a.UserID, &provider, &a.Email, &a.DisplayName, &a.AccessToken, &a.RefreshToken, &expiry, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	a.Provider = model.CalendarProvider(provider)
	var err error
	if a.Expiry, err = model.ParseUTC(expiry); err != nil {
		return nil, fmt.Errorf("parse account expiry: %w", err)
	}
	if a.CreatedAt, err = model.ParseUTC(createdAt); err != nil {
		return nil, fmt.Errorf("parse account created_at: %w", err)
	}
	if a.UpdatedAt, err = model.ParseUTC(updatedAt); err != nil {
		return nil, fmt.Errorf("parse account updated_at: %w", err)
	}
	return &a, nil
}

func scanCalendarSource(row interface{ Scan(...any) error }) (*model.CalendarSource, error) {
	var s model.CalendarSource
	var provider, createdAt, updatedAt string
	var selected, primary int64
	if err := row.Scan(&s.ID, &s.AccountID, &s.UserID, &provider, &s.ExternalID, &s.Summary, &s.Color, &selected, &primary, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	s.Provider = model.CalendarProvider(provider)
	s.Selected = selected != 0
	s.IsPrimary = primary != 0
	var err error
	if s.CreatedAt, err = model.ParseUTC(createdAt); err != nil {
		return nil, fmt.Errorf("parse source created_at: %w", err)
	}
	if s.UpdatedAt, err = model.ParseUTC(updatedAt); err != nil {
		return nil, fmt.Errorf("parse source updated_at: %w", err)
	}
	return &s, nil
}

func (r *CalendarRepo) CreateOAuthState(ctx context.Context, state string, userID int64, provider model.CalendarProvider, ttl time.Duration) error {
	now := time.Now()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO calendar_oauth_states (state, user_id, provider, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		state, userID, string(provider), model.FormatUTC(now.Add(ttl)), model.FormatUTC(now))
	if err != nil {
		return fmt.Errorf("insert calendar oauth state: %w", err)
	}
	return nil
}

func (r *CalendarRepo) ConsumeOAuthState(ctx context.Context, state string, provider model.CalendarProvider) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	var userID int64
	var expiresRaw string
	err = tx.QueryRowContext(ctx,
		`SELECT user_id, expires_at FROM calendar_oauth_states WHERE state = ? AND provider = ?`,
		state, string(provider)).Scan(&userID, &expiresRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("get calendar oauth state: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM calendar_oauth_states WHERE state = ?`, state); err != nil {
		return 0, fmt.Errorf("delete calendar oauth state: %w", err)
	}
	expires, err := model.ParseUTC(expiresRaw)
	if err != nil {
		return 0, fmt.Errorf("parse calendar oauth state expiry: %w", err)
	}
	if time.Now().After(expires) {
		return 0, ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return userID, nil
}

func (r *CalendarRepo) UpsertAccount(ctx context.Context, a *model.CalendarAccount) (*model.CalendarAccount, error) {
	now := model.FormatUTC(time.Now())
	if a.RefreshToken == "" {
		existing, err := r.GetAccountByProvider(ctx, a.UserID, a.Provider)
		if err == nil {
			a.RefreshToken = existing.RefreshToken
		} else if !errors.Is(err, ErrNotFound) {
			return nil, err
		}
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO calendar_accounts
		    (user_id, provider, email, display_name, access_token, refresh_token, expiry, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(user_id, provider) DO UPDATE SET
		    email = excluded.email,
		    display_name = excluded.display_name,
		    access_token = excluded.access_token,
		    refresh_token = excluded.refresh_token,
		    expiry = excluded.expiry,
		    updated_at = excluded.updated_at`,
		a.UserID, string(a.Provider), a.Email, a.DisplayName, a.AccessToken, a.RefreshToken,
		model.FormatUTC(a.Expiry), now, now)
	if err != nil {
		return nil, fmt.Errorf("upsert calendar account: %w", err)
	}
	return r.GetAccountByProvider(ctx, a.UserID, a.Provider)
}

func (r *CalendarRepo) GetAccountByProvider(ctx context.Context, userID int64, provider model.CalendarProvider) (*model.CalendarAccount, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, provider, email, display_name, access_token, refresh_token, expiry, created_at, updated_at
		   FROM calendar_accounts WHERE user_id = ? AND provider = ?`,
		userID, string(provider))
	a, err := scanCalendarAccount(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (r *CalendarRepo) ListAccounts(ctx context.Context, userID int64) ([]model.CalendarAccount, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, provider, email, display_name, access_token, refresh_token, expiry, created_at, updated_at
		   FROM calendar_accounts WHERE user_id = ? ORDER BY provider`,
		userID)
	if err != nil {
		return nil, fmt.Errorf("list calendar accounts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []model.CalendarAccount{}
	for rows.Next() {
		a, err := scanCalendarAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func (r *CalendarRepo) DeleteAccount(ctx context.Context, userID, accountID int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM calendar_accounts WHERE id = ? AND user_id = ?`, accountID, userID)
	if err != nil {
		return fmt.Errorf("delete calendar account: %w", err)
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

func (r *CalendarRepo) UpsertSources(ctx context.Context, account *model.CalendarAccount, sources []model.CalendarSource) error {
	now := model.FormatUTC(time.Now())
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, src := range sources {
		if src.Summary == "" {
			src.Summary = src.ExternalID
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO calendar_sources
			    (account_id, user_id, provider, external_id, summary, color, selected, is_primary, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(account_id, external_id) DO UPDATE SET
			    summary = excluded.summary,
			    color = excluded.color,
			    is_primary = excluded.is_primary,
			    updated_at = excluded.updated_at`,
			account.ID, account.UserID, string(account.Provider), src.ExternalID, src.Summary, src.Color,
			boolInt(src.Selected), boolInt(src.IsPrimary), now, now)
		if err != nil {
			return fmt.Errorf("upsert calendar source: %w", err)
		}
	}
	return tx.Commit()
}

func (r *CalendarRepo) ListSources(ctx context.Context, userID int64) ([]model.CalendarSource, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, account_id, user_id, provider, external_id, summary, color, selected, is_primary, created_at, updated_at
		   FROM calendar_sources WHERE user_id = ? ORDER BY provider, is_primary DESC, summary COLLATE NOCASE`,
		userID)
	if err != nil {
		return nil, fmt.Errorf("list calendar sources: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []model.CalendarSource{}
	for rows.Next() {
		s, err := scanCalendarSource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

func (r *CalendarRepo) ListSelectedSources(ctx context.Context, userID int64, provider model.CalendarProvider) ([]model.CalendarSource, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, account_id, user_id, provider, external_id, summary, color, selected, is_primary, created_at, updated_at
		   FROM calendar_sources WHERE user_id = ? AND provider = ? AND selected = 1
		   ORDER BY is_primary DESC, summary COLLATE NOCASE`,
		userID, string(provider))
	if err != nil {
		return nil, fmt.Errorf("list selected calendar sources: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []model.CalendarSource{}
	for rows.Next() {
		s, err := scanCalendarSource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

func (r *CalendarRepo) SetSourceSelected(ctx context.Context, userID, sourceID int64, selected bool) (*model.CalendarSource, error) {
	now := model.FormatUTC(time.Now())
	res, err := r.db.ExecContext(ctx,
		`UPDATE calendar_sources SET selected = ?, updated_at = ? WHERE id = ? AND user_id = ?`,
		boolInt(selected), now, sourceID, userID)
	if err != nil {
		return nil, fmt.Errorf("set calendar source selected: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, ErrNotFound
	}
	row := r.db.QueryRowContext(ctx,
		`SELECT id, account_id, user_id, provider, external_id, summary, color, selected, is_primary, created_at, updated_at
		   FROM calendar_sources WHERE id = ? AND user_id = ?`,
		sourceID, userID)
	return scanCalendarSource(row)
}

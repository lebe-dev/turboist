package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

type UserRepo struct {
	db *sql.DB
}

func NewUserRepo(db *sql.DB) *UserRepo {
	return &UserRepo{db: db}
}

func scanUser(row interface{ Scan(...any) error }) (*model.User, error) {
	var u model.User
	var createdAt, updatedAt string
	var startedInt, totpEnabledInt int64
	var totpEnabledAt sql.NullString
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash,
		&u.TroikiMediumCapacity, &u.TroikiRestCapacity, &startedInt,
		&u.TOTPSecret, &totpEnabledInt, &totpEnabledAt,
		&createdAt, &updatedAt); err != nil {
		return nil, err
	}
	u.TroikiStarted = startedInt != 0
	u.TOTPEnabled = totpEnabledInt != 0
	if totpEnabledAt.Valid && totpEnabledAt.String != "" {
		t, err := model.ParseUTC(totpEnabledAt.String)
		if err != nil {
			return nil, fmt.Errorf("parse totp_enabled_at: %w", err)
		}
		u.TOTPEnabledAt = &t
	}
	t, err := model.ParseUTC(createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	u.CreatedAt = t
	t, err = model.ParseUTC(updatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}
	u.UpdatedAt = t
	return &u, nil
}

const userSelectCols = `id, username, password_hash, troiki_medium_capacity, troiki_rest_capacity, troiki_started, totp_secret, totp_enabled, totp_enabled_at, created_at, updated_at`

func (r *UserRepo) Exists(ctx context.Context) (bool, error) {
	const op = "repo.users.Exists"
	logQuery(ctx, op)
	var n int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return false, logErr(ctx, op, fmt.Errorf("count users: %w", err))
	}
	return n > 0, nil
}

func (r *UserRepo) Create(ctx context.Context, username, passwordHash string) (*model.User, error) {
	const op = "repo.users.Create"
	logQuery(ctx, op)
	now := model.FormatUTC(time.Now())
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO users (id, username, password_hash, created_at, updated_at) VALUES (1, ?, ?, ?, ?)`,
		username, passwordHash, now, now)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, logErr(ctx, op, ErrConflict)
		}
		return nil, logErr(ctx, op, fmt.Errorf("insert user: %w", err))
	}
	return r.Get(ctx, 1)
}

func (r *UserRepo) Get(ctx context.Context, id int64) (*model.User, error) {
	const op = "repo.users.Get"
	logQuery(ctx, op, id)
	row := r.db.QueryRowContext(ctx,
		`SELECT `+userSelectCols+` FROM users WHERE id = ?`, id)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, logErr(ctx, op, ErrNotFound)
	}
	if err != nil {
		return nil, logErr(ctx, op, err)
	}
	return u, nil
}

func (r *UserRepo) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	const op = "repo.users.GetByUsername"
	logQuery(ctx, op)
	row := r.db.QueryRowContext(ctx,
		`SELECT `+userSelectCols+` FROM users WHERE username = ?`, username)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, logErr(ctx, op, ErrNotFound)
	}
	if err != nil {
		return nil, logErr(ctx, op, err)
	}
	return u, nil
}

func (r *UserRepo) GetSettings(ctx context.Context, id int64) (*model.UserSettings, error) {
	var raw string
	err := r.db.QueryRowContext(ctx, `SELECT settings FROM users WHERE id = ?`, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user settings: %w", err)
	}
	var s model.UserSettings
	if raw != "" && raw != "{}" {
		if err := json.Unmarshal([]byte(raw), &s); err != nil {
			s = model.UserSettings{}
			raw = "{}"
		}
	}
	if raw == "" || raw == "{}" || !jsonObjectHasKey(raw, "calendarHidePastEvents") {
		s.CalendarHidePastEvents = true
	}
	if s.WeeklyUnplannedExcludedLabelIDs == nil {
		s.WeeklyUnplannedExcludedLabelIDs = []int64{}
	}
	if s.BugLabelIDs == nil {
		s.BugLabelIDs = []int64{}
	}
	// Blobs written before migration 048 (and any out-of-range leftovers) fall
	// back to the default cap instead of a meaningless zero that would block
	// pinning outright.
	if s.MaxPinnedTasks < model.MinMaxPinned || s.MaxPinnedTasks > model.MaxMaxPinned {
		s.MaxPinnedTasks = model.DefaultMaxPinned
	}
	if s.MaxPinnedProjects < model.MinMaxPinned || s.MaxPinnedProjects > model.MaxMaxPinned {
		s.MaxPinnedProjects = model.DefaultMaxPinned
	}
	return &s, nil
}

func jsonObjectHasKey(raw string, key string) bool {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return false
	}
	_, ok := obj[key]
	return ok
}

func (r *UserRepo) SetSettings(ctx context.Context, id int64, s *model.UserSettings) error {
	if s.WeeklyUnplannedExcludedLabelIDs == nil {
		s.WeeklyUnplannedExcludedLabelIDs = []int64{}
	}
	if s.BugLabelIDs == nil {
		s.BugLabelIDs = []int64{}
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("encode user settings: %w", err)
	}
	now := model.FormatUTC(time.Now())
	res, err := r.db.ExecContext(ctx,
		`UPDATE users SET settings = ?, updated_at = ? WHERE id = ?`, string(raw), now, id)
	if err != nil {
		return fmt.Errorf("set user settings: %w", err)
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

func (r *UserRepo) GetState(ctx context.Context, id int64) (string, error) {
	var state string
	err := r.db.QueryRowContext(ctx, `SELECT state FROM users WHERE id = ?`, id).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get user state: %w", err)
	}
	return state, nil
}

func (r *UserRepo) SetState(ctx context.Context, id int64, state string) error {
	now := model.FormatUTC(time.Now())
	res, err := r.db.ExecContext(ctx,
		`UPDATE users SET state = ?, updated_at = ? WHERE id = ?`, state, now, id)
	if err != nil {
		return fmt.Errorf("set user state: %w", err)
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

type TroikiCapacity struct {
	Medium  int
	Rest    int
	Started bool
}

func (r *UserRepo) GetTroikiCapacity(ctx context.Context, id int64) (TroikiCapacity, error) {
	var c TroikiCapacity
	var startedInt int64
	err := r.db.QueryRowContext(ctx,
		`SELECT troiki_medium_capacity, troiki_rest_capacity, troiki_started FROM users WHERE id = ?`, id).
		Scan(&c.Medium, &c.Rest, &startedInt)
	if errors.Is(err, sql.ErrNoRows) {
		return TroikiCapacity{}, ErrNotFound
	}
	if err != nil {
		return TroikiCapacity{}, fmt.Errorf("get troiki capacity: %w", err)
	}
	c.Started = startedInt != 0
	return c, nil
}

// StartTroiki snapshots medium/rest capacities to the given counts and flips
// troiki_started=1 in a single UPDATE. Idempotent: re-calling on an already
// started user is a no-op (WHERE troiki_started = 0 guards against re-snapshot
// that would clobber capacities earned by completions after start).
func (r *UserRepo) StartTroiki(ctx context.Context, id int64, mediumCap, restCap int) error {
	now := model.FormatUTC(time.Now())
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET troiki_medium_capacity = ?, troiki_rest_capacity = ?,
		    troiki_started = 1, updated_at = ?
		 WHERE id = ? AND troiki_started = 0`,
		mediumCap, restCap, now, id)
	if err != nil {
		return fmt.Errorf("start troiki: %w", err)
	}
	return nil
}

// ResetTroiki flips troiki_started back to 0 and zeros the medium/rest capacity
// counters in a single UPDATE. Idempotent — safe to call on an already-reset
// user. Used by Troiki Reset.
func (r *UserRepo) ResetTroiki(ctx context.Context, id int64) error {
	now := model.FormatUTC(time.Now())
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET troiki_started = 0, troiki_medium_capacity = 0,
		    troiki_rest_capacity = 0, updated_at = ?
		 WHERE id = ?`,
		now, id)
	if err != nil {
		return fmt.Errorf("reset troiki: %w", err)
	}
	return nil
}

// IncTroikiCapacity bumps the capacity counter for the given target category
// by 1. Only medium and rest are stored; important is a fixed constant.
func (r *UserRepo) IncTroikiCapacity(ctx context.Context, id int64, target model.TroikiCategory) error {
	var col string
	switch target {
	case model.TroikiCategoryMedium:
		col = "troiki_medium_capacity"
	case model.TroikiCategoryRest:
		col = "troiki_rest_capacity"
	default:
		return fmt.Errorf("inc troiki capacity: unsupported target %q", target)
	}
	now := model.FormatUTC(time.Now())
	res, err := r.db.ExecContext(ctx,
		`UPDATE users SET `+col+` = `+col+` + 1, updated_at = ? WHERE id = ?`, now, id)
	if err != nil {
		return fmt.Errorf("inc troiki capacity: %w", err)
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

type TOTPState struct {
	Secret       string
	Enabled      bool
	EnabledAt    *time.Time
	LastUsedStep int64
}

func (r *UserRepo) GetTOTPState(ctx context.Context, id int64) (*TOTPState, error) {
	const op = "repo.users.GetTOTPState"
	logQuery(ctx, op, id)
	var secret string
	var enabledInt, lastStep int64
	var enabledAt sql.NullString
	err := r.db.QueryRowContext(ctx,
		`SELECT totp_secret, totp_enabled, totp_enabled_at, totp_last_used_step FROM users WHERE id = ?`, id).
		Scan(&secret, &enabledInt, &enabledAt, &lastStep)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, logErr(ctx, op, ErrNotFound)
	}
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("get totp state: %w", err))
	}
	s := &TOTPState{Secret: secret, Enabled: enabledInt != 0, LastUsedStep: lastStep}
	if enabledAt.Valid && enabledAt.String != "" {
		t, err := model.ParseUTC(enabledAt.String)
		if err != nil {
			return nil, logErr(ctx, op, fmt.Errorf("parse totp_enabled_at: %w", err))
		}
		s.EnabledAt = &t
	}
	return s, nil
}

// AdvanceTOTPLastUsedStep atomically updates totp_last_used_step to step only
// when step is strictly greater than the currently stored value. Returns true
// when the row was updated (the step is fresh and was accepted). Returns false
// when the step is stale (replay) — the caller must reject the code.
func (r *UserRepo) AdvanceTOTPLastUsedStep(ctx context.Context, id, step int64) (bool, error) {
	const op = "repo.users.AdvanceTOTPLastUsedStep"
	logQuery(ctx, op, id, step)
	res, err := r.db.ExecContext(ctx,
		`UPDATE users SET totp_last_used_step = ? WHERE id = ? AND totp_last_used_step < ?`,
		step, id, step)
	if err != nil {
		return false, logErr(ctx, op, fmt.Errorf("advance totp last used step: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// SetTOTPSecret stores the encrypted TOTP secret without enabling 2FA.
// Used during setup to persist the secret before the user confirms with a code.
//
// The CAS on totp_enabled = 0 prevents a stale concurrent setup from overwriting
// the live secret of an already-enrolled user: if another request enables TOTP
// between this caller's pre-check and write, the write fails with ErrNotFound
// and the caller must surface "already enabled" to the user.
func (r *UserRepo) SetTOTPSecret(ctx context.Context, id int64, encryptedSecret string) error {
	const op = "repo.users.SetTOTPSecret"
	logQuery(ctx, op, id)
	now := model.FormatUTC(time.Now())
	res, err := r.db.ExecContext(ctx,
		`UPDATE users SET totp_secret = ?, updated_at = ? WHERE id = ? AND totp_enabled = 0`,
		encryptedSecret, now, id)
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("set totp secret: %w", err))
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

// EnableTOTP marks 2FA enabled for the user, but only if the stored
// encrypted secret still matches expectedSecret. This guards against a
// concurrent BeginSetup overwriting the secret between confirm-time
// verification and the enable commit — without the CAS the user could be
// enrolled with a secret different from the one they just scanned. Returns
// ErrNotFound when the row is missing OR when the secret has changed (the
// caller must restart setup).
func (r *UserRepo) EnableTOTP(ctx context.Context, id int64, expectedSecret string) error {
	const op = "repo.users.EnableTOTP"
	logQuery(ctx, op, id)
	now := model.FormatUTC(time.Now())
	res, err := r.db.ExecContext(ctx,
		`UPDATE users SET totp_enabled = 1, totp_enabled_at = ?, updated_at = ? WHERE id = ? AND totp_secret = ?`,
		now, now, id, expectedSecret)
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("enable totp: %w", err))
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

// EnableTOTPWithRecoveryCodes atomically enables TOTP and replaces the user's
// recovery codes in a single transaction. The CAS on totp_secret guards against
// a concurrent BeginSetup overwriting the secret between confirm-time
// verification and the enable commit. The CAS on totp_enabled = 0 additionally
// serializes concurrent ConfirmSetup calls so only one set of recovery codes
// wins — without it two confirms using codes from adjacent skew steps could
// both pass AdvanceTOTPLastUsedStep and the later transaction would silently
// overwrite the earlier caller's recovery-code set. Returns ErrNotFound when
// the row is missing, when the secret has changed, or when TOTP was already
// enabled (the caller must surface "already enabled").
//
// Atomicity matters: a non-atomic "enable, then replace codes" can leave the
// user with totp_enabled=1 but no fresh recovery codes if the second write
// fails, locking them out because subsequent ConfirmSetup calls reject with
// ErrAlreadyEnabled.
func (r *UserRepo) EnableTOTPWithRecoveryCodes(ctx context.Context, id int64, expectedSecret string, codeHashes []string) error {
	const op = "repo.users.EnableTOTPWithRecoveryCodes"
	logQuery(ctx, op, id)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("begin: %w", err))
	}
	defer func() { _ = tx.Rollback() }()

	now := model.FormatUTC(time.Now())
	res, err := tx.ExecContext(ctx,
		`UPDATE users SET totp_enabled = 1, totp_enabled_at = ?, updated_at = ? WHERE id = ? AND totp_secret = ? AND totp_enabled = 0`,
		now, now, id, expectedSecret)
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("enable totp: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return logErr(ctx, op, err)
	}
	if n == 0 {
		return ErrNotFound
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM totp_recovery_codes WHERE user_id = ?`, id); err != nil {
		return logErr(ctx, op, fmt.Errorf("delete recovery: %w", err))
	}
	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO totp_recovery_codes (user_id, code_hash, created_at) VALUES (?, ?, ?)`)
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("prepare recovery: %w", err))
	}
	defer func() { _ = stmt.Close() }()
	for _, h := range codeHashes {
		if _, err := stmt.ExecContext(ctx, id, h, now); err != nil {
			return logErr(ctx, op, fmt.Errorf("insert recovery: %w", err))
		}
	}
	if err := tx.Commit(); err != nil {
		return logErr(ctx, op, fmt.Errorf("commit: %w", err))
	}
	return nil
}

func (r *UserRepo) DisableTOTP(ctx context.Context, id int64) error {
	const op = "repo.users.DisableTOTP"
	logQuery(ctx, op, id)
	now := model.FormatUTC(time.Now())
	res, err := r.db.ExecContext(ctx,
		`UPDATE users SET totp_secret = '', totp_enabled = 0, totp_enabled_at = NULL, totp_last_used_step = 0, updated_at = ? WHERE id = ?`,
		now, id)
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("disable totp: %w", err))
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

func (r *UserRepo) UpdatePasswordHash(ctx context.Context, id int64, passwordHash string) error {
	now := model.FormatUTC(time.Now())
	res, err := r.db.ExecContext(ctx,
		`UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`,
		passwordHash, now, id)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
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

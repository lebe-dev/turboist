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
	var startedInt int64
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash,
		&u.TroikiMediumCapacity, &u.TroikiRestCapacity, &startedInt,
		&createdAt, &updatedAt); err != nil {
		return nil, err
	}
	u.TroikiStarted = startedInt != 0
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

func (r *UserRepo) Exists(ctx context.Context) (bool, error) {
	var n int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return false, fmt.Errorf("count users: %w", err)
	}
	return n > 0, nil
}

func (r *UserRepo) Create(ctx context.Context, username, passwordHash string) (*model.User, error) {
	now := model.FormatUTC(time.Now())
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO users (id, username, password_hash, created_at, updated_at) VALUES (1, ?, ?, ?, ?)`,
		username, passwordHash, now, now)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("insert user: %w", err)
	}
	return r.Get(ctx, 1)
}

func (r *UserRepo) Get(ctx context.Context, id int64) (*model.User, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, username, password_hash, troiki_medium_capacity, troiki_rest_capacity, troiki_started, created_at, updated_at FROM users WHERE id = ?`, id)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (r *UserRepo) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, username, password_hash, troiki_medium_capacity, troiki_rest_capacity, troiki_started, created_at, updated_at FROM users WHERE username = ?`, username)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
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
			return &model.UserSettings{}, nil
		}
	}
	if s.WeeklyUnplannedExcludedLabelIDs == nil {
		s.WeeklyUnplannedExcludedLabelIDs = []int64{}
	}
	return &s, nil
}

func (r *UserRepo) SetSettings(ctx context.Context, id int64, s *model.UserSettings) error {
	if s.WeeklyUnplannedExcludedLabelIDs == nil {
		s.WeeklyUnplannedExcludedLabelIDs = []int64{}
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

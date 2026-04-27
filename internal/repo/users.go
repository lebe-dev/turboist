package repo

import (
	"context"
	"database/sql"
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
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &createdAt, &updatedAt); err != nil {
		return nil, err
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
		`SELECT id, username, password_hash, created_at, updated_at FROM users WHERE id = ?`, id)
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
		`SELECT id, username, password_hash, created_at, updated_at FROM users WHERE username = ?`, username)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
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

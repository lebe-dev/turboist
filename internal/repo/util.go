package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

func joinSets(sets []string) string {
	return strings.Join(sets, ", ")
}

// withTx runs fn inside a transaction, committing on success and rolling back
// on error or panic. Mirrors db.WithTx but lives in the repo package so the
// soft-delete cascade helpers can stay self-contained without importing db.
func withTx(ctx context.Context, sqlDB *sql.DB, fn func(*sql.Tx) error) (err error) {
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback()
			return
		}
		err = tx.Commit()
	}()
	err = fn(tx)
	return err
}

// NotFoundOrGone classifies a missing-row lookup for the given table id: it
// returns ErrGone if the row exists but is soft-deleted (the tombstone is final
// — re-edit is a 410), otherwise ErrNotFound. Handlers call this when an initial
// Get (which filters tombstones) reports the row missing, so a PATCH/DELETE on a
// tombstone surfaces as 410 rather than 404 (Federation v1 F0.1, US-3.7 AC2).
func (r *TaskRepo) NotFoundOrGone(ctx context.Context, id int64) error {
	return notFoundOrGone(ctx, r.db, "tasks", id)
}

func (r *ProjectRepo) NotFoundOrGone(ctx context.Context, id int64) error {
	return notFoundOrGone(ctx, r.db, "projects", id)
}

func (r *ProjectSectionRepo) NotFoundOrGone(ctx context.Context, id int64) error {
	return notFoundOrGone(ctx, r.db, "project_sections", id)
}

func (r *LabelRepo) NotFoundOrGone(ctx context.Context, id int64) error {
	return notFoundOrGone(ctx, r.db, "labels", id)
}

func (r *ContextRepo) NotFoundOrGone(ctx context.Context, id int64) error {
	return notFoundOrGone(ctx, r.db, "contexts", id)
}

// notFoundOrGone disambiguates a zero-rows-affected mutation: if the row exists
// but is soft-deleted it returns ErrGone (the tombstone is final — re-edit is a
// 410), otherwise the row genuinely does not exist and it returns ErrNotFound.
func notFoundOrGone(ctx context.Context, sqlDB *sql.DB, table string, id int64) error {
	return notFoundOrGoneQ(ctx, sqlDB, table, id)
}

// notFoundOrGoneTx is notFoundOrGone inside a caller's transaction. The tx-aware
// variants (CreateTx/UpdateTx/DeleteTx) MUST use this rather than the *sql.DB
// form: on SetMaxOpenConns(1) the open tx holds the lone connection, so a
// disambiguation read issued against the pool would deadlock.
func notFoundOrGoneTx(ctx context.Context, tx *sql.Tx, table string, id int64) error {
	return notFoundOrGoneQ(ctx, tx, table, id)
}

// rowQuerier is the subset of *sql.DB / *sql.Tx the tombstone disambiguation
// read needs, so it can run on the pool or inside a caller's transaction.
type rowQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// requireLiveTx reports the liveness of a row inside a caller's transaction for
// a no-op mutation (e.g. an empty PATCH): a live row → nil, a soft-deleted row →
// ErrGone (the tombstone is final, US-3.7 AC2), an absent row → ErrNotFound.
// Unlike notFoundOrGone (which assumes 0 rows were affected and so maps a live
// row to ErrNotFound), this distinguishes the live case and is the correct guard
// for a no-op update that touched no columns.
func requireLiveTx(ctx context.Context, tx *sql.Tx, table string, id int64) error {
	var deleted sql.NullString
	err := tx.QueryRowContext(ctx,
		"SELECT deleted_at FROM "+table+" WHERE id = ?", id).Scan(&deleted)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return logErr(ctx, "repo.requireLiveTx", fmt.Errorf("liveness %s: %w", table, err))
	}
	if deleted.Valid {
		return ErrGone
	}
	return nil
}

func notFoundOrGoneQ(ctx context.Context, q rowQuerier, table string, id int64) error {
	var deleted sql.NullString
	err := q.QueryRowContext(ctx,
		"SELECT deleted_at FROM "+table+" WHERE id = ?", id).Scan(&deleted)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		// A real DB failure during tombstone disambiguation must NOT be returned
		// as a bare error a caller might mistake for "not found" — log it and
		// return it as a non-sentinel error so the handler surfaces a 500, not a
		// masked 404 (Federation v1 F0.1 follow-up).
		return logErr(ctx, "repo.notFoundOrGone", fmt.Errorf("disambiguate %s tombstone: %w", table, err))
	}
	if deleted.Valid {
		return ErrGone
	}
	return ErrNotFound
}

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

// FederationInviteRepo persists per-project share invites (Federation v1 F1.2,
// US-1.2). The invite secret is NEVER stored in plaintext — only its SHA-256
// hash (secret_hash); the table has no plaintext column. invite_id is a UUIDv7.
type FederationInviteRepo struct {
	db *sql.DB
}

func NewFederationInviteRepo(db *sql.DB) *FederationInviteRepo {
	return &FederationInviteRepo{db: db}
}

const federationInviteColumns = `invite_id, local_project_id, secret_hash, permissions, max_uses, used_count, expires_at, revoked_at, consumed_at, created_at`

func scanFederationInvite(row interface{ Scan(...any) error }) (*model.FederationInvite, error) {
	var inv model.FederationInvite
	var permissions string
	var expiresAt, revokedAt, consumedAt sql.NullString
	var createdAt string
	if err := row.Scan(
		&inv.InviteID, &inv.LocalProjectID, &inv.SecretHash, &permissions,
		&inv.MaxUses, &inv.UsedCount, &expiresAt, &revokedAt, &consumedAt, &createdAt,
	); err != nil {
		return nil, err
	}
	inv.Permissions = model.FederationPermission(permissions)

	t, err := model.ParseUTC(createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	inv.CreatedAt = t

	if inv.ExpiresAt, err = parseNullableUTC(expiresAt); err != nil {
		return nil, fmt.Errorf("parse expires_at: %w", err)
	}
	if inv.RevokedAt, err = parseNullableUTC(revokedAt); err != nil {
		return nil, fmt.Errorf("parse revoked_at: %w", err)
	}
	if inv.ConsumedAt, err = parseNullableUTC(consumedAt); err != nil {
		return nil, fmt.Errorf("parse consumed_at: %w", err)
	}
	return &inv, nil
}

// parseNullableUTC parses an optional ISO-8601 UTC timestamp column, returning
// nil when the column is NULL.
func parseNullableUTC(s sql.NullString) (*time.Time, error) {
	if !s.Valid {
		return nil, nil
	}
	t, err := model.ParseUTC(s.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// Create inserts a new invite. The secret_hash is supplied by the caller
// (already SHA-256-hashed by the service); this repo never sees the plaintext.
func (r *FederationInviteRepo) Create(ctx context.Context, inv model.FederationInvite) (*model.FederationInvite, error) {
	const op = "repo.federation_invites.Create"
	logQuery(ctx, op, inv.InviteID, inv.LocalProjectID)

	_, err := r.db.ExecContext(ctx,
		`INSERT INTO federation_invites
		   (invite_id, local_project_id, secret_hash, permissions, max_uses, used_count, expires_at, revoked_at, consumed_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, NULL, NULL, ?)`,
		inv.InviteID, inv.LocalProjectID, inv.SecretHash, string(inv.Permissions),
		inv.MaxUses, inv.UsedCount, formatNullableUTC(inv.ExpiresAt), model.FormatUTC(inv.CreatedAt))
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("insert invite: %w", err))
	}
	return r.Get(ctx, inv.InviteID)
}

// formatNullableUTC renders an optional timestamp to its ISO-8601 UTC string, or
// nil when the pointer is nil (so the column is stored as SQL NULL).
func formatNullableUTC(t *time.Time) any {
	if t == nil {
		return nil
	}
	return model.FormatUTC(*t)
}

// Get returns a single invite by id, or ErrNotFound when it does not exist.
func (r *FederationInviteRepo) Get(ctx context.Context, inviteID string) (*model.FederationInvite, error) {
	const op = "repo.federation_invites.Get"
	logQuery(ctx, op, inviteID)
	row := r.db.QueryRowContext(ctx,
		`SELECT `+federationInviteColumns+` FROM federation_invites WHERE invite_id = ?`, inviteID)
	inv, err := scanFederationInvite(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, logErr(ctx, op, err)
	}
	return inv, nil
}

// Revoke stamps revoked_at on an invite (Federation v1 F1.3, US-1.3 AC2). It is
// idempotent: an already-revoked invite keeps its original revoked_at (the
// UPDATE only fires WHERE revoked_at IS NULL). An unknown invite_id returns
// ErrNotFound. The supplied at must be the same time.Now() the caller uses for
// other side-effects so timestamps stay coherent.
func (r *FederationInviteRepo) Revoke(ctx context.Context, inviteID string, at time.Time) error {
	const op = "repo.federation_invites.Revoke"
	logQuery(ctx, op, inviteID)

	// First confirm the row exists so a missing id is a 404, not a silent no-op
	// (a no-op UPDATE on an already-revoked invite is the idempotent success path,
	// which we must NOT confuse with "row absent").
	if _, err := r.Get(ctx, inviteID); err != nil {
		return err
	}

	_, err := r.db.ExecContext(ctx,
		`UPDATE federation_invites SET revoked_at = ? WHERE invite_id = ? AND revoked_at IS NULL`,
		model.FormatUTC(at), inviteID)
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("revoke invite: %w", err))
	}
	return nil
}

// Delete hard-removes an invite row (Federation v1 F1.3, US-1.3 AC3). It does
// NOT touch federated_projects — a peer that already consumed the invite stays
// joined. An unknown invite_id returns ErrNotFound.
func (r *FederationInviteRepo) Delete(ctx context.Context, inviteID string) error {
	const op = "repo.federation_invites.Delete"
	logQuery(ctx, op, inviteID)

	res, err := r.db.ExecContext(ctx,
		`DELETE FROM federation_invites WHERE invite_id = ?`, inviteID)
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("delete invite: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("rows affected: %w", err))
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetTx returns a single invite by id inside tx, or ErrNotFound when it does not
// exist. The handshake-consume path (F2.2) reads the invite under the same
// transaction that bumps used_count so the check-and-consume is atomic and a
// concurrent second handshake cannot also consume a single-use invite.
func (r *FederationInviteRepo) GetTx(ctx context.Context, tx *sql.Tx, inviteID string) (*model.FederationInvite, error) {
	const op = "repo.federation_invites.GetTx"
	logQuery(ctx, op, inviteID)
	row := tx.QueryRowContext(ctx,
		`SELECT `+federationInviteColumns+` FROM federation_invites WHERE invite_id = ?`, inviteID)
	inv, err := scanFederationInvite(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, logErr(ctx, op, err)
	}
	return inv, nil
}

// ErrInviteNotConsumable is returned by ConsumeTx when the invite row exists but
// is no longer in the active state at consume time — it is revoked, expired, or
// has already reached max_uses (Federation v1 F2.2, US-2.2 AC3 / US-1.2 AC3).
// The self-guarding UPDATE re-checks the active predicate under the same
// transaction, so this is the signal that a concurrent handshake already
// consumed the last use (the TOCTOU loser) — the service maps it to a generic
// ErrHandshakeInvalid (a 401, no disclosure). It is distinct from ErrNotFound
// (row absent) so a caller can tell "raced/spent" from "unknown id".
var ErrInviteNotConsumable = errors.New("repo: federation invite not consumable")

// ConsumeTx atomically consumes one use of an invite inside tx (Federation v1
// F2.2, US-2.2 AC3 / US-1.2 AC3). The UPDATE is self-guarding: it only fires
// WHERE the invite is still active (not revoked, used_count < max_uses, not
// expired at `at`), bumping used_count by one and stamping consumed_at when this
// use reaches max_uses. The guard is the single-use invariant — it closes the
// check-and-consume TOCTOU window where two concurrent handshakes both read
// used_count=0 outside the tx and then both serialize their consume: the guard
// makes the loser's UPDATE match zero rows. RowsAffected()==0 therefore means
// either the row is absent (ErrNotFound) or it raced to a non-active state
// (ErrInviteNotConsumable); both collapse to a generic invalid handshake at the
// service. It runs in the same transaction as the federated_instances /
// federated_projects upserts so a successful handshake records the consumption
// atomically. at must be the same time.Now() the caller stamps elsewhere so the
// timestamps stay coherent.
func (r *FederationInviteRepo) ConsumeTx(ctx context.Context, tx *sql.Tx, inviteID string, at time.Time) error {
	const op = "repo.federation_invites.ConsumeTx"
	logQuery(ctx, op, inviteID)

	atStr := model.FormatUTC(at)
	res, err := tx.ExecContext(ctx,
		`UPDATE federation_invites
		    SET used_count = used_count + 1,
		        consumed_at = CASE WHEN used_count + 1 >= max_uses THEN ? ELSE consumed_at END
		  WHERE invite_id = ?
		    AND revoked_at IS NULL
		    AND used_count < max_uses
		    AND (expires_at IS NULL OR expires_at > ?)`,
		atStr, inviteID, atStr)
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("consume invite: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("rows affected: %w", err))
	}
	if n == 0 {
		// The row is either absent or no longer active. Distinguish so the
		// service can tell an unknown id from a raced/spent invite (both still
		// surface as a generic invalid handshake to the caller — no disclosure).
		if _, getErr := r.GetTx(ctx, tx, inviteID); errors.Is(getErr, ErrNotFound) {
			return ErrNotFound
		}
		return ErrInviteNotConsumable
	}
	return nil
}

// ListByProject returns every invite for a local project, newest first. The
// secret is never reconstructable from these rows (only the hash is stored).
func (r *FederationInviteRepo) ListByProject(ctx context.Context, localProjectID int64) ([]model.FederationInvite, error) {
	const op = "repo.federation_invites.ListByProject"
	logQuery(ctx, op, localProjectID)
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+federationInviteColumns+` FROM federation_invites WHERE local_project_id = ? ORDER BY created_at DESC`,
		localProjectID)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("list invites: %w", err))
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	out := make([]model.FederationInvite, 0)
	for rows.Next() {
		inv, err := scanFederationInvite(rows)
		if err != nil {
			return nil, logErr(ctx, op, err)
		}
		out = append(out, *inv)
	}
	if err := rows.Err(); err != nil {
		return nil, logErr(ctx, op, err)
	}
	return out, nil
}

package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/model"
)

// FederationKeysRepo persists the single-row (id=1) federation trust-plane
// identity for this instance (Federation v1 F0.3). Like app_settings and the
// totp secret, there is exactly one row; the private seed is stored encrypted.
type FederationKeysRepo struct {
	db *sql.DB
}

func NewFederationKeysRepo(db *sql.DB) *FederationKeysRepo {
	return &FederationKeysRepo{db: db}
}

const federationKeysColumns = `id, public_key, private_seed_enc, node_id, display_name, created_at`

// Ensure lazily generates this instance's federation keypair on first call and
// returns the (possibly pre-existing) identity. Generation is a one-shot
// INSERT OR IGNORE so concurrent or repeated calls never regenerate — the first
// writer wins and subsequent callers read the stored row. defaultDisplayName is
// applied only when the row is first created (typically host(BASE_URL)); it is
// never used to overwrite an already-stored display_name.
//
// Safe under SetMaxOpenConns(1): the INSERT OR IGNORE + SELECT round-trip is a
// short startup operation that does not hold the connection across network I/O.
func (r *FederationKeysRepo) Ensure(ctx context.Context, cipher *crypto.TokenCipher, defaultDisplayName string) (*model.FederationKeys, error) {
	const op = "repo.federation_keys.Ensure"
	logQuery(ctx, op)

	kp, err := crypto.GenerateInstanceKeypair(cipher)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("generate keypair: %w", err))
	}
	nodeID := uuid.New().String()
	now := model.FormatUTC(time.Now())

	// INSERT OR IGNORE: a no-op when the singleton row already exists, so the
	// freshly generated keypair above is simply discarded and the stored one
	// returned by the SELECT below.
	_, err = r.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO federation_keys (id, public_key, private_seed_enc, node_id, display_name, created_at)
		 VALUES (1, ?, ?, ?, ?, ?)`,
		kp.PublicKey, kp.PrivateSeedEnc, nodeID, defaultDisplayName, now)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("insert federation keys: %w", err))
	}
	return r.Get(ctx)
}

// Get returns the stored federation identity. Returns ErrNotFound when Ensure
// has not yet run (no keypair has been generated).
func (r *FederationKeysRepo) Get(ctx context.Context) (*model.FederationKeys, error) {
	const op = "repo.federation_keys.Get"
	logQuery(ctx, op)

	row := r.db.QueryRowContext(ctx,
		`SELECT `+federationKeysColumns+` FROM federation_keys WHERE id = 1`)
	fk, err := scanFederationKeys(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("get federation keys: %w", err))
	}
	return fk, nil
}

func scanFederationKeys(row interface{ Scan(...any) error }) (*model.FederationKeys, error) {
	var fk model.FederationKeys
	var createdAt string
	if err := row.Scan(&fk.ID, &fk.PublicKey, &fk.PrivateSeedEnc, &fk.NodeID, &fk.DisplayName, &createdAt); err != nil {
		return nil, err
	}
	t, err := model.ParseUTC(createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	fk.CreatedAt = t
	return &fk, nil
}

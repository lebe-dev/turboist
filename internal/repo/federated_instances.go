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

// FederatedInstanceRepo persists the trust directory of peer instances we have
// shaken hands with (Federation v1 F1.4), keyed by instance_url. display_name is
// the human-readable name the peer carried in its handshake (R24) and is the
// source for the "display_name @ instance.tld" rendering (US-1.4 AC2). The
// handshake (Phase 2) is the writer; F1.4 reads the directory to join peer rows.
type FederatedInstanceRepo struct {
	db *sql.DB
}

func NewFederatedInstanceRepo(db *sql.DB) *FederatedInstanceRepo {
	return &FederatedInstanceRepo{db: db}
}

const federatedInstanceColumns = `instance_url, public_key, display_name, last_contact_at, created_at, updated_at`

func scanFederatedInstance(row interface{ Scan(...any) error }) (*model.FederatedInstance, error) {
	var inst model.FederatedInstance
	var lastContact sql.NullString
	var createdAt, updatedAt string
	if err := row.Scan(
		&inst.InstanceURL, &inst.PublicKey, &inst.DisplayName, &lastContact, &createdAt, &updatedAt,
	); err != nil {
		return nil, err
	}
	if lastContact.Valid && lastContact.String != "" {
		t, err := model.ParseUTC(lastContact.String)
		if err != nil {
			return nil, fmt.Errorf("parse last_contact_at: %w", err)
		}
		inst.LastContactAt = &t
	}
	c, err := model.ParseUTC(createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	inst.CreatedAt = c
	u, err := model.ParseUTC(updatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}
	inst.UpdatedAt = u
	return &inst, nil
}

// Upsert inserts (or, when the instance_url already exists, updates) a peer
// directory row. It is idempotent on instance_url: re-shaking hands with a peer
// refreshes its public_key, display_name, and last_contact_at without
// duplicating the row. created_at is preserved on conflict.
func (r *FederatedInstanceRepo) Upsert(ctx context.Context, inst model.FederatedInstance) error {
	const op = "repo.federated_instances.Upsert"
	logQuery(ctx, op, inst.InstanceURL)
	var lastContact any
	if inst.LastContactAt != nil {
		lastContact = model.FormatUTC(*inst.LastContactAt)
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO federated_instances (instance_url, public_key, display_name, last_contact_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(instance_url) DO UPDATE SET
		   public_key = excluded.public_key,
		   display_name = excluded.display_name,
		   last_contact_at = excluded.last_contact_at,
		   updated_at = excluded.updated_at`,
		inst.InstanceURL, inst.PublicKey, inst.DisplayName, lastContact,
		model.FormatUTC(inst.CreatedAt), model.FormatUTC(inst.UpdatedAt))
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("upsert instance: %w", err))
	}
	return nil
}

// UpsertTx is Upsert inside a transaction (Federation v1 F2.2). The owner
// handshake records the joining peer's directory row (incl. its handshake-
// supplied display_name, R24) in the SAME tx that consumes the invite and
// inserts the federated_projects mapping, so the three never diverge. On
// conflict it refreshes public_key, display_name, and last_contact_at and
// preserves created_at — re-shaking hands with a known peer updates it in place.
func (r *FederatedInstanceRepo) UpsertTx(ctx context.Context, tx *sql.Tx, inst model.FederatedInstance) error {
	const op = "repo.federated_instances.UpsertTx"
	logQuery(ctx, op, inst.InstanceURL)
	var lastContact any
	if inst.LastContactAt != nil {
		lastContact = model.FormatUTC(*inst.LastContactAt)
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO federated_instances (instance_url, public_key, display_name, last_contact_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(instance_url) DO UPDATE SET
		   public_key = excluded.public_key,
		   display_name = excluded.display_name,
		   last_contact_at = excluded.last_contact_at,
		   updated_at = excluded.updated_at`,
		inst.InstanceURL, inst.PublicKey, inst.DisplayName, lastContact,
		model.FormatUTC(inst.CreatedAt), model.FormatUTC(inst.UpdatedAt))
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("upsert instance: %w", err))
	}
	return nil
}

// List returns every peer directory row, ordered by instance_url. It backs the
// startup peer-key cache warm (Federation v1 F4.3 review fix): warming the cache
// from the persisted public_key of every joined peer means the first inbound
// event after a restart verifies against the pinned key instead of triggering a
// cold-cache .well-known fetch — so a real signature mismatch is a genuine key
// rotation, not a transient cold-start fetch error.
func (r *FederatedInstanceRepo) List(ctx context.Context) ([]model.FederatedInstance, error) {
	const op = "repo.federated_instances.List"
	logQuery(ctx, op, "")
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+federatedInstanceColumns+` FROM federated_instances ORDER BY instance_url`)
	if err != nil {
		return nil, logErr(ctx, op, err)
	}
	defer logging.LogClose(ctx, op+".rows", rows)
	var out []model.FederatedInstance
	for rows.Next() {
		inst, err := scanFederatedInstance(rows)
		if err != nil {
			return nil, logErr(ctx, op, err)
		}
		out = append(out, *inst)
	}
	if err := rows.Err(); err != nil {
		return nil, logErr(ctx, op, err)
	}
	return out, nil
}

// TouchLastContact stamps an EXISTING peer directory row's last_contact_at (and
// updated_at) to `at` without touching its public_key or display_name (Federation
// v1 F5.6a, US-6.5 AC1/AC3). It is the freshness touchpoint fired on every
// successful exchange with a peer — an inbound push received from it, a successful
// pull from it — so a joiner's owner-offline derivation (and the owner's per-peer
// stale status) reflects real recency. It is a no-op (rows affected 0, nil error)
// for a peer not in the directory: a touch must never CREATE a trust-directory row
// for an instance we have not shaken hands with (only the handshake/join inserts).
// Idempotent and monotonic-by-caller: callers pass model.FormatUTC(now).
func (r *FederatedInstanceRepo) TouchLastContact(ctx context.Context, instanceURL string, at time.Time) error {
	const op = "repo.federated_instances.TouchLastContact"
	logQuery(ctx, op, instanceURL)
	atStr := model.FormatUTC(at)
	_, err := r.db.ExecContext(ctx,
		`UPDATE federated_instances SET last_contact_at = ?, updated_at = ? WHERE instance_url = ?`,
		atStr, atStr, instanceURL)
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("touch last contact: %w", err))
	}
	return nil
}

// UpdatePublicKey overwrites an EXISTING peer's pinned public_key (and advances
// updated_at) without touching its display_name, last_contact_at, or created_at
// (Federation v1 F5.6b, US-6.4 AC3). It is the durable half of the manual
// "Trust new key" action: the service fetches the peer's new .well-known key and
// persists it here (the in-memory peer-key cache is updated separately via
// Cache.Trust). It returns the affected-row count so the service maps an unknown
// peer (0 rows) to a 404; like TouchLastContact it never CREATES a row — only the
// handshake/join inserts the directory.
func (r *FederatedInstanceRepo) UpdatePublicKey(ctx context.Context, instanceURL, publicKey string, at time.Time) (int, error) {
	const op = "repo.federated_instances.UpdatePublicKey"
	logQuery(ctx, op, instanceURL)
	res, err := r.db.ExecContext(ctx,
		`UPDATE federated_instances SET public_key = ?, updated_at = ? WHERE instance_url = ?`,
		publicKey, model.FormatUTC(at), instanceURL)
	if err != nil {
		return 0, logErr(ctx, op, fmt.Errorf("update public key: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, logErr(ctx, op, fmt.Errorf("update public key rows: %w", err))
	}
	return int(n), nil
}

// Get returns the directory row for a single peer instance_url, or ErrNotFound
// when no row exists.
func (r *FederatedInstanceRepo) Get(ctx context.Context, instanceURL string) (*model.FederatedInstance, error) {
	const op = "repo.federated_instances.Get"
	logQuery(ctx, op, instanceURL)
	row := r.db.QueryRowContext(ctx,
		`SELECT `+federatedInstanceColumns+` FROM federated_instances WHERE instance_url = ?`,
		instanceURL)
	inst, err := scanFederatedInstance(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, logErr(ctx, op, err)
	}
	return inst, nil
}

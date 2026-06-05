package repo

import (
	"context"
	"database/sql"
	"fmt"
)

// FederationRetentionSettings is the persisted, owner-configurable retention
// window override (Federation v1 F6.5, US-8.4). A nil field means "fall back to
// the config / compiled default" so a fresh install behaves exactly as before;
// the live values are clamped (outbox hardcap 30d, §16.3) in the service layer,
// never honored verbatim. Days are stored as INTEGER; a non-positive stored value
// is treated the same as nil (default) by the consumer.
type FederationRetentionSettings struct {
	TombstoneRetentionDays *int
	OutboxRetentionDays    *int
	InboxRetentionDays     *int
}

// FederationRetentionSettingsRepo persists the single-row (id=1) retention
// override table seeded empty by migration 040.
type FederationRetentionSettingsRepo struct {
	db *sql.DB
}

func NewFederationRetentionSettingsRepo(db *sql.DB) *FederationRetentionSettingsRepo {
	return &FederationRetentionSettingsRepo{db: db}
}

// Get returns the persisted retention overrides. A NULL column maps to a nil
// pointer (default applies). The seeded row always exists (migration 040), but a
// missing row is treated as all-default so the call never errors a fresh install.
func (r *FederationRetentionSettingsRepo) Get(ctx context.Context) (FederationRetentionSettings, error) {
	const op = "repo.federation_retention_settings.Get"
	logQuery(ctx, op)
	var tomb, outbox, inbox sql.NullInt64
	err := r.db.QueryRowContext(ctx,
		`SELECT tombstone_retention_days, outbox_retention_days, inbox_retention_days
		   FROM federation_retention_settings WHERE id = 1`).Scan(&tomb, &outbox, &inbox)
	if err == sql.ErrNoRows {
		return FederationRetentionSettings{}, nil
	}
	if err != nil {
		return FederationRetentionSettings{}, logErr(ctx, op, fmt.Errorf("get retention settings: %w", err))
	}
	out := FederationRetentionSettings{}
	if tomb.Valid {
		v := int(tomb.Int64)
		out.TombstoneRetentionDays = &v
	}
	if outbox.Valid {
		v := int(outbox.Int64)
		out.OutboxRetentionDays = &v
	}
	if inbox.Valid {
		v := int(inbox.Int64)
		out.InboxRetentionDays = &v
	}
	return out, nil
}

// Set persists the retention overrides on the single id=1 row (US-8.4 — the admin
// PATCH writer). A nil field stores NULL (revert to default). updated_at stamps
// the change with a wall-clock ISO-8601 UTC timestamp. The row is upserted so the
// call succeeds even if migration 040's seed row was somehow absent.
func (r *FederationRetentionSettingsRepo) Set(ctx context.Context, s FederationRetentionSettings, now string) error {
	const op = "repo.federation_retention_settings.Set"
	logQuery(ctx, op)
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO federation_retention_settings (id, tombstone_retention_days, outbox_retention_days, inbox_retention_days, updated_at)
		 VALUES (1, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   tombstone_retention_days = excluded.tombstone_retention_days,
		   outbox_retention_days    = excluded.outbox_retention_days,
		   inbox_retention_days     = excluded.inbox_retention_days,
		   updated_at               = excluded.updated_at`,
		nullableInt(s.TombstoneRetentionDays), nullableInt(s.OutboxRetentionDays), nullableInt(s.InboxRetentionDays), now)
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("set retention settings: %w", err))
	}
	return nil
}

// nullableInt maps a *int to a sql NULL when nil, or the value otherwise.
func nullableInt(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

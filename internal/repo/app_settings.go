package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/lebe-dev/turboist/internal/model"
)

// AppSettingsRepo persists global application settings as a JSON blob in the
// single-row app_settings table (id=1).
type AppSettingsRepo struct {
	db *sql.DB
}

func NewAppSettingsRepo(db *sql.DB) *AppSettingsRepo {
	return &AppSettingsRepo{db: db}
}

// normalizeAppSettings replaces nil rule slices with empty ones so callers and
// JSON responses always see `[]` instead of `null`.
func normalizeAppSettings(s *model.AppSettings) *model.AppSettings {
	if s.AutoLabels == nil {
		s.AutoLabels = []model.AutoLabelRule{}
	}
	if s.ProjectSuggestions == nil {
		s.ProjectSuggestions = []model.ProjectSuggestionRule{}
	}
	return s
}

func (r *AppSettingsRepo) Get(ctx context.Context) (*model.AppSettings, error) {
	const op = "repo.app_settings.Get"
	logQuery(ctx, op)
	var raw string
	err := r.db.QueryRowContext(ctx, `SELECT data FROM app_settings WHERE id = 1`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return normalizeAppSettings(&model.AppSettings{}), nil
	}
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("get app settings: %w", err))
	}
	return decodeAppSettings(raw), nil
}

// decodeAppSettings turns the stored blob into settings. A blob that no longer
// parses degrades to empty rules rather than failing the read: the rules only
// add suggestions, and losing them is better than losing the response.
func decodeAppSettings(raw string) *model.AppSettings {
	var s model.AppSettings
	if raw != "" && raw != "{}" {
		if err := json.Unmarshal([]byte(raw), &s); err != nil {
			return normalizeAppSettings(&model.AppSettings{})
		}
	}
	return normalizeAppSettings(&s)
}

func (r *AppSettingsRepo) Set(ctx context.Context, s *model.AppSettings) error {
	const op = "repo.app_settings.Set"
	logQuery(ctx, op)
	normalizeAppSettings(s)
	raw, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("encode app settings: %w", err)
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO app_settings (id, data) VALUES (1, ?)
		 ON CONFLICT(id) DO UPDATE SET data = excluded.data`, string(raw))
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("set app settings: %w", err))
	}
	return nil
}

// defaultSyncEpoch matches the column default set when the changelog was added.
// A database whose singleton row somehow went missing reads as the starting
// epoch rather than as zero, which no replica would ever have been handed.
const defaultSyncEpoch int64 = 1

// SyncEpoch reads the counter that invalidates every replica cursor at once. A
// replica presenting a cursor stamped with an older epoch is told to start over
// from a full snapshot instead of resuming against a history that was rewritten
// underneath it.
func (r *AppSettingsRepo) SyncEpoch(ctx context.Context) (int64, error) {
	const op = "repo.app_settings.SyncEpoch"
	logQuery(ctx, op)
	var epoch int64
	err := r.db.QueryRowContext(ctx, `SELECT sync_epoch FROM app_settings WHERE id = 1`).Scan(&epoch)
	if errors.Is(err, sql.ErrNoRows) {
		return defaultSyncEpoch, nil
	}
	if err != nil {
		return 0, logErr(ctx, op, fmt.Errorf("get sync epoch: %w", err))
	}
	return epoch, nil
}

// BumpSyncEpoch advances the counter and returns the new value. Call it whenever
// the change history stops describing the data — a restore replaces the rows
// wholesale, so every cursor that survived it points into a history that no
// longer exists. The bump deliberately does not touch the settings blob: it is
// sync bookkeeping, not a settings edit, and must not be logged as one.
func (r *AppSettingsRepo) BumpSyncEpoch(ctx context.Context) (int64, error) {
	const op = "repo.app_settings.BumpSyncEpoch"
	logQuery(ctx, op)
	var epoch int64
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO app_settings (id, data, sync_epoch) VALUES (1, '{}', ?)
		 ON CONFLICT(id) DO UPDATE SET sync_epoch = app_settings.sync_epoch + 1
		 RETURNING sync_epoch`, defaultSyncEpoch+1).Scan(&epoch)
	if err != nil {
		return 0, logErr(ctx, op, fmt.Errorf("bump sync epoch: %w", err))
	}
	return epoch, nil
}

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
	var s model.AppSettings
	if raw != "" && raw != "{}" {
		if err := json.Unmarshal([]byte(raw), &s); err != nil {
			return normalizeAppSettings(&model.AppSettings{}), nil
		}
	}
	return normalizeAppSettings(&s), nil
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

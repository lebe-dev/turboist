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

func (r *AppSettingsRepo) Get(ctx context.Context) (*model.AppSettings, error) {
	var raw string
	err := r.db.QueryRowContext(ctx, `SELECT data FROM app_settings WHERE id = 1`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return &model.AppSettings{AutoLabels: []model.AutoLabelRule{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get app settings: %w", err)
	}
	var s model.AppSettings
	if raw != "" && raw != "{}" {
		if err := json.Unmarshal([]byte(raw), &s); err != nil {
			return &model.AppSettings{AutoLabels: []model.AutoLabelRule{}}, nil
		}
	}
	if s.AutoLabels == nil {
		s.AutoLabels = []model.AutoLabelRule{}
	}
	return &s, nil
}

func (r *AppSettingsRepo) Set(ctx context.Context, s *model.AppSettings) error {
	if s.AutoLabels == nil {
		s.AutoLabels = []model.AutoLabelRule{}
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("encode app settings: %w", err)
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO app_settings (id, data) VALUES (1, ?)
		 ON CONFLICT(id) DO UPDATE SET data = excluded.data`, string(raw))
	if err != nil {
		return fmt.Errorf("set app settings: %w", err)
	}
	return nil
}

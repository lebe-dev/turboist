package repo

import (
	"context"
	"testing"

	"github.com/lebe-dev/turboist/internal/model"
)

func TestAppSettingsRepo_Get_EmptyDB_ReturnsEmpty(t *testing.T) {
	d := setupTestDB(t)
	r := NewAppSettingsRepo(d)

	got, err := r.Get(context.Background())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatalf("Get must return non-nil settings")
	}
	if got.AutoLabels == nil {
		t.Errorf("AutoLabels must be non-nil empty slice, got nil")
	}
	if len(got.AutoLabels) != 0 {
		t.Errorf("AutoLabels length: got %d, want 0", len(got.AutoLabels))
	}
}

func TestAppSettingsRepo_SetThenGet_RoundTrip(t *testing.T) {
	d := setupTestDB(t)
	r := NewAppSettingsRepo(d)
	ctx := context.Background()

	in := &model.AppSettings{
		AutoLabels: []model.AutoLabelRule{
			{Mask: "urgent", LabelIDs: []int64{1, 2}, IgnoreCase: true},
			{Mask: "FIXME", LabelIDs: []int64{3}, IgnoreCase: false},
		},
	}
	if err := r.Set(ctx, in); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := r.Get(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got.AutoLabels) != 2 {
		t.Fatalf("rules: got %d, want 2", len(got.AutoLabels))
	}
	if got.AutoLabels[0].Mask != "urgent" || !got.AutoLabels[0].IgnoreCase ||
		len(got.AutoLabels[0].LabelIDs) != 2 || got.AutoLabels[0].LabelIDs[0] != 1 || got.AutoLabels[0].LabelIDs[1] != 2 {
		t.Errorf("rule[0] mismatch: %+v", got.AutoLabels[0])
	}
	if got.AutoLabels[1].Mask != "FIXME" || got.AutoLabels[1].IgnoreCase ||
		len(got.AutoLabels[1].LabelIDs) != 1 || got.AutoLabels[1].LabelIDs[0] != 3 {
		t.Errorf("rule[1] mismatch: %+v", got.AutoLabels[1])
	}
}

func TestAppSettingsRepo_Set_UpsertOverwrites(t *testing.T) {
	d := setupTestDB(t)
	r := NewAppSettingsRepo(d)
	ctx := context.Background()

	if err := r.Set(ctx, &model.AppSettings{
		AutoLabels: []model.AutoLabelRule{{Mask: "old", LabelIDs: []int64{1}}},
	}); err != nil {
		t.Fatalf("first set: %v", err)
	}
	if err := r.Set(ctx, &model.AppSettings{
		AutoLabels: []model.AutoLabelRule{{Mask: "new", LabelIDs: []int64{42}}},
	}); err != nil {
		t.Fatalf("second set: %v", err)
	}

	got, err := r.Get(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got.AutoLabels) != 1 || got.AutoLabels[0].Mask != "new" || got.AutoLabels[0].LabelIDs[0] != 42 {
		t.Errorf("expected overwritten settings, got %+v", got.AutoLabels)
	}
}

func TestAppSettingsRepo_Set_NilSliceNormalizedToEmpty(t *testing.T) {
	d := setupTestDB(t)
	r := NewAppSettingsRepo(d)
	ctx := context.Background()

	in := &model.AppSettings{AutoLabels: nil}
	if err := r.Set(ctx, in); err != nil {
		t.Fatalf("set: %v", err)
	}
	if in.AutoLabels == nil {
		t.Errorf("Set must normalize nil AutoLabels to empty slice on the input value")
	}
	got, err := r.Get(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.AutoLabels == nil {
		t.Errorf("Get must return non-nil AutoLabels after Set(nil)")
	}
}

func TestAppSettingsRepo_Get_CorruptJSON_ReturnsEmptyDefault(t *testing.T) {
	d := setupTestDB(t)
	r := NewAppSettingsRepo(d)
	ctx := context.Background()

	if _, err := d.ExecContext(ctx,
		`INSERT INTO app_settings (id, data) VALUES (1, ?)
		 ON CONFLICT(id) DO UPDATE SET data = excluded.data`,
		"not-valid-json{{{"); err != nil {
		t.Fatalf("inject: %v", err)
	}
	got, err := r.Get(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil || got.AutoLabels == nil || len(got.AutoLabels) != 0 {
		t.Errorf("expected fallback empty settings, got %+v", got)
	}
}

func TestAppSettingsRepo_Get_EmptyJSONString_ReturnsEmpty(t *testing.T) {
	d := setupTestDB(t)
	r := NewAppSettingsRepo(d)
	ctx := context.Background()

	for _, raw := range []string{"", "{}"} {
		if _, err := d.ExecContext(ctx,
			`INSERT INTO app_settings (id, data) VALUES (1, ?)
			 ON CONFLICT(id) DO UPDATE SET data = excluded.data`, raw); err != nil {
			t.Fatalf("inject %q: %v", raw, err)
		}
		got, err := r.Get(ctx)
		if err != nil {
			t.Fatalf("get %q: %v", raw, err)
		}
		if got == nil || got.AutoLabels == nil || len(got.AutoLabels) != 0 {
			t.Errorf("expected empty AutoLabels for raw=%q, got %+v", raw, got)
		}
	}
}

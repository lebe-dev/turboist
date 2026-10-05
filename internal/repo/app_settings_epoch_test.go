package repo

import (
	"context"
	"testing"

	"github.com/lebe-dev/turboist/internal/model"
)

// A fresh install must hand out a usable epoch, not zero: replicas stamp their
// cursor with whatever they are told, and a zero would compare unequal to the
// value every later read returns.
func TestAppSettingsRepo_SyncEpochStartsAtOne(t *testing.T) {
	d := setupTestDB(t)
	r := NewAppSettingsRepo(d)

	epoch, err := r.SyncEpoch(context.Background())
	if err != nil {
		t.Fatalf("read epoch: %v", err)
	}
	if epoch != 1 {
		t.Errorf("epoch: got %d, want 1", epoch)
	}
}

func TestAppSettingsRepo_BumpSyncEpochAdvancesByOne(t *testing.T) {
	d := setupTestDB(t)
	r := NewAppSettingsRepo(d)
	ctx := context.Background()

	bumped, err := r.BumpSyncEpoch(ctx)
	if err != nil {
		t.Fatalf("bump epoch: %v", err)
	}
	if bumped != 2 {
		t.Errorf("bumped epoch: got %d, want 2", bumped)
	}

	read, err := r.SyncEpoch(ctx)
	if err != nil {
		t.Fatalf("read epoch: %v", err)
	}
	if read != bumped {
		t.Errorf("epoch after bump: got %d, want %d", read, bumped)
	}

	again, err := r.BumpSyncEpoch(ctx)
	if err != nil {
		t.Fatalf("second bump: %v", err)
	}
	if again != 3 {
		t.Errorf("second bumped epoch: got %d, want 3", again)
	}
}

// The epoch and the settings blob share one row, and each write path must leave
// the other alone — a settings edit that reset the epoch would silently force
// every replica into a full resync.
func TestAppSettingsRepo_EpochAndSettingsDoNotOverwriteEachOther(t *testing.T) {
	d := setupTestDB(t)
	r := NewAppSettingsRepo(d)
	ctx := context.Background()

	if _, err := r.BumpSyncEpoch(ctx); err != nil {
		t.Fatalf("bump epoch: %v", err)
	}
	if err := r.Set(ctx, &model.AppSettings{
		AutoLabels: []model.AutoLabelRule{{Mask: "invoice", LabelIDs: []int64{7}}},
	}); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	epoch, err := r.SyncEpoch(ctx)
	if err != nil {
		t.Fatalf("read epoch: %v", err)
	}
	if epoch != 2 {
		t.Errorf("epoch after settings write: got %d, want 2", epoch)
	}

	if _, err := r.BumpSyncEpoch(ctx); err != nil {
		t.Fatalf("second bump: %v", err)
	}
	settings, err := r.Get(ctx)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if len(settings.AutoLabels) != 1 || settings.AutoLabels[0].Mask != "invoice" {
		t.Errorf("settings after epoch bump: got %+v, want the invoice rule intact", settings.AutoLabels)
	}
}

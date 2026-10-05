package service_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
)

func changeLogSize(t *testing.T, d *sql.DB) int {
	t.Helper()
	var n int
	if err := d.QueryRow(`SELECT COUNT(*) FROM change_log`).Scan(&n); err != nil {
		t.Fatalf("count change log: %v", err)
	}
	return n
}

func syncEpoch(t *testing.T, d *sql.DB) int64 {
	t.Helper()
	epoch, err := repo.NewAppSettingsRepo(d).SyncEpoch(context.Background())
	if err != nil {
		t.Fatalf("read sync epoch: %v", err)
	}
	return epoch
}

// A restore swaps the dataset for a different one wearing the same ids. Every
// cursor handed out before it describes a history that no longer applies, so the
// epoch moves and the log starts empty — a replica is made to reseed instead of
// resuming onto rows the backup never contained.
func TestBackupService_RestoreResetsSyncHistory(t *testing.T) {
	src := setupBackupFixtures(t)
	seedSample(t, src)
	ctx := context.Background()
	payload, err := src.svc.Export(ctx, service.ExportOptions{})
	if err != nil {
		t.Fatalf("export source: %v", err)
	}

	dst := setupBackupFixtures(t)
	if _, err := dst.ctxs.Create(ctx, "personal", "green", false); err != nil {
		t.Fatalf("seed dst: %v", err)
	}
	beforeEpoch := syncEpoch(t, dst.db)
	if changeLogSize(t, dst.db) == 0 {
		t.Fatal("precondition: seeding logged no changes")
	}

	if err := dst.svc.Restore(ctx, payload); err != nil {
		t.Fatalf("restore: %v", err)
	}

	if got := syncEpoch(t, dst.db); got != beforeEpoch+1 {
		t.Errorf("sync epoch: got %d, want %d", got, beforeEpoch+1)
	}
	// The wipe and the re-inserts log a row each; none of them may survive,
	// since replaying the restore change by change is exactly what the epoch
	// bump exists to prevent.
	if got := changeLogSize(t, dst.db); got != 0 {
		t.Errorf("change log after restore: got %d rows, want 0", got)
	}
}

// The delta feed is the thing that has to notice. A cursor taken before the
// restore is refused with an epoch mismatch rather than served a page.
func TestBackupService_RestoreExpiresPreRestoreCursors(t *testing.T) {
	f := setupBackupFixtures(t)
	seedSample(t, f)
	ctx := context.Background()
	changes := repo.NewChangeLogRepo(f.db)

	before, err := changes.Changes(ctx, repo.SyncChangesQuery{})
	if err != nil {
		t.Fatalf("changes before restore: %v", err)
	}
	payload, err := f.svc.Export(ctx, service.ExportOptions{})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if err := f.svc.Restore(ctx, payload); err != nil {
		t.Fatalf("restore: %v", err)
	}

	staleEpoch := before.Epoch
	var mismatch *repo.SyncEpochMismatchError
	_, err = changes.Changes(ctx, repo.SyncChangesQuery{Since: before.Cursor, Epoch: &staleEpoch})
	if !errors.As(err, &mismatch) {
		t.Fatalf("pre-restore cursor: got %v, want an epoch-mismatch error", err)
	}
	if mismatch.Current != staleEpoch+1 {
		t.Errorf("current epoch: got %d, want %d", mismatch.Current, staleEpoch+1)
	}
}

// A restore that fails must leave the sync bookkeeping exactly as it was:
// nothing was replaced, so no client has any reason to reseed.
func TestBackupService_FailedRestoreKeepsSyncHistory(t *testing.T) {
	f := setupBackupFixtures(t)
	seedSample(t, f)
	ctx := context.Background()
	beforeEpoch := syncEpoch(t, f.db)
	beforeChanges := changeLogSize(t, f.db)

	// Two contexts sharing a name violate UNIQUE(name), so the transaction
	// fails after the wipe — the same mid-restore failure the rollback test uses.
	bad := &service.BackupPayload{
		Version:    service.BackupSchemaVersion,
		ExportedAt: "2026-05-19T00:00:00.000Z",
		Data: service.BackupData{
			Contexts: []service.BackupContext{
				{ID: 1, Name: "dup", Color: "blue", CreatedAt: "2026-05-19T00:00:00.000Z", UpdatedAt: "2026-05-19T00:00:00.000Z"},
				{ID: 2, Name: "dup", Color: "red", CreatedAt: "2026-05-19T00:00:00.000Z", UpdatedAt: "2026-05-19T00:00:00.000Z"},
			},
		},
	}
	if err := f.svc.Restore(ctx, bad); err == nil {
		t.Fatal("want restore to fail on duplicate context name")
	}

	if got := syncEpoch(t, f.db); got != beforeEpoch {
		t.Errorf("sync epoch after failed restore: got %d, want %d", got, beforeEpoch)
	}
	if got := changeLogSize(t, f.db); got != beforeChanges {
		t.Errorf("change log after failed restore: got %d rows, want %d", got, beforeChanges)
	}
}

package repo

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

// backdateChanges rewrites the timestamp of the log rows up to seq, standing in
// for history that has simply been sitting there for months.
func backdateChanges(t *testing.T, d *sql.DB, upToSeq int64, at time.Time) {
	t.Helper()
	if _, err := d.Exec(`UPDATE change_log SET changed_at = ? WHERE seq <= ?`,
		model.FormatUTC(at), upToSeq); err != nil {
		t.Fatalf("backdate changes: %v", err)
	}
}

func backdateOneChange(t *testing.T, d *sql.DB, seq int64, at time.Time) {
	t.Helper()
	if _, err := d.Exec(`UPDATE change_log SET changed_at = ? WHERE seq = ?`,
		model.FormatUTC(at), seq); err != nil {
		t.Fatalf("backdate change: %v", err)
	}
}

func changeSeqs(t *testing.T, d *sql.DB) []int64 {
	t.Helper()
	rows, err := d.Query(`SELECT seq FROM change_log ORDER BY seq`)
	if err != nil {
		t.Fatalf("read change seqs: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var out []int64
	for rows.Next() {
		var seq int64
		if err := rows.Scan(&seq); err != nil {
			t.Fatalf("scan seq: %v", err)
		}
		out = append(out, seq)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read change seqs: %v", err)
	}
	return out
}

// seedLabelChanges creates n labels, each logging exactly one change row, and
// answers with the seq of the last one.
func seedLabelChanges(t *testing.T, d *sql.DB, n int) int64 {
	t.Helper()
	ctx := context.Background()
	labels := NewLabelRepo(d)
	for i := range n {
		if _, err := labels.Create(ctx, string(rune('a'+i)), "red", false); err != nil {
			t.Fatalf("create label: %v", err)
		}
	}
	return maxChangeSeq(t, d)
}

func TestChangeLogPrune_DropsOnlyHistoryPastTheCutoff(t *testing.T) {
	d := setupTestDB(t)
	head := seedLabelChanges(t, d, 3)
	if head != 3 {
		t.Fatalf("seeded seqs: got head %d, want 3 rows", head)
	}
	backdateChanges(t, d, 2, time.Now().Add(-100*24*time.Hour))

	removed, err := NewChangeLogRepo(d).Prune(context.Background(), time.Now().Add(-SyncHistoryWindow))
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if removed != 2 {
		t.Errorf("removed: got %d, want 2", removed)
	}
	got := changeSeqs(t, d)
	if len(got) != 1 || got[0] != 3 {
		t.Errorf("retained seqs: got %v, want [3]", got)
	}
}

// Everything is inside the window on a young install, and pruning must then be
// a no-op rather than a daily reason for every client to reseed.
func TestChangeLogPrune_KeepsEverythingInsideTheWindow(t *testing.T) {
	d := setupTestDB(t)
	head := seedLabelChanges(t, d, 3)

	removed, err := NewChangeLogRepo(d).Prune(context.Background(), time.Now().Add(-SyncHistoryWindow))
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if removed != 0 {
		t.Errorf("removed: got %d, want 0", removed)
	}
	if got := maxChangeSeq(t, d); got != head {
		t.Errorf("head after prune: got %d, want %d", got, head)
	}
}

// The refusal boundary has to land exactly on what was removed: a cursor that
// already covers every pruned row lost nothing and must keep resuming, while
// the one below it can no longer be caught up and is told so.
func TestChangeLogPrune_ExpiresOnlyCursorsBelowTheBoundary(t *testing.T) {
	d := setupTestDB(t)
	ctx := context.Background()
	seedLabelChanges(t, d, 3)
	backdateChanges(t, d, 2, time.Now().Add(-100*24*time.Hour))

	r := NewChangeLogRepo(d)
	if _, err := r.Prune(ctx, time.Now().Add(-SyncHistoryWindow)); err != nil {
		t.Fatalf("prune: %v", err)
	}

	if _, err := r.Changes(ctx, SyncChangesQuery{Since: 2}); err != nil {
		t.Errorf("cursor on the boundary should still resume: %v", err)
	}

	var expired *SyncCursorExpiredError
	_, err := r.Changes(ctx, SyncChangesQuery{Since: 1})
	if !errors.As(err, &expired) {
		t.Fatalf("cursor below the boundary: got %v, want a cursor-expired error", err)
	}
	if expired.OldestRetained != 3 {
		t.Errorf("oldestRetained: got %d, want 3", expired.OldestRetained)
	}
}

// A clock that steps backwards can stamp a young seq with an old timestamp. The
// prune must still remove a contiguous prefix: leaving a hole would let a cursor
// underneath it resume and never learn about the rows that were removed above.
func TestChangeLogPrune_LeavesNoHoleWhenTimestampsRunBackwards(t *testing.T) {
	d := setupTestDB(t)
	ctx := context.Background()
	seedLabelChanges(t, d, 3)
	old := time.Now().Add(-100 * 24 * time.Hour)
	backdateOneChange(t, d, 1, old)
	backdateOneChange(t, d, 3, old)

	r := NewChangeLogRepo(d)
	removed, err := r.Prune(ctx, time.Now().Add(-SyncHistoryWindow))
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if removed != 3 {
		t.Errorf("removed: got %d, want 3 (the whole prefix)", removed)
	}
	if got := changeSeqs(t, d); len(got) != 0 {
		t.Errorf("retained seqs: got %v, want none", got)
	}

	// The boundary survives the empty log: it is the highest seq ever handed
	// out, so the client that had seen it all still resumes.
	if _, err := r.Changes(ctx, SyncChangesQuery{Since: 3}); err != nil {
		t.Errorf("cursor on the boundary should still resume: %v", err)
	}
	var expired *SyncCursorExpiredError
	if _, err := r.Changes(ctx, SyncChangesQuery{Since: 2}); !errors.As(err, &expired) {
		t.Fatalf("cursor below the boundary: got %v, want a cursor-expired error", err)
	}
}

// Pruning must never hand a seq out twice: a replica that stored cursor 3 and a
// change that later reused seq 3 would silently skip it.
func TestChangeLogPrune_DoesNotRewindTheSequence(t *testing.T) {
	d := setupTestDB(t)
	ctx := context.Background()
	head := seedLabelChanges(t, d, 2)
	backdateChanges(t, d, head, time.Now().Add(-100*24*time.Hour))

	if _, err := NewChangeLogRepo(d).Prune(ctx, time.Now().Add(-SyncHistoryWindow)); err != nil {
		t.Fatalf("prune: %v", err)
	}
	if _, err := NewLabelRepo(d).Create(ctx, "fresh", "blue", false); err != nil {
		t.Fatalf("create label: %v", err)
	}
	if got := maxChangeSeq(t, d); got <= head {
		t.Errorf("seq after prune: got %d, want above the pruned head %d", got, head)
	}
}

func TestResetSyncHistory_BumpsEpochAndEmptiesTheLog(t *testing.T) {
	d := setupTestDB(t)
	ctx := context.Background()
	seedLabelChanges(t, d, 2)
	before, err := NewAppSettingsRepo(d).SyncEpoch(ctx)
	if err != nil {
		t.Fatalf("read epoch: %v", err)
	}

	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	epoch, err := ResetSyncHistory(ctx, tx)
	if err != nil {
		t.Fatalf("reset sync history: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	if epoch != before+1 {
		t.Errorf("epoch: got %d, want %d", epoch, before+1)
	}
	if got := changeSeqs(t, d); len(got) != 0 {
		t.Errorf("change log after reset: got %v, want empty", got)
	}
}

// The reset is bookkeeping, not a settings edit: it must not show up in the feed
// as an app-settings change, and it must leave the stored settings alone.
func TestResetSyncHistory_LeavesAppSettingsBlobAlone(t *testing.T) {
	d := setupTestDB(t)
	ctx := context.Background()
	settings := NewAppSettingsRepo(d)
	if err := settings.Set(ctx, &model.AppSettings{
		AutoLabels: []model.AutoLabelRule{{Mask: "*urgent*", LabelIDs: []int64{7}}},
	}); err != nil {
		t.Fatalf("set app settings: %v", err)
	}

	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if _, err := ResetSyncHistory(ctx, tx); err != nil {
		t.Fatalf("reset sync history: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	got, err := settings.Get(ctx)
	if err != nil {
		t.Fatalf("get app settings: %v", err)
	}
	if len(got.AutoLabels) != 1 || got.AutoLabels[0].Mask != "*urgent*" {
		t.Errorf("auto labels: got %+v, want the stored rule", got.AutoLabels)
	}
}

// A rolled-back restore leaves the data untouched, so it must leave the history
// describing that data untouched too.
func TestResetSyncHistory_RolledBackKeepsHistory(t *testing.T) {
	d := setupTestDB(t)
	ctx := context.Background()
	head := seedLabelChanges(t, d, 2)
	before, err := NewAppSettingsRepo(d).SyncEpoch(ctx)
	if err != nil {
		t.Fatalf("read epoch: %v", err)
	}

	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if _, err := ResetSyncHistory(ctx, tx); err != nil {
		t.Fatalf("reset sync history: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	after, err := NewAppSettingsRepo(d).SyncEpoch(ctx)
	if err != nil {
		t.Fatalf("read epoch: %v", err)
	}
	if after != before {
		t.Errorf("epoch after rollback: got %d, want %d", after, before)
	}
	if got := maxChangeSeq(t, d); got != head {
		t.Errorf("head after rollback: got %d, want %d", got, head)
	}
}

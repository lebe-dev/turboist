package repo

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

// Prune drops change rows the retention window no longer covers and returns how
// many went. `before` is the cutoff: everything logged earlier is eligible.
//
// It deletes a contiguous prefix — every row up to the highest seq older than
// the cutoff — rather than "every row whose changed_at is old". The two are the
// same log in the normal case, but only the prefix form keeps the expiry
// boundary honest: a replica is refused with "your cursor expired" purely on the
// strength of MIN(seq), so a hole punched in the middle of the log would let a
// cursor below the hole resume and silently miss the changes that were removed.
// Deleting from the front can only ever move the boundary forward, which is
// exactly what the refusal is meant to describe. (A backwards clock is the
// realistic way changed_at and seq disagree, and it must not cost a replica
// data.)
//
// A cursor sitting on the last surviving seq is unaffected: it already holds
// everything that was removed. Nothing resets sqlite_sequence, so seq keeps
// climbing and a pruned-away number is never handed out again.
func (r *ChangeLogRepo) Prune(ctx context.Context, before time.Time) (int64, error) {
	const op = "repo.change_log.Prune"
	logQuery(ctx, op, model.FormatUTC(before))

	// One statement, so the boundary cannot move between choosing it and acting
	// on it. COALESCE keeps "nothing is old enough" as a no-op delete.
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM change_log
		  WHERE seq <= COALESCE((SELECT MAX(seq) FROM change_log WHERE changed_at < ?), 0)`,
		model.FormatUTC(before))
	if err != nil {
		return 0, logErr(ctx, op, fmt.Errorf("prune change log: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, logErr(ctx, op, fmt.Errorf("count pruned changes: %w", err))
	}
	return n, nil
}

// ResetSyncHistory declares every replica cursor void and empties the log, from
// inside the caller's transaction. It answers with the epoch it moved to.
//
// It belongs to a restore: the dataset is replaced wholesale, so the recorded
// history stops describing the rows it points at, and a replica resuming from a
// cursor stamped before it would keep whatever the backup did not contain. The
// epoch is what makes that impossible to miss — a cursor carrying the old one is
// refused outright and the client reseeds from a snapshot.
//
// Running inside the restore's own transaction is the point: a restore that
// rolls back leaves both the data and the log exactly as they were.
func ResetSyncHistory(ctx context.Context, tx *sql.Tx) (int64, error) {
	// The bump does not touch the settings blob, and the trigger watches only
	// that column, so this logs nothing of its own. The singleton is created if
	// it is somehow absent, starting one past the default the column carries.
	var epoch int64
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO app_settings (id, data, sync_epoch) VALUES (?, '{}', ?)
		 ON CONFLICT(id) DO UPDATE SET sync_epoch = app_settings.sync_epoch + 1
		 RETURNING sync_epoch`,
		appSettingsSingletonID, defaultSyncEpoch+1).Scan(&epoch); err != nil {
		return 0, fmt.Errorf("bump sync epoch: %w", err)
	}
	// Last, so the rows the restore itself logged — every wipe and every insert
	// fires a trigger — go with the history they belong to. What survives would
	// be a replay of the restore, which is precisely what no client should be
	// handed piecemeal.
	if _, err := tx.ExecContext(ctx, `DELETE FROM change_log`); err != nil {
		return 0, fmt.Errorf("truncate change log: %w", err)
	}
	return epoch, nil
}

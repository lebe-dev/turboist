package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/lebe-dev/turboist/internal/obs"
	"github.com/lebe-dev/turboist/internal/repo"
)

const (
	// idempotencyCleanupInterval is how often expired idempotency keys are pruned.
	idempotencyCleanupInterval = 12 * time.Hour
	// idempotencyKeyTTL is how long a stored idempotency key is retained before
	// it becomes eligible for pruning.
	idempotencyKeyTTL = 48 * time.Hour
)

// startIdempotencyCleanup runs an immediate prune, then schedules one every 12
// hours until ctx is cancelled. It mirrors auth.StartSessionCleanup and shares
// the same cleanup context, so it stops on graceful shutdown.
func startIdempotencyCleanup(ctx context.Context, keys *repo.IdempotencyRepo, log *slog.Logger) {
	go runIdempotencyCleanup(ctx, keys, log, idempotencyCleanupInterval)
}

func runIdempotencyCleanup(ctx context.Context, keys *repo.IdempotencyRepo, log *slog.Logger, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	pruneIdempotencyOnce(ctx, keys, log)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pruneIdempotencyOnce(ctx, keys, log)
		}
	}
}

func pruneIdempotencyOnce(ctx context.Context, keys *repo.IdempotencyRepo, log *slog.Logger) {
	n, err := keys.DeleteOlderThan(ctx, time.Now().Add(-idempotencyKeyTTL))
	if err != nil {
		obs.CaptureError(err, map[string]string{"op": "main.IdempotencyCleanup"})
		log.Error("idempotency cleanup failed", slog.String("op", "main.IdempotencyCleanup"), slog.String("err", err.Error()))
		return
	}
	if n == 0 {
		log.Debug("idempotency cleanup done", slog.String("op", "main.IdempotencyCleanup"), slog.Int64("removed", n))
		return
	}
	log.Info("idempotency cleanup done", slog.String("op", "main.IdempotencyCleanup"), slog.Int64("removed", n))
}

// changeLogPruneInterval is how often the change log is trimmed back to the
// retention window. Daily is plenty: the window is measured in months, and the
// only cost of a late prune is a few rows nobody reads.
const changeLogPruneInterval = 24 * time.Hour

// startChangeLogPrune trims the change log immediately, then once a day until
// ctx is cancelled. It shares the cleanup context with the other background
// jobs, so it stops on graceful shutdown.
//
// Trimming is what keeps the log from growing for the life of the install. It
// removes only history older than the window a replica is allowed to catch up
// over, so a client that is merely up to date is never affected; one that has
// been offline for longer is told to reseed instead of resuming.
func startChangeLogPrune(ctx context.Context, changes *repo.ChangeLogRepo, log *slog.Logger) {
	go runChangeLogPrune(ctx, changes, log, changeLogPruneInterval)
}

func runChangeLogPrune(ctx context.Context, changes *repo.ChangeLogRepo, log *slog.Logger, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	pruneChangeLogOnce(ctx, changes, log)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pruneChangeLogOnce(ctx, changes, log)
		}
	}
}

func pruneChangeLogOnce(ctx context.Context, changes *repo.ChangeLogRepo, log *slog.Logger) {
	const op = "main.ChangeLogPrune"
	n, err := changes.Prune(ctx, time.Now().Add(-repo.SyncHistoryWindow))
	if err != nil {
		obs.CaptureError(err, map[string]string{"op": op})
		log.Error("change log prune failed", slog.String("op", op), slog.String("err", err.Error()))
		return
	}
	if n == 0 {
		log.Debug("change log prune done", slog.String("op", op), slog.Int64("removed", n))
		return
	}
	log.Info("change log prune done", slog.String("op", op), slog.Int64("removed", n))
}

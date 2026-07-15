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

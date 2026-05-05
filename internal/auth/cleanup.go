package auth

import (
	"context"
	"log/slog"
	"time"

	"github.com/lebe-dev/turboist/internal/repo"
)

const SessionCleanupInterval = 24 * time.Hour

// StartSessionCleanup runs an immediate cleanup, then schedules one every 24 hours
// until ctx is cancelled. It blocks until the goroutine is launched.
func StartSessionCleanup(ctx context.Context, sessions *repo.SessionRepo, log *slog.Logger) {
	go runSessionCleanup(ctx, sessions, log, SessionCleanupInterval)
}

func runSessionCleanup(ctx context.Context, sessions *repo.SessionRepo, log *slog.Logger, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	cleanupOnce(ctx, sessions, log)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cleanupOnce(ctx, sessions, log)
		}
	}
}

func cleanupOnce(ctx context.Context, sessions *repo.SessionRepo, log *slog.Logger) {
	n, err := sessions.Cleanup(ctx)
	if err != nil {
		log.Error("session cleanup failed", "err", err)
		return
	}
	log.Info("session cleanup done", "removed", n)
}

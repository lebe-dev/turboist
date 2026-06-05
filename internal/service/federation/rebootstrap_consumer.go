package federation

import (
	"context"
	"log/slog"

	"github.com/lebe-dev/turboist/internal/service/events"
)

// ReBootstrapConsumer adapts the federation Service to the recovery loop's
// StaleConsumer seam (Federation v1 F4.2). When the recovery pull is answered 410
// stale_pull, the loop calls ConsumeStalePull; this adapter drives the
// transactional re-bootstrap (Service.ReBootstrap — overwrite local state,
// preserve the outbox, stamp the cutoff X) and, on success, publishes a
// federation-origin SSE refresh so every open tab re-reads the project and
// surfaces the re-sync banner carrying the cutoff X (US-4.2 AC4).
//
// The SSE event payload is intentionally coarse (ScopeProjects, no embedded
// data): the existing realtime layer refetches via REST, and the project DTO now
// carries the re-bootstrap marker (reBootstrappedAt) the banner renders — so the
// cutoff X reaches the UI on the next project read, not inline on the event. The
// publish carries NO origin, so it is NOT echo-suppressed (a re-bootstrap is never
// the local user's own mutation, like every other federation-origin change).
type ReBootstrapConsumer struct {
	svc    *Service
	hub    *events.Hub
	userID int64
	log    *slog.Logger
}

// NewReBootstrapConsumer wires the re-bootstrap consumer. A nil hub disables the
// SSE refresh (federation can run headless / in tests); a nil log uses the
// default. The single-user model means the audience is always user id 1.
func NewReBootstrapConsumer(svc *Service, hub *events.Hub, log *slog.Logger) *ReBootstrapConsumer {
	if log == nil {
		log = slog.Default()
	}
	return &ReBootstrapConsumer{svc: svc, hub: hub, userID: 1, log: log}
}

// ConsumeStalePull drives the re-bootstrap and publishes the SSE refresh on
// success. It satisfies recovery.StaleConsumer. A re-bootstrap error is returned
// to the loop (which logs it and leaves the cursor unchanged so the next pass
// retries); the unsent outbox is untouched either way.
func (c *ReBootstrapConsumer) ConsumeStalePull(ctx context.Context, localProjectID int64, peerURL, snapshotURL, asOfHLC string) error {
	res, err := c.svc.ReBootstrap(ctx, localProjectID, peerURL, snapshotURL, asOfHLC)
	if err != nil {
		return err
	}
	c.log.InfoContext(ctx, "federation: re-bootstrapped stale project",
		slog.String("op", "federation.ReBootstrap"),
		slog.Int64("project_id", localProjectID),
		slog.String("peer", peerURL),
		slog.String("cutoff_hlc", res.CutoffHLC),
		slog.String("rebootstrapped_at", res.RebootstrappedAt),
	)
	if c.hub != nil {
		// No origin → delivered to all subscribers (a re-bootstrap is never the
		// caller's own echo, like every federation-origin change). ScopeProjects so
		// the projects store reloads and the re-sync banner picks up reBootstrappedAt.
		c.hub.Publish(ctx, c.userID, events.ScopeProjects)
		c.hub.Publish(ctx, c.userID, events.ScopeTasks)
	}
	return nil
}

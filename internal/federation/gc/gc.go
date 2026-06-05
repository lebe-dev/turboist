// Package gc runs the federation retention garbage collector (Federation v1
// F3.3, US-3.7 AC5). A single ctx-cancellable goroutine (modelled on
// auth.StartSessionCleanup) runs once at startup and then once per interval:
//
//   - hard-DELETE tombstones (soft-deleted rows) of the federated entity tables
//     whose deleted_at predates the tombstone retention window (default 90 days),
//     so a returning offline peer can no longer resurrect a long-dead entity
//     (§8.2/§8.3) — the per-field HLC sidecar is pruned with the row;
//   - purge federation_outbox rows aged past the outbox retention (default 30d,
//     the recovery/pull-replay buffer — §16.3 hardcap 30d);
//   - purge APPLIED federation_inbox rows aged past the inbox retention (the
//     dedup window; un-applied rows are never dropped — NFR-2 at-least-once).
//
// All work runs on the store's own connection (no network I/O), so it never
// holds the lone connection across anything slow (R1). The pass is best-effort:
// a failure on one table is logged and the rest of the pass continues.
package gc

import (
	"context"
	"log/slog"
	"time"

	"github.com/lebe-dev/turboist/internal/federation/store"
	"github.com/lebe-dev/turboist/internal/model"
)

// DefaultInterval is the GC cadence (daily), matching auth session cleanup.
const DefaultInterval = 24 * time.Hour

// Default retention windows (configurable via Config). The tombstone window is
// the resurrection-safety horizon (§8.2 minimum 90d); the outbox/inbox windows
// bound the recovery/dedup buffers (§16.3 outbox hardcap 30d).
const (
	DefaultTombstoneRetention = 90 * 24 * time.Hour
	DefaultOutboxRetention    = 30 * 24 * time.Hour
	DefaultInboxRetention     = 30 * 24 * time.Hour
)

// federatedTables are the soft-deleted entity tables the tombstone GC sweeps.
// Comments/checklist_items are included so their cascade tombstones (F3.3 §8.4)
// are reclaimed too; a build without the F0.2 schema simply has empty tables.
var federatedTables = []string{"tasks", "projects", "project_sections", "comments", "checklist_items"}

// Config holds the retention windows. A zero field falls back to its default, so
// a caller can set only the windows it cares about.
type Config struct {
	TombstoneRetention time.Duration
	OutboxRetention    time.Duration
	InboxRetention     time.Duration
}

func (c Config) withDefaults() Config {
	if c.TombstoneRetention <= 0 {
		c.TombstoneRetention = DefaultTombstoneRetention
	}
	if c.OutboxRetention <= 0 {
		c.OutboxRetention = DefaultOutboxRetention
	}
	if c.InboxRetention <= 0 {
		c.InboxRetention = DefaultInboxRetention
	}
	return c
}

// AuditPruner deletes federation audit-log rows older than a cutoff (Federation
// v1 F6.3, US-7.4 AC2). It is satisfied by *repo.FederationAuditLogRepo; kept as a
// local interface so the gc package holds no hard dependency on the repo type.
type AuditPruner interface {
	DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
}

// Collector drives one GC pass over the store. It is safe to construct with a nil
// logger (slog.Default is substituted).
type Collector struct {
	store *store.Store
	cfg   Config
	log   *slog.Logger
	now   func() time.Time

	// configSource, when set, supplies the LIVE retention windows on each pass so
	// an admin runtime change (Federation v1 F6.5, US-8.4) takes effect on the next
	// sweep WITHOUT a restart. It overrides the static cfg captured at construction.
	// nil → the static cfg is used. The source must already apply defaults + the
	// outbox hardcap clamp (§16.3); the collector still applies withDefaults() as a
	// backstop so a partially-zero source can never disable a window.
	configSource func() Config

	// auditPruner + auditRetention add the federation audit-log sweep (Federation
	// v1 F6.3, US-7.4 AC2). Wired additively by WithAudit; nil pruner → the audit
	// sweep is skipped (a federation-off build never has audit rows to reclaim).
	auditPruner    AuditPruner
	auditRetention time.Duration
}

// NewCollector constructs the retention collector. A nil log uses slog.Default.
func NewCollector(st *store.Store, cfg Config, log *slog.Logger) *Collector {
	if log == nil {
		log = slog.Default()
	}
	return &Collector{store: st, cfg: cfg.withDefaults(), log: log, now: time.Now}
}

// WithClock overrides the collector's wall clock (default time.Now), for
// deterministic retention-cutoff tests. It must be set before Start.
func (c *Collector) WithClock(now func() time.Time) *Collector {
	if now != nil {
		c.now = now
	}
	return c
}

// WithConfigSource wires a live retention-config resolver so a runtime change to
// the windows (Federation v1 F6.5, US-8.4) is picked up on the next pass without a
// restart. It returns the collector for chaining. A nil source keeps the static
// config captured at construction.
func (c *Collector) WithConfigSource(src func() Config) *Collector {
	c.configSource = src
	return c
}

// WithAudit wires the federation audit-log retention sweep (Federation v1 F6.3,
// US-7.4 AC2): each daily pass also hard-deletes audit rows older than retention
// (default 1 year). It returns the collector for chaining. A nil pruner or a
// non-positive retention disables the audit sweep.
func (c *Collector) WithAudit(pruner AuditPruner, retention time.Duration) *Collector {
	c.auditPruner = pruner
	c.auditRetention = retention
	return c
}

// Start launches the GC goroutine: an immediate pass, then one per interval until
// ctx is cancelled (the auth.StartSessionCleanup shape). interval<=0 → daily.
func (c *Collector) Start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = DefaultInterval
	}
	go c.run(ctx, interval)
}

func (c *Collector) run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	c.runLogged(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.runLogged(ctx)
		}
	}
}

func (c *Collector) runLogged(ctx context.Context) {
	if err := c.RunOnce(ctx); err != nil {
		c.log.ErrorContext(ctx, "federation: gc pass failed",
			slog.String("op", "federation.gc.Run"),
			slog.String("err", err.Error()),
		)
	}
}

// RunOnce performs one full GC pass. It is exported so a test (or an admin
// trigger later) can drive collection synchronously. Each step is independent: a
// failure on one is logged and the pass continues so a single bad table never
// blocks reclaiming the others. It returns the first error only for the test
// path; the goroutine logs and continues regardless.
func (c *Collector) RunOnce(ctx context.Context) error {
	now := c.now()
	nowStr := model.FormatUTC(now)
	// Resolve the LIVE windows so an admin runtime change (US-8.4) takes effect on
	// this pass. withDefaults is a backstop so a partially-zero source can never
	// disable a window.
	cfg := c.cfg
	if c.configSource != nil {
		cfg = c.configSource().withDefaults()
	}
	tombstoneCutoff := model.FormatUTC(now.Add(-cfg.TombstoneRetention))
	outboxCutoff := model.FormatUTC(now.Add(-cfg.OutboxRetention))
	inboxCutoff := model.FormatUTC(now.Add(-cfg.InboxRetention))

	var firstErr error
	record := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	for _, table := range federatedTables {
		n, err := c.store.DeleteTombstonesOlderThan(ctx, table, tombstoneCutoff)
		if err != nil {
			c.log.ErrorContext(ctx, "federation: gc tombstone sweep failed",
				slog.String("op", "federation.gc.Run"),
				slog.String("table", table),
				slog.String("err", err.Error()),
			)
			record(err)
			continue
		}
		if n > 0 {
			c.log.InfoContext(ctx, "federation: gc pruned tombstones",
				slog.String("op", "federation.gc.Run"),
				slog.String("table", table),
				slog.Int64("removed", n),
			)
		}
	}

	// Purge aged outbox rows AND advance each project's durable pruned-floor HLC to
	// the max event HLC removed, so the stale-pull 410 gate keeps a record of what
	// aged out even after the outbox is empty (US-3.7 AC4 review fix).
	if n, err := c.store.PurgeOutboxOlderThanAdvancingFloor(ctx, outboxCutoff, nowStr); err != nil {
		c.log.ErrorContext(ctx, "federation: gc outbox purge failed",
			slog.String("op", "federation.gc.Run"), slog.String("err", err.Error()))
		record(err)
	} else if n > 0 {
		c.log.InfoContext(ctx, "federation: gc purged outbox",
			slog.String("op", "federation.gc.Run"), slog.Int64("removed", n))
	}

	if n, err := c.store.PurgeAppliedInboxOlderThan(ctx, inboxCutoff); err != nil {
		c.log.ErrorContext(ctx, "federation: gc inbox purge failed",
			slog.String("op", "federation.gc.Run"), slog.String("err", err.Error()))
		record(err)
	} else if n > 0 {
		c.log.InfoContext(ctx, "federation: gc purged inbox",
			slog.String("op", "federation.gc.Run"), slog.Int64("removed", n))
	}

	// Audit-log retention (Federation v1 F6.3, US-7.4 AC2): hard-delete audit rows
	// older than the configured window (default 1 year). Skipped when no pruner is
	// wired (a federation-off build has no audit rows). The cutoff is a fixed-width
	// TEXT lexical compare like the other sweeps.
	if c.auditPruner != nil && c.auditRetention > 0 {
		auditCutoff := now.Add(-c.auditRetention)
		if n, err := c.auditPruner.DeleteOlderThan(ctx, auditCutoff); err != nil {
			c.log.ErrorContext(ctx, "federation: gc audit purge failed",
				slog.String("op", "federation.gc.Run"), slog.String("err", err.Error()))
			record(err)
		} else if n > 0 {
			c.log.InfoContext(ctx, "federation: gc purged audit log",
				slog.String("op", "federation.gc.Run"), slog.Int64("removed", n))
		}
	}

	return firstErr
}

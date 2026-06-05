package federation

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	fedgc "github.com/lebe-dev/turboist/internal/federation/gc"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// OutboxRetentionHardcap is the §16.3 ceiling on the outbox retention window: a
// configured / admin-set value larger than this is CLAMPED to it (not merely
// validated) so the recovery/replay buffer can never grow unbounded.
const OutboxRetentionHardcap = 30 * 24 * time.Hour

// RetentionWindows is the resolved set of GC retention windows (Federation v1
// F6.5, US-8.4). It is what the admin reads/writes and what the GC consumes.
// Values are whole days on the wire; the duration accessors apply the defaults +
// the outbox hardcap clamp.
type RetentionWindows struct {
	// TombstoneDays / OutboxDays / InboxDays are the persisted overrides. 0 (or
	// negative) means "use the compiled/config default".
	TombstoneDays int
	OutboxDays    int
	InboxDays     int
}

// RetentionService holds the LIVE retention windows behind an atomic.Pointer so an
// admin change (US-8.4) is picked up by the GC on its next pass without a restart
// (Federation v1 F6.5). It is constructed with the config defaults; persisted
// overrides are loaded once at startup and on every admin PATCH. The outbox window
// is hard-capped at OutboxRetentionHardcap (§16.3) when resolved to a duration.
type RetentionService struct {
	repo *repo.FederationRetentionSettingsRepo
	now  func() time.Time

	// defaults are the compiled/config fallbacks used when an override is unset.
	defTombstone time.Duration
	defOutbox    time.Duration
	defInbox     time.Duration

	live atomic.Pointer[RetentionWindows]
}

// NewRetentionService constructs the live retention holder. defTombstone/defOutbox/
// defInbox are the config (or compiled) defaults applied when an override is unset.
// The holder starts with no overrides (all defaults); call Reload to hydrate the
// persisted overrides at startup.
func NewRetentionService(r *repo.FederationRetentionSettingsRepo, defTombstone, defOutbox, defInbox time.Duration) *RetentionService {
	s := &RetentionService{
		repo:         r,
		now:          time.Now,
		defTombstone: defTombstone,
		defOutbox:    defOutbox,
		defInbox:     defInbox,
	}
	s.live.Store(&RetentionWindows{})
	return s
}

// Reload loads the persisted overrides into the live holder (Federation v1 F6.5).
// Called at startup and after every admin PATCH so the GC reads the latest.
func (s *RetentionService) Reload(ctx context.Context) error {
	if s.repo == nil {
		return nil
	}
	got, err := s.repo.Get(ctx)
	if err != nil {
		return fmt.Errorf("load retention settings: %w", err)
	}
	s.live.Store(&RetentionWindows{
		TombstoneDays: intOrZero(got.TombstoneRetentionDays),
		OutboxDays:    intOrZero(got.OutboxRetentionDays),
		InboxDays:     intOrZero(got.InboxRetentionDays),
	})
	return nil
}

// Get returns the current live override days (0 = default applies).
func (s *RetentionService) Get() RetentionWindows {
	w := s.live.Load()
	if w == nil {
		return RetentionWindows{}
	}
	return *w
}

// Update persists the given overrides and refreshes the live holder so the next
// GC pass uses them (US-8.4 runtime change). A non-positive day value is stored as
// NULL (revert to default). The outbox value is NOT clamped on write — the clamp is
// applied when resolved to a duration (GCConfig) so the stored value stays the
// user's intent while the EFFECTIVE window respects the §16.3 hardcap.
func (s *RetentionService) Update(ctx context.Context, w RetentionWindows) error {
	if s.repo == nil {
		return ErrKeyMissing
	}
	persist := repo.FederationRetentionSettings{
		TombstoneRetentionDays: positiveOrNil(w.TombstoneDays),
		OutboxRetentionDays:    positiveOrNil(w.OutboxDays),
		InboxRetentionDays:     positiveOrNil(w.InboxDays),
	}
	if err := s.repo.Set(ctx, persist, model.FormatUTC(s.now())); err != nil {
		return fmt.Errorf("persist retention settings: %w", err)
	}
	return s.Reload(ctx)
}

// GCConfig resolves the live overrides + defaults into the gc.Config the collector
// consumes, applying the outbox hardcap clamp (§16.3). It is the func wired into
// the GC's config source so each pass reads the freshest windows.
func (s *RetentionService) GCConfig() fedgc.Config {
	w := s.Get()
	return fedgc.Config{
		TombstoneRetention: resolveDays(w.TombstoneDays, s.defTombstone),
		OutboxRetention:    clampOutbox(resolveDays(w.OutboxDays, s.defOutbox)),
		InboxRetention:     resolveDays(w.InboxDays, s.defInbox),
	}
}

// resolveDays maps a day override to a duration, falling back to def when the
// override is non-positive.
func resolveDays(days int, def time.Duration) time.Duration {
	if days <= 0 {
		return def
	}
	return time.Duration(days) * 24 * time.Hour
}

// clampOutbox enforces the §16.3 outbox hardcap.
func clampOutbox(d time.Duration) time.Duration {
	if d > OutboxRetentionHardcap {
		return OutboxRetentionHardcap
	}
	return d
}

func intOrZero(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func positiveOrNil(v int) *int {
	if v <= 0 {
		return nil
	}
	return &v
}

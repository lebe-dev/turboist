package federation_test

import (
	"context"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/repo"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

func newRetentionSvc(t *testing.T) (*fedsvc.RetentionService, context.Context) {
	t.Helper()
	d, _, _, _ := setup(t)
	r := repo.NewFederationRetentionSettingsRepo(d)
	svc := fedsvc.NewRetentionService(r, 90*24*time.Hour, 30*24*time.Hour, 30*24*time.Hour)
	ctx := context.Background()
	if err := svc.Reload(ctx); err != nil {
		t.Fatalf("reload: %v", err)
	}
	return svc, ctx
}

// TestRetention_DefaultsWhenUnset asserts the resolved GC config uses the config
// defaults when no override is persisted (Federation v1 F6.5, US-8.4).
func TestRetention_DefaultsWhenUnset(t *testing.T) {
	svc, _ := newRetentionSvc(t)
	cfg := svc.GCConfig()
	if cfg.TombstoneRetention != 90*24*time.Hour {
		t.Errorf("tombstone default: got %v, want 90d", cfg.TombstoneRetention)
	}
	if cfg.OutboxRetention != 30*24*time.Hour {
		t.Errorf("outbox default: got %v, want 30d", cfg.OutboxRetention)
	}
	if cfg.InboxRetention != 30*24*time.Hour {
		t.Errorf("inbox default: got %v, want 30d", cfg.InboxRetention)
	}
}

// TestRetention_RuntimeChange asserts an Update is persisted and reflected by the
// next GCConfig read WITHOUT reconstructing the service (US-8.4 runtime change).
func TestRetention_RuntimeChange(t *testing.T) {
	svc, ctx := newRetentionSvc(t)
	if err := svc.Update(ctx, fedsvc.RetentionWindows{TombstoneDays: 120, InboxDays: 10}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	cfg := svc.GCConfig()
	if cfg.TombstoneRetention != 120*24*time.Hour {
		t.Errorf("tombstone after update: got %v, want 120d", cfg.TombstoneRetention)
	}
	if cfg.InboxRetention != 10*24*time.Hour {
		t.Errorf("inbox after update: got %v, want 10d", cfg.InboxRetention)
	}
	// Outbox left unset → still the default.
	if cfg.OutboxRetention != 30*24*time.Hour {
		t.Errorf("outbox unchanged: got %v, want 30d default", cfg.OutboxRetention)
	}

	// The persisted override is reflected in the live holder.
	got := svc.Get()
	if got.TombstoneDays != 120 {
		t.Errorf("persisted tombstone days: got %d, want 120", got.TombstoneDays)
	}
}

// TestRetention_OutboxHardcapClamp asserts an outbox window over the §16.3 hardcap
// is CLAMPED to 30 days when resolved, even though the stored value keeps the
// user's larger intent (US-8.4 risk: "outbox hardcap clamp not just validate").
func TestRetention_OutboxHardcapClamp(t *testing.T) {
	svc, ctx := newRetentionSvc(t)
	if err := svc.Update(ctx, fedsvc.RetentionWindows{OutboxDays: 365}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	cfg := svc.GCConfig()
	if cfg.OutboxRetention != fedsvc.OutboxRetentionHardcap {
		t.Errorf("outbox clamp: got %v, want %v (30d hardcap)", cfg.OutboxRetention, fedsvc.OutboxRetentionHardcap)
	}
	// The stored intent is preserved unchanged.
	if got := svc.Get().OutboxDays; got != 365 {
		t.Errorf("stored outbox days: got %d, want 365 (clamp is on resolve, not write)", got)
	}
}

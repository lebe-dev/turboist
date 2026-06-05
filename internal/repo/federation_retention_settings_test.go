package repo

import (
	"context"
	"testing"
)

// TestFederationRetentionSettings_DefaultEmpty asserts the seeded row reads back
// all-nil (defaults apply) on a fresh DB (Federation v1 F6.5, US-8.4).
func TestFederationRetentionSettings_DefaultEmpty(t *testing.T) {
	d := setupTestDB(t)
	r := NewFederationRetentionSettingsRepo(d)
	got, err := r.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.TombstoneRetentionDays != nil || got.OutboxRetentionDays != nil || got.InboxRetentionDays != nil {
		t.Errorf("fresh retention settings: got %+v, want all nil (defaults apply)", got)
	}
}

// TestFederationRetentionSettings_SetGetRoundTrip asserts a Set persists and a
// subsequent Get reads back the same overrides; a nil field reverts to NULL.
func TestFederationRetentionSettings_SetGetRoundTrip(t *testing.T) {
	d := setupTestDB(t)
	r := NewFederationRetentionSettingsRepo(d)
	ctx := context.Background()

	tomb := 120
	outbox := 20
	if err := r.Set(ctx, FederationRetentionSettings{
		TombstoneRetentionDays: &tomb,
		OutboxRetentionDays:    &outbox,
		// InboxRetentionDays left nil → NULL.
	}, "2026-06-04T00:00:00.000Z"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := r.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.TombstoneRetentionDays == nil || *got.TombstoneRetentionDays != 120 {
		t.Errorf("tombstone: got %v, want 120", got.TombstoneRetentionDays)
	}
	if got.OutboxRetentionDays == nil || *got.OutboxRetentionDays != 20 {
		t.Errorf("outbox: got %v, want 20", got.OutboxRetentionDays)
	}
	if got.InboxRetentionDays != nil {
		t.Errorf("inbox: got %v, want nil (left unset)", got.InboxRetentionDays)
	}

	// Re-Set with all nil reverts every override to NULL.
	if err := r.Set(ctx, FederationRetentionSettings{}, "2026-06-04T01:00:00.000Z"); err != nil {
		t.Fatalf("Set revert: %v", err)
	}
	got, err = r.Get(ctx)
	if err != nil {
		t.Fatalf("Get after revert: %v", err)
	}
	if got.TombstoneRetentionDays != nil || got.OutboxRetentionDays != nil || got.InboxRetentionDays != nil {
		t.Errorf("after revert: got %+v, want all nil", got)
	}
}

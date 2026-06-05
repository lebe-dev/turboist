package model

import (
	"testing"
	"time"
)

// TestFederationInvite_Status_AllFourCases asserts the canonical derived-status
// precedence (Federation v1 F1.3, US-1.3 AC1): revoked > consumed > expired >
// active. This single helper is shared by the list path and the handshake
// consume path so a revoked-and-also-expired invite never reports two states.
func TestFederationInvite_Status_AllFourCases(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	cases := []struct {
		name string
		inv  FederationInvite
		want InviteStatus
	}{
		{
			name: "active: not revoked, not consumed, not expired",
			inv:  FederationInvite{MaxUses: 1, UsedCount: 0, ExpiresAt: &future},
			want: InviteStatusActive,
		},
		{
			name: "active when no expiry is set",
			inv:  FederationInvite{MaxUses: 1, UsedCount: 0, ExpiresAt: nil},
			want: InviteStatusActive,
		},
		{
			name: "expired: expires_at in the past",
			inv:  FederationInvite{MaxUses: 1, UsedCount: 0, ExpiresAt: &past},
			want: InviteStatusExpired,
		},
		{
			name: "consumed: used_count >= max_uses",
			inv:  FederationInvite{MaxUses: 1, UsedCount: 1, ExpiresAt: &future},
			want: InviteStatusConsumed,
		},
		{
			name: "consumed wins over expired",
			inv:  FederationInvite{MaxUses: 1, UsedCount: 1, ExpiresAt: &past},
			want: InviteStatusConsumed,
		},
		{
			name: "revoked wins over consumed and expired",
			inv:  FederationInvite{MaxUses: 1, UsedCount: 1, ExpiresAt: &past, RevokedAt: &past},
			want: InviteStatusRevoked,
		},
		{
			name: "revoked wins over active",
			inv:  FederationInvite{MaxUses: 5, UsedCount: 0, ExpiresAt: &future, RevokedAt: &now},
			want: InviteStatusRevoked,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.inv.Status(now); got != tc.want {
				t.Errorf("Status: got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestFederationInvite_IsConsumable asserts the consume-path predicate agrees
// with the derived status: an invite is consumable only when its derived status
// is active (US-1.3 AC1 shared helper, US-1.2 AC3 single-use enforcement).
func TestFederationInvite_IsConsumable(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)

	active := FederationInvite{MaxUses: 1, UsedCount: 0, ExpiresAt: &future}
	if !active.IsConsumable(now) {
		t.Errorf("active invite should be consumable")
	}
	revoked := FederationInvite{MaxUses: 1, UsedCount: 0, ExpiresAt: &future, RevokedAt: &past}
	if revoked.IsConsumable(now) {
		t.Errorf("revoked invite must not be consumable")
	}
	expired := FederationInvite{MaxUses: 1, UsedCount: 0, ExpiresAt: &past}
	if expired.IsConsumable(now) {
		t.Errorf("expired invite must not be consumable")
	}
	consumed := FederationInvite{MaxUses: 1, UsedCount: 1, ExpiresAt: &future}
	if consumed.IsConsumable(now) {
		t.Errorf("fully-consumed invite must not be consumable")
	}
}

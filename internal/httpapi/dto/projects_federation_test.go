package dto

import "testing"

// TestWithFederationOwnerOffline covers the owner-offline overlay (Federation v1
// F5.6a, US-6.5 AC1): a true flag sets OwnerOffline on the DTO; a false flag
// leaves it unset. The overlay never touches read-only/lost fields — owner-offline
// is a transient queued state, not a permanent read-only one (US-6.5 AC2).
func TestWithFederationOwnerOffline(t *testing.T) {
	base := ProjectDTO{ID: 1}

	on := base.WithFederationOwnerOffline(true)
	if !on.OwnerOffline {
		t.Errorf("OwnerOffline: got false, want true")
	}
	if on.FederationLost || on.FederationLostReason != nil {
		t.Errorf("owner-offline must not mark the copy lost/read-only: lost=%v reason=%v", on.FederationLost, on.FederationLostReason)
	}

	off := base.WithFederationOwnerOffline(false)
	if off.OwnerOffline {
		t.Errorf("OwnerOffline: got true, want false")
	}
}

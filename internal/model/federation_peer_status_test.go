package model

import (
	"testing"
	"time"
)

// TestPeerStatus_Precedence asserts the canonical derived-status precedence for a
// federated peer (Federation v1 F1.4, US-1.4; "left" added F5.5, US-6.3 AC2):
// revoked > left > paused > stale(>24h) > active. The status is derived from the
// per-project row first (revoked/left/paused) and contact recency second (stale),
// so a revoked-and-stale peer reports revoked, a left peer reports left even if it
// was recently contacted, and a paused-and-stale peer reports paused.
func TestPeerStatus_Precedence(t *testing.T) {
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-1 * time.Hour)
	old := now.Add(-25 * time.Hour)

	cases := []struct {
		name        string
		revoked     bool
		paused      bool
		lostReason  FederationLostReason
		lastContact *time.Time
		want        PeerStatus
	}{
		{name: "revoked beats left+paused+stale", revoked: true, paused: true, lostReason: FederationLostLeft, lastContact: &old, want: PeerStatusRevoked},
		{name: "left beats paused+stale", revoked: false, paused: true, lostReason: FederationLostLeft, lastContact: &old, want: PeerStatusLeft},
		{name: "left beats recent contact", revoked: false, paused: false, lostReason: FederationLostLeft, lastContact: &recent, want: PeerStatusLeft},
		{name: "paused beats stale", revoked: false, paused: true, lastContact: &old, want: PeerStatusPaused},
		{name: "stale when last contact > 24h", revoked: false, paused: false, lastContact: &old, want: PeerStatusStale},
		{name: "active when last contact recent", revoked: false, paused: false, lastContact: &recent, want: PeerStatusActive},
		{name: "stale when never contacted", revoked: false, paused: false, lastContact: nil, want: PeerStatusStale},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DerivePeerStatus(tc.revoked, tc.paused, tc.lostReason, tc.lastContact, now)
			if got != tc.want {
				t.Errorf("DerivePeerStatus: got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestPeerStatus_StaleBoundary asserts the >24h staleness boundary is strict:
// exactly 24h is still active, just past 24h is stale (US-1.4 AC3).
func TestPeerStatus_StaleBoundary(t *testing.T) {
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	exactly24h := now.Add(-PeerStaleAfter)
	if got := DerivePeerStatus(false, false, FederationLostNone, &exactly24h, now); got != PeerStatusActive {
		t.Errorf("exactly 24h: got %q, want active", got)
	}
	justOver := now.Add(-PeerStaleAfter - time.Second)
	if got := DerivePeerStatus(false, false, FederationLostNone, &justOver, now); got != PeerStatusStale {
		t.Errorf("just over 24h: got %q, want stale", got)
	}
}

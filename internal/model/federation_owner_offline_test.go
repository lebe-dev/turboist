package model

import (
	"testing"
	"time"
)

// TestDeriveOwnerOffline_Boundaries covers the owner-death detection a JOINER
// uses for the read-only/queued fallback (Federation v1 F5.6a, US-6.5 AC1). The
// owner is declared "offline" only once its last successful contact is OLDER than
// the configured owner-timeout window; a never-contacted owner is offline; an
// owner contacted within (or exactly at) the window is online so a brief outage
// does not falsely flip the badge.
func TestDeriveOwnerOffline_Boundaries(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	const timeout = 30 * 24 * time.Hour

	within := now.Add(-timeout + time.Hour) // contacted just inside the window
	exact := now.Add(-timeout)              // contacted exactly timeout ago
	beyond := now.Add(-timeout - time.Hour) // contacted just past the window

	cases := []struct {
		name        string
		lastContact *time.Time
		want        bool
	}{
		{name: "never contacted is offline", lastContact: nil, want: true},
		{name: "contacted within window is online", lastContact: &within, want: false},
		{name: "contacted exactly at boundary is still online (strict)", lastContact: &exact, want: false},
		{name: "contacted beyond window is offline (US-6.5 AC1)", lastContact: &beyond, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DeriveOwnerOffline(tc.lastContact, timeout, now)
			if got != tc.want {
				t.Errorf("DeriveOwnerOffline: got %v, want %v", got, tc.want)
			}
		})
	}
}

// TestDeriveOwnerOffline_NonPositiveTimeout asserts a non-positive timeout never
// declares the owner offline — owner-offline detection is disabled rather than
// declaring every owner dead, so a misconfigured/zero timeout fails safe (the
// joiner keeps treating the link as live).
func TestDeriveOwnerOffline_NonPositiveTimeout(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	old := now.Add(-365 * 24 * time.Hour)
	if DeriveOwnerOffline(&old, 0, now) {
		t.Errorf("zero timeout must not declare owner offline")
	}
	if DeriveOwnerOffline(nil, -time.Hour, now) {
		t.Errorf("negative timeout must not declare owner offline")
	}
}

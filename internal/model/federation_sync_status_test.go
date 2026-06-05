package model

import "testing"

// TestSyncStatus_Precedence asserts the canonical derived sync-status precedence
// for a federated project (Federation v1 F4.3, US-4.3): key_mismatch > unreachable
// > pending > synced. The status is the WORST per-peer health rolled up across the
// project's peers, so one key-mismatched peer turns the whole project red even if
// others are synced, and an unreachable peer beats a merely-pending one.
func TestSyncStatus_Precedence(t *testing.T) {
	cases := []struct {
		name        string
		keyMismatch bool
		unreachable bool
		pending     bool
		want        SyncStatus
	}{
		{name: "key_mismatch beats all", keyMismatch: true, unreachable: true, pending: true, want: SyncStatusKeyMismatch},
		{name: "unreachable beats pending", keyMismatch: false, unreachable: true, pending: true, want: SyncStatusUnreachable},
		{name: "pending when undelivered overdue", keyMismatch: false, unreachable: false, pending: true, want: SyncStatusPending},
		{name: "synced when clean", keyMismatch: false, unreachable: false, pending: false, want: SyncStatusSynced},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DeriveSyncStatus(tc.keyMismatch, tc.unreachable, tc.pending)
			if got != tc.want {
				t.Errorf("DeriveSyncStatus: got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestSyncStatus_Valid asserts only the four canonical states are valid (a drift
// guard for the wire contract the badge maps).
func TestSyncStatus_Valid(t *testing.T) {
	for _, s := range []SyncStatus{SyncStatusSynced, SyncStatusPending, SyncStatusUnreachable, SyncStatusKeyMismatch} {
		if !s.IsValid() {
			t.Errorf("%q should be valid", s)
		}
	}
	if SyncStatus("bogus").IsValid() {
		t.Errorf("bogus sync status should be invalid")
	}
}

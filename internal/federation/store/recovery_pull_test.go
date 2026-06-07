package store_test

import (
	"context"
	"database/sql"
	"testing"
)

// seedFedPeerRow inserts a federated_projects mapping row for a project so the
// pull-target read has a joined peer to enumerate. is_owner=0 is a JOINED peer
// (the recovery loop pulls FROM it); is_owner=1 is the owner self-row (never a
// pull target — the owner does not pull from itself).
func seedFedPeerRow(t *testing.T, d *sql.DB, pid int64, peerURL, remoteProjectID string, isOwner, paused, revoked bool, lastReceivedHLC string) {
	t.Helper()
	owner, p, r := 0, 0, 0
	if isOwner {
		owner = 1
	}
	if paused {
		p = 1
	}
	if revoked {
		r = 1
	}
	var lastRecv any
	if lastReceivedHLC != "" {
		lastRecv = lastReceivedHLC
	}
	if _, err := d.Exec(
		`INSERT INTO federated_projects
		   (local_project_id, peer_instance_url, remote_project_id, is_owner, origin_instance_url, permissions, paused, revoked, protocol_version, last_received_hlc, joined_at)
		 VALUES (?, ?, ?, ?, ?, 'write', ?, ?, 1, ?, '2024-01-01T00:00:00.000Z')`,
		pid, peerURL, remoteProjectID, owner, peerURL, p, r, lastRecv); err != nil {
		t.Fatalf("seed federated_projects row: %v", err)
	}
}

// TestListPullTargets_OnlyJoinedReachablePeers asserts the recovery loop's pull
// target read returns only JOINED (is_owner=0), non-revoked, non-paused peer rows
// — never the owner self-row, a revoked peer, or a paused peer — each carrying its
// remote_project_id and last_received_hlc cursor (Federation v1 F4.1, US-4.1).
func TestListPullTargets_OnlyJoinedReachablePeers(t *testing.T) {
	d, s := openMigratedDB(t)
	ctx := context.Background()

	joined := seedProjectRow(t, d, "Joined")
	owned := seedProjectRow(t, d, "Owned")
	pausedP := seedProjectRow(t, d, "Paused")
	revokedP := seedProjectRow(t, d, "Revoked")

	// A joined peer the loop SHOULD pull from, with a cursor.
	seedFedPeerRow(t, d, joined, "https://owner.example", "remote-abc", false, false, false, "00000000010000-0000-nodeO")
	// An owner self-row: never a pull target (the owner does not pull from itself).
	seedFedPeerRow(t, d, owned, "https://self.example", "self-xyz", true, false, false, "")
	// A paused peer: excluded (events accumulate; no pull).
	seedFedPeerRow(t, d, pausedP, "https://paused.example", "remote-p", false, true, false, "00000000020000-0000-nodeP")
	// A revoked peer: excluded (trust terminated).
	seedFedPeerRow(t, d, revokedP, "https://revoked.example", "remote-r", false, false, true, "00000000030000-0000-nodeR")

	targets, err := s.ListPullTargets(ctx)
	if err != nil {
		t.Fatalf("list pull targets: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("pull targets: got %d, want 1 (only the reachable joined peer)", len(targets))
	}
	got := targets[0]
	if got.LocalProjectID != joined {
		t.Errorf("local project: got %d, want %d", got.LocalProjectID, joined)
	}
	if got.PeerInstanceURL != "https://owner.example" {
		t.Errorf("peer url: got %q, want owner.example", got.PeerInstanceURL)
	}
	if got.RemoteProjectID != "remote-abc" {
		t.Errorf("remote project id: got %q, want remote-abc", got.RemoteProjectID)
	}
	if got.LastReceivedHLC != "00000000010000-0000-nodeO" {
		t.Errorf("cursor: got %q, want the joined peer's last_received_hlc", got.LastReceivedHLC)
	}
}

// TestListPullTargets_SkipsLostProject asserts a JOINED peer whose mapping has
// been marked federation_lost (the joiner voluntarily LEFT, F5.5/US-6.3; or the
// owner revoked it, or the owner died) is NOT a pull target. A lost copy is a
// plain local project with a severed trust link — the recovery loop must stop
// pulling from it, otherwise a left project keeps catching up and (on a 410 stale
// pull) re-bootstrapping, silently resurrecting the federation the user removed.
func TestListPullTargets_SkipsLostProject(t *testing.T) {
	d, s := openMigratedDB(t)
	ctx := context.Background()

	pid := seedProjectRow(t, d, "Left")
	seedFedPeerRow(t, d, pid, "https://owner.example", "remote-abc", false, false, false, "00000000010000-0000-nodeO")
	if _, err := d.Exec(
		`UPDATE federated_projects SET lost = 1, lost_reason = 'left' WHERE local_project_id = ? AND is_owner = 0`,
		pid); err != nil {
		t.Fatalf("mark federated_projects lost: %v", err)
	}

	targets, err := s.ListPullTargets(ctx)
	if err != nil {
		t.Fatalf("list pull targets: %v", err)
	}
	if len(targets) != 0 {
		t.Fatalf("pull targets for lost (left) project: got %d, want 0", len(targets))
	}
}

// TestListPullTargets_SkipsTombstonedProject asserts a soft-deleted parent
// project's joined peer is NOT a pull target — a tombstoned project must not be
// re-bootstrapped from its peer.
func TestListPullTargets_SkipsTombstonedProject(t *testing.T) {
	d, s := openMigratedDB(t)
	ctx := context.Background()

	pid := seedProjectRow(t, d, "Doomed")
	seedFedPeerRow(t, d, pid, "https://owner.example", "remote-abc", false, false, false, "00000000010000-0000-nodeO")
	if _, err := d.Exec(`UPDATE projects SET deleted_at = '2024-02-01T00:00:00.000Z' WHERE id = ?`, pid); err != nil {
		t.Fatalf("soft-delete project: %v", err)
	}

	targets, err := s.ListPullTargets(ctx)
	if err != nil {
		t.Fatalf("list pull targets: %v", err)
	}
	if len(targets) != 0 {
		t.Fatalf("pull targets for tombstoned project: got %d, want 0", len(targets))
	}
}

// TestAdvanceLastReceivedHLC_Monotonic asserts the cursor advances to a strictly
// greater HLC, is a no-op for a lower-or-equal HLC (never rewinds), and is scoped
// to the exact (project, peer) row (Federation v1 F4.1 — cursor monotonic;
// partial-apply must not rewind a peer that has moved forward).
func TestAdvanceLastReceivedHLC_Monotonic(t *testing.T) {
	d, s := openMigratedDB(t)
	ctx := context.Background()
	pid := seedProjectRow(t, d, "Joined")
	const peer = "https://owner.example"
	seedFedPeerRow(t, d, pid, peer, "remote-abc", false, false, false, "00000000010000-0000-nodeO")

	// Advance forward.
	if err := s.AdvanceLastReceivedHLC(ctx, pid, peer, "00000000020000-0000-nodeO"); err != nil {
		t.Fatalf("advance forward: %v", err)
	}
	if got := readCursor(t, d, pid, peer); got != "00000000020000-0000-nodeO" {
		t.Errorf("after forward advance: got %q, want the new HLC", got)
	}

	// A lower HLC must NOT rewind the cursor (monotonic).
	if err := s.AdvanceLastReceivedHLC(ctx, pid, peer, "00000000015000-0000-nodeO"); err != nil {
		t.Fatalf("advance backward attempt: %v", err)
	}
	if got := readCursor(t, d, pid, peer); got != "00000000020000-0000-nodeO" {
		t.Errorf("after lower HLC: got %q, want unchanged (monotonic)", got)
	}

	// An equal HLC is a no-op too.
	if err := s.AdvanceLastReceivedHLC(ctx, pid, peer, "00000000020000-0000-nodeO"); err != nil {
		t.Fatalf("advance equal: %v", err)
	}
	if got := readCursor(t, d, pid, peer); got != "00000000020000-0000-nodeO" {
		t.Errorf("after equal HLC: got %q, want unchanged", got)
	}
}

// TestAdvanceLastReceivedHLC_FromEmptyCursor asserts a peer whose cursor is still
// empty (a fresh peer that has not yet received anything) advances to the first
// HLC.
func TestAdvanceLastReceivedHLC_FromEmptyCursor(t *testing.T) {
	d, s := openMigratedDB(t)
	ctx := context.Background()
	pid := seedProjectRow(t, d, "Joined")
	const peer = "https://owner.example"
	seedFedPeerRow(t, d, pid, peer, "remote-abc", false, false, false, "")

	if err := s.AdvanceLastReceivedHLC(ctx, pid, peer, "00000000010000-0000-nodeO"); err != nil {
		t.Fatalf("advance from empty: %v", err)
	}
	if got := readCursor(t, d, pid, peer); got != "00000000010000-0000-nodeO" {
		t.Errorf("from empty cursor: got %q, want the first HLC", got)
	}
}

func readCursor(t *testing.T, d *sql.DB, pid int64, peer string) string {
	t.Helper()
	var cur sql.NullString
	if err := d.QueryRow(
		`SELECT last_received_hlc FROM federated_projects WHERE local_project_id = ? AND peer_instance_url = ?`,
		pid, peer).Scan(&cur); err != nil {
		t.Fatalf("read cursor: %v", err)
	}
	return cur.String
}

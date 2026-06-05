package repo

import (
	"context"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

// insertJoinerRow inserts a joined (is_owner=0) federated_projects row mapping
// local project 1 to the given origin owner, with read permission. It is the
// shape the F5.4 revoke/mark-lost paths operate on.
func insertJoinerRow(t *testing.T, r *FederatedProjectRepo, peerURL, originURL string) {
	t.Helper()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := r.db.Exec(
		`INSERT INTO federated_projects
		   (local_project_id, peer_instance_url, remote_project_id, is_owner, origin_instance_url, permissions, protocol_version, joined_at)
		 VALUES (1, ?, 'remote-cid', 0, ?, 'read', 1, ?)`,
		peerURL, originURL, model.FormatUTC(now),
	); err != nil {
		t.Fatalf("seed joiner row %s: %v", peerURL, err)
	}
}

// TestFederatedProjectRepo_RevokeSetsFlag asserts Revoke flips revoked=1 on the
// peer row, reports 1 affected row, and is idempotent (Federation v1 F5.4,
// US-6.2 AC1).
func TestFederatedProjectRepo_RevokeSetsFlag(t *testing.T) {
	d := setupTestDB(t)
	seedFederatedProjectRow(t, d)
	r := NewFederatedProjectRepo(d)
	ctx := context.Background()
	insertJoinerRow(t, r, "https://peer.example", "https://owner.example")

	n, err := r.Revoke(ctx, 1, "https://peer.example")
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if n != 1 {
		t.Fatalf("revoke affected: got %d, want 1", n)
	}
	fp, err := r.Get(ctx, 1, "https://peer.example")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !fp.Revoked {
		t.Errorf("revoked: got false, want true")
	}

	// Idempotent: a second revoke still matches the row (1 affected) and stays revoked.
	n, err = r.Revoke(ctx, 1, "https://peer.example")
	if err != nil {
		t.Fatalf("revoke again: %v", err)
	}
	if n != 1 {
		t.Errorf("revoke again affected: got %d, want 1 (idempotent)", n)
	}
}

// TestFederatedProjectRepo_RevokeUnknownPeer asserts revoking a peer that is not
// joined reports 0 affected rows (the service maps that to a 404).
func TestFederatedProjectRepo_RevokeUnknownPeer(t *testing.T) {
	d := setupTestDB(t)
	seedFederatedProjectRow(t, d)
	r := NewFederatedProjectRepo(d)
	n, err := r.Revoke(context.Background(), 1, "https://ghost.example")
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if n != 0 {
		t.Errorf("revoke unknown peer affected: got %d, want 0", n)
	}
}

// TestFederatedProjectRepo_RevokeNeverTouchesSelfRow asserts the owner self-row
// (is_owner=1) can never be revoked: Revoke targets only is_owner=0 rows.
func TestFederatedProjectRepo_RevokeNeverTouchesSelfRow(t *testing.T) {
	d := setupTestDB(t)
	seedFederatedProjectRow(t, d)
	r := NewFederatedProjectRepo(d)
	ctx := context.Background()
	if _, err := d.Exec(
		`INSERT INTO federated_projects
		   (local_project_id, peer_instance_url, remote_project_id, is_owner, origin_instance_url, permissions, protocol_version, joined_at)
		 VALUES (1, 'https://me.example', '', 1, 'https://me.example', 'admin', 1, '2026-01-01T00:00:00.000Z')`,
	); err != nil {
		t.Fatalf("seed self-row: %v", err)
	}
	n, err := r.Revoke(ctx, 1, "https://me.example")
	if err != nil {
		t.Fatalf("revoke self: %v", err)
	}
	if n != 0 {
		t.Errorf("revoke self-row affected: got %d, want 0", n)
	}
	self, err := r.SelfRow(ctx, 1)
	if err != nil {
		t.Fatalf("self-row: %v", err)
	}
	if self.Revoked {
		t.Errorf("self-row revoked: got true, want false (never revokable)")
	}
}

// TestFederatedProjectRepo_MarkLostByOrigin asserts MarkLost stamps lost +
// lost_reason on the joiner's mapping to its origin owner, reports the
// transition, and is idempotent/sticky on redelivery (Federation v1 F5.4,
// US-6.2 AC3 + idempotency).
func TestFederatedProjectRepo_MarkLostByOrigin(t *testing.T) {
	d := setupTestDB(t)
	seedFederatedProjectRow(t, d)
	r := NewFederatedProjectRepo(d)
	ctx := context.Background()
	insertJoinerRow(t, r, "https://owner.example", "https://owner.example")

	transitioned, err := r.MarkLost(ctx, 1, "https://owner.example", model.FederationLostRevoked)
	if err != nil {
		t.Fatalf("mark lost: %v", err)
	}
	if !transitioned {
		t.Fatalf("first mark lost: got transitioned=false, want true")
	}
	fp, err := r.Get(ctx, 1, "https://owner.example")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !fp.Lost || fp.LostReason != model.FederationLostRevoked {
		t.Errorf("after mark lost: got (lost=%v, reason=%q), want (true, revoked)", fp.Lost, fp.LostReason)
	}

	// Idempotent + sticky: re-applying the same revoke does NOT re-transition and
	// does NOT overwrite the reason (US-6.2 AC4 offline-return self-detect lands on
	// an already-lost row).
	transitioned, err = r.MarkLost(ctx, 1, "https://owner.example", model.FederationLostRevoked)
	if err != nil {
		t.Fatalf("mark lost again: %v", err)
	}
	if transitioned {
		t.Errorf("second mark lost: got transitioned=true, want false (idempotent)")
	}
}

// TestFederatedProjectRepo_MarkLostUnknownOrigin asserts marking a project that
// has no joiner row for the origin is a no-op (false, no error).
func TestFederatedProjectRepo_MarkLostUnknownOrigin(t *testing.T) {
	d := setupTestDB(t)
	seedFederatedProjectRow(t, d)
	r := NewFederatedProjectRepo(d)
	transitioned, err := r.MarkLost(context.Background(), 1, "https://nobody.example", model.FederationLostRevoked)
	if err != nil {
		t.Fatalf("mark lost: %v", err)
	}
	if transitioned {
		t.Errorf("mark lost unknown origin: got transitioned=true, want false")
	}
}

// TestFederatedProjectRepo_MarkLeftByPeer asserts MarkLeftByPeer stamps lost +
// reason="left" on the OWNER's mapping to a specific peer (keyed on
// peer_instance_url, not origin), reports the transition, and is idempotent/sticky
// on redelivery (Federation v1 F5.5, US-6.3 AC2). This is the symmetric
// owner-side counterpart of MarkLost (which is keyed on origin for the joiner).
func TestFederatedProjectRepo_MarkLeftByPeer(t *testing.T) {
	d := setupTestDB(t)
	seedFederatedProjectRow(t, d)
	r := NewFederatedProjectRepo(d)
	ctx := context.Background()
	// Owner side: the peer row's origin is the OWNER's own URL; we key off the peer.
	insertJoinerRow(t, r, "https://peer.example", "https://me.example")

	transitioned, err := r.MarkLeftByPeer(ctx, 1, "https://peer.example")
	if err != nil {
		t.Fatalf("mark left: %v", err)
	}
	if !transitioned {
		t.Fatalf("first mark left: got transitioned=false, want true")
	}
	fp, err := r.Get(ctx, 1, "https://peer.example")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !fp.Lost || fp.LostReason != model.FederationLostLeft {
		t.Errorf("after mark left: got (lost=%v, reason=%q), want (true, left)", fp.Lost, fp.LostReason)
	}

	// Idempotent + sticky: re-applying the same leave does NOT re-transition.
	transitioned, err = r.MarkLeftByPeer(ctx, 1, "https://peer.example")
	if err != nil {
		t.Fatalf("mark left again: %v", err)
	}
	if transitioned {
		t.Errorf("second mark left: got transitioned=true, want false (idempotent)")
	}
}

// TestFederatedProjectRepo_MarkLeftByPeerUnknown asserts marking an unknown peer
// is a no-op (false, no error).
func TestFederatedProjectRepo_MarkLeftByPeerUnknown(t *testing.T) {
	d := setupTestDB(t)
	seedFederatedProjectRow(t, d)
	r := NewFederatedProjectRepo(d)
	transitioned, err := r.MarkLeftByPeer(context.Background(), 1, "https://nobody.example")
	if err != nil {
		t.Fatalf("mark left: %v", err)
	}
	if transitioned {
		t.Errorf("mark left unknown peer: got transitioned=true, want false")
	}
}

// TestFederatedProjectRepo_MarkLeftNeverTouchesSelfRow asserts the owner self-row
// (is_owner=1) can never be marked left: MarkLeftByPeer targets only is_owner=0.
func TestFederatedProjectRepo_MarkLeftNeverTouchesSelfRow(t *testing.T) {
	d := setupTestDB(t)
	seedFederatedProjectRow(t, d)
	r := NewFederatedProjectRepo(d)
	ctx := context.Background()
	if _, err := d.Exec(
		`INSERT INTO federated_projects
		   (local_project_id, peer_instance_url, remote_project_id, is_owner, origin_instance_url, permissions, protocol_version, joined_at)
		 VALUES (1, 'https://me.example', '', 1, 'https://me.example', 'admin', 1, '2026-01-01T00:00:00.000Z')`,
	); err != nil {
		t.Fatalf("seed self-row: %v", err)
	}
	transitioned, err := r.MarkLeftByPeer(ctx, 1, "https://me.example")
	if err != nil {
		t.Fatalf("mark left self: %v", err)
	}
	if transitioned {
		t.Errorf("mark left self-row: got transitioned=true, want false (never markable)")
	}
}

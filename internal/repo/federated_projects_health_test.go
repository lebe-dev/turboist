package repo

import (
	"context"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

// TestMarkKeyMismatch_Sticky asserts MarkKeyMismatch stamps key_mismatch_at on
// first observation and is sticky: a later observation does NOT overwrite the
// original timestamp (Federation v1 F4.3, US-4.3 AC4 — the marker stays put until
// an operator explicitly re-trusts the key in F5.6b). It returns whether the row
// transitioned (was NULL → now set) so the caller can publish an SSE only on the
// transition.
func TestMarkKeyMismatch_Sticky(t *testing.T) {
	d := setupTestDB(t)
	seedFederatedProjectRow(t, d)
	r := NewFederatedProjectRepo(d)
	ctx := context.Background()

	if err := r.UpsertPeerRow(ctx, model.FederatedProject{
		LocalProjectID:    1,
		PeerInstanceURL:   "https://bob.example",
		OriginInstanceURL: "https://me.example",
		Permissions:       model.FederationPermissionWrite,
		ProtocolVersion:   1,
		JoinedAt:          time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("seed peer: %v", err)
	}

	first := "2026-06-03T10:00:00.000Z"
	transitioned, err := r.MarkKeyMismatch(ctx, 1, "https://bob.example", first)
	if err != nil {
		t.Fatalf("first MarkKeyMismatch: %v", err)
	}
	if !transitioned {
		t.Errorf("first mark: got transitioned=false, want true (NULL → set)")
	}

	// A second observation later must NOT move the timestamp (sticky) and must
	// report no transition (so no duplicate SSE).
	second := "2026-06-03T11:00:00.000Z"
	transitioned, err = r.MarkKeyMismatch(ctx, 1, "https://bob.example", second)
	if err != nil {
		t.Fatalf("second MarkKeyMismatch: %v", err)
	}
	if transitioned {
		t.Errorf("second mark: got transitioned=true, want false (already mismatched)")
	}

	rows, err := r.ListPeerHealthByProject(ctx, 1)
	if err != nil {
		t.Fatalf("ListPeerHealthByProject: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("peer health rows: got %d, want 1", len(rows))
	}
	if rows[0].KeyMismatchAt != first {
		t.Errorf("key_mismatch_at: got %q, want %q (sticky, first observation wins)", rows[0].KeyMismatchAt, first)
	}
}

// TestClearKeyMismatch_ResetsMarkerAndAllowsReMark asserts ClearKeyMismatch wipes
// the sticky key_mismatch_at marker (Federation v1 F5.6b, US-6.4 AC3 — what the
// manual "Trust new key" action calls). After clearing, a fresh mismatch can be
// stamped again (the marker is re-armable for a LATER rotation), and clearing a
// peer with no marker is a clean no-op (0 rows, nil error).
func TestClearKeyMismatch_ResetsMarkerAndAllowsReMark(t *testing.T) {
	d := setupTestDB(t)
	seedFederatedProjectRow(t, d)
	r := NewFederatedProjectRepo(d)
	ctx := context.Background()

	if err := r.UpsertPeerRow(ctx, model.FederatedProject{
		LocalProjectID:    1,
		PeerInstanceURL:   "https://bob.example",
		OriginInstanceURL: "https://me.example",
		Permissions:       model.FederationPermissionWrite,
		ProtocolVersion:   1,
		JoinedAt:          time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("seed peer: %v", err)
	}

	// Clearing a peer with no marker is a no-op.
	n, err := r.ClearKeyMismatch(ctx, 1, "https://bob.example")
	if err != nil {
		t.Fatalf("clear no-marker: %v", err)
	}
	if n != 0 {
		t.Errorf("clear with no marker: got %d rows, want 0", n)
	}

	if _, err := r.MarkKeyMismatch(ctx, 1, "https://bob.example", "2026-06-03T10:00:00.000Z"); err != nil {
		t.Fatalf("mark: %v", err)
	}

	// Clear the marker (the trust-key action).
	n, err = r.ClearKeyMismatch(ctx, 1, "https://bob.example")
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if n != 1 {
		t.Fatalf("clear rows: got %d, want 1", n)
	}
	rows, err := r.ListPeerHealthByProject(ctx, 1)
	if err != nil {
		t.Fatalf("list health: %v", err)
	}
	if len(rows) != 1 || rows[0].KeyMismatchAt != "" {
		t.Fatalf("after clear: key_mismatch_at not empty: %+v", rows)
	}

	// A LATER rotation can be stamped again (the marker is re-armable).
	transitioned, err := r.MarkKeyMismatch(ctx, 1, "https://bob.example", "2026-06-03T12:00:00.000Z")
	if err != nil {
		t.Fatalf("re-mark: %v", err)
	}
	if !transitioned {
		t.Errorf("re-mark after clear: got transitioned=false, want true")
	}
}

// TestListPeerHealthByProject_ExcludesSelfRow asserts the health list returns the
// per-peer health inputs (revoked/paused/key_mismatch_at/last_contact_at) and
// excludes the owner self-row (US-4.3 — status is about PEERS, not self).
func TestListPeerHealthByProject_ExcludesSelfRow(t *testing.T) {
	d := setupTestDB(t)
	seedFederatedProjectRow(t, d)
	r := NewFederatedProjectRepo(d)
	instances := NewFederatedInstanceRepo(d)
	ctx := context.Background()

	// Self-row (owner): must be excluded.
	if _, err := d.Exec(
		`INSERT INTO federated_projects (local_project_id, peer_instance_url, remote_project_id, is_owner, origin_instance_url, permissions, protocol_version, joined_at)
		 VALUES (1, 'https://me.example', '', 1, 'https://me.example', 'admin', 1, '2024-01-01T00:00:00.000Z')`,
	); err != nil {
		t.Fatalf("seed self-row: %v", err)
	}

	contact := time.Date(2026, 6, 3, 9, 0, 0, 0, time.UTC)
	if err := instances.Upsert(ctx, model.FederatedInstance{
		InstanceURL: "https://bob.example", PublicKey: "pk", DisplayName: "Bob",
		LastContactAt: &contact,
		CreatedAt:     contact, UpdatedAt: contact,
	}); err != nil {
		t.Fatalf("seed instance: %v", err)
	}
	if err := r.UpsertPeerRow(ctx, model.FederatedProject{
		LocalProjectID:    1,
		PeerInstanceURL:   "https://bob.example",
		OriginInstanceURL: "https://me.example",
		Permissions:       model.FederationPermissionWrite,
		ProtocolVersion:   1,
		JoinedAt:          time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("seed peer: %v", err)
	}

	rows, err := r.ListPeerHealthByProject(ctx, 1)
	if err != nil {
		t.Fatalf("ListPeerHealthByProject: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows: got %d, want 1 (self-row excluded)", len(rows))
	}
	if rows[0].PeerInstanceURL != "https://bob.example" {
		t.Errorf("peer url: got %q, want bob", rows[0].PeerInstanceURL)
	}
	if rows[0].LastContactAt == nil {
		t.Errorf("last_contact_at: got nil, want joined from federated_instances")
	}
	if rows[0].KeyMismatchAt != "" {
		t.Errorf("key_mismatch_at: got %q, want empty (no mismatch)", rows[0].KeyMismatchAt)
	}
}

// TestListOwnedFederatedProjectIDs asserts the enumeration returns every project
// that has been enabled for federation (has an is_owner=1 self-row), so the
// status endpoint computes one status per shared project (US-4.3).
func TestListOwnedFederatedProjectIDs(t *testing.T) {
	d := setupTestDB(t)
	seedFederatedProjectRow(t, d)
	r := NewFederatedProjectRepo(d)
	ctx := context.Background()

	if _, err := d.Exec(
		`INSERT INTO federated_projects (local_project_id, peer_instance_url, remote_project_id, is_owner, origin_instance_url, permissions, protocol_version, joined_at)
		 VALUES (1, 'https://me.example', '', 1, 'https://me.example', 'admin', 1, '2024-01-01T00:00:00.000Z')`,
	); err != nil {
		t.Fatalf("seed self-row: %v", err)
	}

	ids, err := r.ListOwnedFederatedProjectIDs(ctx)
	if err != nil {
		t.Fatalf("ListOwnedFederatedProjectIDs: %v", err)
	}
	if len(ids) != 1 || ids[0] != 1 {
		t.Fatalf("owned project ids: got %v, want [1]", ids)
	}
}

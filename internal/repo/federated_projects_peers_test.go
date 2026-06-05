package repo

import (
	"context"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

// seedPeerRow inserts a federated_projects peer row (is_owner=0) plus a matching
// federated_instances directory row carrying the display_name + last_contact_at.
func seedPeerRow(t *testing.T, r *FederatedProjectRepo, instRepo *FederatedInstanceRepo, peerURL, displayName string, lastContact *time.Time, paused, revoked bool) {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := instRepo.Upsert(ctx, model.FederatedInstance{
		InstanceURL:   peerURL,
		PublicKey:     "pk-" + peerURL,
		DisplayName:   displayName,
		LastContactAt: lastContact,
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("seed instance %s: %v", peerURL, err)
	}
	p := 0
	if paused {
		p = 1
	}
	rv := 0
	if revoked {
		rv = 1
	}
	if _, err := instRepo.db.Exec(
		`INSERT INTO federated_projects
		   (local_project_id, peer_instance_url, remote_project_id, is_owner, origin_instance_url, permissions, paused, revoked, protocol_version, last_sent_hlc, joined_at)
		 VALUES (1, ?, 'remote-cid', 0, 'https://me.example', 'write', ?, ?, 1, '0000000000000-00000-node', ?)`,
		peerURL, p, rv, model.FormatUTC(now),
	); err != nil {
		t.Fatalf("seed peer row %s: %v", peerURL, err)
	}
}

// TestListPeersByProject_ExcludesSelfAndJoinsDisplayName asserts the peers query
// excludes the owner self-row and joins federated_instances.display_name +
// last_contact_at for each remote peer (US-1.4 AC1, AC2).
func TestListPeersByProject_ExcludesSelfAndJoinsDisplayName(t *testing.T) {
	d := setupTestDB(t)
	seedFederatedProjectRow(t, d)
	r := NewFederatedProjectRepo(d)
	instRepo := NewFederatedInstanceRepo(d)
	ctx := context.Background()

	// Owner self-row (must be excluded from the peers list).
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := d.Exec(
		`INSERT INTO federated_projects
		   (local_project_id, peer_instance_url, remote_project_id, is_owner, origin_instance_url, permissions, paused, revoked, protocol_version, joined_at)
		 VALUES (1, 'https://me.example', '', 1, 'https://me.example', 'admin', 0, 0, 1, ?)`,
		model.FormatUTC(now),
	); err != nil {
		t.Fatalf("seed self-row: %v", err)
	}

	lc := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	seedPeerRow(t, r, instRepo, "https://bob.example", "Bob's Box", &lc, false, false)

	peers, err := r.ListPeersByProject(ctx, 1)
	if err != nil {
		t.Fatalf("ListPeersByProject: %v", err)
	}
	if len(peers) != 1 {
		t.Fatalf("peer count: got %d, want 1 (self-row excluded)", len(peers))
	}
	got := peers[0]
	if got.PeerInstanceURL != "https://bob.example" {
		t.Errorf("peerInstanceURL: got %q, want https://bob.example", got.PeerInstanceURL)
	}
	if got.DisplayName != "Bob's Box" {
		t.Errorf("displayName: got %q, want Bob's Box", got.DisplayName)
	}
	if got.LastContactAt == nil || !got.LastContactAt.Equal(lc) {
		t.Errorf("lastContactAt: got %v, want %v", got.LastContactAt, lc)
	}
	if got.LastSentHLC != "0000000000000-00000-node" {
		t.Errorf("lastSentHLC: got %q, want the seeded hlc", got.LastSentHLC)
	}
	if got.Permissions != model.FederationPermissionWrite {
		t.Errorf("permissions: got %q, want write", got.Permissions)
	}
}

// TestListPeersByProject_PausedRevokedFlags asserts the paused/revoked per-row
// flags survive the join so the service can derive status from them (US-1.4 AC1).
func TestListPeersByProject_PausedRevokedFlags(t *testing.T) {
	d := setupTestDB(t)
	seedFederatedProjectRow(t, d)
	r := NewFederatedProjectRepo(d)
	instRepo := NewFederatedInstanceRepo(d)
	ctx := context.Background()

	seedPeerRow(t, r, instRepo, "https://paused.example", "Paused", nil, true, false)
	seedPeerRow(t, r, instRepo, "https://revoked.example", "Revoked", nil, false, true)

	peers, err := r.ListPeersByProject(ctx, 1)
	if err != nil {
		t.Fatalf("ListPeersByProject: %v", err)
	}
	if len(peers) != 2 {
		t.Fatalf("peer count: got %d, want 2", len(peers))
	}
	byURL := map[string]FederatedPeer{}
	for _, p := range peers {
		byURL[p.PeerInstanceURL] = p
	}
	if !byURL["https://paused.example"].Paused {
		t.Errorf("paused peer: Paused flag not set")
	}
	if !byURL["https://revoked.example"].Revoked {
		t.Errorf("revoked peer: Revoked flag not set")
	}
}

// TestFederatedInstanceRepo_UpsertAndGet asserts the instance directory upsert is
// idempotent on instance_url and round-trips display_name + last_contact_at.
func TestFederatedInstanceRepo_UpsertAndGet(t *testing.T) {
	d := setupTestDB(t)
	instRepo := NewFederatedInstanceRepo(d)
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	lc := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	inst := model.FederatedInstance{
		InstanceURL:   "https://carol.example",
		PublicKey:     "pk-1",
		DisplayName:   "Carol",
		LastContactAt: &lc,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := instRepo.Upsert(ctx, inst); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	// Re-upsert with a new display name; the row must be updated, not duplicated.
	inst.DisplayName = "Carol Renamed"
	if err := instRepo.Upsert(ctx, inst); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}

	got, err := instRepo.Get(ctx, "https://carol.example")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.DisplayName != "Carol Renamed" {
		t.Errorf("displayName: got %q, want Carol Renamed", got.DisplayName)
	}
	if got.LastContactAt == nil || !got.LastContactAt.Equal(lc) {
		t.Errorf("lastContactAt: got %v, want %v", got.LastContactAt, lc)
	}
}

// TestFederatedInstanceRepo_List asserts List returns every directory row ordered
// by instance_url. It backs the startup peer-key cache warm (Federation v1 F4.3
// review fix) so a real signature mismatch is a genuine key rotation, not a
// cold-cache fetch error.
func TestFederatedInstanceRepo_List(t *testing.T) {
	d := setupTestDB(t)
	instRepo := NewFederatedInstanceRepo(d)
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	if err := instRepo.Upsert(ctx, model.FederatedInstance{
		InstanceURL: "https://bob.example", PublicKey: "pk-bob", DisplayName: "Bob",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("upsert bob: %v", err)
	}
	if err := instRepo.Upsert(ctx, model.FederatedInstance{
		InstanceURL: "https://alice.example", PublicKey: "pk-alice", DisplayName: "Alice",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("upsert alice: %v", err)
	}

	got, err := instRepo.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("list len: got %d, want 2", len(got))
	}
	if got[0].InstanceURL != "https://alice.example" || got[1].InstanceURL != "https://bob.example" {
		t.Errorf("list order: got %q,%q, want alice,bob (by instance_url)", got[0].InstanceURL, got[1].InstanceURL)
	}
	if got[0].PublicKey != "pk-alice" {
		t.Errorf("publicKey carried: got %q, want pk-alice", got[0].PublicKey)
	}
}

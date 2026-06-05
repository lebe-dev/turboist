package repo

import (
	"context"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

// TestPeerInstancesByProjectIDs_NamedNonRevokedPeers asserts the resolver returns
// the named peer instances ({instance_url, display_name}) for non-owner,
// non-revoked peers of each project in ONE query (Federation v1 F6.4, US-7.1 AC3 /
// the no-N+1 data contract). The owner self-row is excluded; revoked peers are
// excluded (they are no longer visible); a project with only the owner self-row
// resolves to an empty list.
func TestPeerInstancesByProjectIDs_NamedNonRevokedPeers(t *testing.T) {
	d := setupTestDB(t)
	seedFederatedProjectRow(t, d) // context 1 + project 1
	r := NewFederatedProjectRepo(d)
	instances := NewFederatedInstanceRepo(d)
	seedProjectN(t, r, 2, "fp-cid-2")
	seedProjectN(t, r, 3, "fp-cid-3")
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ctx := context.Background()

	// Directory rows carry the handshake-supplied display_name (R24, US-7.1 AC3).
	for _, ins := range []model.FederatedInstance{
		{InstanceURL: "https://alice.example", PublicKey: "pk", DisplayName: "Alice", CreatedAt: now, UpdatedAt: now},
		{InstanceURL: "https://bob.example", PublicKey: "pk", DisplayName: "Bob", CreatedAt: now, UpdatedAt: now},
		{InstanceURL: "https://revoked.example", PublicKey: "pk", DisplayName: "Revoked", CreatedAt: now, UpdatedAt: now},
		{InstanceURL: "https://owner.example", PublicKey: "pk", DisplayName: "Owner", CreatedAt: now, UpdatedAt: now},
	} {
		if err := instances.Upsert(ctx, ins); err != nil {
			t.Fatalf("seed instance %s: %v", ins.InstanceURL, err)
		}
	}

	// Project 1: owner self-row + two live peers + one revoked peer.
	mustUpsert(t, r, model.FederatedProject{LocalProjectID: 1, PeerInstanceURL: "https://me.example", OriginInstanceURL: "https://me.example", IsOwner: true, Permissions: model.FederationPermissionAdmin, ProtocolVersion: 1, JoinedAt: now})
	mustUpsert(t, r, model.FederatedProject{LocalProjectID: 1, PeerInstanceURL: "https://alice.example", OriginInstanceURL: "https://me.example", IsOwner: false, Permissions: model.FederationPermissionWrite, ProtocolVersion: 1, JoinedAt: now})
	mustUpsert(t, r, model.FederatedProject{LocalProjectID: 1, PeerInstanceURL: "https://bob.example", OriginInstanceURL: "https://me.example", IsOwner: false, Permissions: model.FederationPermissionRead, ProtocolVersion: 1, JoinedAt: now})
	mustUpsert(t, r, model.FederatedProject{LocalProjectID: 1, PeerInstanceURL: "https://revoked.example", OriginInstanceURL: "https://me.example", IsOwner: false, Revoked: true, Permissions: model.FederationPermissionWrite, ProtocolVersion: 1, JoinedAt: now})
	// Project 2: joined copy (this instance is a peer of owner.example). The owner
	// row is is_owner=0 but it is the ORIGIN — it must NOT be listed as a peer the
	// project is "visible to" (a joined copy has no outbound peer audience).
	mustUpsert(t, r, model.FederatedProject{LocalProjectID: 2, PeerInstanceURL: "https://owner.example", OriginInstanceURL: "https://owner.example", IsOwner: false, Permissions: model.FederationPermissionWrite, ProtocolVersion: 1, JoinedAt: now})
	// Project 3: owner self-row only — no peers yet.
	mustUpsert(t, r, model.FederatedProject{LocalProjectID: 3, PeerInstanceURL: "https://me.example", OriginInstanceURL: "https://me.example", IsOwner: true, Permissions: model.FederationPermissionAdmin, ProtocolVersion: 1, JoinedAt: now})

	got, err := r.PeerInstancesByProjectIDs(ctx, []int64{1, 2, 3}, "https://me.example")
	if err != nil {
		t.Fatalf("PeerInstancesByProjectIDs: %v", err)
	}

	// Project 1: alice + bob, named; revoked excluded; self-row excluded.
	p1 := got[1]
	if len(p1) != 2 {
		t.Fatalf("project 1 peers: got %d, want 2 (self + revoked excluded), peers=%+v", len(p1), p1)
	}
	byURL := map[string]model.PeerInstance{}
	for _, pi := range p1 {
		byURL[pi.InstanceURL] = pi
	}
	if byURL["https://alice.example"].DisplayName != "Alice" {
		t.Errorf("alice displayName: got %q, want Alice (US-7.1 AC3)", byURL["https://alice.example"].DisplayName)
	}
	if byURL["https://bob.example"].DisplayName != "Bob" {
		t.Errorf("bob displayName: got %q, want Bob", byURL["https://bob.example"].DisplayName)
	}
	if _, ok := byURL["https://revoked.example"]; ok {
		t.Errorf("revoked peer must not be listed as visible-to")
	}

	// Project 2: the joined copy's origin owner is NOT a visible-to peer.
	if len(got[2]) != 0 {
		t.Errorf("project 2 (joined copy) peers: got %d, want 0 (origin owner is not an audience peer)", len(got[2]))
	}

	// Project 3: owner self-row only — no peers.
	if len(got[3]) != 0 {
		t.Errorf("project 3 peers: got %d, want 0 (self-row only)", len(got[3]))
	}
}

// TestPeerInstancesByProjectIDs_ExcludesLeftPeer asserts a peer that VOLUNTARILY
// left a project (lost=1, lost_reason='left', revoked still 0 — see MarkLeftByPeer)
// is dropped from the visible-to audience exactly as a revoked peer is (Federation
// v1 F6.4, US-7.1). The departed peer must not linger in the 'visible to N peers'
// badge, the QuickAdd new-task hint, or the federation overview peers array.
func TestPeerInstancesByProjectIDs_ExcludesLeftPeer(t *testing.T) {
	d := setupTestDB(t)
	seedFederatedProjectRow(t, d) // context 1 + project 1
	r := NewFederatedProjectRepo(d)
	instances := NewFederatedInstanceRepo(d)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ctx := context.Background()

	for _, ins := range []model.FederatedInstance{
		{InstanceURL: "https://stay.example", PublicKey: "pk", DisplayName: "Stay", CreatedAt: now, UpdatedAt: now},
		{InstanceURL: "https://gone.example", PublicKey: "pk", DisplayName: "Gone", CreatedAt: now, UpdatedAt: now},
	} {
		if err := instances.Upsert(ctx, ins); err != nil {
			t.Fatalf("seed instance %s: %v", ins.InstanceURL, err)
		}
	}

	// Owner self-row + a live peer + a peer that will leave.
	mustUpsert(t, r, model.FederatedProject{LocalProjectID: 1, PeerInstanceURL: "https://me.example", OriginInstanceURL: "https://me.example", IsOwner: true, Permissions: model.FederationPermissionAdmin, ProtocolVersion: 1, JoinedAt: now})
	mustUpsert(t, r, model.FederatedProject{LocalProjectID: 1, PeerInstanceURL: "https://stay.example", OriginInstanceURL: "https://me.example", IsOwner: false, Permissions: model.FederationPermissionWrite, ProtocolVersion: 1, JoinedAt: now})
	mustUpsert(t, r, model.FederatedProject{LocalProjectID: 1, PeerInstanceURL: "https://gone.example", OriginInstanceURL: "https://me.example", IsOwner: false, Permissions: model.FederationPermissionWrite, ProtocolVersion: 1, JoinedAt: now})

	// The peer leaves voluntarily: lost=1, lost_reason='left', revoked stays 0.
	transitioned, err := r.MarkLeftByPeer(ctx, 1, "https://gone.example")
	if err != nil {
		t.Fatalf("MarkLeftByPeer: %v", err)
	}
	if !transitioned {
		t.Fatalf("MarkLeftByPeer: got transitioned=false, want true")
	}

	got, err := r.PeerInstancesByProjectIDs(ctx, []int64{1}, "https://me.example")
	if err != nil {
		t.Fatalf("PeerInstancesByProjectIDs: %v", err)
	}

	p1 := got[1]
	if len(p1) != 1 {
		t.Fatalf("project 1 peers: got %d, want 1 (left peer excluded), peers=%+v", len(p1), p1)
	}
	if p1[0].InstanceURL != "https://stay.example" {
		t.Errorf("remaining peer: got %q, want https://stay.example", p1[0].InstanceURL)
	}
	for _, pi := range p1 {
		if pi.InstanceURL == "https://gone.example" {
			t.Errorf("left peer https://gone.example must not be listed as visible-to (US-7.1)")
		}
	}
}

// TestPeerInstancesByProjectIDs_IncludesPausedPeer asserts a PAUSED peer remains in
// the visible-to audience: pausing keeps the trust link and the data already
// shared, so the peer is still part of the project's current audience (Federation
// v1 F6.4, US-7.1 — documented intent, not an oversight).
func TestPeerInstancesByProjectIDs_IncludesPausedPeer(t *testing.T) {
	d := setupTestDB(t)
	seedFederatedProjectRow(t, d)
	r := NewFederatedProjectRepo(d)
	instances := NewFederatedInstanceRepo(d)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ctx := context.Background()

	if err := instances.Upsert(ctx, model.FederatedInstance{InstanceURL: "https://paused.example", PublicKey: "pk", DisplayName: "Paused", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("seed instance: %v", err)
	}

	mustUpsert(t, r, model.FederatedProject{LocalProjectID: 1, PeerInstanceURL: "https://me.example", OriginInstanceURL: "https://me.example", IsOwner: true, Permissions: model.FederationPermissionAdmin, ProtocolVersion: 1, JoinedAt: now})
	mustUpsert(t, r, model.FederatedProject{LocalProjectID: 1, PeerInstanceURL: "https://paused.example", OriginInstanceURL: "https://me.example", IsOwner: false, Paused: true, Permissions: model.FederationPermissionWrite, ProtocolVersion: 1, JoinedAt: now})

	got, err := r.PeerInstancesByProjectIDs(ctx, []int64{1}, "https://me.example")
	if err != nil {
		t.Fatalf("PeerInstancesByProjectIDs: %v", err)
	}
	if len(got[1]) != 1 {
		t.Fatalf("project 1 peers: got %d, want 1 (paused peer stays visible)", len(got[1]))
	}
	if got[1][0].InstanceURL != "https://paused.example" {
		t.Errorf("paused peer: got %q, want https://paused.example", got[1][0].InstanceURL)
	}
}

// TestPeerInstancesByProjectIDs_Empty asserts an empty id slice short-circuits to
// an empty map without a query (Federation v1 F6.4).
func TestPeerInstancesByProjectIDs_Empty(t *testing.T) {
	d := setupTestDB(t)
	r := NewFederatedProjectRepo(d)
	got, err := r.PeerInstancesByProjectIDs(context.Background(), nil, "https://me.example")
	if err != nil {
		t.Fatalf("PeerInstancesByProjectIDs: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("empty ids: got %d entries, want 0", len(got))
	}
}

// TestPeerInstancesByProjectIDs_FallsBackToURL asserts a peer whose directory row
// has not been written yet (empty display_name) still appears with its URL as the
// display name, so the new-task hint can always render a name (Federation v1 F6.4).
func TestPeerInstancesByProjectIDs_FallsBackToURL(t *testing.T) {
	d := setupTestDB(t)
	seedFederatedProjectRow(t, d)
	r := NewFederatedProjectRepo(d)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ctx := context.Background()

	mustUpsert(t, r, model.FederatedProject{LocalProjectID: 1, PeerInstanceURL: "https://me.example", OriginInstanceURL: "https://me.example", IsOwner: true, Permissions: model.FederationPermissionAdmin, ProtocolVersion: 1, JoinedAt: now})
	// No federated_instances row for this peer → display_name LEFT JOINs to NULL.
	mustUpsert(t, r, model.FederatedProject{LocalProjectID: 1, PeerInstanceURL: "https://noname.example", OriginInstanceURL: "https://me.example", IsOwner: false, Permissions: model.FederationPermissionWrite, ProtocolVersion: 1, JoinedAt: now})

	got, err := r.PeerInstancesByProjectIDs(ctx, []int64{1}, "https://me.example")
	if err != nil {
		t.Fatalf("PeerInstancesByProjectIDs: %v", err)
	}
	if len(got[1]) != 1 {
		t.Fatalf("project 1 peers: got %d, want 1", len(got[1]))
	}
	if got[1][0].DisplayName != "https://noname.example" {
		t.Errorf("fallback displayName: got %q, want the URL", got[1][0].DisplayName)
	}
}

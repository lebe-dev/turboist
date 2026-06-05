package repo

import (
	"context"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

// seedProjectN inserts a project with the given id (context id=1 must already
// exist) so additional federated_projects rows have a distinct FK target.
func seedProjectN(t *testing.T, r *FederatedProjectRepo, id int64, clientID string) {
	t.Helper()
	if _, err := r.db.ExecContext(context.Background(),
		`INSERT INTO projects (id, context_id, title, description, color, status, is_pinned, client_id, created_at, updated_at)
		 VALUES (?, 1, 'p', '', 'blue', 'open', 0, ?, '2024-01-01T00:00:00.000Z', '2024-01-01T00:00:00.000Z')`,
		id, clientID); err != nil {
		t.Fatalf("seed project %d: %v", id, err)
	}
}

// TestFederationSurfaceByProjectIDs_OwnerSelfRow asserts that for an owner-enabled
// project the surface reports IsOwner=true and the local instance as origin
// (Federation v1 F2.4, US-2.4 AC1). The owner must never be read-only on its own
// project.
func TestFederationSurfaceByProjectIDs_OwnerSelfRow(t *testing.T) {
	d := setupTestDB(t)
	seedFederatedProjectRow(t, d)
	r := NewFederatedProjectRepo(d)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := r.UpsertPeerRow(context.Background(), model.FederatedProject{
		LocalProjectID:    1,
		PeerInstanceURL:   "https://me.example",
		OriginInstanceURL: "https://me.example",
		IsOwner:           true,
		Permissions:       model.FederationPermissionAdmin,
		ProtocolVersion:   1,
		JoinedAt:          now,
	}); err != nil {
		t.Fatalf("seed self row: %v", err)
	}

	got, err := r.FederationSurfaceByProjectIDs(context.Background(), []int64{1})
	if err != nil {
		t.Fatalf("surface: %v", err)
	}
	s, ok := got[1]
	if !ok {
		t.Fatalf("project 1 missing from surface")
	}
	if !s.IsOwner {
		t.Errorf("isOwner: got %v, want true", s.IsOwner)
	}
	if s.OriginInstanceURL != "https://me.example" {
		t.Errorf("originInstance: got %q, want https://me.example", s.OriginInstanceURL)
	}
	if s.Permissions != model.FederationPermissionAdmin {
		t.Errorf("permissions: got %q, want admin", s.Permissions)
	}
}

// TestFederationSurfaceByProjectIDs_JoinerReadRow asserts that for a joined
// read-only project the surface reports IsOwner=false, the owner as origin, and
// the granted read permission (Federation v1 F2.4, US-2.4 AC1/AC2). This is the
// data the DTO exposes and the read-only guard keys on.
func TestFederationSurfaceByProjectIDs_JoinerReadRow(t *testing.T) {
	d := setupTestDB(t)
	seedFederatedProjectRow(t, d)
	r := NewFederatedProjectRepo(d)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// Joiner row: is_owner=0, peer == origin == owner URL, permissions=read.
	if err := r.UpsertPeerRow(context.Background(), model.FederatedProject{
		LocalProjectID:    1,
		PeerInstanceURL:   "https://owner.example",
		OriginInstanceURL: "https://owner.example",
		IsOwner:           false,
		Permissions:       model.FederationPermissionRead,
		ProtocolVersion:   1,
		JoinedAt:          now,
	}); err != nil {
		t.Fatalf("seed joiner row: %v", err)
	}

	got, err := r.FederationSurfaceByProjectIDs(context.Background(), []int64{1})
	if err != nil {
		t.Fatalf("surface: %v", err)
	}
	s, ok := got[1]
	if !ok {
		t.Fatalf("project 1 missing from surface")
	}
	if s.IsOwner {
		t.Errorf("isOwner: got true, want false (joiner)")
	}
	if s.OriginInstanceURL != "https://owner.example" {
		t.Errorf("originInstance: got %q, want https://owner.example", s.OriginInstanceURL)
	}
	if s.Permissions != model.FederationPermissionRead {
		t.Errorf("permissions: got %q, want read", s.Permissions)
	}
}

// TestFederationSurfaceByProjectIDs_NonFederated asserts a project with no
// federated_projects row is simply absent from the surface map (Federation v1
// F2.4): the DTO then omits the federation fields and the guard is a no-op.
func TestFederationSurfaceByProjectIDs_NonFederated(t *testing.T) {
	d := setupTestDB(t)
	seedFederatedProjectRow(t, d)
	r := NewFederatedProjectRepo(d)

	got, err := r.FederationSurfaceByProjectIDs(context.Background(), []int64{1})
	if err != nil {
		t.Fatalf("surface: %v", err)
	}
	if _, ok := got[1]; ok {
		t.Errorf("non-federated project 1 should be absent from surface, got %+v", got[1])
	}
}

// TestFederationSurfaceByProjectIDs_BatchMixed asserts the surface resolves many
// project ids in one query (no N+1) and prefers the owner self-row over any peer
// row for the same project (Federation v1 F2.4).
func TestFederationSurfaceByProjectIDs_BatchMixed(t *testing.T) {
	d := setupTestDB(t)
	seedFederatedProjectRow(t, d) // project 1
	r := NewFederatedProjectRepo(d)
	seedProjectN(t, r, 2, "fp-cid-2")
	seedProjectN(t, r, 3, "fp-cid-3")
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Project 1: owner with an additional peer — owner self-row must win.
	mustUpsert(t, r, model.FederatedProject{LocalProjectID: 1, PeerInstanceURL: "https://me.example", OriginInstanceURL: "https://me.example", IsOwner: true, Permissions: model.FederationPermissionAdmin, ProtocolVersion: 1, JoinedAt: now})
	mustUpsert(t, r, model.FederatedProject{LocalProjectID: 1, PeerInstanceURL: "https://bob.example", OriginInstanceURL: "https://me.example", IsOwner: false, Permissions: model.FederationPermissionWrite, ProtocolVersion: 1, JoinedAt: now})
	// Project 2: joined read-only.
	mustUpsert(t, r, model.FederatedProject{LocalProjectID: 2, PeerInstanceURL: "https://owner.example", OriginInstanceURL: "https://owner.example", IsOwner: false, Permissions: model.FederationPermissionRead, ProtocolVersion: 1, JoinedAt: now})
	// Project 3: non-federated (no row).

	got, err := r.FederationSurfaceByProjectIDs(context.Background(), []int64{1, 2, 3})
	if err != nil {
		t.Fatalf("surface: %v", err)
	}
	if !got[1].IsOwner || got[1].Permissions != model.FederationPermissionAdmin {
		t.Errorf("project 1: got %+v, want owner/admin (self-row wins)", got[1])
	}
	if got[2].IsOwner || got[2].Permissions != model.FederationPermissionRead {
		t.Errorf("project 2: got %+v, want joiner/read", got[2])
	}
	if _, ok := got[3]; ok {
		t.Errorf("project 3 should be absent (non-federated)")
	}
}

// TestFederationSurfaceByProjectIDs_OwnerLastContact asserts the surface joins the
// OWNER instance's last_contact_at onto a JOINED project row (Federation v1 F5.6a,
// US-6.5 AC1) so the joiner can derive owner-offline. For a joined row the owner is
// origin_instance_url; the directory contact recency is surfaced verbatim. The
// owner's OWN project (self-row) carries no owner-offline notion, so its surface
// owner last-contact stays nil.
func TestFederationSurfaceByProjectIDs_OwnerLastContact(t *testing.T) {
	d := setupTestDB(t)
	seedFederatedProjectRow(t, d) // project 1
	r := NewFederatedProjectRepo(d)
	seedProjectN(t, r, 2, "fp-cid-2")
	instances := NewFederatedInstanceRepo(d)
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	contact := time.Date(2026, 5, 30, 9, 0, 0, 0, time.UTC)

	// The owner's directory row carries last_contact_at — the freshness the joiner
	// uses to detect owner-death (US-6.5 AC1).
	if err := instances.Upsert(ctx, model.FederatedInstance{
		InstanceURL: "https://owner.example", PublicKey: "pk", DisplayName: "Owner",
		LastContactAt: &contact, CreatedAt: contact, UpdatedAt: contact,
	}); err != nil {
		t.Fatalf("seed owner instance: %v", err)
	}

	// Project 1: joined copy of owner.example.
	mustUpsert(t, r, model.FederatedProject{LocalProjectID: 1, PeerInstanceURL: "https://owner.example", OriginInstanceURL: "https://owner.example", IsOwner: false, Permissions: model.FederationPermissionWrite, ProtocolVersion: 1, JoinedAt: now})
	// Project 2: the owner's OWN federated project (self-row) — no owner-offline.
	mustUpsert(t, r, model.FederatedProject{LocalProjectID: 2, PeerInstanceURL: "https://me.example", OriginInstanceURL: "https://me.example", IsOwner: true, Permissions: model.FederationPermissionAdmin, ProtocolVersion: 1, JoinedAt: now})

	got, err := r.FederationSurfaceByProjectIDs(ctx, []int64{1, 2})
	if err != nil {
		t.Fatalf("surface: %v", err)
	}
	if got[1].OwnerLastContactAt == nil {
		t.Fatalf("project 1 owner last_contact_at: got nil, want %s (joined from owner directory)", contact)
	}
	if !got[1].OwnerLastContactAt.Equal(contact) {
		t.Errorf("project 1 owner last_contact_at: got %s, want %s", *got[1].OwnerLastContactAt, contact)
	}
	if got[2].OwnerLastContactAt != nil {
		t.Errorf("project 2 (owner self-row) owner last_contact_at: got %s, want nil", *got[2].OwnerLastContactAt)
	}
}

func mustUpsert(t *testing.T, r *FederatedProjectRepo, fp model.FederatedProject) {
	t.Helper()
	if err := r.UpsertPeerRow(context.Background(), fp); err != nil {
		t.Fatalf("upsert row: %v", err)
	}
}

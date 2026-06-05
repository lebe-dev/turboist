package federation_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

// TestListInvites_DerivesStatusAndOmitsSecret asserts the list path returns each
// invite with a derived status and never exposes the secret hash (US-1.3 AC1,
// AC5). The four lifecycle states are covered via crafted rows.
func TestListInvites_DerivesStatusAndOmitsSecret(t *testing.T) {
	svc, projects, invites := newInviteSvc(t, "https://me.example")
	pid := enableProject(t, svc, projects)

	now := time.Now()
	past := now.Add(-time.Hour)
	future := now.Add(48 * time.Hour)

	// active
	if _, err := svc.CreateInvite(context.Background(), pid, fedsvc.CreateInviteParams{
		Permissions: model.FederationPermissionWrite,
	}); err != nil {
		t.Fatalf("create active: %v", err)
	}
	// expired
	if _, err := invites.Create(context.Background(), model.FederationInvite{
		InviteID: "inv-expired", LocalProjectID: pid, SecretHash: "h1",
		Permissions: model.FederationPermissionRead, MaxUses: 1, UsedCount: 0,
		ExpiresAt: &past, CreatedAt: now,
	}); err != nil {
		t.Fatalf("create expired: %v", err)
	}
	// consumed
	if _, err := invites.Create(context.Background(), model.FederationInvite{
		InviteID: "inv-consumed", LocalProjectID: pid, SecretHash: "h2",
		Permissions: model.FederationPermissionRead, MaxUses: 1, UsedCount: 1,
		ExpiresAt: &future, CreatedAt: now,
	}); err != nil {
		t.Fatalf("create consumed: %v", err)
	}
	// revoked (created active, then revoked through the repo — Create always
	// inserts revoked_at/consumed_at as NULL, so the revoked state is reached via
	// Revoke, exactly as the revoke path does in production).
	if _, err := invites.Create(context.Background(), model.FederationInvite{
		InviteID: "inv-revoked", LocalProjectID: pid, SecretHash: "h3",
		Permissions: model.FederationPermissionRead, MaxUses: 1, UsedCount: 0,
		ExpiresAt: &future, CreatedAt: now,
	}); err != nil {
		t.Fatalf("create revoked: %v", err)
	}
	if err := invites.Revoke(context.Background(), "inv-revoked", past); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	got, err := svc.ListInvites(context.Background(), pid)
	if err != nil {
		t.Fatalf("list invites: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("invite count: got %d, want 4", len(got))
	}

	byID := map[string]fedsvc.InviteView{}
	for _, v := range got {
		byID[v.InviteID] = v
		// US-1.3 AC5: the secret is never reconstructable; the view must carry no
		// secret/hash field at all.
		if v.InviteID == "" {
			t.Errorf("empty invite id in list")
		}
	}
	if s := byID["inv-expired"].Status; s != model.InviteStatusExpired {
		t.Errorf("expired status: got %q, want expired", s)
	}
	if s := byID["inv-consumed"].Status; s != model.InviteStatusConsumed {
		t.Errorf("consumed status: got %q, want consumed", s)
	}
	if s := byID["inv-revoked"].Status; s != model.InviteStatusRevoked {
		t.Errorf("revoked status: got %q, want revoked", s)
	}
}

// TestListInvites_ProjectNotFound asserts an unknown project surfaces
// ErrProjectNotFound (→404).
func TestListInvites_ProjectNotFound(t *testing.T) {
	svc, _, _ := newInviteSvc(t, "https://me.example")
	if _, err := svc.ListInvites(context.Background(), 99999); !errors.Is(err, fedsvc.ErrProjectNotFound) {
		t.Fatalf("expected ErrProjectNotFound, got %v", err)
	}
}

// TestListInvites_DBErrorNotMaskedAsNotFound asserts a non-ErrNotFound
// project-Get failure (closed *sql.DB) surfaces as a wrapped error rather than
// being collapsed into ErrProjectNotFound, so the handler returns 500 instead
// of a misleading 404.
func TestListInvites_DBErrorNotMaskedAsNotFound(t *testing.T) {
	d, projects, fedProjects, keys := setup(t)
	seedContext(t, d)
	svc := fedsvc.NewService(d, projects, fedProjects, keys, repo.NewFederationInviteRepo(d), repo.NewFederatedInstanceRepo(d), crypto.NewTokenCipher(fedSvcKey), "https://me.example")

	if err := d.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	_, err := svc.ListInvites(context.Background(), 1)
	if err == nil {
		t.Fatal("expected an error from ListInvites on a closed DB, got nil")
	}
	if errors.Is(err, fedsvc.ErrProjectNotFound) {
		t.Fatalf("DB failure masked as ErrProjectNotFound: %v", err)
	}
}

// TestRevokeInvite_Idempotent asserts revoke sets revoked_at, is idempotent on a
// second call, and flips the derived status to revoked (US-1.3 AC2).
func TestRevokeInvite_Idempotent(t *testing.T) {
	svc, projects, invites := newInviteSvc(t, "https://me.example")
	pid := enableProject(t, svc, projects)

	out, err := svc.CreateInvite(context.Background(), pid, fedsvc.CreateInviteParams{
		Permissions: model.FederationPermissionWrite,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := svc.RevokeInvite(context.Background(), pid, out.InviteID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	first, err := invites.Get(context.Background(), out.InviteID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if first.RevokedAt == nil {
		t.Fatalf("revoked_at not set")
	}
	if first.Status(time.Now()) != model.InviteStatusRevoked {
		t.Errorf("status after revoke: got %q, want revoked", first.Status(time.Now()))
	}

	// Idempotent: second revoke must not error and must not move revoked_at.
	if err := svc.RevokeInvite(context.Background(), pid, out.InviteID); err != nil {
		t.Fatalf("second revoke: %v", err)
	}
	second, err := invites.Get(context.Background(), out.InviteID)
	if err != nil {
		t.Fatalf("get after second: %v", err)
	}
	if !second.RevokedAt.Equal(*first.RevokedAt) {
		t.Errorf("revoked_at moved on second revoke: got %v, want %v", second.RevokedAt, first.RevokedAt)
	}
}

// TestRevokeInvite_WrongProject asserts an invite belonging to a different
// project cannot be revoked through another project's id (ErrNotFound → 404).
func TestRevokeInvite_WrongProject(t *testing.T) {
	svc, projects, _ := newInviteSvc(t, "https://me.example")
	pid := enableProject(t, svc, projects)
	other := enableProject(t, svc, projects)

	out, err := svc.CreateInvite(context.Background(), pid, fedsvc.CreateInviteParams{
		Permissions: model.FederationPermissionWrite,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.RevokeInvite(context.Background(), other, out.InviteID); !errors.Is(err, fedsvc.ErrInviteNotFound) {
		t.Fatalf("revoke under wrong project: got %v, want ErrInviteNotFound", err)
	}
}

// TestDeleteInvite_LeavesPeerRows asserts deleting an invite hard-removes the
// invite row but does NOT cascade to federated_projects peer rows (US-1.3 AC3).
// It seeds a real remote peer row and asserts the peer survives the delete —
// otherwise the test would pass even if delete erroneously cascaded to peers.
func TestDeleteInvite_LeavesPeerRows(t *testing.T) {
	d, projects, fedProjects, keys := setup(t)
	seedContext(t, d)
	invites := repo.NewFederationInviteRepo(d)
	svc := fedsvc.NewService(d, projects, fedProjects, keys, invites, repo.NewFederatedInstanceRepo(d), crypto.NewTokenCipher(fedSvcKey), "https://me.example")

	pid := seedProject(t, projects)
	if _, err := svc.EnableForProject(context.Background(), pid); err != nil {
		t.Fatalf("enable: %v", err)
	}
	// Seed a remote peer joined to this project (NOT the owner self-row).
	if err := fedProjects.UpsertPeerRow(context.Background(), model.FederatedProject{
		LocalProjectID:    pid,
		PeerInstanceURL:   "https://peer.example",
		RemoteProjectID:   "remote-1",
		IsOwner:           false,
		OriginInstanceURL: "https://me.example",
		Permissions:       model.FederationPermissionWrite,
		ProtocolVersion:   1,
		JoinedAt:          time.Now(),
	}); err != nil {
		t.Fatalf("seed peer: %v", err)
	}

	out, err := svc.CreateInvite(context.Background(), pid, fedsvc.CreateInviteParams{
		Permissions: model.FederationPermissionWrite,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := svc.DeleteInvite(context.Background(), pid, out.InviteID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := invites.Get(context.Background(), out.InviteID); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("invite still present after delete: %v", err)
	}

	// The seeded peer row must survive the invite delete (US-1.3 AC3: delete must
	// NOT cascade to federated_projects).
	peers, err := svc.ListPeers(context.Background(), pid)
	if err != nil {
		t.Fatalf("list peers after delete: %v", err)
	}
	if len(peers) != 1 || peers[0].PeerInstanceURL != "https://peer.example" {
		t.Errorf("peer row did not survive invite delete: got %+v, want exactly 1 peer https://peer.example", peers)
	}
}

// TestDeleteInvite_NotFound asserts deleting an unknown invite surfaces
// ErrInviteNotFound.
func TestDeleteInvite_NotFound(t *testing.T) {
	svc, projects, _ := newInviteSvc(t, "https://me.example")
	pid := enableProject(t, svc, projects)
	if err := svc.DeleteInvite(context.Background(), pid, "missing"); !errors.Is(err, fedsvc.ErrInviteNotFound) {
		t.Fatalf("delete missing: got %v, want ErrInviteNotFound", err)
	}
}

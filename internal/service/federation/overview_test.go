package federation_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

// newOverviewSvc builds a federation service wired with the instance directory so
// Overview can join peer display_name (Federation v1 F6.4, US-7.1).
func newOverviewSvc(t *testing.T, instanceURL string) (*sql.DB, *fedsvc.Service, *repo.ProjectRepo, *repo.FederatedProjectRepo, *repo.FederatedInstanceRepo) {
	t.Helper()
	d, projects, fedProjects, keys := setup(t)
	seedContext(t, d)
	instances := repo.NewFederatedInstanceRepo(d)
	svc := fedsvc.NewService(d, projects, fedProjects, keys, repo.NewFederationInviteRepo(d), instances, crypto.NewTokenCipher(fedSvcKey), instanceURL)
	return d, svc, projects, fedProjects, instances
}

// TestOverview_RoleDerivationAndPeerList asserts the overview reports each
// federated project's role (owner/peer/read-only) and its named peer list
// (Federation v1 F6.4, US-7.1 AC1). Non-federated projects are excluded.
func TestOverview_RoleDerivationAndPeerList(t *testing.T) {
	d, svc, projects, fp, instances := newOverviewSvc(t, "https://me.example")
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Project A: owner-enabled, two named peers.
	ownerPID := seedProject(t, projects)
	if _, err := svc.EnableForProject(ctx, ownerPID); err != nil {
		t.Fatalf("enable owner project: %v", err)
	}
	for _, ins := range []model.FederatedInstance{
		{InstanceURL: "https://alice.example", PublicKey: "pk", DisplayName: "Alice", CreatedAt: now, UpdatedAt: now},
		{InstanceURL: "https://bob.example", PublicKey: "pk", DisplayName: "Bob", CreatedAt: now, UpdatedAt: now},
	} {
		if err := instances.Upsert(ctx, ins); err != nil {
			t.Fatalf("seed instance %s: %v", ins.InstanceURL, err)
		}
	}
	if err := fp.UpsertPeerRow(ctx, model.FederatedProject{LocalProjectID: ownerPID, PeerInstanceURL: "https://alice.example", OriginInstanceURL: "https://me.example", Permissions: model.FederationPermissionWrite, ProtocolVersion: 1, JoinedAt: now}); err != nil {
		t.Fatalf("seed alice: %v", err)
	}
	if err := fp.UpsertPeerRow(ctx, model.FederatedProject{LocalProjectID: ownerPID, PeerInstanceURL: "https://bob.example", OriginInstanceURL: "https://me.example", Permissions: model.FederationPermissionRead, ProtocolVersion: 1, JoinedAt: now}); err != nil {
		t.Fatalf("seed bob: %v", err)
	}

	// Project B: a joined read-only copy from owner.example.
	joinedPID := seedProject(t, projects)
	markFederated(t, d, joinedPID)
	if err := instances.Upsert(ctx, model.FederatedInstance{InstanceURL: "https://owner.example", PublicKey: "pk", DisplayName: "Owner", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("seed owner instance: %v", err)
	}
	if err := fp.UpsertPeerRow(ctx, model.FederatedProject{LocalProjectID: joinedPID, PeerInstanceURL: "https://owner.example", OriginInstanceURL: "https://owner.example", Permissions: model.FederationPermissionRead, ProtocolVersion: 1, JoinedAt: now}); err != nil {
		t.Fatalf("seed joined: %v", err)
	}

	// Project C: a plain non-federated project — must be excluded.
	_ = seedProject(t, projects)

	rows, err := svc.Overview(ctx)
	if err != nil {
		t.Fatalf("Overview: %v", err)
	}

	byID := map[int64]fedsvc.ProjectOverview{}
	for _, r := range rows {
		byID[r.LocalProjectID] = r
	}
	if len(rows) != 2 {
		t.Fatalf("overview rows: got %d, want 2 (non-federated excluded), rows=%+v", len(rows), rows)
	}

	owner, ok := byID[ownerPID]
	if !ok {
		t.Fatalf("owner project missing from overview")
	}
	if owner.Role != model.FederationRoleOwner {
		t.Errorf("owner role: got %q, want owner (US-7.1 AC1)", owner.Role)
	}
	if len(owner.Peers) != 2 {
		t.Fatalf("owner peers: got %d, want 2, peers=%+v", len(owner.Peers), owner.Peers)
	}
	peerNames := map[string]string{}
	for _, p := range owner.Peers {
		peerNames[p.InstanceURL] = p.DisplayName
	}
	if peerNames["https://alice.example"] != "Alice" || peerNames["https://bob.example"] != "Bob" {
		t.Errorf("owner peer names: got %+v, want Alice/Bob (US-7.1 AC1)", peerNames)
	}

	joined, ok := byID[joinedPID]
	if !ok {
		t.Fatalf("joined project missing from overview")
	}
	if joined.Role != model.FederationRoleReadOnly {
		t.Errorf("joined role: got %q, want read-only (US-7.1 AC1)", joined.Role)
	}
	// A joined read-only copy has no outbound audience peers of its own.
	if len(joined.Peers) != 0 {
		t.Errorf("joined peers: got %d, want 0 (origin owner is not an audience)", len(joined.Peers))
	}
}

// markFederated flips is_federated on an existing project so a JOINED copy
// (which is not enabled via the owner-enable path) is surfaced by Overview.
func markFederated(t *testing.T, d *sql.DB, pid int64) {
	t.Helper()
	if _, err := d.ExecContext(context.Background(),
		`UPDATE projects SET is_federated = 1 WHERE id = ?`, pid); err != nil {
		t.Fatalf("mark federated: %v", err)
	}
}

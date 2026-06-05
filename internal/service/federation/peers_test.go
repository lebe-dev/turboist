package federation_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/federation/store"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

// newPeersSvc builds a federation service wired with the instance directory repo
// (so ListPeers can join display_name) and returns it alongside the repos needed
// to seed peer fixtures.
func newPeersSvc(t *testing.T, instanceURL string) (*fedsvc.Service, *repo.ProjectRepo, *repo.FederatedProjectRepo, *repo.FederatedInstanceRepo) {
	t.Helper()
	d, projects, fedProjects, keys := setup(t)
	seedContext(t, d)
	instances := repo.NewFederatedInstanceRepo(d)
	svc := fedsvc.NewService(d, projects, fedProjects, keys, repo.NewFederationInviteRepo(d), instances, crypto.NewTokenCipher(fedSvcKey), instanceURL)
	return svc, projects, fedProjects, instances
}

// seedPeer inserts a federated_instances directory row plus a federated_projects
// peer mapping (is_owner=0) for the given project.
func seedPeer(t *testing.T, fp *repo.FederatedProjectRepo, instances *repo.FederatedInstanceRepo, pid int64, peerURL, displayName string, lastContact *time.Time, paused, revoked bool) {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := instances.Upsert(ctx, model.FederatedInstance{
		InstanceURL:   peerURL,
		PublicKey:     "pk",
		DisplayName:   displayName,
		LastContactAt: lastContact,
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("seed instance: %v", err)
	}
	if err := fp.UpsertPeerRow(ctx, model.FederatedProject{
		LocalProjectID:    pid,
		PeerInstanceURL:   peerURL,
		RemoteProjectID:   "remote-cid",
		IsOwner:           false,
		OriginInstanceURL: "https://me.example",
		Permissions:       model.FederationPermissionWrite,
		Paused:            paused,
		Revoked:           revoked,
		ProtocolVersion:   1,
		LastSentHLC:       "0000000000000-00000-node",
		JoinedAt:          now,
	}); err != nil {
		t.Fatalf("seed peer row: %v", err)
	}
}

// TestListPeers_ExcludesSelfAndDerivesStatus asserts ListPeers excludes the owner
// self-row, joins display_name, and derives status with the revoked > paused >
// stale > active precedence (US-1.4 AC1, AC2, AC3). pendingDelivery is present and
// 0 until the outbox lands (US-1.4 AC4 partial).
func TestListPeers_ExcludesSelfAndDerivesStatus(t *testing.T) {
	svc, projects, fp, instances := newPeersSvc(t, "https://me.example")
	pid := seedProject(t, projects)
	if _, err := svc.EnableForProject(context.Background(), pid); err != nil {
		t.Fatalf("enable: %v", err)
	}

	recent := time.Now().Add(-1 * time.Hour)
	old := time.Now().Add(-48 * time.Hour)
	seedPeer(t, fp, instances, pid, "https://active.example", "Active Peer", &recent, false, false)
	seedPeer(t, fp, instances, pid, "https://stale.example", "Stale Peer", &old, false, false)
	seedPeer(t, fp, instances, pid, "https://paused.example", "Paused Peer", &recent, true, false)
	seedPeer(t, fp, instances, pid, "https://revoked.example", "Revoked Peer", &recent, false, true)

	peers, err := svc.ListPeers(context.Background(), pid)
	if err != nil {
		t.Fatalf("ListPeers: %v", err)
	}
	if len(peers) != 4 {
		t.Fatalf("peer count: got %d, want 4 (self-row excluded, US-1.4 AC1)", len(peers))
	}

	byURL := map[string]fedsvc.PeerView{}
	for _, p := range peers {
		byURL[p.PeerInstanceURL] = p
		// US-1.4 AC4 partial: pendingDelivery present and 0 (no outbox yet).
		if p.PendingDelivery != 0 {
			t.Errorf("%s pendingDelivery: got %d, want 0", p.PeerInstanceURL, p.PendingDelivery)
		}
		// US-1.4 AC1: self-row never appears.
		if p.PeerInstanceURL == "https://me.example" {
			t.Errorf("self-row leaked into peers list")
		}
	}

	if got := byURL["https://active.example"]; got.Status != model.PeerStatusActive {
		t.Errorf("active peer status: got %q, want active", got.Status)
	}
	if got := byURL["https://active.example"]; got.DisplayName != "Active Peer" {
		t.Errorf("active peer displayName: got %q, want Active Peer (US-1.4 AC2)", got.DisplayName)
	}
	if got := byURL["https://stale.example"]; got.Status != model.PeerStatusStale {
		t.Errorf("stale peer status: got %q, want stale (US-1.4 AC3)", got.Status)
	}
	if got := byURL["https://paused.example"]; got.Status != model.PeerStatusPaused {
		t.Errorf("paused peer status: got %q, want paused", got.Status)
	}
	if got := byURL["https://revoked.example"]; got.Status != model.PeerStatusRevoked {
		t.Errorf("revoked peer status: got %q, want revoked", got.Status)
	}
}

// TestListPeers_DerivesLeftStatus asserts a peer that voluntarily LEFT (the owner
// marked its mapping lost with reason="left") is reported with the distinct
// PeerStatusLeft so the owner UI renders "left" rather than active/stale
// (Federation v1 F5.5, US-6.3 AC2).
func TestListPeers_DerivesLeftStatus(t *testing.T) {
	svc, projects, fp, instances := newPeersSvc(t, "https://me.example")
	pid := seedProject(t, projects)
	if _, err := svc.EnableForProject(context.Background(), pid); err != nil {
		t.Fatalf("enable: %v", err)
	}
	recent := time.Now().Add(-1 * time.Hour)
	seedPeer(t, fp, instances, pid, "https://gone.example", "Gone Peer", &recent, false, false)
	// The peer left: the owner marks its mapping lost with reason=left.
	if _, err := fp.MarkLeftByPeer(context.Background(), pid, "https://gone.example"); err != nil {
		t.Fatalf("mark left: %v", err)
	}

	peers, err := svc.ListPeers(context.Background(), pid)
	if err != nil {
		t.Fatalf("ListPeers: %v", err)
	}
	if len(peers) != 1 {
		t.Fatalf("peer count: got %d, want 1", len(peers))
	}
	if peers[0].Status != model.PeerStatusLeft {
		t.Errorf("left peer status: got %q, want left (US-6.3 AC2)", peers[0].Status)
	}
}

// TestListPeers_PendingDeliveryReflectsOutbox asserts that with the F3.2 sync
// store wired, ListPeers reports the real count of events not yet delivered to a
// peer (US-3.2 AC4 — the delivery-overdue signal), and 0 once delivered.
func TestListPeers_PendingDeliveryReflectsOutbox(t *testing.T) {
	d, projects, fedProjects, keys := setup(t)
	seedContext(t, d)
	instances := repo.NewFederatedInstanceRepo(d)
	st := store.New(d)
	svc := fedsvc.NewService(d, projects, fedProjects, keys, repo.NewFederationInviteRepo(d), instances, crypto.NewTokenCipher(fedSvcKey), "https://me.example").
		WithSyncStore(st)

	ctx := context.Background()
	pid := seedProject(t, projects)
	if _, err := svc.EnableForProject(ctx, pid); err != nil {
		t.Fatalf("enable: %v", err)
	}
	recent := time.Now().Add(-1 * time.Hour)
	seedPeer(t, fedProjects, instances, pid, "https://a.example", "Peer A", &recent, false, false)

	// Two undelivered outbox events for the project.
	for _, id := range []string{"e1", "e2"} {
		tx, _ := d.BeginTx(ctx, nil)
		if err := st.InsertOutboxTx(ctx, tx, id, pid, `{}`, 1, "2024-01-01T00:00:00.000Z"); err != nil {
			t.Fatalf("insert outbox: %v", err)
		}
		_ = tx.Commit()
	}

	peers, err := svc.ListPeers(ctx, pid)
	if err != nil {
		t.Fatalf("ListPeers: %v", err)
	}
	if len(peers) != 1 || peers[0].PendingDelivery != 2 {
		t.Fatalf("pending delivery: got %+v, want one peer with pendingDelivery=2", peers)
	}

	// After delivery, pending drops to 0.
	batch, _ := st.ListUndeliveredForPeer(ctx, pid, "https://a.example", 100)
	for _, ev := range batch {
		if err := st.MarkDelivered(ctx, ev.ID, "https://a.example"); err != nil {
			t.Fatalf("mark delivered: %v", err)
		}
	}
	peers, err = svc.ListPeers(ctx, pid)
	if err != nil {
		t.Fatalf("ListPeers after delivery: %v", err)
	}
	if peers[0].PendingDelivery != 0 {
		t.Errorf("pending delivery after delivery: got %d, want 0", peers[0].PendingDelivery)
	}
}

// TestListPeers_ProjectNotFound asserts ListPeers reports ErrProjectNotFound for
// an unknown project so the handler can map it to 404.
func TestListPeers_ProjectNotFound(t *testing.T) {
	svc, _, _, _ := newPeersSvc(t, "https://me.example")
	if _, err := svc.ListPeers(context.Background(), 99999); !errors.Is(err, fedsvc.ErrProjectNotFound) {
		t.Fatalf("expected ErrProjectNotFound, got %v", err)
	}
}

// TestPausePeer_SetsFlag asserts PausePeer flips paused=true on the (project,
// peer) row so the outbox stops fanning out to it (Federation v1 F5.3, US-6.1
// AC1). The link stays trusted (non-destructive). The peers list reflects the
// paused status (US-6.1 AC3).
func TestPausePeer_SetsFlag(t *testing.T) {
	svc, projects, fp, instances := newPeersSvc(t, "https://me.example")
	ctx := context.Background()
	pid := seedProject(t, projects)
	if _, err := svc.EnableForProject(ctx, pid); err != nil {
		t.Fatalf("enable: %v", err)
	}
	recent := time.Now().Add(-1 * time.Hour)
	seedPeer(t, fp, instances, pid, "https://bob.example", "Bob", &recent, false, false)

	if err := svc.PausePeer(ctx, pid, "https://bob.example"); err != nil {
		t.Fatalf("PausePeer: %v", err)
	}

	row, err := fp.Get(ctx, pid, "https://bob.example")
	if err != nil {
		t.Fatalf("get peer: %v", err)
	}
	if !row.Paused {
		t.Errorf("paused: got false, want true (US-6.1 AC1)")
	}
	if row.Revoked {
		t.Errorf("revoked: got true, want false (pause is non-destructive)")
	}

	// US-6.1 AC3: the peers list surfaces the paused status with the row preserved.
	peers, err := svc.ListPeers(ctx, pid)
	if err != nil {
		t.Fatalf("list peers: %v", err)
	}
	if len(peers) != 1 || peers[0].Status != model.PeerStatusPaused {
		t.Fatalf("peers list: got %+v, want one peer with status paused (US-6.1 AC3)", peers)
	}
}

// TestResumePeer_ClearsFlagAndFlushes asserts ResumePeer flips paused back to
// false (Federation v1 F5.3, US-6.1 AC2) and fires the resume-flush hook so the
// accumulated outbox events are pushed promptly rather than on the next tick.
func TestResumePeer_ClearsFlagAndFlushes(t *testing.T) {
	svc, projects, fp, instances := newPeersSvc(t, "https://me.example")
	ctx := context.Background()
	pid := seedProject(t, projects)
	if _, err := svc.EnableForProject(ctx, pid); err != nil {
		t.Fatalf("enable: %v", err)
	}
	recent := time.Now().Add(-1 * time.Hour)
	seedPeer(t, fp, instances, pid, "https://bob.example", "Bob", &recent, true /*paused*/, false)

	flushed := false
	svc = svc.WithResumeFlush(func() { flushed = true })

	if err := svc.ResumePeer(ctx, pid, "https://bob.example"); err != nil {
		t.Fatalf("ResumePeer: %v", err)
	}

	row, err := fp.Get(ctx, pid, "https://bob.example")
	if err != nil {
		t.Fatalf("get peer: %v", err)
	}
	if row.Paused {
		t.Errorf("paused after resume: got true, want false (US-6.1 AC2)")
	}
	if !flushed {
		t.Errorf("resume-flush hook not fired: accumulated events would not push until the next tick (US-6.1 AC2)")
	}
}

// TestPausePeer_UnknownPeer asserts pausing a peer that is not joined to the
// project reports ErrPeerNotFound so the handler maps it to 404.
func TestPausePeer_UnknownPeer(t *testing.T) {
	svc, projects, _, _ := newPeersSvc(t, "https://me.example")
	ctx := context.Background()
	pid := seedProject(t, projects)
	if _, err := svc.EnableForProject(ctx, pid); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if err := svc.PausePeer(ctx, pid, "https://nobody.example"); !errors.Is(err, fedsvc.ErrPeerNotFound) {
		t.Fatalf("expected ErrPeerNotFound, got %v", err)
	}
}

// TestPausePeer_ProjectNotFound asserts pausing on an unknown project reports
// ErrProjectNotFound (→404).
func TestPausePeer_ProjectNotFound(t *testing.T) {
	svc, _, _, _ := newPeersSvc(t, "https://me.example")
	if err := svc.PausePeer(context.Background(), 99999, "https://bob.example"); !errors.Is(err, fedsvc.ErrProjectNotFound) {
		t.Fatalf("expected ErrProjectNotFound, got %v", err)
	}
}

// TestListPeers_DBErrorNotMaskedAsNotFound asserts that a non-ErrNotFound
// project-Get failure (here a closed *sql.DB) is NOT collapsed into
// ErrProjectNotFound — it must surface as a wrapped error so the handler returns
// 500 instead of a misleading 404 (the owner would otherwise be told their
// project does not exist while the real cause is a broken DB).
func TestListPeers_DBErrorNotMaskedAsNotFound(t *testing.T) {
	d, projects, fedProjects, keys := setup(t)
	seedContext(t, d)
	instances := repo.NewFederatedInstanceRepo(d)
	svc := fedsvc.NewService(d, projects, fedProjects, keys, repo.NewFederationInviteRepo(d), instances, crypto.NewTokenCipher(fedSvcKey), "https://me.example")

	// Close the DB so projects.Get fails with a connection error rather than
	// sql.ErrNoRows / repo.ErrNotFound.
	if err := d.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	_, err := svc.ListPeers(context.Background(), 1)
	if err == nil {
		t.Fatal("expected an error from ListPeers on a closed DB, got nil")
	}
	if errors.Is(err, fedsvc.ErrProjectNotFound) {
		t.Fatalf("DB failure masked as ErrProjectNotFound: %v", err)
	}
}

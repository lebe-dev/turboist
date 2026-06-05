package federation_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/federation/store"
	"github.com/lebe-dev/turboist/internal/repo"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

// newRevokeSvc builds a federation service wired with the sync store (so the
// federation_revoke event is enqueued into the outbox) and returns it alongside
// the repos + store needed to seed peers and inspect the outbox.
func newRevokeSvc(t *testing.T, instanceURL string) (*fedsvc.Service, *repo.ProjectRepo, *repo.FederatedProjectRepo, *repo.FederatedInstanceRepo, *store.Store) {
	t.Helper()
	d, projects, fedProjects, keys := setup(t)
	seedContext(t, d)
	instances := repo.NewFederatedInstanceRepo(d)
	st := store.New(d)
	svc := fedsvc.NewService(d, projects, fedProjects, keys, repo.NewFederationInviteRepo(d), instances, crypto.NewTokenCipher(fedSvcKey), instanceURL).
		WithSyncStore(st)
	return svc, projects, fedProjects, instances, st
}

// TestRevokePeer_SetsFlagAndEnqueuesEvent asserts RevokePeer flips revoked=1 and
// enqueues + directly delivers a signed federation_revoke control event to the
// revoked peer (Federation v1 F5.4, US-6.2 AC1).
func TestRevokePeer_SetsFlagAndEnqueuesEvent(t *testing.T) {
	svc, projects, fp, instances, st := newRevokeSvc(t, "https://me.example")
	ctx := context.Background()
	pid := seedProject(t, projects)
	if _, err := svc.EnableForProject(ctx, pid); err != nil {
		t.Fatalf("enable: %v", err)
	}
	recent := time.Now().Add(-1 * time.Hour)
	seedPeer(t, fp, instances, pid, "https://bob.example", "Bob", &recent, false, false)

	var sent []string
	var sentPeer string
	svc = svc.WithRevokeSender(func(_ context.Context, peerURL string, payloads []string) error {
		sentPeer = peerURL
		sent = payloads
		return nil
	})

	if err := svc.RevokePeer(ctx, pid, "https://bob.example"); err != nil {
		t.Fatalf("RevokePeer: %v", err)
	}

	// AC1: revoked=1 on the peer row.
	row, err := fp.Get(ctx, pid, "https://bob.example")
	if err != nil {
		t.Fatalf("get peer: %v", err)
	}
	if !row.Revoked {
		t.Errorf("revoked: got false, want true (US-6.2 AC1)")
	}

	// AC1: the owner sent the peer a federation_revoke event.
	if sentPeer != "https://bob.example" {
		t.Errorf("revoke delivered to: got %q, want https://bob.example", sentPeer)
	}
	if len(sent) != 1 {
		t.Fatalf("revoke payloads: got %d, want 1", len(sent))
	}
	var evt events.Event
	if err := json.Unmarshal([]byte(sent[0]), &evt); err != nil {
		t.Fatalf("decode revoke event: %v", err)
	}
	if evt.Op != events.OpRevoke {
		t.Errorf("revoke event op: got %q, want %q", evt.Op, events.OpRevoke)
	}
	if evt.OriginInstance != "https://me.example" || evt.Author != "https://me.example" {
		t.Errorf("revoke event author/origin: got %q/%q, want https://me.example", evt.Author, evt.OriginInstance)
	}
	if evt.Signature == "" {
		t.Errorf("revoke event must be signed")
	}

	// The event is durably recorded in the outbox (crash-safe at-least-once) and was
	// marked delivered to the revoked peer (pending count 0 after the direct push).
	n, err := st.PendingDeliveryCount(ctx, pid, "https://bob.example")
	if err != nil {
		t.Fatalf("pending count: %v", err)
	}
	if n != 0 {
		t.Errorf("revoke pending after delivery: got %d, want 0", n)
	}
}

// TestRevokePeer_OfflineLeavesEventPending asserts that when the direct revoke
// delivery fails (peer offline, US-6.2 AC4) the revoke STILL takes effect
// (revoked=1) and the event stays pending in the outbox — the peer self-detects
// the revoke on its next sync via the 403.
func TestRevokePeer_OfflineLeavesEventPending(t *testing.T) {
	svc, projects, fp, instances, st := newRevokeSvc(t, "https://me.example")
	ctx := context.Background()
	pid := seedProject(t, projects)
	if _, err := svc.EnableForProject(ctx, pid); err != nil {
		t.Fatalf("enable: %v", err)
	}
	recent := time.Now().Add(-1 * time.Hour)
	seedPeer(t, fp, instances, pid, "https://bob.example", "Bob", &recent, false, false)

	svc = svc.WithRevokeSender(func(_ context.Context, _ string, _ []string) error {
		return errors.New("peer offline")
	})

	if err := svc.RevokePeer(ctx, pid, "https://bob.example"); err != nil {
		t.Fatalf("RevokePeer must succeed even when delivery fails: %v", err)
	}
	row, err := fp.Get(ctx, pid, "https://bob.example")
	if err != nil {
		t.Fatalf("get peer: %v", err)
	}
	if !row.Revoked {
		t.Errorf("revoked after offline delivery: got false, want true (US-6.2 AC4 — revoke still takes effect)")
	}
	n, err := st.PendingDeliveryCount(ctx, pid, "https://bob.example")
	if err != nil {
		t.Fatalf("pending count: %v", err)
	}
	if n != 1 {
		t.Errorf("revoke pending after failed delivery: got %d, want 1 (peer self-detects on return)", n)
	}
}

// TestRevokePeer_NotFannedOutToOtherPeers asserts the federation_revoke event is
// point-to-point: it is pre-stamped delivered to every OTHER peer so the normal
// fan-out never sends it to anyone but the revoked peer.
func TestRevokePeer_NotFannedOutToOtherPeers(t *testing.T) {
	svc, projects, fp, instances, st := newRevokeSvc(t, "https://me.example")
	ctx := context.Background()
	pid := seedProject(t, projects)
	if _, err := svc.EnableForProject(ctx, pid); err != nil {
		t.Fatalf("enable: %v", err)
	}
	recent := time.Now().Add(-1 * time.Hour)
	seedPeer(t, fp, instances, pid, "https://bob.example", "Bob", &recent, false, false)
	seedPeer(t, fp, instances, pid, "https://carol.example", "Carol", &recent, false, false)

	svc = svc.WithRevokeSender(func(_ context.Context, _ string, _ []string) error { return nil })
	if err := svc.RevokePeer(ctx, pid, "https://bob.example"); err != nil {
		t.Fatalf("RevokePeer: %v", err)
	}

	// Carol must NOT have the revoke event pending (it is point-to-point to Bob).
	n, err := st.PendingDeliveryCount(ctx, pid, "https://carol.example")
	if err != nil {
		t.Fatalf("pending count carol: %v", err)
	}
	if n != 0 {
		t.Errorf("revoke leaked to other peer: carol pending got %d, want 0", n)
	}
}

// TestRevokePeer_UnknownPeer asserts revoking a peer not joined to the project
// reports ErrPeerNotFound (→404).
func TestRevokePeer_UnknownPeer(t *testing.T) {
	svc, projects, _, _, _ := newRevokeSvc(t, "https://me.example")
	ctx := context.Background()
	pid := seedProject(t, projects)
	if _, err := svc.EnableForProject(ctx, pid); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if err := svc.RevokePeer(ctx, pid, "https://nobody.example"); !errors.Is(err, fedsvc.ErrPeerNotFound) {
		t.Fatalf("expected ErrPeerNotFound, got %v", err)
	}
}

// TestRevokePeer_ProjectNotFound asserts revoking on an unknown project reports
// ErrProjectNotFound (→404).
func TestRevokePeer_ProjectNotFound(t *testing.T) {
	svc, _, _, _, _ := newRevokeSvc(t, "https://me.example")
	if err := svc.RevokePeer(context.Background(), 99999, "https://bob.example"); !errors.Is(err, fedsvc.ErrProjectNotFound) {
		t.Fatalf("expected ErrProjectNotFound, got %v", err)
	}
}

// TestResumePeer_RevokedRejected asserts resuming a REVOKED peer is rejected with
// ErrPeerRevoked and does NOT clear any state — revoke is irreversible (Federation
// v1 F5.4, US-6.2 AC5).
func TestResumePeer_RevokedRejected(t *testing.T) {
	svc, projects, fp, instances, _ := newRevokeSvc(t, "https://me.example")
	ctx := context.Background()
	pid := seedProject(t, projects)
	if _, err := svc.EnableForProject(ctx, pid); err != nil {
		t.Fatalf("enable: %v", err)
	}
	recent := time.Now().Add(-1 * time.Hour)
	// A peer that is both paused and revoked: a resume must still be rejected.
	seedPeer(t, fp, instances, pid, "https://bob.example", "Bob", &recent, true /*paused*/, true /*revoked*/)

	flushed := false
	svc = svc.WithResumeFlush(func() { flushed = true })

	if err := svc.ResumePeer(ctx, pid, "https://bob.example"); !errors.Is(err, fedsvc.ErrPeerRevoked) {
		t.Fatalf("expected ErrPeerRevoked, got %v", err)
	}
	if flushed {
		t.Errorf("resume-flush must NOT fire on a revoked peer (US-6.2 AC5)")
	}
	// The peer stays revoked and paused — nothing was cleared.
	row, err := fp.Get(ctx, pid, "https://bob.example")
	if err != nil {
		t.Fatalf("get peer: %v", err)
	}
	if !row.Revoked || !row.Paused {
		t.Errorf("revoked peer state after rejected resume: got (revoked=%v, paused=%v), want (true, true)", row.Revoked, row.Paused)
	}
}

// TestRevokePeer_NoSyncStore asserts the revoke still flips the flag on a
// federation-off / partial build (no sync store wired): the event is not enqueued
// but the peer is still revoked (rejected on any inbound).
func TestRevokePeer_NoSyncStore(t *testing.T) {
	d, projects, fedProjects, keys := setup(t)
	seedContext(t, d)
	instances := repo.NewFederatedInstanceRepo(d)
	svc := fedsvc.NewService(d, projects, fedProjects, keys, repo.NewFederationInviteRepo(d), instances, crypto.NewTokenCipher(fedSvcKey), "https://me.example")
	ctx := context.Background()
	pid := seedProject(t, projects)
	if _, err := svc.EnableForProject(ctx, pid); err != nil {
		t.Fatalf("enable: %v", err)
	}
	recent := time.Now().Add(-1 * time.Hour)
	seedPeer(t, fedProjects, instances, pid, "https://bob.example", "Bob", &recent, false, false)

	if err := svc.RevokePeer(ctx, pid, "https://bob.example"); err != nil {
		t.Fatalf("RevokePeer (no sync store): %v", err)
	}
	row, err := fedProjects.Get(ctx, pid, "https://bob.example")
	if err != nil {
		t.Fatalf("get peer: %v", err)
	}
	if !row.Revoked {
		t.Errorf("revoked (no sync store): got false, want true")
	}
}

package federation_test

import (
	"context"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

// TestHealth_OKWhenCleanAndFresh asserts an owned federated project with a fresh
// peer and an empty outbox reports status=ok, the instance_url, the supported
// protocol versions, and outbox_depth 0 (Federation v1 F6.5, US-8.1).
func TestHealth_OKWhenCleanAndFresh(t *testing.T) {
	svc, _, projects, fp, instances, _ := newStatusSvc(t, "https://me.example")
	ctx := context.Background()
	pid := seedProject(t, projects)
	if _, err := svc.EnableForProject(ctx, pid); err != nil {
		t.Fatalf("enable: %v", err)
	}
	recent := time.Now().Add(-1 * time.Hour)
	seedPeer(t, fp, instances, pid, "https://bob.example", "Bob", &recent, false, false)

	h, err := svc.Health(ctx)
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if h.Status != fedsvc.HealthOK {
		t.Errorf("status: got %q, want ok (US-8.1)", h.Status)
	}
	if h.InstanceURL != "https://me.example" {
		t.Errorf("instanceURL: got %q", h.InstanceURL)
	}
	if len(h.ProtocolVersions) == 0 {
		t.Errorf("protocolVersions empty")
	}
	if h.OutboxDepth != 0 {
		t.Errorf("outboxDepth: got %d, want 0", h.OutboxDepth)
	}
	if len(h.Peers) != 1 || h.Peers[0].InstanceURL != "https://bob.example" || h.Peers[0].DisplayName != "Bob" {
		t.Errorf("peers: got %+v, want one Bob peer", h.Peers)
	}
}

// TestHealth_DegradedWhenOutboxPending asserts a pending (undelivered) outbox
// event flips the rolled-up status to degraded while peers stay fresh (US-8.1).
func TestHealth_DegradedWhenOutboxPending(t *testing.T) {
	svc, d, projects, fp, instances, st := newStatusSvc(t, "https://me.example")
	ctx := context.Background()
	pid := seedProject(t, projects)
	if _, err := svc.EnableForProject(ctx, pid); err != nil {
		t.Fatalf("enable: %v", err)
	}
	recent := time.Now().Add(-1 * time.Hour)
	seedPeer(t, fp, instances, pid, "https://bob.example", "Bob", &recent, false, false)

	tx, _ := d.BeginTx(ctx, nil)
	if err := st.InsertOutboxTx(ctx, tx, "e-pending", pid, `{}`, 1, model.FormatUTC(time.Now())); err != nil {
		t.Fatalf("insert outbox: %v", err)
	}
	_ = tx.Commit()

	h, err := svc.Health(ctx)
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if h.Status != fedsvc.HealthDegraded {
		t.Errorf("status: got %q, want degraded (US-8.1)", h.Status)
	}
	if h.OutboxDepth != 1 {
		t.Errorf("outboxDepth: got %d, want 1", h.OutboxDepth)
	}
}

// TestHealth_PeersStaleWhenPeerUnreachable asserts a peer not contacted in >24h
// flips the rolled-up status to peers_stale, taking precedence over a pending
// outbox (US-8.1).
func TestHealth_PeersStaleWhenPeerUnreachable(t *testing.T) {
	svc, d, projects, fp, instances, st := newStatusSvc(t, "https://me.example")
	ctx := context.Background()
	pid := seedProject(t, projects)
	if _, err := svc.EnableForProject(ctx, pid); err != nil {
		t.Fatalf("enable: %v", err)
	}
	stale := time.Now().Add(-48 * time.Hour)
	seedPeer(t, fp, instances, pid, "https://bob.example", "Bob", &stale, false, false)

	// Even with a pending event, peers_stale wins.
	tx, _ := d.BeginTx(ctx, nil)
	if err := st.InsertOutboxTx(ctx, tx, "e-pending", pid, `{}`, 1, model.FormatUTC(time.Now())); err != nil {
		t.Fatalf("insert outbox: %v", err)
	}
	_ = tx.Commit()

	h, err := svc.Health(ctx)
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if h.Status != fedsvc.HealthPeersStale {
		t.Errorf("status: got %q, want peers_stale (US-8.1)", h.Status)
	}
	if len(h.Peers) != 1 || h.Peers[0].Status != model.PeerStatusStale {
		t.Errorf("peers: got %+v, want one stale peer", h.Peers)
	}
}

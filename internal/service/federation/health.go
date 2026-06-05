package federation

import (
	"context"
	"fmt"
	"time"

	"github.com/lebe-dev/turboist/internal/federation/protocol"
	"github.com/lebe-dev/turboist/internal/model"
)

// HealthStatus is the rolled-up federation liveness state (Federation v1 F6.5,
// US-8.1). It is server-derived from the outbox depth + per-peer staleness:
//
//	ok          — nothing pending, every active peer fresh;
//	degraded    — at least one event is pending delivery (outbox depth > 0);
//	peers_stale — at least one active peer has not been contacted within
//	              model.PeerStaleAfter (24h). peers_stale takes precedence over
//	              degraded because an unreachable peer is the more actionable
//	              signal (delivery is blocked, not merely queued).
type HealthStatus string

const (
	// HealthOK — federation is healthy (no pending events, no stale peers).
	HealthOK HealthStatus = "ok"
	// HealthDegraded — events are pending delivery but every peer is reachable.
	HealthDegraded HealthStatus = "degraded"
	// HealthPeersStale — at least one active peer is unreachable (>24h).
	HealthPeersStale HealthStatus = "peers_stale"
)

// HealthPeer is one peer's liveness line in the health report.
type HealthPeer struct {
	InstanceURL   string
	DisplayName   string
	Status        model.PeerStatus
	LastContactAt *time.Time
}

// Health is the federation liveness report (Federation v1 F6.5, US-8.1) backing
// GET /federation/health. instance_url + protocol_versions identify this instance;
// outbox_depth + status are the rolled-up ops signals; peers carries the per-peer
// detail (returned only behind the admin guard — the public liveness handler
// strips it). It is a pure server read (no network I/O, R1).
type Health struct {
	InstanceURL      string
	ProtocolVersions []int
	OutboxDepth      int
	Status           HealthStatus
	Peers            []HealthPeer
}

// Health computes the federation liveness report. It aggregates the per-peer
// health of every owned federated project (deduplicated by instance_url, worst
// status winning) and the live outbox depth. Without a sync store wired the
// outbox depth is reported as 0 (a federation-off-ish build) — the status then
// reflects only peer staleness.
func (s *Service) Health(ctx context.Context) (Health, error) {
	now := s.now()
	out := Health{
		InstanceURL:      s.instanceURL,
		ProtocolVersions: protocol.SupportedProtocolVersions,
	}

	if s.syncStore != nil {
		depth, err := s.syncStore.OutboxDepth(ctx)
		if err != nil {
			return Health{}, fmt.Errorf("outbox depth: %w", err)
		}
		out.OutboxDepth = depth
	}

	ids, err := s.fedProjects.ListOwnedFederatedProjectIDs(ctx)
	if err != nil {
		return Health{}, fmt.Errorf("list owned federated projects: %w", err)
	}

	// Deduplicate peers by instance_url across all owned projects, keeping the
	// worst (stale > active) status so the same peer shared on several projects
	// surfaces once with its most-actionable state.
	seen := make(map[string]int) // instance_url -> index into out.Peers
	var anyStale bool
	for _, pid := range ids {
		peers, err := s.fedProjects.ListPeerHealthByProject(ctx, pid)
		if err != nil {
			return Health{}, fmt.Errorf("peer health for %d: %w", pid, err)
		}
		for _, p := range peers {
			if p.Revoked || p.Paused {
				continue // not an active delivery target — never marks the instance stale.
			}
			status := model.DerivePeerStatus(p.Revoked, p.Paused, model.FederationLostNone, p.LastContactAt, now)
			if status == model.PeerStatusStale {
				anyStale = true
			}
			if idx, ok := seen[p.PeerInstanceURL]; ok {
				// Keep the worse status: a stale sighting beats an active one.
				if status == model.PeerStatusStale {
					out.Peers[idx].Status = status
					out.Peers[idx].LastContactAt = p.LastContactAt
				}
				continue
			}
			seen[p.PeerInstanceURL] = len(out.Peers)
			out.Peers = append(out.Peers, HealthPeer{
				InstanceURL:   p.PeerInstanceURL,
				DisplayName:   p.DisplayName,
				Status:        status,
				LastContactAt: p.LastContactAt,
			})
		}
	}

	out.Status = deriveHealthStatus(out.OutboxDepth, anyStale)
	return out, nil
}

// deriveHealthStatus rolls the depth + staleness signals into a single status.
// peers_stale takes precedence over degraded (an unreachable peer is the more
// actionable signal); ok only when nothing is pending and every peer is fresh.
func deriveHealthStatus(outboxDepth int, anyStale bool) HealthStatus {
	if anyStale {
		return HealthPeersStale
	}
	if outboxDepth > 0 {
		return HealthDegraded
	}
	return HealthOK
}

package federation

import (
	"context"
	"fmt"

	"github.com/lebe-dev/turboist/internal/model"
)

// ProjectOverview is one federated project's privacy/federation overview row
// (Federation v1 F6.4, US-7.1 AC1): the local project id + title, this instance's
// derived Role (owner | peer | read-only), and the named peer list the project is
// visible to. It backs GET /api/v1/federation/overview — the owner's "Privacy /
// Federation overview" table. Peers carries the non-owner, non-revoked audience
// (empty for a joined copy, which has no outbound audience of its own).
type ProjectOverview struct {
	LocalProjectID int64
	Title          string
	Role           model.FederationRole
	Peers          []model.PeerInstance
}

// Overview computes the per-project federation overview for every federation-
// enabled project on this instance (Federation v1 F6.4, US-7.1 AC1). It is the
// JWT-only owner read backing GET /api/v1/federation/overview. Non-federated
// projects are excluded. The role is derived in the single canonical place
// (model.DeriveFederationRole) from the resolved per-project surface (is_owner +
// granted permission); the peer list is the named audience joined from the
// instance directory.
//
// Connection discipline + no N+1: exactly TWO reads run — one to list the
// federated projects with their resolved surface, one batched
// PeerInstancesByProjectIDs to resolve every project's peers (keyed by
// local_project_id). No per-project query is issued.
func (s *Service) Overview(ctx context.Context) ([]ProjectOverview, error) {
	summaries, err := s.fedProjects.ListFederatedProjectsOverview(ctx)
	if err != nil {
		return nil, fmt.Errorf("list federated projects overview: %w", err)
	}
	if len(summaries) == 0 {
		return []ProjectOverview{}, nil
	}

	ids := make([]int64, 0, len(summaries))
	for _, sm := range summaries {
		ids = append(ids, sm.LocalProjectID)
	}
	peersByID, err := s.fedProjects.PeerInstancesByProjectIDs(ctx, ids, s.instanceURL)
	if err != nil {
		return nil, fmt.Errorf("resolve peer instances: %w", err)
	}

	out := make([]ProjectOverview, 0, len(summaries))
	for _, sm := range summaries {
		peers := peersByID[sm.LocalProjectID]
		if peers == nil {
			peers = []model.PeerInstance{}
		}
		out = append(out, ProjectOverview{
			LocalProjectID: sm.LocalProjectID,
			Title:          sm.Title,
			Role:           model.DeriveFederationRole(sm.IsOwner, sm.Permissions),
			Peers:          peers,
		})
	}
	return out, nil
}

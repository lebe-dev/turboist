package dto

import "github.com/lebe-dev/turboist/internal/model"

type CreateProjectRequest struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Color       string   `json:"color"`
	Labels      []string `json:"labels"`
	ProjectType string   `json:"projectType"`
}

type PatchProjectRequest struct {
	Title       *string   `json:"title"`
	Description *string   `json:"description"`
	Color       *string   `json:"color"`
	ContextID   *int64    `json:"contextId"`
	Labels      *[]string `json:"labels"`
	IsPrivate   *bool     `json:"isPrivate"`
	ProjectType *string   `json:"projectType"`
}

type ProjectDTO struct {
	ID          int64   `json:"id"`
	ContextID   int64   `json:"contextId"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Color       string  `json:"color"`
	Status      string  `json:"status"`
	ProjectType string  `json:"projectType"`
	IsPinned    bool    `json:"isPinned"`
	PinnedAt    *string `json:"pinnedAt"`
	IsPrivate   bool    `json:"isPrivate"`
	IsFederated bool    `json:"isFederated"`
	// Federation surface (Federation v1 F2.4, US-2.4 AC1/AC2). Populated via a
	// LEFT JOIN on federated_projects by the project handler; nil for
	// non-federated projects. OriginInstance is the owner instance the local
	// project mirrors; FederationPermissions is this instance's grant
	// (read|write|admin); IsOwner is true for the owner's own federated project
	// (its is_owner=1 self-row) and false for a joined peer copy. The frontend
	// renders the origin/role badges and disables editing when
	// FederationPermissions == "read" (the backend guard is authoritative, §9.2).
	OriginInstance        *string `json:"originInstance"`
	FederationPermissions *string `json:"federationPermissions"`
	IsOwner               bool    `json:"isOwner"`
	// ReBootstrappedAt is the wall-clock cutoff X of the most recent 410-stale
	// re-bootstrap of this joined project (Federation v1 F4.2, US-4.2 AC4), or nil
	// if it has never been re-bootstrapped. The joiner UI renders a dismissible
	// re-sync banner naming this timestamp ("your unsent changes from before {X}
	// were preserved but may have been overridden"). It is null for the owner's own
	// project and for any project that has only ever been initial-bootstrapped.
	ReBootstrappedAt *string `json:"reBootstrappedAt"`
	// FederationLost reports that this joined copy's trust link to the owner is
	// permanently gone (Federation v1 F5.4, US-6.2 AC3; shared with F5.5 / F5.6a).
	// FederationLostReason disambiguates why (revoked|left|owner-dead). When lost
	// with a read-only reason (revoked/owner-dead) the UI renders the copy read-only
	// — the backend guard remains authoritative. Both are zero-valued (false / nil)
	// for a healthy or non-federated project.
	FederationLost       bool    `json:"federationLost"`
	FederationLostReason *string `json:"federationLostReason"`
	// OwnerOffline reports that this JOINED copy's OWNER instance has been
	// unreachable past the owner-timeout window (Federation v1 F5.6a, US-6.5 AC1).
	// Unlike FederationLost it is a DERIVED, transient signal: while true the joiner
	// keeps editing — edits queue in federation_outbox and flush + LWW-resolve when
	// the owner returns (US-6.5 AC2/AC3) — and the UI surfaces a "pending — owner
	// offline" badge WITHOUT locking controls. It is false for the owner's own
	// project, for non-federated projects, and for a joined copy whose owner is
	// fresh or whose link is already permanently lost.
	OwnerOffline bool `json:"ownerOffline"`
	// PeerInstances is the per-project named peer audience this project is visible
	// to (Federation v1 F6.4, US-7.1 AC3): the non-owner, non-revoked peers, each as
	// {instanceUrl, displayName}, resolved ONCE at bootstrap in a single batched
	// query (no N+1). The new-task editor reads this array for the task's project
	// locally to render the explicit instance hint ("visible to peers: alice.example,
	// bob.example"), and the "visible to N peers" task badge derives N from
	// PeerInstances.length. It is an empty array for a non-federated project, the
	// owner's own project with no peers yet, and a joined copy (which has no outbound
	// audience of its own).
	PeerInstances  []PeerInstanceDTO `json:"peerInstances"`
	TroikiCategory *string           `json:"troikiCategory"`
	Labels         []LabelDTO        `json:"labels"`
	ClientID       string            `json:"clientId"`
	DeletedAt      *string           `json:"deletedAt"`
	CreatedAt      string            `json:"createdAt"`
	UpdatedAt      string            `json:"updatedAt"`
}

// WithFederationSurface overlays the resolved federation role
// (origin/permissions/owner) onto a project DTO (Federation v1 F2.4, US-2.4). It
// is additive — a non-federated project leaves OriginInstance /
// FederationPermissions nil and IsOwner false. originInstance/permissions are the
// federated_projects surface; isOwner distinguishes the owner's own project (its
// controls stay enabled) from a joined peer copy.
func (p ProjectDTO) WithFederationSurface(originInstance, permissions string, isOwner bool) ProjectDTO {
	origin := originInstance
	perms := permissions
	p.OriginInstance = &origin
	p.FederationPermissions = &perms
	p.IsOwner = isOwner
	return p
}

// WithFederationLost overlays the lost-status marker onto a project DTO
// (Federation v1 F5.4, US-6.2 AC3). A not-lost project (empty reason) leaves the
// fields zero-valued. It is additive on top of WithFederationSurface.
func (p ProjectDTO) WithFederationLost(lost bool, reason string) ProjectDTO {
	if !lost {
		return p
	}
	p.FederationLost = true
	r := reason
	p.FederationLostReason = &r
	return p
}

// WithFederationOwnerOffline overlays the derived owner-offline marker onto a
// project DTO (Federation v1 F5.6a, US-6.5 AC1). A fresh-owner / non-offline
// project leaves OwnerOffline false. It is additive on top of
// WithFederationSurface; the caller passes the already-derived flag (the DTO
// layer holds no clock). Editing is NOT locked by this flag — local edits stay
// allowed and queue while the owner is offline (US-6.5 AC2).
func (p ProjectDTO) WithFederationOwnerOffline(offline bool) ProjectDTO {
	p.OwnerOffline = offline
	return p
}

// WithPeerInstances overlays the per-project named peer audience onto a project
// DTO (Federation v1 F6.4, US-7.1 AC3). A nil/empty slice leaves PeerInstances an
// empty array (never null) so the frontend can always read .length. It is additive
// on top of WithFederationSurface; callers pass the already-resolved instances
// (the DTO layer issues no query).
func (p ProjectDTO) WithPeerInstances(peers []PeerInstanceDTO) ProjectDTO {
	if len(peers) == 0 {
		p.PeerInstances = []PeerInstanceDTO{}
		return p
	}
	p.PeerInstances = peers
	return p
}

// WithReBootstrapMarker overlays the 410-stale re-bootstrap cutoff X onto a
// project DTO (Federation v1 F4.2, US-4.2 AC4). An empty rebootstrappedAt leaves
// ReBootstrappedAt nil (never re-bootstrapped), so the joiner UI shows no re-sync
// banner. It is additive on top of WithFederationSurface.
func (p ProjectDTO) WithReBootstrapMarker(rebootstrappedAt string) ProjectDTO {
	if rebootstrappedAt == "" {
		return p
	}
	at := rebootstrappedAt
	p.ReBootstrappedAt = &at
	return p
}

func ProjectFromModel(p model.Project) ProjectDTO {
	labels := make([]LabelDTO, len(p.Labels))
	for i, l := range p.Labels {
		labels[i] = LabelFromModel(l)
	}
	var troikiCat *string
	if p.TroikiCategory != nil {
		s := string(*p.TroikiCategory)
		troikiCat = &s
	}
	pt := p.Type
	if pt == "" {
		pt = model.ProjectTypeGeneric
	}
	return ProjectDTO{
		ID:             p.ID,
		ContextID:      p.ContextID,
		Title:          p.Title,
		Description:    p.Description,
		Color:          p.Color,
		Status:         string(p.Status),
		ProjectType:    string(pt),
		IsPinned:       p.IsPinned,
		PinnedAt:       FormatTimePtr(p.PinnedAt),
		IsPrivate:      p.IsPrivate,
		IsFederated:    p.IsFederated,
		PeerInstances:  []PeerInstanceDTO{},
		TroikiCategory: troikiCat,
		Labels:         labels,
		ClientID:       p.ClientID,
		DeletedAt:      FormatTimePtr(p.DeletedAt),
		CreatedAt:      FormatTime(p.CreatedAt),
		UpdatedAt:      FormatTime(p.UpdatedAt),
	}
}

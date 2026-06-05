package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/lebe-dev/turboist/internal/logging"
	"github.com/lebe-dev/turboist/internal/model"
)

// FederatedProjectRepo persists the per-(local project, peer) federation mapping
// (Federation v1 F1.1). The owner's own instance is stored as a self-row with
// is_owner=1 and peer_instance_url == origin_instance_url == this instance's
// URL; remote peers are added by the Phase 2 handshake.
type FederatedProjectRepo struct {
	db *sql.DB
}

func NewFederatedProjectRepo(db *sql.DB) *FederatedProjectRepo {
	return &FederatedProjectRepo{db: db}
}

const federatedProjectColumns = `local_project_id, peer_instance_url, remote_project_id, is_owner, origin_instance_url, permissions, paused, revoked, protocol_version, last_sent_hlc, last_received_hlc, joined_at, rebootstrap_cutoff_hlc, rebootstrapped_at, lost, lost_reason`

// qualifyFederatedProjectColumns is federatedProjectColumns with an fp. prefix on
// every column, for queries that alias federated_projects as fp and JOIN another
// table (e.g. SelfRow joins projects, which shares column names like created_at).
const qualifyFederatedProjectColumns = `fp.local_project_id, fp.peer_instance_url, fp.remote_project_id, fp.is_owner, fp.origin_instance_url, fp.permissions, fp.paused, fp.revoked, fp.protocol_version, fp.last_sent_hlc, fp.last_received_hlc, fp.joined_at, fp.rebootstrap_cutoff_hlc, fp.rebootstrapped_at, fp.lost, fp.lost_reason`

func scanFederatedProject(row interface{ Scan(...any) error }) (*model.FederatedProject, error) {
	var fp model.FederatedProject
	var isOwner, paused, revoked, lost int
	var lastSent, lastRecv, cutoffHLC, rebootAt sql.NullString
	var joinedAt, permissions, lostReason string
	if err := row.Scan(
		&fp.LocalProjectID, &fp.PeerInstanceURL, &fp.RemoteProjectID, &isOwner,
		&fp.OriginInstanceURL, &permissions, &paused, &revoked, &fp.ProtocolVersion,
		&lastSent, &lastRecv, &joinedAt, &cutoffHLC, &rebootAt, &lost, &lostReason,
	); err != nil {
		return nil, err
	}
	fp.IsOwner = isOwner == 1
	fp.Paused = paused == 1
	fp.Revoked = revoked == 1
	fp.Lost = lost == 1
	fp.LostReason = model.FederationLostReason(lostReason)
	fp.Permissions = model.FederationPermission(permissions)
	fp.LastSentHLC = lastSent.String
	fp.LastReceivedHLC = lastRecv.String
	fp.RebootstrapCutoffHLC = cutoffHLC.String
	fp.RebootstrappedAt = rebootAt.String
	t, err := model.ParseUTC(joinedAt)
	if err != nil {
		return nil, fmt.Errorf("parse joined_at: %w", err)
	}
	fp.JoinedAt = t
	return &fp, nil
}

// UpsertSelfRowTx inserts (or, when it already exists, leaves intact) the
// owner's is_owner=1 self-row for a freshly federated project, inside the given
// transaction so EnableForProject is atomic with the is_federated flip. The
// upsert is idempotent: re-enabling an already-federated project does NOT
// duplicate the row or reset its joined_at (US-1.1 AC1 idempotency).
func (r *FederatedProjectRepo) UpsertSelfRowTx(ctx context.Context, tx *sql.Tx, fp model.FederatedProject) error {
	const op = "repo.federated_projects.UpsertSelfRowTx"
	logQuery(ctx, op, fp.LocalProjectID, fp.PeerInstanceURL)
	owner := 0
	if fp.IsOwner {
		owner = 1
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO federated_projects
		   (local_project_id, peer_instance_url, remote_project_id, is_owner, origin_instance_url, permissions, paused, revoked, protocol_version, joined_at)
		 VALUES (?, ?, ?, ?, ?, ?, 0, 0, ?, ?)
		 ON CONFLICT(local_project_id, peer_instance_url) DO NOTHING`,
		fp.LocalProjectID, fp.PeerInstanceURL, fp.RemoteProjectID, owner,
		fp.OriginInstanceURL, string(fp.Permissions), fp.ProtocolVersion,
		model.FormatUTC(fp.JoinedAt))
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("upsert self-row: %w", err))
	}
	return nil
}

// UpsertPeerRow inserts (or leaves intact) a remote-peer federation mapping row
// (is_owner=0) for a project. It is the F1.4-test / Phase-2-handshake entry point
// for adding a joined peer; it is idempotent on the (local_project_id, peer)
// composite PK. Unlike UpsertSelfRowTx it persists the paused/revoked/last_sent_hlc
// fields so seeded fixtures and the handshake can express a peer's full state.
func (r *FederatedProjectRepo) UpsertPeerRow(ctx context.Context, fp model.FederatedProject) error {
	const op = "repo.federated_projects.UpsertPeerRow"
	logQuery(ctx, op, fp.LocalProjectID, fp.PeerInstanceURL)
	owner := boolToInt(fp.IsOwner)
	paused := boolToInt(fp.Paused)
	revoked := boolToInt(fp.Revoked)
	var lastSent, lastRecv any
	if fp.LastSentHLC != "" {
		lastSent = fp.LastSentHLC
	}
	if fp.LastReceivedHLC != "" {
		lastRecv = fp.LastReceivedHLC
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO federated_projects
		   (local_project_id, peer_instance_url, remote_project_id, is_owner, origin_instance_url, permissions, paused, revoked, protocol_version, last_sent_hlc, last_received_hlc, joined_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(local_project_id, peer_instance_url) DO NOTHING`,
		fp.LocalProjectID, fp.PeerInstanceURL, fp.RemoteProjectID, owner,
		fp.OriginInstanceURL, string(fp.Permissions), paused, revoked, fp.ProtocolVersion,
		lastSent, lastRecv, model.FormatUTC(fp.JoinedAt))
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("upsert peer-row: %w", err))
	}
	return nil
}

// UpsertPeerRowTx inserts (or, on conflict, refreshes) a remote-peer mapping row
// inside tx (Federation v1 F2.2). The owner handshake writes the joined peer in
// the SAME tx that consumes the invite + upserts the instance directory row, so
// the mapping cannot exist without its consumption being recorded (and vice
// versa). Unlike UpsertSelfRowTx it is NOT a no-op on conflict: a retried
// handshake with the same key refreshes permissions / protocol_version /
// remote_project_id while preserving joined_at (idempotent re-join). is_owner is
// always 0 — this is never the self-row.
func (r *FederatedProjectRepo) UpsertPeerRowTx(ctx context.Context, tx *sql.Tx, fp model.FederatedProject) error {
	const op = "repo.federated_projects.UpsertPeerRowTx"
	logQuery(ctx, op, fp.LocalProjectID, fp.PeerInstanceURL)
	paused := boolToInt(fp.Paused)
	revoked := boolToInt(fp.Revoked)
	var lastSent, lastRecv any
	if fp.LastSentHLC != "" {
		lastSent = fp.LastSentHLC
	}
	if fp.LastReceivedHLC != "" {
		lastRecv = fp.LastReceivedHLC
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO federated_projects
		   (local_project_id, peer_instance_url, remote_project_id, is_owner, origin_instance_url, permissions, paused, revoked, protocol_version, last_sent_hlc, last_received_hlc, joined_at)
		 VALUES (?, ?, ?, 0, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(local_project_id, peer_instance_url) DO UPDATE SET
		   remote_project_id = excluded.remote_project_id,
		   origin_instance_url = excluded.origin_instance_url,
		   permissions = excluded.permissions,
		   protocol_version = excluded.protocol_version`,
		fp.LocalProjectID, fp.PeerInstanceURL, fp.RemoteProjectID,
		fp.OriginInstanceURL, string(fp.Permissions), paused, revoked, fp.ProtocolVersion,
		lastSent, lastRecv, model.FormatUTC(fp.JoinedAt))
	if err != nil {
		return logErr(ctx, op, fmt.Errorf("upsert peer-row tx: %w", err))
	}
	return nil
}

// MarkReBootstrapTx records the outcome of a 410-stale re-bootstrap on the joined
// peer mapping row (Federation v1 F4.2, US-4.2 AC4), inside the SAME tx the
// snapshot overwrite runs in: it advances last_received_hlc to the snapshot's
// as_of (cutoffHLC) and stamps the re-bootstrap marker (cutoffHLC +
// rebootstrappedAt wall-clock) so the joiner UI can render the dismissible re-sync
// banner naming the cutoff X. It targets only the joined row (is_owner=0) for the
// (project, owner) pair; the owner self-row is never re-bootstrapped. It NEVER
// touches federation_outbox (R3). last_received_hlc only ever moves forward to the
// snapshot cutoff (a re-bootstrap is always a catch-up to as_of).
func (r *FederatedProjectRepo) MarkReBootstrapTx(ctx context.Context, tx *sql.Tx, localProjectID int64, peerInstanceURL, cutoffHLC, rebootstrappedAt string) error {
	const op = "repo.federated_projects.MarkReBootstrapTx"
	logQuery(ctx, op, localProjectID, peerInstanceURL)
	if _, err := tx.ExecContext(ctx,
		`UPDATE federated_projects
		    SET last_received_hlc = ?, rebootstrap_cutoff_hlc = ?, rebootstrapped_at = ?
		  WHERE local_project_id = ? AND peer_instance_url = ? AND is_owner = 0`,
		cutoffHLC, cutoffHLC, rebootstrappedAt, localProjectID, peerInstanceURL); err != nil {
		return logErr(ctx, op, fmt.Errorf("mark re-bootstrap: %w", err))
	}
	return nil
}

// FederatedPeer is one joined peer row for the peers list (Federation v1 F1.4,
// US-1.4): the per-project federated_projects mapping enriched with the peer's
// display_name + last_contact_at from the federated_instances directory. The
// owner self-row (is_owner=1) is excluded by ListPeersByProject. Status is derived
// by the service from Revoked/Paused + LastContactAt.
type FederatedPeer struct {
	PeerInstanceURL string
	DisplayName     string
	Permissions     model.FederationPermission
	Paused          bool
	Revoked         bool
	// Lost / LostReason carry the owner-side terminal state of this peer mapping.
	// When a peer VOLUNTARILY leaves (Federation v1 F5.5, US-6.3 AC2) the owner
	// marks its row lost with reason "left", which the service derives into the
	// distinct PeerStatusLeft. Lost is false on a healthy peer.
	Lost       bool
	LostReason model.FederationLostReason
	// KeyMismatchAt is the sticky timestamp of a detected peer key CHANGE
	// (Federation v1 F5.6b, US-6.4 AC2). Non-empty → the peer's inbound events are
	// being rejected until an operator re-trusts the new key (the "Trust new key"
	// incident alert). Empty in the healthy case.
	KeyMismatchAt string
	LastSentHLC   string
	LastContactAt *time.Time
	JoinedAt      time.Time
}

// ListPeersByProject returns every remote peer for a local project (US-1.4 AC1),
// joining federated_instances.display_name + last_contact_at (US-1.4 AC2). The
// owner self-row (is_owner=1) is excluded. A LEFT JOIN keeps a peer visible even
// if its directory row has not been written yet (display_name empty,
// last_contact_at nil → the service derives "stale"). Ordered by joined_at.
func (r *FederatedProjectRepo) ListPeersByProject(ctx context.Context, localProjectID int64) ([]FederatedPeer, error) {
	const op = "repo.federated_projects.ListPeersByProject"
	logQuery(ctx, op, localProjectID)
	// JOIN projects p (deleted_at IS NULL) so a soft-deleted parent project's
	// federation rows are never surfaced: ProjectRepo.Delete is a soft-delete, so
	// the federated_projects rows survive (kept for Phase-3 delete-propagation),
	// but a tombstoned project must not show ghost peers (item 7).
	rows, err := r.db.QueryContext(ctx,
		`SELECT fp.peer_instance_url,
		        COALESCE(fi.display_name, ''),
		        fp.permissions, fp.paused, fp.revoked, fp.lost, fp.lost_reason,
		        COALESCE(fp.key_mismatch_at, ''), fp.last_sent_hlc,
		        fi.last_contact_at, fp.joined_at
		   FROM federated_projects fp
		   JOIN projects p ON p.id = fp.local_project_id AND p.deleted_at IS NULL
		   LEFT JOIN federated_instances fi ON fi.instance_url = fp.peer_instance_url
		  WHERE fp.local_project_id = ? AND fp.is_owner = 0
		  ORDER BY fp.joined_at ASC`,
		localProjectID)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("list peers: %w", err))
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	out := make([]FederatedPeer, 0)
	for rows.Next() {
		var peer FederatedPeer
		var permissions, lostReason string
		var paused, revoked, lost int
		var lastSent, lastContact sql.NullString
		var joinedAt string
		if err := rows.Scan(
			&peer.PeerInstanceURL, &peer.DisplayName, &permissions, &paused, &revoked,
			&lost, &lostReason, &peer.KeyMismatchAt, &lastSent, &lastContact, &joinedAt,
		); err != nil {
			return nil, logErr(ctx, op, err)
		}
		peer.Permissions = model.FederationPermission(permissions)
		peer.Paused = paused == 1
		peer.Revoked = revoked == 1
		peer.Lost = lost == 1
		peer.LostReason = model.FederationLostReason(lostReason)
		peer.LastSentHLC = lastSent.String
		if lastContact.Valid && lastContact.String != "" {
			t, err := model.ParseUTC(lastContact.String)
			if err != nil {
				return nil, logErr(ctx, op, fmt.Errorf("parse last_contact_at: %w", err))
			}
			peer.LastContactAt = &t
		}
		j, err := model.ParseUTC(joinedAt)
		if err != nil {
			return nil, logErr(ctx, op, fmt.Errorf("parse joined_at: %w", err))
		}
		peer.JoinedAt = j
		out = append(out, peer)
	}
	if err := rows.Err(); err != nil {
		return nil, logErr(ctx, op, err)
	}
	return out, nil
}

// FederationSurface is the per-project federation role surfaced on the project
// DTO and keyed on by the read-only mutation guard (Federation v1 F2.4, US-2.4).
// For an owner-enabled project it reflects the is_owner=1 self-row (IsOwner=true,
// admin); for a joined project it reflects the row mapping the local project to
// its origin owner (IsOwner=false, the granted read|write permission). It is the
// resolved single row per project — the self-row always wins over peer rows.
type FederationSurface struct {
	OriginInstanceURL string
	Permissions       model.FederationPermission
	IsOwner           bool
	// RebootstrappedAt is the wall-clock cutoff X of the most recent 410-stale
	// re-bootstrap of this project (Federation v1 F4.2, US-4.2 AC4), or empty if
	// the project has never been re-bootstrapped. The joiner UI renders a
	// dismissible re-sync banner naming this timestamp. It comes from the joined
	// row (is_owner=0); the owner self-row is never re-bootstrapped.
	RebootstrappedAt string
	// Lost / LostReason surface that this joined copy's trust link is permanently
	// gone (Federation v1 F5.4, US-6.2 AC3; shared with F5.5 / F5.6a). When Lost is
	// true the UI renders the copy as lost and (for a read-only reason) disables
	// editing — the backend guard remains authoritative. They come from the joined
	// row; the owner self-row is never lost.
	Lost       bool
	LostReason model.FederationLostReason
	// OwnerLastContactAt is the most recent successful contact with the OWNER
	// instance this JOINED copy mirrors (Federation v1 F5.6a, US-6.5 AC1), joined
	// from the federated_instances directory on the joined row's origin_instance_url.
	// The service derives owner-death from it (model.DeriveOwnerOffline against the
	// owner-timeout window) to surface the "pending — owner offline" badge while
	// keeping local edits queued, not blocked (US-6.5 AC2/AC3). It is nil for the
	// owner's OWN federated project (the self-row has no owner-offline notion) and
	// nil when the owner has never been contacted (→ derived offline).
	OwnerLastContactAt *time.Time
}

// FederationSurfaceByProjectIDs resolves the federation surface for many local
// projects in ONE query (no N+1, Federation v1 F2.4). Projects with no
// federated_projects row are simply absent from the returned map, so the DTO
// leaves their federation fields null and the guard is a no-op. When a project
// has both the owner self-row and additional peer rows, the self-row wins
// (is_owner DESC) so the owner is never treated as read-only on its own project.
func (r *FederatedProjectRepo) FederationSurfaceByProjectIDs(ctx context.Context, ids []int64) (map[int64]FederationSurface, error) {
	const op = "repo.federated_projects.FederationSurfaceByProjectIDs"
	out := make(map[int64]FederationSurface, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	logQuery(ctx, op, len(ids))
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	// is_owner DESC, joined_at ASC orders the self-row first for each project; the
	// loop keeps only the first row seen per local_project_id, so the self-row (or
	// the earliest-joined origin row when there is no self-row) wins.
	// JOIN projects p (deleted_at IS NULL) so a soft-deleted parent project's
	// federation surface is never resolved: its federated_projects rows survive
	// the soft-delete but a tombstoned project must not render as an editable (or
	// read-only) federated surface (item 7).
	// LEFT JOIN federated_instances on the JOINED row's owner (origin_instance_url)
	// so the owner's last_contact_at rides along (Federation v1 F5.6a, US-6.5 AC1):
	// the joiner derives owner-death from it. The owner self-row's origin is itself,
	// but its owner-offline notion is meaningless, so the scan only keeps the
	// contact for non-owner rows (is_owner=0).
	rows, err := r.db.QueryContext(ctx,
		`SELECT fp.local_project_id, fp.origin_instance_url, fp.permissions, fp.is_owner, fp.rebootstrapped_at, fp.lost, fp.lost_reason, fi.last_contact_at
		   FROM federated_projects fp
		   JOIN projects p ON p.id = fp.local_project_id AND p.deleted_at IS NULL
		   LEFT JOIN federated_instances fi ON fi.instance_url = fp.origin_instance_url
		  WHERE fp.local_project_id IN (`+strings.Join(placeholders, ",")+`)
		  ORDER BY fp.local_project_id, fp.is_owner DESC, fp.joined_at ASC`,
		args...)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("query federation surface: %w", err))
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	for rows.Next() {
		var localID int64
		var origin, permissions, lostReason string
		var isOwner, lost int
		var rebootAt, ownerContact sql.NullString
		if err := rows.Scan(&localID, &origin, &permissions, &isOwner, &rebootAt, &lost, &lostReason, &ownerContact); err != nil {
			return nil, logErr(ctx, op, err)
		}
		if _, seen := out[localID]; seen {
			continue
		}
		s := FederationSurface{
			OriginInstanceURL: origin,
			Permissions:       model.FederationPermission(permissions),
			IsOwner:           isOwner == 1,
			RebootstrappedAt:  rebootAt.String,
			Lost:              lost == 1,
			LostReason:        model.FederationLostReason(lostReason),
		}
		// Owner contact recency only applies to a JOINED copy (is_owner=0): the
		// owner's own project has no owner-offline notion (US-6.5).
		if !s.IsOwner && ownerContact.Valid && ownerContact.String != "" {
			t, err := model.ParseUTC(ownerContact.String)
			if err != nil {
				return nil, logErr(ctx, op, fmt.Errorf("parse owner last_contact_at: %w", err))
			}
			s.OwnerLastContactAt = &t
		}
		out[localID] = s
	}
	if err := rows.Err(); err != nil {
		return nil, logErr(ctx, op, err)
	}
	return out, nil
}

// FederatedProjectSummary is one federated project's identity + resolved role for
// the privacy/federation overview (Federation v1 F6.4, US-7.1 AC1): the local
// int64 id, the project Title, and the surface (IsOwner + granted Permissions) the
// role is derived from. The named peer list is resolved separately (one batched
// PeerInstancesByProjectIDs call) so the overview never issues a per-project query.
type FederatedProjectSummary struct {
	LocalProjectID int64
	Title          string
	IsOwner        bool
	Permissions    model.FederationPermission
}

// ListFederatedProjectsOverview returns every live (non-soft-deleted) federated
// project with its resolved federation surface in ONE query (Federation v1 F6.4,
// US-7.1 AC1; no N+1). A project's surface is the self-row when present (owner) or
// the earliest-joined peer row otherwise (a joined copy), matched by the same
// is_owner DESC, joined_at ASC ordering FederationSurfaceByProjectIDs uses, so the
// overview and the project DTO agree on the role. Non-federated projects
// (is_federated=0 or no federated_projects row) are excluded. Ordered by title for
// a stable list.
func (r *FederatedProjectRepo) ListFederatedProjectsOverview(ctx context.Context) ([]FederatedProjectSummary, error) {
	const op = "repo.federated_projects.ListFederatedProjectsOverview"
	logQuery(ctx, op)
	rows, err := r.db.QueryContext(ctx,
		`SELECT p.id, p.title, fp.is_owner, fp.permissions
		   FROM projects p
		   JOIN federated_projects fp ON fp.local_project_id = p.id
		  WHERE p.is_federated = 1 AND p.deleted_at IS NULL
		  ORDER BY p.id, fp.is_owner DESC, fp.joined_at ASC`)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("list federated projects: %w", err))
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	// is_owner DESC, joined_at ASC orders the self-row (or earliest peer row) first
	// per project; keep only the first row seen per id so the resolved role matches
	// FederationSurfaceByProjectIDs (the self-row always wins).
	seen := make(map[int64]struct{})
	out := make([]FederatedProjectSummary, 0)
	for rows.Next() {
		var s FederatedProjectSummary
		var isOwner int
		var permissions string
		if err := rows.Scan(&s.LocalProjectID, &s.Title, &isOwner, &permissions); err != nil {
			return nil, logErr(ctx, op, err)
		}
		if _, ok := seen[s.LocalProjectID]; ok {
			continue
		}
		seen[s.LocalProjectID] = struct{}{}
		s.IsOwner = isOwner == 1
		s.Permissions = model.FederationPermission(permissions)
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, logErr(ctx, op, err)
	}
	// Stable order by title (secondary id) for the overview UI.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Title != out[j].Title {
			return out[i].Title < out[j].Title
		}
		return out[i].LocalProjectID < out[j].LocalProjectID
	})
	return out, nil
}

// PeerInstancesByProjectIDs resolves, in ONE query (no N+1, Federation v1 F6.4,
// US-7.1 AC3), the named peer audience of many local projects keyed by
// local_project_id. For each project it returns the non-owner, NON-REVOKED,
// NON-LOST peers the project is visible to — each as {InstanceURL, DisplayName} —
// joining the handshake-supplied display_name from the federated_instances
// directory. A peer that left (lost=1, lost_reason='left') is excluded just like a
// revoked one, so a departed peer never lingers in the visible-to audience; PAUSED
// peers stay (pausing keeps the trust link and already-shared data). The
// owner self-row (is_owner=1) is excluded; so is the JOINED copy's origin owner
// (a project this instance JOINED has no outbound peer audience of its own — its
// only is_owner=0 row points back at the owner, identified by
// origin_instance_url == peer_instance_url), so a joined copy resolves to an empty
// list. A peer whose directory row has no display_name yet falls back to its URL,
// so the new-task hint can always render a name. Projects with no qualifying peer
// are simply absent from the map (the DTO then exposes an empty array). A
// soft-deleted parent project surfaces no peers. selfInstanceURL is this
// instance's federation identity, used to exclude the owner self-row by URL.
func (r *FederatedProjectRepo) PeerInstancesByProjectIDs(ctx context.Context, ids []int64, selfInstanceURL string) (map[int64][]model.PeerInstance, error) {
	const op = "repo.federated_projects.PeerInstancesByProjectIDs"
	out := make(map[int64][]model.PeerInstance, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	logQuery(ctx, op, len(ids))
	placeholders := make([]string, len(ids))
	args := make([]any, 0, len(ids)+1)
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	args = append(args, selfInstanceURL)
	// Exclude: the owner self-row (is_owner=1); revoked peers; LOST peers (a peer
	// that voluntarily left sets lost=1, lost_reason='left' while keeping revoked=0,
	// see MarkLeftByPeer — a departed/owner-dead peer is no longer part of the
	// current audience and must be excluded exactly as a revoked one is, so the
	// 'visible to N peers' badge, the QuickAdd new-task hint, and the federation
	// overview never misrepresent a project's audience, US-7.1); the JOINED copy's
	// origin owner (origin_instance_url == peer_instance_url marks the row that
	// points at the owner of a joined copy, which is never an outbound audience);
	// and any row whose peer is THIS instance (defensive — the self-row should
	// already be is_owner=1, but a stale peer row pointing at ourselves must never
	// list us as our own audience). PAUSED peers are intentionally NOT excluded:
	// pausing keeps the trust link and the data already shared, so a paused peer
	// remains part of the current audience. COALESCE the display_name to the URL so
	// the hint always has a name. Ordered by joined_at for a stable list.
	rows, err := r.db.QueryContext(ctx,
		`SELECT fp.local_project_id, fp.peer_instance_url,
		        COALESCE(NULLIF(fi.display_name, ''), fp.peer_instance_url)
		   FROM federated_projects fp
		   JOIN projects p ON p.id = fp.local_project_id AND p.deleted_at IS NULL
		   LEFT JOIN federated_instances fi ON fi.instance_url = fp.peer_instance_url
		  WHERE fp.local_project_id IN (`+strings.Join(placeholders, ",")+`)
		    AND fp.is_owner = 0
		    AND fp.revoked = 0
		    AND fp.lost = 0
		    AND fp.peer_instance_url <> fp.origin_instance_url
		    AND fp.peer_instance_url <> ?
		  ORDER BY fp.local_project_id, fp.joined_at ASC`,
		args...)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("query peer instances: %w", err))
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	for rows.Next() {
		var localID int64
		var pi model.PeerInstance
		if err := rows.Scan(&localID, &pi.InstanceURL, &pi.DisplayName); err != nil {
			return nil, logErr(ctx, op, err)
		}
		out[localID] = append(out[localID], pi)
	}
	if err := rows.Err(); err != nil {
		return nil, logErr(ctx, op, err)
	}
	return out, nil
}

// PeerHealth is one peer's status-derivation inputs for a project (Federation v1
// F4.3, US-4.3): the per-project mapping flags (Revoked/Paused) + the sticky
// KeyMismatchAt marker, joined with LastContactAt from the instance directory.
// The service rolls these (plus the outbox overdue-pending signal) into a single
// SyncStatus per project. KeyMismatchAt is empty when no signature mismatch has
// been observed.
type PeerHealth struct {
	PeerInstanceURL string
	DisplayName     string
	Revoked         bool
	Paused          bool
	KeyMismatchAt   string
	LastContactAt   *time.Time
}

// ListPeerHealthByProject returns the status-derivation inputs for every remote
// peer of a project (Federation v1 F4.3, US-4.3), joining last_contact_at from
// the instance directory. The owner self-row (is_owner=1) is excluded — sync
// status is about the PEERS, not this instance. A LEFT JOIN keeps a peer with no
// directory row visible (last_contact_at nil → the service treats it as
// unreachable). A soft-deleted parent project surfaces no peers (item 7).
func (r *FederatedProjectRepo) ListPeerHealthByProject(ctx context.Context, localProjectID int64) ([]PeerHealth, error) {
	const op = "repo.federated_projects.ListPeerHealthByProject"
	logQuery(ctx, op, localProjectID)
	rows, err := r.db.QueryContext(ctx,
		`SELECT fp.peer_instance_url, COALESCE(fi.display_name, ''), fp.paused, fp.revoked, COALESCE(fp.key_mismatch_at, ''), fi.last_contact_at
		   FROM federated_projects fp
		   JOIN projects p ON p.id = fp.local_project_id AND p.deleted_at IS NULL
		   LEFT JOIN federated_instances fi ON fi.instance_url = fp.peer_instance_url
		  WHERE fp.local_project_id = ? AND fp.is_owner = 0
		  ORDER BY fp.joined_at ASC`,
		localProjectID)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("list peer health: %w", err))
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	out := make([]PeerHealth, 0)
	for rows.Next() {
		var h PeerHealth
		var paused, revoked int
		var lastContact sql.NullString
		if err := rows.Scan(&h.PeerInstanceURL, &h.DisplayName, &paused, &revoked, &h.KeyMismatchAt, &lastContact); err != nil {
			return nil, logErr(ctx, op, err)
		}
		h.Paused = paused == 1
		h.Revoked = revoked == 1
		if lastContact.Valid && lastContact.String != "" {
			t, err := model.ParseUTC(lastContact.String)
			if err != nil {
				return nil, logErr(ctx, op, fmt.Errorf("parse last_contact_at: %w", err))
			}
			h.LastContactAt = &t
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, logErr(ctx, op, err)
	}
	return out, nil
}

// ListOwnedFederatedProjectIDs returns the local project ids that have been
// enabled for federation (carry an is_owner=1 self-row, Federation v1 F4.3) and
// are not soft-deleted, so the status endpoint computes one SyncStatus per shared
// project. Ordered ascending for a stable response.
func (r *FederatedProjectRepo) ListOwnedFederatedProjectIDs(ctx context.Context) ([]int64, error) {
	const op = "repo.federated_projects.ListOwnedFederatedProjectIDs"
	logQuery(ctx, op)
	rows, err := r.db.QueryContext(ctx,
		`SELECT DISTINCT fp.local_project_id
		   FROM federated_projects fp
		   JOIN projects p ON p.id = fp.local_project_id AND p.deleted_at IS NULL
		  WHERE fp.is_owner = 1
		  ORDER BY fp.local_project_id ASC`)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("list owned federated project ids: %w", err))
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	out := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, logErr(ctx, op, err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, logErr(ctx, op, err)
	}
	return out, nil
}

// OwnerSelfInstanceURLs returns the distinct origin_instance_url values stored on
// this instance's OWNER self-rows (is_owner=1) — the federation identity (BASE_URL)
// this install used when it enabled federation (Federation v1 F6.5, US-8.5 AC2).
// Comparing them to the CURRENT BASE_URL after a restore detects an instance_url
// change (R27): a mismatch means the backup was restored under a new URL, so the
// existing mappings must be marked read-only history rather than kept syncing under
// a URL peers will reject. A fresh install with no federated project returns empty.
func (r *FederatedProjectRepo) OwnerSelfInstanceURLs(ctx context.Context) ([]string, error) {
	const op = "repo.federated_projects.OwnerSelfInstanceURLs"
	logQuery(ctx, op)
	rows, err := r.db.QueryContext(ctx,
		`SELECT DISTINCT origin_instance_url FROM federated_projects WHERE is_owner = 1`)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("owner self urls: %w", err))
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	out := make([]string, 0)
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, logErr(ctx, op, err)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, logErr(ctx, op, err)
	}
	return out, nil
}

// MarkAllLostInstanceURLChanged marks EVERY non-lost federation mapping row
// (owner self-rows AND peer rows) lost with reason=instance_url_changed
// (Federation v1 F6.5, US-8.5 AC2, R27). It is the history-preservation action
// after a restore under a new BASE_URL: the rows are NOT deleted — they stay
// locally readable as history while outbound/inbound sync is halted and the user is
// prompted to re-invite under the new URL. It targets only lost=0 rows so a re-run
// is idempotent, and returns the number of rows transitioned so the caller can WARN
// with a count. The keypair (federation_keys) is untouched (no key regen).
func (r *FederatedProjectRepo) MarkAllLostInstanceURLChanged(ctx context.Context) (int64, error) {
	const op = "repo.federated_projects.MarkAllLostInstanceURLChanged"
	logQuery(ctx, op)
	res, err := r.db.ExecContext(ctx,
		`UPDATE federated_projects SET lost = 1, lost_reason = ? WHERE lost = 0`,
		string(model.FederationLostInstanceURLChanged))
	if err != nil {
		return 0, logErr(ctx, op, fmt.Errorf("mark all lost instance_url_changed: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, logErr(ctx, op, fmt.Errorf("mark all lost rows: %w", err))
	}
	return n, nil
}

// MarkKeyMismatch stamps the sticky key_mismatch_at marker on a (local project,
// peer) row the FIRST time a signature mismatch is observed for that peer
// (Federation v1 F4.3, US-4.3 AC4). It is STICKY: the UPDATE only fires when
// key_mismatch_at is still NULL, so a later mismatch does NOT move the timestamp
// (the marker stays put until an operator re-trusts the new key in F5.6b). It
// returns whether the row TRANSITIONED (was NULL → now set) so the caller
// publishes the ScopeFederation SSE only once, on the transition, not on every
// rejected event. A mismatch on a non-existent (project, peer) row is a no-op.
func (r *FederatedProjectRepo) MarkKeyMismatch(ctx context.Context, localProjectID int64, peerInstanceURL, at string) (bool, error) {
	const op = "repo.federated_projects.MarkKeyMismatch"
	logQuery(ctx, op, localProjectID, peerInstanceURL)
	res, err := r.db.ExecContext(ctx,
		`UPDATE federated_projects
		    SET key_mismatch_at = ?
		  WHERE local_project_id = ? AND peer_instance_url = ? AND is_owner = 0 AND key_mismatch_at IS NULL`,
		at, localProjectID, peerInstanceURL)
	if err != nil {
		return false, logErr(ctx, op, fmt.Errorf("mark key mismatch: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, logErr(ctx, op, fmt.Errorf("mark key mismatch rows: %w", err))
	}
	return n > 0, nil
}

// ClearKeyMismatch wipes the sticky key_mismatch_at marker on a (local project,
// peer) row when an operator explicitly re-trusts the peer's new key (Federation
// v1 F5.6b, US-6.4 AC3 — the manual "Trust new key" action). It is the ONLY writer
// that clears the marker MarkKeyMismatch set (F4.3 never auto-clears it, US-4.3
// AC4 sticky). After clearing, MarkKeyMismatch can stamp a fresh marker for a
// LATER rotation. It targets only a peer row (is_owner=0). It returns the
// affected-row count so the service can tell whether a marker was actually
// cleared; clearing a peer with no marker is a no-op (0 rows, nil error).
func (r *FederatedProjectRepo) ClearKeyMismatch(ctx context.Context, localProjectID int64, peerInstanceURL string) (int, error) {
	const op = "repo.federated_projects.ClearKeyMismatch"
	logQuery(ctx, op, localProjectID, peerInstanceURL)
	res, err := r.db.ExecContext(ctx,
		`UPDATE federated_projects
		    SET key_mismatch_at = NULL
		  WHERE local_project_id = ? AND peer_instance_url = ? AND is_owner = 0 AND key_mismatch_at IS NOT NULL`,
		localProjectID, peerInstanceURL)
	if err != nil {
		return 0, logErr(ctx, op, fmt.Errorf("clear key mismatch: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, logErr(ctx, op, fmt.Errorf("clear key mismatch rows: %w", err))
	}
	return int(n), nil
}

// SetPaused flips the paused flag on a single (local project, peer) mapping row
// (Federation v1 F5.3, US-6.1 AC1/AC2). It is the non-destructive pause/resume
// control: it touches ONLY the paused column, leaving permissions, the sync
// cursors, and the trust link intact (unlike a revoke). It targets only a peer
// row (is_owner=0) so the owner self-row can never be paused. It returns the
// affected-row count so the service maps a missing peer (0 rows) to a 404. It is
// idempotent: re-pausing an already-paused peer is a no-op write that still
// reports 1 affected row (the row matched).
func (r *FederatedProjectRepo) SetPaused(ctx context.Context, localProjectID int64, peerInstanceURL string, paused bool) (int, error) {
	const op = "repo.federated_projects.SetPaused"
	logQuery(ctx, op, localProjectID, peerInstanceURL)
	res, err := r.db.ExecContext(ctx,
		`UPDATE federated_projects
		    SET paused = ?
		  WHERE local_project_id = ? AND peer_instance_url = ? AND is_owner = 0`,
		boolToInt(paused), localProjectID, peerInstanceURL)
	if err != nil {
		return 0, logErr(ctx, op, fmt.Errorf("set paused: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, logErr(ctx, op, fmt.Errorf("set paused rows: %w", err))
	}
	return int(n), nil
}

// Revoke permanently flips revoked=1 on a single (local project, peer) mapping
// row (Federation v1 F5.4, US-6.2 AC1). It is the OWNER-side control: a revoked
// peer is dropped from outbound fan-out (PeersForProject) and its inbound traffic
// is rejected 403 federation_revoked (the inbox validator). It targets only a peer
// row (is_owner=0) so the owner self-row can never be revoked. Revoke is
// IRREVERSIBLE (no un-revoke; re-collaboration needs a fresh invite, US-6.2 AC5),
// so there is no companion un-revoke method. It returns the affected-row count so
// the service maps a missing peer (0 rows) to a 404; it is idempotent — re-revoking
// an already-revoked peer is a no-op write that still reports 1 affected row.
func (r *FederatedProjectRepo) Revoke(ctx context.Context, localProjectID int64, peerInstanceURL string) (int, error) {
	const op = "repo.federated_projects.Revoke"
	logQuery(ctx, op, localProjectID, peerInstanceURL)
	res, err := r.db.ExecContext(ctx,
		`UPDATE federated_projects
		    SET revoked = 1
		  WHERE local_project_id = ? AND peer_instance_url = ? AND is_owner = 0`,
		localProjectID, peerInstanceURL)
	if err != nil {
		return 0, logErr(ctx, op, fmt.Errorf("revoke peer: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, logErr(ctx, op, fmt.Errorf("revoke peer rows: %w", err))
	}
	return int(n), nil
}

// RevokeTx is Revoke inside a caller transaction (Federation v1 F5.4, US-6.2 AC1)
// so the revoked flag flip and the federation_revoke outbox enqueue commit or roll
// back together. Same semantics as Revoke: targets only is_owner=0, returns the
// affected-row count, idempotent.
func (r *FederatedProjectRepo) RevokeTx(ctx context.Context, tx *sql.Tx, localProjectID int64, peerInstanceURL string) (int, error) {
	const op = "repo.federated_projects.RevokeTx"
	logQuery(ctx, op, localProjectID, peerInstanceURL)
	res, err := tx.ExecContext(ctx,
		`UPDATE federated_projects
		    SET revoked = 1
		  WHERE local_project_id = ? AND peer_instance_url = ? AND is_owner = 0`,
		localProjectID, peerInstanceURL)
	if err != nil {
		return 0, logErr(ctx, op, fmt.Errorf("revoke peer tx: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, logErr(ctx, op, fmt.Errorf("revoke peer tx rows: %w", err))
	}
	return int(n), nil
}

// MarkLost stamps the lost flag + reason on the JOINER's mapping to its origin
// owner (Federation v1 F5.4, US-6.2 AC3/AC4; shared with F5.5 / F5.6a). It targets
// the is_owner=0 row whose origin_instance_url is the owner that revoked us (or
// that we left / that died), so the local copy is rendered lost. It is IDEMPOTENT
// and STICKY: the UPDATE only fires when lost is still 0, so re-applying the same
// revoke (an at-least-once redelivery, or the offline-return self-detect after the
// in-band revoke already landed) does NOT overwrite an existing reason. It returns
// whether the row TRANSITIONED (was not-lost → now lost) so the caller publishes a
// refresh SSE / records audit only once. Marking a non-existent (project, owner)
// row is a no-op (false, nil).
func (r *FederatedProjectRepo) MarkLost(ctx context.Context, localProjectID int64, originInstanceURL string, reason model.FederationLostReason) (bool, error) {
	const op = "repo.federated_projects.MarkLost"
	logQuery(ctx, op, localProjectID, originInstanceURL)
	res, err := r.db.ExecContext(ctx,
		`UPDATE federated_projects
		    SET lost = 1, lost_reason = ?
		  WHERE local_project_id = ? AND origin_instance_url = ? AND is_owner = 0 AND lost = 0`,
		string(reason), localProjectID, originInstanceURL)
	if err != nil {
		return false, logErr(ctx, op, fmt.Errorf("mark lost: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, logErr(ctx, op, fmt.Errorf("mark lost rows: %w", err))
	}
	return n > 0, nil
}

// MarkLostTx is MarkLost inside a caller transaction (Federation v1 F5.5, US-6.3
// AC1) so the JOINER's "left" marker commits atomically with the federation_leave
// outbox enqueue. Same semantics as MarkLost: keys on origin_instance_url, targets
// is_owner=0, idempotent/sticky on lost=0, returns whether the row transitioned.
func (r *FederatedProjectRepo) MarkLostTx(ctx context.Context, tx *sql.Tx, localProjectID int64, originInstanceURL string, reason model.FederationLostReason) (bool, error) {
	const op = "repo.federated_projects.MarkLostTx"
	logQuery(ctx, op, localProjectID, originInstanceURL)
	res, err := tx.ExecContext(ctx,
		`UPDATE federated_projects
		    SET lost = 1, lost_reason = ?
		  WHERE local_project_id = ? AND origin_instance_url = ? AND is_owner = 0 AND lost = 0`,
		string(reason), localProjectID, originInstanceURL)
	if err != nil {
		return false, logErr(ctx, op, fmt.Errorf("mark lost tx: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, logErr(ctx, op, fmt.Errorf("mark lost tx rows: %w", err))
	}
	return n > 0, nil
}

// MarkLeftByPeer stamps the lost flag + reason="left" on the OWNER's mapping to a
// specific peer that VOLUNTARILY left a project (Federation v1 F5.5, US-6.3 AC2).
// It is the symmetric owner-side counterpart of MarkLost: where MarkLost keys on
// origin_instance_url (the joiner marking its mapping to the owner that revoked it),
// this keys on peer_instance_url (the owner marking the specific peer that left).
// It targets only is_owner=0 rows so the owner self-row can never be marked. It is
// IDEMPOTENT and STICKY: the UPDATE only fires when lost is still 0, so a
// redelivered leave (at-least-once) does NOT re-transition or overwrite the reason
// (e.g. a peer the owner had already revoked stays revoked, not "left", because
// that row is lost=0 only if it was never revoked — a revoked peer carries
// revoked=1 with lost still 0, so to keep revoke authoritative the caller checks
// revoked first; see MarkLeftByPeerTx for the in-tx leg the inbox apply uses). It
// returns whether the row TRANSITIONED so the caller records the audit / status
// change once. Marking an unknown peer is a no-op (false, nil).
func (r *FederatedProjectRepo) MarkLeftByPeer(ctx context.Context, localProjectID int64, peerInstanceURL string) (bool, error) {
	const op = "repo.federated_projects.MarkLeftByPeer"
	logQuery(ctx, op, localProjectID, peerInstanceURL)
	res, err := r.db.ExecContext(ctx,
		`UPDATE federated_projects
		    SET lost = 1, lost_reason = ?
		  WHERE local_project_id = ? AND peer_instance_url = ? AND is_owner = 0 AND lost = 0`,
		string(model.FederationLostLeft), localProjectID, peerInstanceURL)
	if err != nil {
		return false, logErr(ctx, op, fmt.Errorf("mark left: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, logErr(ctx, op, fmt.Errorf("mark left rows: %w", err))
	}
	return n > 0, nil
}

// MarkLeftByPeerTx is MarkLeftByPeer inside a caller transaction (Federation v1
// F5.5, US-6.3 AC2) so the owner's "left" marker commits atomically with the inbox
// dedup applied_at stamp when applying a federation_leave control event. Same
// semantics as MarkLeftByPeer: targets is_owner=0, idempotent/sticky on lost=0,
// returns whether the row transitioned. The extra AND revoked = 0 is
// defense-in-depth: a peer the owner has already revoked must NOT also be
// transitioned to the softer "left" state (revoke is terminal and takes
// precedence), so a stray/late federation_leave from a revoked peer is a no-op.
func (r *FederatedProjectRepo) MarkLeftByPeerTx(ctx context.Context, tx *sql.Tx, localProjectID int64, peerInstanceURL string) (bool, error) {
	const op = "repo.federated_projects.MarkLeftByPeerTx"
	logQuery(ctx, op, localProjectID, peerInstanceURL)
	res, err := tx.ExecContext(ctx,
		`UPDATE federated_projects
		    SET lost = 1, lost_reason = ?
		  WHERE local_project_id = ? AND peer_instance_url = ? AND is_owner = 0 AND lost = 0 AND revoked = 0`,
		string(model.FederationLostLeft), localProjectID, peerInstanceURL)
	if err != nil {
		return false, logErr(ctx, op, fmt.Errorf("mark left tx: %w", err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, logErr(ctx, op, fmt.Errorf("mark left tx rows: %w", err))
	}
	return n > 0, nil
}

// boolToInt maps a Go bool to the 0/1 integer the SQLite CHECK columns store.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Get returns the federation mapping for a single (local project, peer).
// Returns ErrNotFound when no row exists.
//
// JOIN projects p (deleted_at IS NULL), matching SelfRow's pattern: a
// soft-deleted parent project keeps its mapping rows in the DB (Phase-3
// delete-propagation needs them) but must report ErrNotFound to surface reads so
// a tombstoned-but-federated project can never be fetched as live (item 8).
func (r *FederatedProjectRepo) Get(ctx context.Context, localProjectID int64, peerInstanceURL string) (*model.FederatedProject, error) {
	const op = "repo.federated_projects.Get"
	logQuery(ctx, op, localProjectID, peerInstanceURL)
	row := r.db.QueryRowContext(ctx,
		`SELECT `+qualifyFederatedProjectColumns+`
		   FROM federated_projects fp
		   JOIN projects p ON p.id = fp.local_project_id AND p.deleted_at IS NULL
		  WHERE fp.local_project_id = ? AND fp.peer_instance_url = ?`,
		localProjectID, peerInstanceURL)
	fp, err := scanFederatedProject(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, logErr(ctx, op, err)
	}
	return fp, nil
}

// ListByProject returns every federation mapping row for a local project,
// ordered by joined_at. The owner self-row is included; callers that render a
// peers table exclude it (it is identifiable by is_owner=1).
func (r *FederatedProjectRepo) ListByProject(ctx context.Context, localProjectID int64) ([]model.FederatedProject, error) {
	const op = "repo.federated_projects.ListByProject"
	logQuery(ctx, op, localProjectID)
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+federatedProjectColumns+` FROM federated_projects WHERE local_project_id = ? ORDER BY joined_at ASC`,
		localProjectID)
	if err != nil {
		return nil, logErr(ctx, op, fmt.Errorf("list federated projects: %w", err))
	}
	defer logging.LogClose(ctx, op+".rows", rows)

	out := make([]model.FederatedProject, 0)
	for rows.Next() {
		fp, err := scanFederatedProject(rows)
		if err != nil {
			return nil, logErr(ctx, op, err)
		}
		out = append(out, *fp)
	}
	if err := rows.Err(); err != nil {
		return nil, logErr(ctx, op, err)
	}
	return out, nil
}

// SelfRow returns the owner's is_owner=1 self-row for a project, or ErrNotFound
// when the project has not been enabled for federation.
func (r *FederatedProjectRepo) SelfRow(ctx context.Context, localProjectID int64) (*model.FederatedProject, error) {
	const op = "repo.federated_projects.SelfRow"
	logQuery(ctx, op, localProjectID)
	// JOIN projects p (deleted_at IS NULL): a soft-deleted project keeps its
	// self-row in the DB (Phase-3 delete-propagation needs it) but must report
	// ErrNotFound to UI/surface reads so it never renders as editable (item 7).
	row := r.db.QueryRowContext(ctx,
		`SELECT `+qualifyFederatedProjectColumns+`
		   FROM federated_projects fp
		   JOIN projects p ON p.id = fp.local_project_id AND p.deleted_at IS NULL
		  WHERE fp.local_project_id = ? AND fp.is_owner = 1`,
		localProjectID)
	fp, err := scanFederatedProject(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, logErr(ctx, op, err)
	}
	return fp, nil
}

// JoinerRow returns the joiner's is_owner=0 mapping to its origin owner for a
// project (Federation v1 F5.5, US-6.3), or ErrNotFound when the project is not a
// joined federated copy (it is the owner's own project, or not federated at all).
// A joined project has exactly one is_owner=0 mapping (to its single origin owner
// in v1), so this returns that row; the caller reads OriginInstanceURL (the owner
// to send the federation_leave to) and RemoteProjectID (the owner's project
// client_id the leave targets). It JOINs projects (deleted_at IS NULL) so a
// tombstoned project reports ErrNotFound.
func (r *FederatedProjectRepo) JoinerRow(ctx context.Context, localProjectID int64) (*model.FederatedProject, error) {
	const op = "repo.federated_projects.JoinerRow"
	logQuery(ctx, op, localProjectID)
	row := r.db.QueryRowContext(ctx,
		`SELECT `+qualifyFederatedProjectColumns+`
		   FROM federated_projects fp
		   JOIN projects p ON p.id = fp.local_project_id AND p.deleted_at IS NULL
		  WHERE fp.local_project_id = ? AND fp.is_owner = 0
		  ORDER BY fp.joined_at ASC
		  LIMIT 1`,
		localProjectID)
	fp, err := scanFederatedProject(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, logErr(ctx, op, err)
	}
	return fp, nil
}

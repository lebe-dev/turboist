// Federated-project surface helpers (Federation v1 F2.4, US-2.4).
//
// A project may be a local-only project, the owner's own federated project, or a
// read-only/writable copy joined from another instance. These pure helpers
// centralise the role derivation so the project header, the project list, and the
// project page all agree on when controls are disabled and which badges show. The
// backend 403 guard (federation_read_only) remains the authoritative enforcement;
// these helpers only drive the UI affordances.

import { ApiError } from '$lib/api/errors';
import type { PeerInstance, Project } from '$lib/api/types';

// visiblePeers returns the named peer audience a project is shared with
// (Federation v1 F6.4, US-7.1 AC3). It reads the per-project peerInstances array
// resolved at bootstrap — the non-owner, non-revoked peers — so the "visible to N
// peers" badge and the new-task editor hint share one source. An empty array (a
// non-federated project, or one with no peers yet) means nothing to show.
export function visiblePeers(project: Pick<Project, 'peerInstances'>): PeerInstance[] {
	return project.peerInstances ?? [];
}

// peerNamesLabel renders the peer audience as a comma-joined human list of names
// for the editor hint and badge tooltip ("alice.example, bob.example", US-7.1 AC3
// — the explicit instance list, NOT a bare count). It prefers the handshake-
// supplied displayName, falling back to the bare instanceUrl.
export function peerNamesLabel(peers: PeerInstance[]): string {
	return peers.map((p) => p.displayName || p.instanceUrl).join(', ');
}

// isReadOnlyFederated reports whether a JOINED federated copy must be rendered
// read-only (US-2.4 AC4 UI leg; Federation v1 F5.4 US-6.2 AC3). A copy is
// read-only when, being federated and NOT owned locally, EITHER:
//   - it was permanently lost with a read-only reason — revoked (US-6.2) or
//     owner-dead (US-6.5) — regardless of its original permission grant, OR
//   - it is merely granted read permission.
// The owner's own federated project (isOwner) and write/admin grants are
// editable; a voluntarily-LEFT copy (lost reason 'left', F5.5) is NOT read-only —
// it becomes a plain editable local project; non-federated projects are always
// editable. The backend 403 guard remains authoritative.
export function isReadOnlyFederated(
	project: Pick<Project, 'isFederated' | 'isOwner' | 'federationPermissions' | 'federationLost' | 'federationLostReason'>
): boolean {
	if (!project.isFederated) return false;
	// A project lost to an instance_url change (Federation v1 F6.5, US-8.5 AC2) is
	// read-only HISTORY regardless of ownership — even the owner's OWN project, since
	// its mappings were marked lost on a restore under a new BASE_URL. Checked BEFORE
	// the isOwner short-circuit so the owner's history copy is not treated editable.
	if (project.federationLost && project.federationLostReason === 'instance_url_changed') return true;
	if (project.isOwner) return false;
	if (project.federationLost) {
		return project.federationLostReason === 'revoked' || project.federationLostReason === 'owner-dead';
	}
	return project.federationPermissions === 'read';
}

// isJoinedFederated reports whether a project is a copy joined from another
// instance (federated, not the local owner). Such a project renders its origin
// instance badge regardless of permission grade (US-2.4 AC2).
export function isJoinedFederated(project: Pick<Project, 'isFederated' | 'isOwner'>): boolean {
	return project.isFederated && !project.isOwner;
}

// isOwnerOffline reports whether a JOINED copy's owner has gone unreachable past
// the owner-timeout window so the UI shows a "pending — owner offline" badge
// (Federation v1 F5.6a, US-6.5 AC1). It is true only for a joined copy (federated,
// not the local owner) whose owner is offline AND whose link is NOT already
// permanently lost (a revoked/left copy has its own terminal surface and is not
// merely "owner offline"). Crucially this is INFORMATIONAL only — owner-offline
// does NOT lock editing (US-6.5 AC2): local edits stay enabled and queue until the
// owner returns, so isReadOnlyFederated deliberately ignores this flag.
export function isOwnerOffline(
	project: Pick<Project, 'isFederated' | 'isOwner' | 'ownerOffline' | 'federationLost'>
): boolean {
	if (!project.isFederated) return false;
	if (project.isOwner) return false;
	if (project.federationLost) return false;
	return project.ownerOffline;
}

// federationRole is the coarse role label for a project's federation state, used
// to choose the badge variant (US-2.4 AC1/AC2):
//   - 'none'  — not federated;
//   - 'owner' — the owner's own federated project (controls stay enabled);
//   - 'write' — a joined writable copy;
//   - 'read'  — a joined read-only copy (controls disabled).
export type FederationRole = 'none' | 'owner' | 'write' | 'read';

export function federationRole(project: Pick<Project, 'isFederated' | 'isOwner' | 'federationPermissions'>): FederationRole {
	if (!project.isFederated) return 'none';
	if (project.isOwner) return 'owner';
	return project.federationPermissions === 'read' ? 'read' : 'write';
}

// isReadOnlyFederationError reports whether a thrown error is the backend
// read-only guard rejection (403 federation_read_only, Federation v1 F2.4, US-2.4
// AC4). The project page maps it to a graceful read-only toast instead of a
// generic failure message, so a write attempt that slipped past the disabled UI
// (or arrived via a stale optimistic state) reverts cleanly without crashing.
export function isReadOnlyFederationError(err: unknown): boolean {
	return err instanceof ApiError && err.code === 'federation_read_only';
}

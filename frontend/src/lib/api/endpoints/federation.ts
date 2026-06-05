import type { ApiClient } from '../client';
import type {
	AuditResponse,
	CreateInviteRequest,
	CreateInviteResponse,
	DeadLetterEntry,
	FederationHealth,
	Invite,
	JoinInvite,
	JoinPreview,
	JoinResult,
	OverviewResponse,
	Peer,
	RetentionSettings,
	SyncStatus,
	UpdateRetentionRequest
} from '../types';

// Federation endpoints (Federation v1). F1.2 lands invite creation; F1.3 adds
// invite list/revoke/delete; F1.4 adds the project peers list; later milestones
// add the join flow.
// These are JWT-only owner-control-plane routes, not the signed peer-to-peer
// trust plane.
export const federation = {
	// createInvite mints a one-time share invite for a federated project
	// (Federation v1 F1.2, US-1.2). The response carries the plaintext secret and
	// the shareable join link exactly once — the secret is never re-served, so
	// the caller must surface/copy it immediately.
	createInvite(
		client: ApiClient,
		projectId: number,
		body: CreateInviteRequest
	): Promise<CreateInviteResponse> {
		return client.fetch(`/api/v1/projects/${projectId}/invites`, {
			method: 'POST',
			body
		});
	},

	// listInvites returns every invite for a project with its derived lifecycle
	// status (Federation v1 F1.3, US-1.3 AC1). The response carries NO secret —
	// the secret is shown to the owner once at creation and never re-served
	// (US-1.3 AC5).
	listInvites(client: ApiClient, projectId: number): Promise<Invite[]> {
		return client.fetch(`/api/v1/projects/${projectId}/invites`);
	},

	// revokeInvite stamps revoked_at on an invite, flipping its derived status to
	// revoked (Federation v1 F1.3, US-1.3 AC2). It is idempotent server-side.
	revokeInvite(client: ApiClient, projectId: number, inviteId: string): Promise<void> {
		return client.fetch(`/api/v1/projects/${projectId}/invites/${inviteId}/revoke`, {
			method: 'POST'
		});
	},

	// deleteInvite hard-removes an invite row (Federation v1 F1.3, US-1.3 AC3). It
	// does NOT remove peers that already consumed the invite.
	deleteInvite(client: ApiClient, projectId: number, inviteId: string): Promise<void> {
		return client.fetch(`/api/v1/projects/${projectId}/invites/${inviteId}`, {
			method: 'DELETE'
		});
	},

	// listPeers returns every remote instance joined to a federated project, each
	// with its handshake-supplied displayName and server-derived collaboration
	// status (Federation v1 F1.4, US-1.4 AC1/AC2/AC3). The owner self-row is
	// excluded server-side. pendingDelivery is 0 until the Phase-3 outbox lands
	// (US-1.4 AC4 partial).
	listPeers(client: ApiClient, projectId: number): Promise<Peer[]> {
		return client.fetch(`/api/v1/projects/${projectId}/federation/peers`);
	},

	// pausePeer temporarily pauses exchange with one peer of a project without
	// breaking the trust link (Federation v1 F5.3, US-6.1 AC1). The owner's outbox
	// stops fanning out to the peer (events accumulate) and the peer's inbound
	// traffic is rejected with 403 federation_paused. The peer URL rides in the body
	// so a peer URL with a scheme/slashes never needs URL-encoding into the path.
	pausePeer(client: ApiClient, projectId: number, instanceUrl: string): Promise<void> {
		return client.fetch(`/api/v1/projects/${projectId}/federation/peers/pause`, {
			method: 'POST',
			body: { instanceUrl }
		});
	},

	// resumePeer un-pauses a previously paused peer (Federation v1 F5.3, US-6.1
	// AC2): the owner's outbox resumes and flushes the events that accumulated while
	// the peer was paused.
	resumePeer(client: ApiClient, projectId: number, instanceUrl: string): Promise<void> {
		return client.fetch(`/api/v1/projects/${projectId}/federation/peers/resume`, {
			method: 'POST',
			body: { instanceUrl }
		});
	},

	// revokePeer PERMANENTLY revokes one peer's access to a project (Federation v1
	// F5.4, US-6.2). The owner flips revoked=1, sends the peer a federation_revoke
	// event (so it self-marks federation_lost / read-only), and halts: the peer is
	// dropped from fan-out and its inbound traffic is rejected 403. Revoke is
	// IRREVERSIBLE (re-collaboration needs a fresh invite) — the UI confirms first.
	// The peer URL rides in the body, so it never needs URL-encoding into the path.
	revokePeer(client: ApiClient, projectId: number, instanceUrl: string): Promise<void> {
		return client.fetch(`/api/v1/projects/${projectId}/federation/peers`, {
			method: 'DELETE',
			body: { instanceUrl }
		});
	},

	// trustKey manually re-trusts a peer's NEW key after a key-change incident
	// (Federation v1 F5.6b, US-6.4 AC3). When a peer rotated its key, this instance
	// rejected its events 401 and recorded a sticky incident (US-6.4 AC1/AC2 — NO
	// auto-refetch). An operator with out-of-band confidence the rotation is genuine
	// confirms, then POSTs here: the server fetches the peer's CURRENT .well-known
	// key, overwrites the pinned key, clears the incident, and audits. The peer URL
	// rides in the body, so it never needs URL-encoding into the path.
	trustKey(client: ApiClient, projectId: number, instanceUrl: string): Promise<void> {
		return client.fetch(`/api/v1/projects/${projectId}/federation/peers/trust-key`, {
			method: 'POST',
			body: { instanceUrl }
		});
	},

	// leaveProject VOLUNTARILY leaves a JOINED federated project (Federation v1
	// F5.5, US-6.3). The joiner sends the owner a federation_leave (so the owner
	// marks it "left" and stops fanning out, US-6.3 AC2) and marks its OWN local copy
	// federation_lost with reason="left" — a plain editable local project with no
	// further outbound sync (US-6.3 AC1/AC3). It is idempotent server-side; the
	// project id is in the path and there is no body. → 204.
	leaveProject(client: ApiClient, projectId: number): Promise<void> {
		return client.fetch(`/api/v1/projects/${projectId}/federation/leave`, {
			method: 'POST'
		});
	},

	// status returns the per-project federation sync status for every shared
	// project (Federation v1 F4.3, US-4.3): synced / pending / unreachable /
	// key_mismatch. It is a pure server read (there is no client outbox); the owner
	// UI renders a colour-coded badge on each federated project's header. Non-
	// federated projects are absent from the response.
	status(client: ApiClient): Promise<SyncStatus[]> {
		return client.fetch(`/api/v1/federation/status`);
	},

	// overview returns the privacy/federation overview (Federation v1 F6.4, US-7.1
	// AC1): every federated project with this instance's role (owner|peer|read-only)
	// and the named peer list (instanceUrl + displayName) it is visible to. Non-
	// federated projects are absent. A pure JWT-only owner server read backing the
	// "Privacy / Federation overview" table.
	overview(client: ApiClient): Promise<OverviewResponse> {
		return client.fetch(`/api/v1/federation/overview`);
	},

	// deadLetter returns the parked, permanently-failed outbound events for the
	// owner diagnostics view (Federation v1 F4.4, US-4.4 AC3): events a peer
	// rejected with a 4xx (≠429) the worker did not retry. Newest-first, metadata
	// only (no payload). A stable empty array when nothing failed.
	deadLetter(client: ApiClient): Promise<DeadLetterEntry[]> {
		return client.fetch(`/api/v1/federation/dead-letter`);
	},

	// audit returns the federation audit log (Federation v1 F6.3, US-7.4): the
	// security-relevant federation events the owner can browse to investigate
	// anomalies (handshake/revoke/signature failure/replay/key change), newest-first,
	// plus the "possible attack on peer X" alerts derived from recent signature-
	// failure bursts (US-7.4 AC3). An optional peer narrows the list; limit/offset
	// paginate. JWT-only owner-control-plane route (requires the settings:read scope
	// for API tokens; JWT sessions always pass).
	audit(
		client: ApiClient,
		opts: { peer?: string; kind?: string; limit?: number; offset?: number } = {}
	): Promise<AuditResponse> {
		const query: Record<string, string | number> = {};
		if (opts.peer) query.peer = opts.peer;
		if (opts.kind) query.kind = opts.kind;
		if (opts.limit != null) query.limit = opts.limit;
		if (opts.offset != null) query.offset = opts.offset;
		return client.fetch(`/api/v1/federation/audit`, { query });
	},

	// health returns the federation liveness report WITH per-peer detail (Federation
	// v1 F6.5, US-8.1): instanceUrl, protocolVersions, uptimeS, the live outboxDepth,
	// the rolled-up status (ok|degraded|peers_stale), and the peers array. This is the
	// JWT admin route — the public /federation/health probe omits the peer detail.
	health(client: ApiClient): Promise<FederationHealth> {
		return client.fetch(`/api/v1/federation/health`);
	},

	// getRetention returns the current retention overrides + the resolved effective
	// windows + the outbox hardcap (Federation v1 F6.5, US-8.4). A *Days override of
	// 0 means "use the default"; the effective* fields show what the GC will use.
	getRetention(client: ApiClient): Promise<RetentionSettings> {
		return client.fetch(`/api/v1/federation/retention`);
	},

	// updateRetention runtime-updates the retention windows (Federation v1 F6.5,
	// US-8.4). Omitted fields keep their value; a sent 0 reverts to default. The
	// change takes effect on the next GC pass WITHOUT a restart; the outbox window's
	// effective value stays clamped to the 30-day hardcap. Returns the resolved
	// settings so the UI re-renders the effective windows without a second round-trip.
	updateRetention(client: ApiClient, body: UpdateRetentionRequest): Promise<RetentionSettings> {
		return client.fetch(`/api/v1/federation/retention`, {
			method: 'PATCH',
			body
		});
	},

	// backupUrl is the federation-aware physical backup download path (Federation v1
	// F6.5, US-8.5): a VACUUM INTO copy of the whole DB including the federation
	// tables + keypair, served as a .db SQLite file. The UI triggers a browser
	// download via this URL rather than fetching into memory (the file is large).
	backupUrl(): string {
		return `/api/v1/federation/backup`;
	},

	// preview fetches a read-only summary of the project behind an invite
	// (Federation v1 F2.1, US-2.1 AC3). The invite id + secret are POSTed to OUR
	// OWN instance's JWT endpoint, which fetches the owner server-to-server — the
	// secret never travels browser→owner (no CORS leak; the secret stays
	// server-side). The endpoint itself lands with the handshake in F2.2; this
	// is the typed transport the join page already calls.
	preview(client: ApiClient, body: JoinInvite): Promise<JoinPreview> {
		return client.fetch(`/api/v1/federation/preview`, {
			method: 'POST',
			body
		});
	},

	// join accepts an invite and drives the owner handshake + snapshot bootstrap
	// (Federation v1 F2.1 stepper, US-2.1 AC4). It returns the locally-mapped
	// federated project once the bootstrap completes. The endpoint lands in F2.2;
	// this is the typed transport the join page's Accept action calls.
	join(client: ApiClient, body: JoinInvite): Promise<JoinResult> {
		return client.fetch(`/api/v1/federation/join`, {
			method: 'POST',
			body
		});
	}
};

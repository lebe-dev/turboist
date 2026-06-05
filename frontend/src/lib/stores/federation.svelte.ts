import { federation as federationApi } from '$lib/api/endpoints/federation';
import { getApiClient } from '$lib/api/client';
import { ApiError } from '$lib/api/errors';
import type {
	AuditEntry,
	DeadLetterEntry,
	FederationHealth,
	OverviewProject,
	RetentionSettings,
	SignatureFailureAlert,
	SyncStatus,
	UpdateRetentionRequest
} from '$lib/api/types';

// federationStore is the online-only store backing the per-project federation
// sync-status indicator (Federation v1 F4.3, US-4.3) and the dead-letter
// diagnostics view (Federation v1 F4.4, US-4.4 AC3). It holds the server-derived
// status of every shared project keyed by local project id, refreshed on load and
// on a ScopeFederation SSE transition. There is NO client outbox — the status is
// purely a server read (R: status is server-read), so the store never mutates it
// locally.
class FederationStore {
	private statuses = $state<SyncStatus[]>([]);
	private deadLetterEntries = $state<DeadLetterEntry[]>([]);
	private auditEntries = $state<AuditEntry[]>([]);
	private auditAlertList = $state<SignatureFailureAlert[]>([]);
	private overviewProjects = $state<OverviewProject[]>([]);
	private healthReport = $state<FederationHealth | null>(null);
	private retentionSettings = $state<RetentionSettings | null>(null);

	// load fetches the per-project sync status for every shared project (US-4.3).
	// It REPLACES the held statuses so a transition (e.g. a peer's key mismatch
	// going red) is reflected on the next reload, and a project that left
	// federation drops out. A federation-disabled instance surfaces an error the
	// caller swallows; the badges then simply do not render.
	async load(): Promise<void> {
		this.statuses = await federationApi.status(getApiClient());
	}

	// forProject returns the sync status of one project, or undefined when the
	// project is not federated (the badge is hidden then).
	forProject(projectId: number): SyncStatus | undefined {
		return this.statuses.find((s) => s.projectId === projectId);
	}

	// deadLetter exposes the parked, permanently-failed outbound events for the
	// diagnostics view (Federation v1 F4.4, US-4.4 AC3).
	get deadLetter(): DeadLetterEntry[] {
		return this.deadLetterEntries;
	}

	// loadDeadLetter fetches the parked dead-letter events (newest-first). It
	// REPLACES the held list. A federation-disabled instance surfaces an error the
	// caller swallows; the view then shows nothing.
	async loadDeadLetter(): Promise<void> {
		this.deadLetterEntries = await federationApi.deadLetter(getApiClient());
	}

	// audit exposes the federation audit-log rows for the owner audit view
	// (Federation v1 F6.3, US-7.4 AC1).
	get audit(): AuditEntry[] {
		return this.auditEntries;
	}

	// auditAlerts exposes the "possible attack on peer X" alerts derived from the
	// recent signature-failure counts (Federation v1 F6.3, US-7.4 AC3).
	get auditAlerts(): SignatureFailureAlert[] {
		return this.auditAlertList;
	}

	// loadAudit fetches the audit log (newest-first) + the signature-failure alerts,
	// REPLACING both held lists. An optional peer narrows the rows. A federation-
	// disabled instance surfaces an error the caller swallows; the view shows nothing.
	async loadAudit(opts: { peer?: string; limit?: number } = {}): Promise<void> {
		const res = await federationApi.audit(getApiClient(), opts);
		this.auditEntries = res.entries;
		this.auditAlertList = res.alerts;
	}

	// overview exposes the privacy/federation overview rows (Federation v1 F6.4,
	// US-7.1 AC1): every federated project with this instance's role + named peer
	// list.
	get overview(): OverviewProject[] {
		return this.overviewProjects;
	}

	// loadOverview fetches the federation overview (every federated project, its
	// role + named peer list), REPLACING the held list. A federation-disabled
	// instance surfaces an error the caller swallows; the view then shows nothing.
	async loadOverview(): Promise<void> {
		const res = await federationApi.overview(getApiClient());
		this.overviewProjects = res.projects;
	}

	// health exposes the federation liveness report (Federation v1 F6.5, US-8.1):
	// instanceUrl, protocolVersions, uptimeS, outboxDepth, status, and the peers
	// detail. null until loaded (the panel renders nothing then).
	get health(): FederationHealth | null {
		return this.healthReport;
	}

	// loadHealth fetches the federation liveness report (admin view, with peers),
	// REPLACING the held report. A federation-disabled instance surfaces an error
	// the caller swallows; the panel then shows nothing.
	async loadHealth(): Promise<void> {
		this.healthReport = await federationApi.health(getApiClient());
	}

	// retention exposes the current retention settings (Federation v1 F6.5, US-8.4):
	// the overrides + the resolved effective windows + the outbox hardcap. null until
	// loaded.
	get retention(): RetentionSettings | null {
		return this.retentionSettings;
	}

	// loadRetention fetches the current retention settings, REPLACING the held value.
	async loadRetention(): Promise<void> {
		this.retentionSettings = await federationApi.getRetention(getApiClient());
	}

	// updateRetention runtime-updates the retention windows (US-8.4) and stores the
	// resolved settings the server returns so the UI re-renders the effective windows
	// immediately. The change takes effect on the next GC pass without a restart.
	async updateRetention(body: UpdateRetentionRequest): Promise<void> {
		this.retentionSettings = await federationApi.updateRetention(getApiClient(), body);
	}

	// clear empties the store (logout / teardown).
	clear(): void {
		this.statuses = [];
		this.deadLetterEntries = [];
		this.auditEntries = [];
		this.auditAlertList = [];
		this.overviewProjects = [];
		this.healthReport = null;
		this.retentionSettings = null;
	}
}

export const federationStore = new FederationStore();

// swallowFederationLoadError is the narrowed catch handler for the best-effort
// federationStore.load() calls in the app shell. A federation-OFF (or not-yet-
// keyed) instance answers the status read with the EXPECTED `federation_key_missing`
// ApiError — there is simply no status to show, so that one is swallowed silently
// (the badges just do not render). ANY OTHER failure (a real backend/network fault)
// is surfaced via console.warn so a broken status surface is diagnosable instead of
// vanishing into an empty catch.
export function swallowFederationLoadError(err: unknown): void {
	if (err instanceof ApiError && err.code === 'federation_key_missing') {
		return;
	}
	console.warn('federation: status load failed', err);
}

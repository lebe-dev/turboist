import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';

// The store calls getApiClient() before hitting the (spied) endpoint; stub it so
// the test does not need a fully initialised ApiClient singleton.
vi.mock('$lib/api/client', () => ({
	getApiClient: () => ({}) as never
}));

import { federationStore } from './federation.svelte';
import { federation as federationApi } from '$lib/api/endpoints/federation';
import type { DeadLetterEntry, OverviewProject, SyncStatus } from '$lib/api/types';

describe('federationStore (Federation v1 F4.3, US-4.3)', () => {
	beforeEach(() => {
		federationStore.clear();
	});
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it('load() fetches statuses and exposes them keyed by project id', async () => {
		const rows: SyncStatus[] = [
			{ projectId: 7, status: 'synced', pendingCount: 0, unreachablePeer: '', keyMismatchPeer: '' },
			{
				projectId: 9,
				status: 'pending',
				pendingCount: 3,
				unreachablePeer: '',
				keyMismatchPeer: ''
			}
		];
		vi.spyOn(federationApi, 'status').mockResolvedValueOnce(rows);

		await federationStore.load();

		expect(federationStore.forProject(7)?.status).toBe('synced');
		expect(federationStore.forProject(9)?.status).toBe('pending');
		expect(federationStore.forProject(9)?.pendingCount).toBe(3);
	});

	it('forProject returns undefined for a non-federated project', async () => {
		vi.spyOn(federationApi, 'status').mockResolvedValueOnce([]);
		await federationStore.load();
		expect(federationStore.forProject(123)).toBeUndefined();
	});

	it('a later load replaces stale statuses (transition reflected)', async () => {
		vi.spyOn(federationApi, 'status')
			.mockResolvedValueOnce([
				{ projectId: 7, status: 'synced', pendingCount: 0, unreachablePeer: '', keyMismatchPeer: '' }
			])
			.mockResolvedValueOnce([
				{
					projectId: 7,
					status: 'key_mismatch',
					pendingCount: 0,
					unreachablePeer: '',
					keyMismatchPeer: 'https://bob.example'
				}
			]);

		await federationStore.load();
		expect(federationStore.forProject(7)?.status).toBe('synced');
		await federationStore.load();
		expect(federationStore.forProject(7)?.status).toBe('key_mismatch');
	});

	it('clear empties the store', async () => {
		vi.spyOn(federationApi, 'status').mockResolvedValueOnce([
			{ projectId: 7, status: 'synced', pendingCount: 0, unreachablePeer: '', keyMismatchPeer: '' }
		]);
		await federationStore.load();
		expect(federationStore.forProject(7)).toBeDefined();
		federationStore.clear();
		expect(federationStore.forProject(7)).toBeUndefined();
	});

	it('loadDeadLetter() fetches the parked dead-letter entries (Federation v1 F4.4, US-4.4 AC3)', async () => {
		const rows: DeadLetterEntry[] = [
			{
				eventId: 'e2',
				peerInstanceUrl: 'https://bob.example',
				projectId: 9,
				statusCode: 403,
				reason: 'federation_read_only',
				failedAt: '2026-06-03T10:00:05.000Z'
			}
		];
		vi.spyOn(federationApi, 'deadLetter').mockResolvedValueOnce(rows);

		await federationStore.loadDeadLetter();

		expect(federationStore.deadLetter).toHaveLength(1);
		expect(federationStore.deadLetter[0].eventId).toBe('e2');
		expect(federationStore.deadLetter[0].reason).toBe('federation_read_only');
	});

	it('clear empties the dead-letter list too', async () => {
		vi.spyOn(federationApi, 'deadLetter').mockResolvedValueOnce([
			{
				eventId: 'e1',
				peerInstanceUrl: 'https://bob.example',
				projectId: 9,
				statusCode: 400,
				reason: 'federation_author_mismatch',
				failedAt: '2026-06-03T10:00:00.000Z'
			}
		]);
		await federationStore.loadDeadLetter();
		expect(federationStore.deadLetter).toHaveLength(1);
		federationStore.clear();
		expect(federationStore.deadLetter).toHaveLength(0);
	});

	it('loadOverview() fetches the overview rows with role + named peers (Federation v1 F6.4, US-7.1 AC1)', async () => {
		const rows: OverviewProject[] = [
			{
				projectId: 5,
				title: 'Shared',
				role: 'owner',
				peers: [{ instanceUrl: 'https://alice.example', displayName: 'Alice' }]
			}
		];
		vi.spyOn(federationApi, 'overview').mockResolvedValueOnce({ projects: rows });

		await federationStore.loadOverview();

		expect(federationStore.overview).toHaveLength(1);
		expect(federationStore.overview[0].role).toBe('owner');
		expect(federationStore.overview[0].peers[0].displayName).toBe('Alice');
	});

	it('clear empties the overview list too', async () => {
		vi.spyOn(federationApi, 'overview').mockResolvedValueOnce({
			projects: [{ projectId: 5, title: 'Shared', role: 'owner', peers: [] }]
		});
		await federationStore.loadOverview();
		expect(federationStore.overview).toHaveLength(1);
		federationStore.clear();
		expect(federationStore.overview).toHaveLength(0);
	});

	it('loadHealth() fetches the liveness report (Federation v1 F6.5, US-8.1)', async () => {
		vi.spyOn(federationApi, 'health').mockResolvedValueOnce({
			instanceUrl: 'https://me.example',
			protocolVersions: [1],
			uptimeS: 5,
			outboxDepth: 3,
			status: 'degraded',
			peers: []
		});
		await federationStore.loadHealth();
		expect(federationStore.health?.status).toBe('degraded');
		expect(federationStore.health?.outboxDepth).toBe(3);
	});

	it('updateRetention() stores the resolved settings (US-8.4) and clear resets them', async () => {
		vi.spyOn(federationApi, 'updateRetention').mockResolvedValueOnce({
			tombstoneDays: 120,
			outboxDays: 365,
			inboxDays: 0,
			outboxHardcapDays: 30,
			effectiveTombstoneDays: 120,
			effectiveOutboxDays: 30,
			effectiveInboxDays: 30
		});
		await federationStore.updateRetention({ tombstoneDays: 120, outboxDays: 365 });
		expect(federationStore.retention?.effectiveOutboxDays).toBe(30); // clamped
		expect(federationStore.retention?.tombstoneDays).toBe(120);
		federationStore.clear();
		expect(federationStore.retention).toBeNull();
		expect(federationStore.health).toBeNull();
	});
});

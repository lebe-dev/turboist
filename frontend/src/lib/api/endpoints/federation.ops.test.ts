import { describe, expect, it, vi, afterEach } from 'vitest';
import { ApiClient } from '../client';
import { federation as federationApi } from './federation';
import type { FederationHealth, RetentionSettings } from '../types';

function jsonResponse(body: unknown, status = 200): Response {
	return new Response(JSON.stringify(body), {
		status,
		headers: { 'Content-Type': 'application/json' }
	});
}

function makeClient() {
	const fetchMock = vi.fn<typeof fetch>();
	const client = new ApiClient({
		fetchImpl: fetchMock as unknown as typeof fetch,
		getAccessToken: () => 'tok',
		setAccessToken: () => {},
		onRefreshFailure: () => {}
	});
	return { client, fetchMock };
}

describe('federation ops endpoints (Federation v1 F6.5)', () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it('health GETs /api/v1/federation/health (US-8.1)', async () => {
		const { client, fetchMock } = makeClient();
		const res: FederationHealth = {
			instanceUrl: 'https://me.example',
			protocolVersions: [1],
			uptimeS: 10,
			outboxDepth: 2,
			status: 'degraded',
			peers: [{ instanceUrl: 'https://bob.example', displayName: 'Bob', status: 'active' }]
		};
		fetchMock.mockResolvedValueOnce(jsonResponse(res));

		const result = await federationApi.health(client);
		const [url] = fetchMock.mock.calls[0];
		expect(String(url)).toContain('/api/v1/federation/health');
		expect(result.status).toBe('degraded');
		expect(result.outboxDepth).toBe(2);
	});

	it('getRetention GETs /api/v1/federation/retention (US-8.4)', async () => {
		const { client, fetchMock } = makeClient();
		const res: RetentionSettings = {
			tombstoneDays: 0,
			outboxDays: 0,
			inboxDays: 0,
			outboxHardcapDays: 30,
			effectiveTombstoneDays: 90,
			effectiveOutboxDays: 30,
			effectiveInboxDays: 30
		};
		fetchMock.mockResolvedValueOnce(jsonResponse(res));

		const result = await federationApi.getRetention(client);
		const [url] = fetchMock.mock.calls[0];
		expect(String(url)).toContain('/api/v1/federation/retention');
		expect(result.outboxHardcapDays).toBe(30);
	});

	it('updateRetention PATCHes /api/v1/federation/retention with the body (US-8.4)', async () => {
		const { client, fetchMock } = makeClient();
		const res: RetentionSettings = {
			tombstoneDays: 120,
			outboxDays: 365,
			inboxDays: 0,
			outboxHardcapDays: 30,
			effectiveTombstoneDays: 120,
			effectiveOutboxDays: 30,
			effectiveInboxDays: 30
		};
		fetchMock.mockResolvedValueOnce(jsonResponse(res));

		const result = await federationApi.updateRetention(client, { tombstoneDays: 120, outboxDays: 365 });
		const [url, init] = fetchMock.mock.calls[0];
		expect(String(url)).toContain('/api/v1/federation/retention');
		expect((init as RequestInit).method).toMatch(/PATCH/i);
		// Effective outbox stays clamped at the hardcap even though the stored value is 365.
		expect(result.effectiveOutboxDays).toBe(30);
		expect(result.outboxDays).toBe(365);
	});

	it('backupUrl returns the federation backup download path (US-8.5)', () => {
		expect(federationApi.backupUrl()).toBe('/api/v1/federation/backup');
	});
});

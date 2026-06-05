import { describe, expect, it, vi, afterEach } from 'vitest';
import { ApiClient } from '../client';
import { federation as federationApi } from './federation';
import type { AuditResponse } from '../types';

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

describe('federation audit (Federation v1 F6.3, US-7.4)', () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it('audit GETs /api/v1/federation/audit and returns entries + alerts', async () => {
		const { client, fetchMock } = makeClient();
		const res: AuditResponse = {
			entries: [
				{
					id: 2,
					kind: 'signature_invalid',
					outcome: 'rejected',
					peerInstanceUrl: 'https://bob.example',
					detail: 'transport signature invalid',
					createdAt: '2026-06-03T10:00:05.000Z'
				}
			],
			alerts: [{ peerInstanceUrl: 'https://bob.example', count: 11, threshold: 10 }]
		};
		fetchMock.mockResolvedValueOnce(jsonResponse(res));

		const result = await federationApi.audit(client);

		expect(fetchMock).toHaveBeenCalledTimes(1);
		const [url, init] = fetchMock.mock.calls[0];
		expect(String(url)).toContain('/api/v1/federation/audit');
		expect((init as RequestInit | undefined)?.method ?? 'GET').toMatch(/GET/i);
		expect(result.entries).toHaveLength(1);
		expect(result.entries[0].kind).toBe('signature_invalid');
		expect(result.alerts).toHaveLength(1);
		expect(result.alerts[0].count).toBe(11);
	});

	it('audit forwards the peer filter + pagination as query params', async () => {
		const { client, fetchMock } = makeClient();
		fetchMock.mockResolvedValueOnce(jsonResponse({ entries: [], alerts: [] }));

		await federationApi.audit(client, { peer: 'https://alice.example', limit: 25, offset: 50 });

		const [url] = fetchMock.mock.calls[0];
		const s = String(url);
		expect(s).toContain('peer=https');
		expect(s).toContain('limit=25');
		expect(s).toContain('offset=50');
	});
});

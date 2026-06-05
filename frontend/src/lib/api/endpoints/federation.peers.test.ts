import { describe, expect, it, vi, afterEach } from 'vitest';
import { ApiClient } from '../client';
import { federation as federationApi } from './federation';
import type { Peer } from '../types';

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

describe('federation peers list (Federation v1 F1.4)', () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it('listPeers GETs the per-project peers route and returns rows with status + displayName (US-1.4 AC1, AC2, AC3)', async () => {
		const { client, fetchMock } = makeClient();
		const rows: Peer[] = [
			{
				instanceUrl: 'https://bob.example',
				displayName: "Bob's Box",
				permissions: 'write',
				status: 'active',
				lastSentHlc: '0000000000000-00000-node',
				lastContactAt: '2026-06-01T00:00:00.000Z',
				joinedAt: '2026-01-01T00:00:00.000Z',
				pendingDelivery: 0,
				keyMismatchAt: ''
			},
			{
				instanceUrl: 'https://carol.example',
				displayName: 'Carol',
				permissions: 'read',
				status: 'stale',
				lastSentHlc: '',
				lastContactAt: '',
				joinedAt: '2026-01-02T00:00:00.000Z',
				pendingDelivery: 0,
				keyMismatchAt: ''
			}
		];
		fetchMock.mockResolvedValueOnce(jsonResponse(rows));

		const result = await federationApi.listPeers(client, 7);

		expect(fetchMock).toHaveBeenCalledTimes(1);
		const [url, init] = fetchMock.mock.calls[0];
		expect(String(url)).toContain('/api/v1/projects/7/federation/peers');
		expect((init as RequestInit | undefined)?.method ?? 'GET').toMatch(/GET/i);
		expect(result).toHaveLength(2);
		expect(result[0].displayName).toBe("Bob's Box");
		expect(result[0].status).toBe('active');
		// US-1.4 AC4 partial: pendingDelivery is present and 0 until the outbox lands.
		expect(result[0].pendingDelivery).toBe(0);
		expect(result[1].status).toBe('stale');
	});

	it('pausePeer POSTs the peer URL in the body to the pause route (US-6.1 AC1)', async () => {
		const { client, fetchMock } = makeClient();
		fetchMock.mockResolvedValueOnce(new Response(null, { status: 204 }));

		await federationApi.pausePeer(client, 7, 'https://bob.example');

		expect(fetchMock).toHaveBeenCalledTimes(1);
		const [url, init] = fetchMock.mock.calls[0];
		expect(String(url)).toContain('/api/v1/projects/7/federation/peers/pause');
		expect((init as RequestInit | undefined)?.method).toMatch(/POST/i);
		// The peer URL rides in the body, never the path (no URL-encoding needed).
		expect(JSON.parse(String((init as RequestInit).body))).toEqual({
			instanceUrl: 'https://bob.example'
		});
	});

	it('resumePeer POSTs the peer URL in the body to the resume route (US-6.1 AC2)', async () => {
		const { client, fetchMock } = makeClient();
		fetchMock.mockResolvedValueOnce(new Response(null, { status: 204 }));

		await federationApi.resumePeer(client, 7, 'https://bob.example');

		expect(fetchMock).toHaveBeenCalledTimes(1);
		const [url, init] = fetchMock.mock.calls[0];
		expect(String(url)).toContain('/api/v1/projects/7/federation/peers/resume');
		expect((init as RequestInit | undefined)?.method).toMatch(/POST/i);
		expect(JSON.parse(String((init as RequestInit).body))).toEqual({
			instanceUrl: 'https://bob.example'
		});
	});

	it('revokePeer DELETEs the peers route with the peer URL in the body (Federation v1 F5.4, US-6.2 AC1)', async () => {
		const { client, fetchMock } = makeClient();
		fetchMock.mockResolvedValueOnce(new Response(null, { status: 204 }));

		await federationApi.revokePeer(client, 7, 'https://bob.example');

		expect(fetchMock).toHaveBeenCalledTimes(1);
		const [url, init] = fetchMock.mock.calls[0];
		expect(String(url)).toContain('/api/v1/projects/7/federation/peers');
		expect((init as RequestInit | undefined)?.method).toMatch(/DELETE/i);
		// The peer URL rides in the body, never the path (no URL-encoding needed).
		expect(JSON.parse(String((init as RequestInit).body))).toEqual({
			instanceUrl: 'https://bob.example'
		});
	});

	it('trustKey POSTs the peer URL in the body to the trust-key route (Federation v1 F5.6b, US-6.4 AC3)', async () => {
		const { client, fetchMock } = makeClient();
		fetchMock.mockResolvedValueOnce(new Response(null, { status: 204 }));

		await federationApi.trustKey(client, 7, 'https://bob.example');

		expect(fetchMock).toHaveBeenCalledTimes(1);
		const [url, init] = fetchMock.mock.calls[0];
		expect(String(url)).toContain('/api/v1/projects/7/federation/peers/trust-key');
		expect((init as RequestInit | undefined)?.method).toMatch(/POST/i);
		// The peer URL rides in the body, never the path (no URL-encoding needed).
		expect(JSON.parse(String((init as RequestInit).body))).toEqual({
			instanceUrl: 'https://bob.example'
		});
	});

	it('leaveProject POSTs the per-project leave route with no body (Federation v1 F5.5, US-6.3 AC1)', async () => {
		const { client, fetchMock } = makeClient();
		fetchMock.mockResolvedValueOnce(new Response(null, { status: 204 }));

		await federationApi.leaveProject(client, 7);

		expect(fetchMock).toHaveBeenCalledTimes(1);
		const [url, init] = fetchMock.mock.calls[0];
		expect(String(url)).toContain('/api/v1/projects/7/federation/leave');
		expect((init as RequestInit | undefined)?.method).toMatch(/POST/i);
	});
});

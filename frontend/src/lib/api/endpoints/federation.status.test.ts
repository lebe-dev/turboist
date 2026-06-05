import { describe, expect, it, vi, afterEach } from 'vitest';
import { ApiClient } from '../client';
import { federation as federationApi } from './federation';
import type { SyncStatus } from '../types';

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

describe('federation sync status (Federation v1 F4.3, US-4.3)', () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it('status GETs /api/v1/federation/status and returns per-project rows', async () => {
		const { client, fetchMock } = makeClient();
		const rows: SyncStatus[] = [
			{ projectId: 7, status: 'synced', pendingCount: 0, unreachablePeer: '', keyMismatchPeer: '' },
			{
				projectId: 9,
				status: 'key_mismatch',
				pendingCount: 0,
				unreachablePeer: '',
				keyMismatchPeer: 'https://bob.example'
			}
		];
		fetchMock.mockResolvedValueOnce(jsonResponse(rows));

		const result = await federationApi.status(client);

		expect(fetchMock).toHaveBeenCalledTimes(1);
		const [url, init] = fetchMock.mock.calls[0];
		expect(String(url)).toContain('/api/v1/federation/status');
		expect((init as RequestInit | undefined)?.method ?? 'GET').toMatch(/GET/i);
		expect(result).toHaveLength(2);
		expect(result[0].status).toBe('synced');
		expect(result[1].status).toBe('key_mismatch');
		expect(result[1].keyMismatchPeer).toBe('https://bob.example');
	});
});

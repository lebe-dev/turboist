import { describe, expect, it, vi, afterEach } from 'vitest';
import { ApiClient } from '../client';
import { federation as federationApi } from './federation';
import type { OverviewResponse } from '../types';

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

describe('federation overview (Federation v1 F6.4, US-7.1 AC1)', () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it('overview GETs /api/v1/federation/overview and returns role + named peers', async () => {
		const { client, fetchMock } = makeClient();
		const res: OverviewResponse = {
			projects: [
				{
					projectId: 5,
					title: 'Shared',
					role: 'owner',
					peers: [
						{ instanceUrl: 'https://alice.example', displayName: 'Alice' },
						{ instanceUrl: 'https://bob.example', displayName: 'Bob' }
					]
				}
			]
		};
		fetchMock.mockResolvedValueOnce(jsonResponse(res));

		const result = await federationApi.overview(client);

		expect(fetchMock).toHaveBeenCalledTimes(1);
		const [url, init] = fetchMock.mock.calls[0];
		expect(String(url)).toContain('/api/v1/federation/overview');
		expect((init as RequestInit | undefined)?.method ?? 'GET').toMatch(/GET/i);
		expect(result.projects).toHaveLength(1);
		expect(result.projects[0].role).toBe('owner');
		// US-7.1 AC3: the named peer list, not a bare count.
		expect(result.projects[0].peers.map((p) => p.displayName)).toEqual(['Alice', 'Bob']);
	});
});

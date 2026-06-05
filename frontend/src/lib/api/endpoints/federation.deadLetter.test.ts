import { describe, expect, it, vi, afterEach } from 'vitest';
import { ApiClient } from '../client';
import { federation as federationApi } from './federation';
import type { DeadLetterEntry } from '../types';

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

describe('federation dead-letter (Federation v1 F4.4, US-4.4 AC3)', () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it('deadLetter GETs /api/v1/federation/dead-letter and returns parked entries', async () => {
		const { client, fetchMock } = makeClient();
		const rows: DeadLetterEntry[] = [
			{
				eventId: 'e2',
				peerInstanceUrl: 'https://bob.example',
				projectId: 9,
				statusCode: 403,
				reason: 'federation_read_only',
				failedAt: '2026-06-03T10:00:05.000Z'
			},
			{
				eventId: 'e1',
				peerInstanceUrl: 'https://bob.example',
				projectId: 9,
				statusCode: 400,
				reason: 'federation_author_mismatch',
				failedAt: '2026-06-03T10:00:00.000Z'
			}
		];
		fetchMock.mockResolvedValueOnce(jsonResponse(rows));

		const result = await federationApi.deadLetter(client);

		expect(fetchMock).toHaveBeenCalledTimes(1);
		const [url, init] = fetchMock.mock.calls[0];
		expect(String(url)).toContain('/api/v1/federation/dead-letter');
		expect((init as RequestInit | undefined)?.method ?? 'GET').toMatch(/GET/i);
		expect(result).toHaveLength(2);
		expect(result[0].eventId).toBe('e2');
		expect(result[0].statusCode).toBe(403);
		expect(result[0].reason).toBe('federation_read_only');
	});

	it('deadLetter returns an empty array when nothing is parked', async () => {
		const { client, fetchMock } = makeClient();
		fetchMock.mockResolvedValueOnce(jsonResponse([]));

		const result = await federationApi.deadLetter(client);
		expect(result).toEqual([]);
	});
});

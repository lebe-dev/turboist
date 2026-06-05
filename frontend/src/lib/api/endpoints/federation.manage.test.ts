import { describe, expect, it, vi, afterEach } from 'vitest';
import { ApiClient } from '../client';
import { federation as federationApi } from './federation';
import type { Invite } from '../types';

function jsonResponse(body: unknown, status = 200): Response {
	return new Response(JSON.stringify(body), {
		status,
		headers: { 'Content-Type': 'application/json' }
	});
}

function noContent(): Response {
	return new Response(null, { status: 204 });
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

describe('federation invite management (Federation v1 F1.3)', () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it('listInvites GETs the per-project invites route and returns rows with status, no secret (US-1.3 AC1, AC5)', async () => {
		const { client, fetchMock } = makeClient();
		const rows: Invite[] = [
			{
				inviteId: '01J0000000000000000000000A',
				permissions: 'write',
				maxUses: 1,
				usedCount: 0,
				status: 'active',
				expiresAt: '2030-01-01T00:00:00.000Z',
				revokedAt: '',
				consumedAt: '',
				createdAt: '2026-01-01T00:00:00.000Z'
			}
		];
		fetchMock.mockResolvedValueOnce(jsonResponse(rows));

		const result = await federationApi.listInvites(client, 7);

		expect(fetchMock).toHaveBeenCalledTimes(1);
		const [url, init] = fetchMock.mock.calls[0];
		expect(String(url)).toContain('/api/v1/projects/7/invites');
		expect((init as RequestInit).method ?? 'GET').toMatch(/GET/i);
		expect(result).toHaveLength(1);
		expect(result[0].status).toBe('active');
		// US-1.3 AC5: the list shape carries no secret field.
		expect('secret' in result[0]).toBe(false);
	});

	it('revokeInvite POSTs to the per-invite revoke route (US-1.3 AC2)', async () => {
		const { client, fetchMock } = makeClient();
		fetchMock.mockResolvedValueOnce(noContent());

		await federationApi.revokeInvite(client, 7, 'inv-1');

		expect(fetchMock).toHaveBeenCalledTimes(1);
		const [url, init] = fetchMock.mock.calls[0];
		expect(String(url)).toContain('/api/v1/projects/7/invites/inv-1/revoke');
		expect((init as RequestInit).method).toBe('POST');
	});

	it('deleteInvite DELETEs the per-invite route (US-1.3 AC3)', async () => {
		const { client, fetchMock } = makeClient();
		fetchMock.mockResolvedValueOnce(noContent());

		await federationApi.deleteInvite(client, 7, 'inv-1');

		expect(fetchMock).toHaveBeenCalledTimes(1);
		const [url, init] = fetchMock.mock.calls[0];
		expect(String(url)).toContain('/api/v1/projects/7/invites/inv-1');
		expect(String(url)).not.toContain('/revoke');
		expect((init as RequestInit).method).toBe('DELETE');
	});
});

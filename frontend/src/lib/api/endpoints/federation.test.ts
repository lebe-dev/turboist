import { describe, expect, it, vi, afterEach } from 'vitest';
import { ApiClient } from '../client';
import { federation as federationApi } from './federation';
import type { CreateInviteResponse, JoinPreview, JoinResult } from '../types';

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

describe('federation.createInvite (Federation v1 F1.2)', () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it('POSTs to the per-project invites route and returns the one-time link (US-1.2 AC1)', async () => {
		const { client, fetchMock } = makeClient();
		const response: CreateInviteResponse = {
			inviteId: '01J0000000000000000000000A',
			secret: 'super-secret-256-bit',
			link: 'https://my-instance.tld/federation/join#invite=01J0000000000000000000000A.super-secret-256-bit',
			permissions: 'write',
			maxUses: 1,
			expiresAt: '2030-01-01T00:00:00.000Z'
		};
		fetchMock.mockResolvedValueOnce(jsonResponse(response));

		const result = await federationApi.createInvite(client, 7, { permissions: 'write' });

		expect(fetchMock).toHaveBeenCalledTimes(1);
		const [url, init] = fetchMock.mock.calls[0];
		expect(String(url)).toContain('/api/v1/projects/7/invites');
		expect((init as RequestInit).method).toBe('POST');
		expect(JSON.parse(String((init as RequestInit).body))).toEqual({ permissions: 'write' });

		// US-1.2 AC1 + AC6: the secret rides in the URL fragment, never the query.
		expect(result.link).toContain('/federation/join#invite=');
		expect(result.link).not.toContain('?');
		expect(result.secret).toBe('super-secret-256-bit');
	});

	it('forwards custom maxUses and expiresAt in the body', async () => {
		const { client, fetchMock } = makeClient();
		fetchMock.mockResolvedValueOnce(
			jsonResponse({
				inviteId: 'x',
				secret: 's',
				link: 'https://t/federation/join#invite=x.s',
				permissions: 'read',
				maxUses: 5,
				expiresAt: '2030-01-01T00:00:00.000Z'
			})
		);

		await federationApi.createInvite(client, 9, {
			permissions: 'read',
			maxUses: 5,
			expiresAt: '2030-01-01T00:00:00.000Z'
		});

		const [, init] = fetchMock.mock.calls[0];
		expect(JSON.parse(String((init as RequestInit).body))).toEqual({
			permissions: 'read',
			maxUses: 5,
			expiresAt: '2030-01-01T00:00:00.000Z'
		});
	});
});

describe('federation.preview (Federation v1 F2.1, US-2.1 AC3)', () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it('POSTs the invite id + secret server-side (never browser→owner) and returns the preview', async () => {
		const { client, fetchMock } = makeClient();
		const preview: JoinPreview = {
			projectName: 'Roadmap',
			ownerInstanceUrl: 'https://alice.example',
			ownerDisplayName: 'alice.example',
			permissions: 'write',
			protocolVersion: 1
		};
		fetchMock.mockResolvedValueOnce(jsonResponse(preview));

		const result = await federationApi.preview(client, {
			inviteId: 'theid',
			secret: 'thesecret',
			ownerInstanceUrl: 'https://alice.example'
		});

		expect(fetchMock).toHaveBeenCalledTimes(1);
		const [url, init] = fetchMock.mock.calls[0];
		// Server-side preview keeps the secret out of the browser→owner CORS path:
		// it goes to OUR instance's JWT endpoint, which fetches the owner. The owner
		// instance URL travels in the body so our instance knows where to handshake.
		expect(String(url)).toContain('/api/v1/federation/preview');
		expect((init as RequestInit).method).toBe('POST');
		expect(JSON.parse(String((init as RequestInit).body))).toEqual({
			inviteId: 'theid',
			secret: 'thesecret',
			ownerInstanceUrl: 'https://alice.example'
		});
		expect(result.projectName).toBe('Roadmap');
		expect(result.ownerInstanceUrl).toBe('https://alice.example');
	});
});

describe('federation.join (Federation v1 F2.1, US-2.1 AC4)', () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it('POSTs the invite to the JWT join endpoint and returns the joined project', async () => {
		const { client, fetchMock } = makeClient();
		const joined: JoinResult = {
			projectId: 42,
			projectName: 'Roadmap',
			permissions: 'write'
		};
		fetchMock.mockResolvedValueOnce(jsonResponse(joined));

		const result = await federationApi.join(client, {
			inviteId: 'theid',
			secret: 'thesecret',
			ownerInstanceUrl: 'https://alice.example'
		});

		expect(fetchMock).toHaveBeenCalledTimes(1);
		const [url, init] = fetchMock.mock.calls[0];
		expect(String(url)).toContain('/api/v1/federation/join');
		expect((init as RequestInit).method).toBe('POST');
		expect(JSON.parse(String((init as RequestInit).body))).toEqual({
			inviteId: 'theid',
			secret: 'thesecret',
			ownerInstanceUrl: 'https://alice.example'
		});
		expect(result.projectId).toBe(42);
	});
});

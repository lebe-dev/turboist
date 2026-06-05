import { describe, expect, it, vi, afterEach } from 'vitest';
import { ApiClient } from '../client';
import { projects as projectsApi } from './projects';
import type { Project } from '../types';

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

function fedProject(over: Partial<Project> = {}): Project {
	return {
		id: 7,
		contextId: 1,
		title: 'Shared',
		description: '',
		color: '#fff',
		status: 'open',
		projectType: 'generic',
		isPinned: false,
		pinnedAt: null,
		isPrivate: false,
		isFederated: true,
		originInstance: null,
		federationPermissions: null,
		isOwner: false,
		reBootstrappedAt: null,
		federationLost: false,
		federationLostReason: null,
		ownerOffline: false,
		peerInstances: [],
		labels: [],
		troikiCategory: null,
		clientId: '',
		deletedAt: null,
		createdAt: '',
		updatedAt: '',
		...over
	};
}

describe('projects.enableFederation (Federation v1 F1.1)', () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it('POSTs to the per-project enable route and returns the updated project (US-1.1)', async () => {
		const { client, fetchMock } = makeClient();
		fetchMock.mockResolvedValueOnce(jsonResponse(fedProject({ id: 7, isFederated: true })));

		const result = await projectsApi.enableFederation(client, 7);

		expect(fetchMock).toHaveBeenCalledTimes(1);
		const [url, init] = fetchMock.mock.calls[0];
		expect(String(url)).toContain('/api/v1/projects/7/federation/enable');
		expect((init as RequestInit).method).toBe('POST');
		expect(result.isFederated).toBe(true);
		expect(result.id).toBe(7);
	});
});

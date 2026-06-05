import { render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// The component drives federationStore.loadOverview(), which calls getApiClient()
// then the (spied) federation.overview endpoint. Stub the client so no real
// ApiClient singleton is needed.
vi.mock('$lib/api/client', () => ({
	getApiClient: () => ({}) as never
}));

import OverviewSection from './OverviewSection.svelte';
import { federation as federationApi } from '$lib/api/endpoints/federation';
import { federationStore } from '$lib/stores/federation.svelte';
import type { OverviewProject, OverviewResponse } from '$lib/api/types';

function response(projects: OverviewProject[] = []): OverviewResponse {
	return { projects };
}

describe('OverviewSection (Federation v1 F6.4, US-7.1 AC1)', () => {
	beforeEach(() => {
		federationStore.clear();
	});
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it('renders each federated project with its role + named peer list (AC1)', async () => {
		vi.spyOn(federationApi, 'overview').mockResolvedValue(
			response([
				{
					projectId: 5,
					title: 'Shared roadmap',
					role: 'owner',
					peers: [
						{ instanceUrl: 'https://alice.example', displayName: 'Alice' },
						{ instanceUrl: 'https://bob.example', displayName: 'Bob' }
					]
				}
			])
		);

		render(OverviewSection);

		await waitFor(() => {
			expect(screen.getByText('Shared roadmap')).toBeTruthy();
		});
		// Role label (owner) and the NAMED peer list (US-7.1 AC1/AC3 — not a count).
		expect(screen.getByText('Owner')).toBeTruthy();
		expect(screen.getByText(/Alice, Bob/)).toBeTruthy();
	});

	it('renders the read-only role for a joined copy', async () => {
		vi.spyOn(federationApi, 'overview').mockResolvedValue(
			response([{ projectId: 9, title: 'Joined', role: 'read-only', peers: [] }])
		);

		render(OverviewSection);

		await waitFor(() => {
			expect(screen.getByText('Joined')).toBeTruthy();
		});
		expect(screen.getByText('Read-only')).toBeTruthy();
	});

	it('shows the empty state when no projects are federated', async () => {
		vi.spyOn(federationApi, 'overview').mockResolvedValue(response([]));

		render(OverviewSection);

		await waitFor(() => {
			expect(screen.getByText('No projects are federated yet.')).toBeTruthy();
		});
	});
});

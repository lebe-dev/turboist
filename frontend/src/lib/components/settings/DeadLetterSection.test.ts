import { render, screen, waitFor, fireEvent } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// The component drives federationStore.loadDeadLetter(), which calls getApiClient()
// then the (spied) federation.deadLetter endpoint. Stub the client so no real
// ApiClient singleton is needed.
vi.mock('$lib/api/client', () => ({
	getApiClient: () => ({}) as never
}));

import DeadLetterSection from './DeadLetterSection.svelte';
import { federation as federationApi } from '$lib/api/endpoints/federation';
import { federationStore } from '$lib/stores/federation.svelte';
import type { DeadLetterEntry } from '$lib/api/types';

function entry(over: Partial<DeadLetterEntry> = {}): DeadLetterEntry {
	return {
		eventId: 'e1',
		peerInstanceUrl: 'https://bob.example',
		projectId: 9,
		statusCode: 403,
		reason: 'federation_read_only',
		failedAt: '2026-06-03T10:00:00.000Z',
		...over
	};
}

describe('DeadLetterSection (Federation v1 F4.4, US-4.4 AC3)', () => {
	beforeEach(() => {
		federationStore.clear();
	});
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it('renders parked dead-letter entries with peer, reason and event id', async () => {
		vi.spyOn(federationApi, 'deadLetter').mockResolvedValue([
			entry({ eventId: 'e2', reason: 'federation_read_only', statusCode: 403 })
		]);

		render(DeadLetterSection);

		await waitFor(() => {
			expect(screen.getByText('https://bob.example')).toBeTruthy();
		});
		expect(screen.getByText(/federation_read_only/)).toBeTruthy();
		expect(screen.getByText(/e2/)).toBeTruthy();
	});

	it('shows the empty state when no events have failed', async () => {
		vi.spyOn(federationApi, 'deadLetter').mockResolvedValue([]);

		render(DeadLetterSection);

		await waitFor(() => {
			expect(
				screen.getByText('No failed events. Everything has been delivered or is retrying.')
			).toBeTruthy();
		});
	});

	it('re-fetches on the refresh button (server-read, no client outbox)', async () => {
		const spy = vi
			.spyOn(federationApi, 'deadLetter')
			.mockResolvedValueOnce([])
			.mockResolvedValueOnce([entry({ eventId: 'e9' })]);

		render(DeadLetterSection);
		await waitFor(() => {
			expect(spy).toHaveBeenCalledTimes(1);
		});

		await fireEvent.click(screen.getByText('Refresh'));
		await waitFor(() => {
			expect(spy).toHaveBeenCalledTimes(2);
			expect(screen.getByText(/e9/)).toBeTruthy();
		});
	});
});

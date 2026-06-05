import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('$lib/api/client', () => ({
	getApiClient: () => ({}) as never
}));

import OpsSection from './OpsSection.svelte';
import { federation as federationApi } from '$lib/api/endpoints/federation';
import { federationStore } from '$lib/stores/federation.svelte';
import type { FederationHealth, RetentionSettings } from '$lib/api/types';

function health(over: Partial<FederationHealth> = {}): FederationHealth {
	return {
		instanceUrl: 'https://me.example',
		protocolVersions: [1],
		uptimeS: 42,
		outboxDepth: 0,
		status: 'ok',
		peers: [],
		...over
	};
}

function retention(over: Partial<RetentionSettings> = {}): RetentionSettings {
	return {
		tombstoneDays: 0,
		outboxDays: 0,
		inboxDays: 0,
		outboxHardcapDays: 30,
		effectiveTombstoneDays: 90,
		effectiveOutboxDays: 30,
		effectiveInboxDays: 30,
		...over
	};
}

describe('OpsSection (Federation v1 F6.5)', () => {
	beforeEach(() => {
		federationStore.clear();
	});
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it('renders the health status + outbox depth (US-8.1)', async () => {
		vi.spyOn(federationApi, 'health').mockResolvedValue(health({ status: 'degraded', outboxDepth: 3 }));
		vi.spyOn(federationApi, 'getRetention').mockResolvedValue(retention());

		render(OpsSection);

		await waitFor(() => {
			expect(screen.getByTestId('ops-health-status').textContent).toContain('Degraded');
		});
		expect(screen.getByText('3')).toBeTruthy(); // outbox depth value
	});

	it('warns when the entered outbox window exceeds the hardcap (US-8.4)', async () => {
		vi.spyOn(federationApi, 'health').mockResolvedValue(health());
		vi.spyOn(federationApi, 'getRetention').mockResolvedValue(retention({ outboxDays: 365 }));

		render(OpsSection);

		await waitFor(() => {
			expect(screen.getByTestId('ops-outbox-cap-warning')).toBeTruthy();
		});
	});

	it('saves a retention change via updateRetention (US-8.4)', async () => {
		vi.spyOn(federationApi, 'health').mockResolvedValue(health());
		vi.spyOn(federationApi, 'getRetention').mockResolvedValue(retention());
		const update = vi
			.spyOn(federationApi, 'updateRetention')
			.mockResolvedValue(retention({ tombstoneDays: 120, effectiveTombstoneDays: 120 }));

		render(OpsSection);

		await waitFor(() => {
			expect(screen.getByText('Save retention')).toBeTruthy();
		});
		await fireEvent.click(screen.getByText('Save retention'));

		await waitFor(() => {
			expect(update).toHaveBeenCalledTimes(1);
		});
	});
});

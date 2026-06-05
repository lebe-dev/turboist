import { render, screen, waitFor, fireEvent } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// The component drives federationStore.loadAudit(), which calls getApiClient()
// then the (spied) federation.audit endpoint. Stub the client so no real
// ApiClient singleton is needed.
vi.mock('$lib/api/client', () => ({
	getApiClient: () => ({}) as never
}));

import AuditSection from './AuditSection.svelte';
import { federation as federationApi } from '$lib/api/endpoints/federation';
import { federationStore } from '$lib/stores/federation.svelte';
import type { AuditEntry, AuditResponse, SignatureFailureAlert } from '$lib/api/types';

function entry(over: Partial<AuditEntry> = {}): AuditEntry {
	return {
		id: 1,
		kind: 'signature_invalid',
		outcome: 'rejected',
		peerInstanceUrl: 'https://bob.example',
		detail: 'transport signature invalid',
		createdAt: '2026-06-03T10:00:00.000Z',
		...over
	};
}

function response(
	entries: AuditEntry[] = [],
	alerts: SignatureFailureAlert[] = []
): AuditResponse {
	return { entries, alerts };
}

describe('AuditSection (Federation v1 F6.3, US-7.4)', () => {
	beforeEach(() => {
		federationStore.clear();
	});
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it('renders audit rows with kind, peer and detail (AC1)', async () => {
		vi.spyOn(federationApi, 'audit').mockResolvedValue(
			response([entry({ id: 7, peerInstanceUrl: 'https://alice.example' })])
		);

		render(AuditSection);

		await waitFor(() => {
			expect(screen.getByText('https://alice.example')).toBeTruthy();
		});
		expect(screen.getByText('Signature invalid')).toBeTruthy();
		expect(screen.getByText('transport signature invalid')).toBeTruthy();
		expect(screen.getByText('Rejected')).toBeTruthy();
	});

	it('renders the "possible attack on peer X" banner (AC3)', async () => {
		vi.spyOn(federationApi, 'audit').mockResolvedValue(
			response(
				[entry({ id: 1 })],
				[{ peerInstanceUrl: 'https://attacker.example', count: 12, threshold: 10 }]
			)
		);

		render(AuditSection);

		await waitFor(() => {
			expect(screen.getByText(/Possible attack on https:\/\/attacker\.example/)).toBeTruthy();
		});
		expect(screen.getByText(/12 signature failures/)).toBeTruthy();
	});

	it('shows the empty state when nothing has been recorded', async () => {
		vi.spyOn(federationApi, 'audit').mockResolvedValue(response());

		render(AuditSection);

		await waitFor(() => {
			expect(
				screen.getByText('No federation activity has been recorded yet.')
			).toBeTruthy();
		});
	});

	it('re-fetches on the refresh button (server-read)', async () => {
		const spy = vi
			.spyOn(federationApi, 'audit')
			.mockResolvedValueOnce(response())
			.mockResolvedValueOnce(response([entry({ id: 99, detail: 'nonce replay', kind: 'replay' })]));

		render(AuditSection);
		await waitFor(() => {
			expect(spy).toHaveBeenCalledTimes(1);
		});

		await fireEvent.click(screen.getByText('Refresh'));
		await waitFor(() => {
			expect(spy).toHaveBeenCalledTimes(2);
			expect(screen.getByText('nonce replay')).toBeTruthy();
		});
	});
});

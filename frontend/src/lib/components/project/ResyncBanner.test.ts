import { render, screen, fireEvent } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import ResyncBanner from './ResyncBanner.svelte';
import type { Project } from '$lib/api/types';

function project(over: Partial<Project> = {}): Project {
	return {
		id: 1,
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
		originInstance: 'https://owner.example',
		federationPermissions: 'write',
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

afterEach(() => {
	vi.restoreAllMocks();
});

describe('ResyncBanner (Federation v1 F4.2, US-4.2 AC4)', () => {
	it('renders nothing when the project was never re-bootstrapped', () => {
		render(ResyncBanner, { project: project({ reBootstrappedAt: null }) });
		expect(screen.queryByText('Re-synced from the owner')).toBeNull();
	});

	it('surfaces the cutoff X when the project was re-bootstrapped (US-4.2 AC4)', () => {
		render(ResyncBanner, { project: project({ reBootstrappedAt: '2026-06-03T09:30:00.000Z' }) });
		// The banner title and the "preserved but may have been overridden" body show.
		expect(screen.getByText('Re-synced from the owner')).toBeTruthy();
		const body = screen.getByText(/preserved but may have been overridden/);
		expect(body).toBeTruthy();
		// The cutoff timestamp X is rendered (the year is a stable, locale-independent
		// substring of the formatted date).
		expect(body.textContent).toContain('2026');
	});

	it('dismisses the banner without re-rendering it for the same cutoff', async () => {
		render(ResyncBanner, { project: project({ reBootstrappedAt: '2026-06-03T09:30:00.000Z' }) });
		expect(screen.getByText('Re-synced from the owner')).toBeTruthy();
		await fireEvent.click(screen.getByLabelText('Dismiss'));
		expect(screen.queryByText('Re-synced from the owner')).toBeNull();
	});
});

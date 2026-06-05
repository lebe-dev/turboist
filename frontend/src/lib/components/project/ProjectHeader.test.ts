import { render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import ProjectHeader from './ProjectHeader.svelte';
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
		isFederated: false,
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

afterEach(() => {
	vi.restoreAllMocks();
});

describe('ProjectHeader federation badges (Federation v1 F2.4, US-2.4 AC1/AC2)', () => {
	it('renders the federated badge for any federated project (US-2.4 AC1)', () => {
		render(ProjectHeader, { project: project({ isFederated: true, isOwner: true, federationPermissions: 'admin' }) });
		expect(screen.getByText('Federated')).toBeTruthy();
	});

	it('renders the origin-instance badge for a joined project (US-2.4 AC2)', () => {
		render(ProjectHeader, {
			project: project({
				isFederated: true,
				isOwner: false,
				federationPermissions: 'read',
				originInstance: 'https://owner.example'
			})
		});
		// The origin is surfaced so the joiner sees where the project comes from.
		expect(screen.getByText(/owner\.example/)).toBeTruthy();
	});

	it('renders a read-only badge for a joined read-only project (US-2.4 AC4 UI leg)', () => {
		render(ProjectHeader, {
			project: project({
				isFederated: true,
				isOwner: false,
				federationPermissions: 'read',
				originInstance: 'https://owner.example'
			})
		});
		expect(screen.getByText('Read-only')).toBeTruthy();
	});

	it('does NOT render the read-only badge for a writable joined project (US-2.4 AC3)', () => {
		render(ProjectHeader, {
			project: project({
				isFederated: true,
				isOwner: false,
				federationPermissions: 'write',
				originInstance: 'https://owner.example'
			})
		});
		expect(screen.queryByText('Read-only')).toBeNull();
	});

	it("does NOT render the read-only badge for the owner's own project (controls stay enabled)", () => {
		render(ProjectHeader, {
			project: project({ isFederated: true, isOwner: true, federationPermissions: 'admin' })
		});
		expect(screen.queryByText('Read-only')).toBeNull();
	});

	it('renders no federation badges for a non-federated project', () => {
		render(ProjectHeader, { project: project({ isFederated: false }) });
		expect(screen.queryByText('Federated')).toBeNull();
		expect(screen.queryByText('Read-only')).toBeNull();
	});
});

describe('ProjectHeader owner-offline badge (Federation v1 F5.6a, US-6.5 AC1)', () => {
	it('renders the "pending — owner offline" badge for a joined copy whose owner is offline', () => {
		render(ProjectHeader, {
			project: project({
				isFederated: true,
				isOwner: false,
				federationPermissions: 'write',
				originInstance: 'https://owner.example',
				ownerOffline: true
			})
		});
		expect(screen.getByText('Pending — owner offline')).toBeTruthy();
		// Edits are NOT locked while the owner is offline (US-6.5 AC2): no read-only badge.
		expect(screen.queryByText('Read-only')).toBeNull();
	});

	it('does NOT render the owner-offline badge when the owner is fresh', () => {
		render(ProjectHeader, {
			project: project({ isFederated: true, isOwner: false, federationPermissions: 'write', ownerOffline: false })
		});
		expect(screen.queryByText('Pending — owner offline')).toBeNull();
	});

	it("does NOT render the owner-offline badge for the owner's own project", () => {
		render(ProjectHeader, {
			project: project({ isFederated: true, isOwner: true, federationPermissions: 'admin', ownerOffline: true })
		});
		expect(screen.queryByText('Pending — owner offline')).toBeNull();
	});
});

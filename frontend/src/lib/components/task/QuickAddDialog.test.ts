import { render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { projectsStore } from '$lib/stores/projects.svelte';
import { labelsStore } from '$lib/stores/labels.svelte';
import type { Project } from '$lib/api/types';
import QuickAddDialog from './QuickAddDialog.svelte';

// QuickAddDialog reads several auxiliary stores; the real singletons return safe
// empty defaults out of the box, so only the projects store needs seeding. The
// IsMobile hook reads matchMedia, which jsdom provides via the test setup.

function makeProject(over: Partial<Project> = {}): Project {
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

describe('QuickAddDialog federation new-task hint (Federation v1 F6.4, US-7.1 AC3)', () => {
	beforeEach(() => {
		projectsStore.setItems([]);
		labelsStore.setItems?.([]);
	});
	afterEach(() => {
		vi.restoreAllMocks();
		projectsStore.setItems([]);
	});

	it('lists the NAMED peer instances (not a bare count) when the default project is federated', async () => {
		projectsStore.setItems([
			makeProject({
				id: 1,
				isFederated: true,
				isOwner: true,
				peerInstances: [
					{ instanceUrl: 'https://alice.example', displayName: 'alice.example' },
					{ instanceUrl: 'https://bob.example', displayName: 'bob.example' }
				]
			})
		]);

		render(QuickAddDialog, { open: true, defaultProjectId: 1 });

		const hint = await waitFor(() => screen.getByTestId('federation-new-task-hint'));
		// US-7.1 AC3: the explicit instance list, not just "2 peers".
		expect(hint.textContent).toContain('alice.example');
		expect(hint.textContent).toContain('bob.example');
	});

	it('renders no hint for a non-federated project', async () => {
		projectsStore.setItems([makeProject({ id: 1, isFederated: false })]);

		render(QuickAddDialog, { open: true, defaultProjectId: 1 });

		await waitFor(() => {
			expect(screen.queryByTestId('federation-new-task-hint')).toBeNull();
		});
	});
});

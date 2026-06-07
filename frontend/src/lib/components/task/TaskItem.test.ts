import { render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { projectsStore } from '$lib/stores/projects.svelte';
import type { Project, Task } from '$lib/api/types';
import TaskItem from './TaskItem.svelte';

// TaskItem reads page.url.pathname (for badge visibility) and resolves task hrefs.
vi.mock('$app/state', () => ({
	page: { params: {}, url: new URL('http://localhost/project/1') }
}));
vi.mock('$app/paths', () => ({ resolve: (p: string) => p }));

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

function makeTask(over: Partial<Task> = {}): Task {
	return {
		id: 10,
		title: 'Locked task',
		description: '',
		inboxId: null,
		contextId: null,
		projectId: 1,
		sectionId: null,
		parentId: null,
		priority: 'no-priority',
		status: 'open',
		dueAt: null,
		dueHasTime: false,
		deadlineAt: null,
		deadlineHasTime: false,
		dayPart: 'none',
		planState: 'none',
		isPinned: false,
		pinnedAt: null,
		isPrivate: false,
		completedAt: null,
		recurrenceRule: null,
		postponeCount: 0,
		labels: [],
		url: '',
		federated: false,
		visibleToPeers: 0,
		clientId: '',
		deletedAt: null,
		createdAt: '',
		updatedAt: '',
		...over
	};
}

afterEach(() => {
	vi.restoreAllMocks();
	projectsStore.setItems([]);
});

describe('TaskItem read-only federated lockout (Federation v1 F5.2, US-5.1 AC4 UI leg)', () => {
	beforeEach(() => {
		projectsStore.setItems([]);
	});

	it('disables the complete checkbox and hides the actions menu for a read-only federated task', () => {
		projectsStore.setItems([
			makeProject({ id: 1, isFederated: true, isOwner: false, federationPermissions: 'read', originInstance: 'https://owner.example' })
		]);
		render(TaskItem, { task: makeTask({ projectId: 1 }), mutator: {} as never, onToggle: vi.fn() });

		const checkbox = screen.getByRole('button', { name: /mark complete|mark incomplete/i });
		expect((checkbox as HTMLButtonElement).disabled).toBe(true);

		// The actions menu (which carries edit/delete/move) must not render.
		expect(screen.queryByRole('button', { name: /more|actions|task actions/i })).toBeNull();
	});

	it('renders the read-only lock badge for a read-only federated task', () => {
		projectsStore.setItems([
			makeProject({ id: 1, isFederated: true, isOwner: false, federationPermissions: 'read', originInstance: 'https://owner.example' })
		]);
		render(TaskItem, { task: makeTask({ projectId: 1 }), mutator: {} as never, onToggle: vi.fn() });

		expect(screen.getByTestId('task-readonly-lock')).toBeTruthy();
	});

	it('does NOT lock a task in a WRITABLE joined federated project', () => {
		projectsStore.setItems([
			makeProject({ id: 1, isFederated: true, isOwner: false, federationPermissions: 'write', originInstance: 'https://owner.example' })
		]);
		render(TaskItem, { task: makeTask({ projectId: 1 }), mutator: {} as never, onToggle: vi.fn() });

		const checkbox = screen.getByRole('button', { name: /mark complete|mark incomplete/i });
		expect((checkbox as HTMLButtonElement).disabled).toBe(false);
		expect(screen.queryByTestId('task-readonly-lock')).toBeNull();
	});

	it('does NOT lock a task in the owner\'s OWN federated project', () => {
		projectsStore.setItems([
			makeProject({ id: 1, isFederated: true, isOwner: true, federationPermissions: 'admin' })
		]);
		render(TaskItem, { task: makeTask({ projectId: 1 }), mutator: {} as never, onToggle: vi.fn() });

		const checkbox = screen.getByRole('button', { name: /mark complete|mark incomplete/i });
		expect((checkbox as HTMLButtonElement).disabled).toBe(false);
		expect(screen.queryByTestId('task-readonly-lock')).toBeNull();
	});

	it('does NOT lock a plain non-federated task', () => {
		projectsStore.setItems([makeProject({ id: 1, isFederated: false })]);
		render(TaskItem, { task: makeTask({ projectId: 1 }), mutator: {} as never, onToggle: vi.fn() });

		const checkbox = screen.getByRole('button', { name: /mark complete|mark incomplete/i });
		expect((checkbox as HTMLButtonElement).disabled).toBe(false);
		expect(screen.queryByTestId('task-readonly-lock')).toBeNull();
	});

	it('does NOT lock an inbox task that has no project', () => {
		projectsStore.setItems([
			makeProject({ id: 1, isFederated: true, isOwner: false, federationPermissions: 'read', originInstance: 'https://owner.example' })
		]);
		render(TaskItem, { task: makeTask({ projectId: null, inboxId: 1 }), mutator: {} as never, onToggle: vi.fn() });

		const checkbox = screen.getByRole('button', { name: /mark complete|mark incomplete/i });
		expect((checkbox as HTMLButtonElement).disabled).toBe(false);
		expect(screen.queryByTestId('task-readonly-lock')).toBeNull();
	});
});

describe('TaskItem "visible to N peers" badge (Federation v1 F6.4, US-7.1 AC2)', () => {
	beforeEach(() => {
		projectsStore.setItems([]);
	});

	it('renders the visible-to-N-peers badge with the project peer count', () => {
		projectsStore.setItems([
			makeProject({
				id: 1,
				isFederated: true,
				isOwner: true,
				federationPermissions: 'admin',
				peerInstances: [
					{ instanceUrl: 'https://alice.example', displayName: 'Alice' },
					{ instanceUrl: 'https://bob.example', displayName: 'Bob' }
				]
			})
		]);
		render(TaskItem, { task: makeTask({ projectId: 1 }), mutator: {} as never, onToggle: vi.fn() });

		const badge = screen.getByTestId('visible-to-peers-badge');
		// The badge is icon-only; the count and the named instances (US-7.1 AC3 — not
		// a bare count) both live in the title/aria-label.
		expect(badge.getAttribute('title')).toContain('2');
		expect(badge.getAttribute('title')).toContain('Alice');
		expect(badge.getAttribute('title')).toContain('Bob');
	});

	it('renders NO badge for a project with no peers', () => {
		projectsStore.setItems([
			makeProject({ id: 1, isFederated: true, isOwner: true, peerInstances: [] })
		]);
		render(TaskItem, { task: makeTask({ projectId: 1 }), mutator: {} as never, onToggle: vi.fn() });

		expect(screen.queryByTestId('visible-to-peers-badge')).toBeNull();
	});

	it('renders NO badge for a non-federated project', () => {
		projectsStore.setItems([makeProject({ id: 1, isFederated: false })]);
		render(TaskItem, { task: makeTask({ projectId: 1 }), mutator: {} as never, onToggle: vi.fn() });

		expect(screen.queryByTestId('visible-to-peers-badge')).toBeNull();
	});
});

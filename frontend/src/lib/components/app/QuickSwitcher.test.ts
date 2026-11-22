import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const { createInboxMock, createInProjectMock, searchMock, toastMock } = vi.hoisted(() => ({
	createInboxMock: vi.fn(),
	createInProjectMock: vi.fn(),
	searchMock: vi.fn().mockResolvedValue({ tasks: { items: [] } }),
	toastMock: { success: vi.fn(), error: vi.fn(), info: vi.fn() }
}));

vi.mock('svelte-sonner', () => ({ toast: toastMock }));
vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));
vi.mock('$lib/api/client', () => ({ getApiClient: () => ({}) }));
vi.mock('$lib/api/endpoints/views', () => ({
	views: { search: searchMock }
}));
vi.mock('$lib/api/endpoints/tasks', () => ({
	tasks: {
		createInbox: createInboxMock,
		inbox: vi.fn().mockResolvedValue({ count: 0, warnThresholdExceeded: false })
	}
}));
vi.mock('$lib/api/endpoints/projects', () => ({
	projects: { createTask: createInProjectMock }
}));

import type { Project } from '$lib/api/types';
import { projectsStore } from '$lib/stores/projects.svelte';
import { recentProjectsStore } from '$lib/stores/recentProjects.svelte';
import { recentItemsStore } from '$lib/stores/recentItems.svelte';
import QuickSwitcher from './QuickSwitcher.svelte';

/** The shape `tasks.createInbox` and `projects.createTask` answer with, as far as the panel reads it. */
function createdTask(id: number, title: string, labels: { id: number; name: string }[] = []) {
	return { id, title, labels };
}

function project(id: number, title: string): Project {
	return {
		id,
		contextId: 1,
		title,
		description: '',
		color: 'blue',
		status: 'open',
		projectType: 'generic',
		isPinned: false,
		pinnedAt: null,
		isPrivate: false,
		labels: [],
		troikiCategory: null,
		createdAt: '2026-01-01T00:00:00.000Z',
		updatedAt: '2026-01-01T00:00:00.000Z'
	};
}

async function typeTitle(title: string): Promise<void> {
	await fireEvent.input(screen.getByRole('combobox'), { target: { value: title } });
}

beforeEach(() => {
	recentItemsStore.clear();
	recentProjectsStore.clear();
	projectsStore.clear();
	searchMock.mockReset();
	searchMock.mockResolvedValue({ tasks: { items: [] } });
});

afterEach(() => {
	createInboxMock.mockReset();
	createInProjectMock.mockReset();
	toastMock.success.mockReset();
	toastMock.error.mockReset();
});

describe('QuickSwitcher', () => {
	it('remembers a task added to the inbox, so it leads the recents next time', async () => {
		createInboxMock.mockResolvedValue(createdTask(42, 'Call mom', [{ id: 3, name: 'family' }]));
		const { rerender } = render(QuickSwitcher, { open: true });

		await typeTitle('Call mom');
		await fireEvent.click(screen.getByText('Add to Inbox: Call mom'));

		await waitFor(() =>
			expect(recentItemsStore.entries[0]).toEqual({
				kind: 'task',
				id: 42,
				title: 'Call mom',
				labels: ['family']
			})
		);

		// Reopening the panel shows what was just written down first.
		await rerender({ open: false });
		await rerender({ open: true });

		const rows = screen.getAllByRole('option');
		expect(rows[0].textContent).toContain('Call mom');
	});

	it('remembers a task added for today', async () => {
		const garden = project(7, 'Garden');
		projectsStore.setItems([garden]);
		recentProjectsStore.visit(7);
		createInProjectMock.mockResolvedValue(createdTask(8, 'Buy seeds'));
		render(QuickSwitcher, { open: true });

		await typeTitle('Buy seeds');
		await fireEvent.click(screen.getByText('Add for today to Garden: Buy seeds'));

		await waitFor(() =>
			expect(recentItemsStore.entries[0]).toEqual({
				kind: 'task',
				id: 8,
				title: 'Buy seeds',
				labels: []
			})
		);
	});

	it('does not remember a task the request refused', async () => {
		createInboxMock.mockRejectedValue(new Error('nope'));
		render(QuickSwitcher, { open: true });

		await typeTitle('Call mom');
		await fireEvent.click(screen.getByText('Add to Inbox: Call mom'));

		await waitFor(() => expect(toastMock.error).toHaveBeenCalledOnce());
		expect(recentItemsStore.entries).toEqual([]);
	});

	it('shows a new answer from its first row, not from where the last one was left', async () => {
		const many = (from: number, label: string) =>
			Array.from({ length: 20 }, (_, i) => createdTask(from + i, `${label} ${from + i}`));
		searchMock.mockImplementation(async (_client, params) => ({
			tasks: { items: (params as { q: string }).q === 'ma' ? many(1, 'Match') : many(101, 'Fresh') }
		}));
		render(QuickSwitcher, { open: true });

		await typeTitle('ma');
		await waitFor(() => expect(screen.getByText('Match 1')).toBeTruthy());

		const list = document.getElementById('quick-switcher-list') as HTMLDivElement;
		list.scrollTop = 300;

		await typeTitle('match');
		await waitFor(() => expect(screen.getByText('Fresh 101')).toBeTruthy());

		await waitFor(() => expect(list.scrollTop).toBe(0));
	});
});

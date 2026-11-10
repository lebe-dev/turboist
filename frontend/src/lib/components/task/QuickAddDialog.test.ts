import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { labelsStore } from '$lib/stores/labels.svelte';
import { projectsStore } from '$lib/stores/projects.svelte';
import { todayProjectsStore } from '$lib/stores/todayProjects.svelte';
import QuickAddDialog from './QuickAddDialog.svelte';

afterEach(() => {
	labelsStore.clear();
	vi.restoreAllMocks();
});

describe('QuickAddDialog subtask labels', () => {
	it.each(['desktop', 'mobile'])('lets the parent supply labels on %s', async (viewport) => {
		vi.spyOn(window, 'matchMedia').mockImplementation(
			(query) =>
				({
					matches: viewport === 'mobile',
					media: query,
					onchange: null,
					addListener: () => {},
					removeListener: () => {},
					addEventListener: () => {},
					removeEventListener: () => {},
					dispatchEvent: () => false
				}) as MediaQueryList
		);
		const onSubmit = vi.fn();
		render(QuickAddDialog, {
			props: { open: true, defaultTitle: 'Child', defaultParentId: 7, onSubmit }
		});

		const dialog = await screen.findByRole('dialog');
		await fireEvent.submit(dialog.querySelector('form')!);
		await waitFor(() => expect(onSubmit).toHaveBeenCalledOnce());
		expect(onSubmit.mock.calls[0][0].labels).toBeUndefined();
	});

	it('keeps an explicit request to remove every inherited label', async () => {
		labelsStore.setItems([
			{
				id: 3,
				name: 'work',
				color: 'blue',
				description: '',
				isFavourite: false,
				isPrivate: false,
				createdAt: '2026-01-01T00:00:00Z',
				updatedAt: '2026-01-01T00:00:00Z'
			}
		]);
		const onSubmit = vi.fn();
		render(QuickAddDialog, {
			props: {
				open: true,
				defaultTitle: 'Child',
				defaultParentId: 7,
				defaultLabelIds: [3],
				onSubmit
			}
		});

		await fireEvent.click(await screen.findByRole('button', { name: 'work' }));
		const dialog = await screen.findByRole('dialog');
		await fireEvent.submit(dialog.querySelector('form')!);
		await waitFor(() => expect(onSubmit).toHaveBeenCalledOnce());
		expect(onSubmit.mock.calls[0][0].labels).toEqual([]);
	});
});

describe('QuickAddDialog today projects', () => {
	const project = (id: number, title: string) =>
		({
			id,
			title,
			status: 'active',
			isPinned: false,
			pinnedAt: null,
			labels: []
		}) as unknown as Parameters<typeof projectsStore.setItems>[0][number];

	afterEach(() => {
		todayProjectsStore.clear();
		projectsStore.clear();
	});

	it('offers the projects published by the Today page and files into the tapped one', async () => {
		projectsStore.setItems([project(1, 'Alpha'), project(2, 'Beta'), project(3, 'Gamma')]);
		todayProjectsStore.set([2, 3]);
		const onSubmit = vi.fn();
		render(QuickAddDialog, { props: { open: true, defaultTitle: 'Task', onSubmit } });

		const group = await screen.findByRole('group', { name: 'Today projects' });
		expect(group.textContent).toContain('Beta');
		expect(group.textContent).toContain('Gamma');
		expect(group.textContent).not.toContain('Alpha');

		await fireEvent.click(screen.getByRole('button', { name: 'Gamma' }));
		const dialog = await screen.findByRole('dialog');
		await fireEvent.submit(dialog.querySelector('form')!);
		await waitFor(() => expect(onSubmit).toHaveBeenCalledOnce());
		expect(onSubmit.mock.calls[0][1].projectId).toBe(3);
	});

	it('shows no row when nothing was published', async () => {
		projectsStore.setItems([project(1, 'Alpha')]);
		render(QuickAddDialog, { props: { open: true } });
		await screen.findByRole('dialog');
		expect(screen.queryByRole('group', { name: 'Today projects' })).toBeNull();
	});
});

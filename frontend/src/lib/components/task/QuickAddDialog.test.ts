import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { labelsStore } from '$lib/stores/labels.svelte';
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

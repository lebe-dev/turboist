import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { labelsStore } from '$lib/stores/labels.svelte';
import { projectsStore } from '$lib/stores/projects.svelte';
import { todayProjectsStore } from '$lib/stores/todayProjects.svelte';
import { todayLockStore } from '$lib/stores/todayLock.svelte';
import { nowStore } from '$lib/stores/now.svelte';
import { settingsStore } from '$lib/stores/settings.svelte';
import { appSettingsStore } from '$lib/stores/appSettings.svelte';
import { setApiClient, type ApiClient } from '$lib/api/client';
import { today as todayApi } from '$lib/api/endpoints/today';
import QuickAddDialog from './QuickAddDialog.svelte';

afterEach(async () => {
	labelsStore.clear();
	vi.restoreAllMocks();
	// bits-ui releases its body scroll lock on a short timer after the dialog unmounts;
	// let it fire while the DOM still exists.
	await new Promise((resolve) => setTimeout(resolve, 40));
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

describe('QuickAddDialog under the Today lock', () => {
	const project = (id: number, title: string) =>
		({
			id,
			title,
			status: 'active',
			isPinned: false,
			pinnedAt: null,
			labels: []
		}) as unknown as Parameters<typeof projectsStore.setItems>[0][number];
	const inHours = (h: number) => new Date(Date.now() + h * 3_600_000).toISOString();

	beforeEach(() => {
		// The lock state is read through the shared client; the endpoint itself is stubbed per test.
		setApiClient({} as ApiClient);
	});

	afterEach(() => {
		todayLockStore.clear();
		todayProjectsStore.clear();
		projectsStore.clear();
	});

	function renderFor(projectId: number | null, dueDate = nowStore.todayKey) {
		projectsStore.setItems([project(1, 'Focus'), project(2, 'Other')]);
		todayProjectsStore.set([1]);
		return render(QuickAddDialog, {
			props: {
				open: true,
				defaultTitle: 'Task',
				defaultProjectId: projectId,
				defaultDueDate: dueDate
			}
		});
	}

	it('warns before taking work from another project into a locked today', async () => {
		todayLockStore.setUntil(inHours(48));
		renderFor(2);
		const hint = await screen.findByTestId('today-lock-hint');
		expect(hint.textContent).toContain('Until midnight, today takes only today’s projects');
	});

	it('disables the submit button while the lock would refuse the task', async () => {
		todayLockStore.setUntil(inHours(48));
		renderFor(2);
		await screen.findByTestId('today-lock-hint');
		const button = screen.getByRole('button', { name: 'Add task' }) as HTMLButtonElement;
		expect(button.disabled).toBe(true);
	});

	it("keeps the submit button enabled for one of today's projects", async () => {
		todayLockStore.setUntil(inHours(48));
		renderFor(1);
		await screen.findByRole('dialog');
		const button = screen.getByRole('button', { name: 'Add task' }) as HTMLButtonElement;
		expect(button.disabled).toBe(false);
	});

	it("blocks a project outside today's set read from the server on any screen", async () => {
		// A task or project page: the Today screen has published nothing.
		vi.spyOn(todayApi, 'lockState').mockResolvedValue({
			active: true,
			lockedUntil: inHours(48),
			projectIds: [1]
		});
		todayLockStore.setUntil(inHours(48));
		projectsStore.setItems([project(1, 'Focus'), project(2, 'Other')]);
		render(QuickAddDialog, {
			props: {
				open: true,
				defaultTitle: 'Task',
				defaultProjectId: 2,
				defaultDueDate: nowStore.todayKey
			}
		});
		await screen.findByTestId('today-lock-hint');
		const button = screen.getByRole('button', { name: 'Add task' }) as HTMLButtonElement;
		expect(button.disabled).toBe(true);
	});

	it('lets a project through when the server lists it, even if Today shows none of its tasks', async () => {
		vi.spyOn(todayApi, 'lockState').mockResolvedValue({
			active: true,
			lockedUntil: inHours(48),
			projectIds: [2]
		});
		todayLockStore.setUntil(inHours(48));
		projectsStore.setItems([project(1, 'Focus'), project(2, 'Other')]);
		render(QuickAddDialog, {
			props: {
				open: true,
				defaultTitle: 'Task',
				defaultProjectId: 2,
				defaultDueDate: nowStore.todayKey
			}
		});
		await waitFor(() => expect(todayLockStore.allowedProjectIds).toEqual([2]));
		expect(screen.queryByTestId('today-lock-hint')).toBeNull();
		const button = screen.getByRole('button', { name: 'Add task' }) as HTMLButtonElement;
		expect(button.disabled).toBe(false);
	});

	it('treats an always-allowed project as open while the server set is not known yet', async () => {
		vi.spyOn(todayApi, 'lockState').mockRejectedValue(new Error('offline'));
		todayLockStore.setUntil(inHours(48));
		projectsStore.setItems([
			project(1, 'Focus'),
			{ ...project(2, 'Other'), todayLockExempt: true } as ReturnType<typeof project>
		]);
		todayProjectsStore.set([1]);
		render(QuickAddDialog, {
			props: {
				open: true,
				defaultTitle: 'Task',
				defaultProjectId: 2,
				defaultDueDate: nowStore.todayKey
			}
		});
		await screen.findByRole('dialog');
		expect(screen.queryByTestId('today-lock-hint')).toBeNull();
	});

	it('warns for the inbox, which a locked today never takes', async () => {
		todayLockStore.setUntil(inHours(48));
		renderFor(null);
		expect(await screen.findByTestId('today-lock-hint')).toBeTruthy();
	});

	it("says nothing for one of today's projects", async () => {
		todayLockStore.setUntil(inHours(48));
		renderFor(1);
		await screen.findByRole('dialog');
		expect(screen.queryByTestId('today-lock-hint')).toBeNull();
	});

	it('says nothing for another day', async () => {
		todayLockStore.setUntil(inHours(48));
		renderFor(2, '2999-01-01');
		await screen.findByRole('dialog');
		expect(screen.queryByTestId('today-lock-hint')).toBeNull();
	});

	it('says nothing while no lock runs', async () => {
		renderFor(2);
		await screen.findByRole('dialog');
		expect(screen.queryByTestId('today-lock-hint')).toBeNull();
	});
});

describe('QuickAddDialog swap button', () => {
	it('exchanges the two fields', async () => {
		render(QuickAddDialog, {
			props: {
				open: true,
				defaultTitle: 'Fix the fence',
				defaultDescription: 'Two boards are loose.'
			}
		});
		await screen.findByRole('dialog');

		await fireEvent.click(screen.getByTestId('quick-add-swap'));

		expect((screen.getByLabelText('Task names') as HTMLTextAreaElement).value).toBe(
			'Two boards are loose.'
		);
		expect((screen.getByLabelText('Description') as HTMLTextAreaElement).value).toBe(
			'Fix the fence'
		);
	});

	it('saves the swapped text under the field it now sits in, without submitting on the click', async () => {
		const onSubmit = vi.fn();
		render(QuickAddDialog, {
			props: {
				open: true,
				defaultTitle: 'Fix the fence',
				defaultDescription: 'Two boards are loose.',
				onSubmit
			}
		});
		const dialog = await screen.findByRole('dialog');

		await fireEvent.click(screen.getByTestId('quick-add-swap'));
		expect(onSubmit).not.toHaveBeenCalled();

		await fireEvent.submit(dialog.querySelector('form')!);
		await waitFor(() => expect(onSubmit).toHaveBeenCalledOnce());
		expect(onSubmit.mock.calls[0][0].title).toBe('Two boards are loose.');
		expect(onSubmit.mock.calls[0][0].description).toBe('Fix the fence');
	});

	it('puts everything back when pressed again', async () => {
		render(QuickAddDialog, {
			props: {
				open: true,
				defaultTitle: 'Fix the fence',
				defaultDescription: 'Two boards are loose.'
			}
		});
		await screen.findByRole('dialog');

		await fireEvent.click(screen.getByTestId('quick-add-swap'));
		await fireEvent.click(screen.getByTestId('quick-add-swap'));

		expect((screen.getByLabelText('Task names') as HTMLTextAreaElement).value).toBe(
			'Fix the fence'
		);
		expect((screen.getByLabelText('Description') as HTMLTextAreaElement).value).toBe(
			'Two boards are loose.'
		);
	});
});

describe('QuickAddDialog bug shortcut', () => {
	const label = (id: number, name: string) => ({
		id,
		name,
		color: 'blue',
		description: '',
		isFavourite: false,
		isPrivate: false,
		createdAt: '2026-01-01T00:00:00Z',
		updatedAt: '2026-01-01T00:00:00Z'
	});

	it('is hidden while no bug labels are configured', async () => {
		settingsStore.value = { ...settingsStore.value, bugLabelIds: [] };
		render(QuickAddDialog, { props: { open: true, defaultTitle: 'x' } });
		await screen.findByRole('dialog');
		expect(screen.queryByTestId('quick-add-bug')).toBeNull();
	});

	it('adds the bug labels with the highest priority and takes them back', async () => {
		labelsStore.setItems([label(3, 'bug'), label(4, 'urgent')]);
		settingsStore.value = { ...settingsStore.value, bugLabelIds: [3, 4] };
		const onSubmit = vi.fn();
		render(QuickAddDialog, { props: { open: true, defaultTitle: 'Crash', onSubmit } });
		const dialog = await screen.findByRole('dialog');
		const button = screen.getByTestId('quick-add-bug');

		await fireEvent.click(button);
		expect(button.getAttribute('aria-pressed')).toBe('true');
		await fireEvent.submit(dialog.querySelector('form')!);
		await waitFor(() => expect(onSubmit).toHaveBeenCalledOnce());
		expect(onSubmit.mock.calls[0][0].priority).toBe('high');
		expect(onSubmit.mock.calls[0][0].labels).toEqual(['bug', 'urgent']);
	});

	it('restores the previous priority when switched off', async () => {
		labelsStore.setItems([label(3, 'bug')]);
		settingsStore.value = { ...settingsStore.value, bugLabelIds: [3] };
		const onSubmit = vi.fn();
		render(QuickAddDialog, { props: { open: true, defaultTitle: 'Crash', onSubmit } });
		const dialog = await screen.findByRole('dialog');
		const button = screen.getByTestId('quick-add-bug');

		await fireEvent.click(button);
		await fireEvent.click(button);
		await fireEvent.submit(dialog.querySelector('form')!);
		await waitFor(() => expect(onSubmit).toHaveBeenCalledOnce());
		expect(onSubmit.mock.calls[0][0].priority).toBe('no-priority');
		expect(onSubmit.mock.calls[0][0].labels).toEqual([]);
	});
});

describe('QuickAddDialog bug projects', () => {
	const project = (id: number, title: string, projectType: 'software' | 'generic') =>
		({
			id,
			title,
			status: 'active',
			projectType,
			isPinned: false,
			pinnedAt: null,
			labels: []
		}) as unknown as Parameters<typeof projectsStore.setItems>[0][number];
	const bugLabel = {
		id: 3,
		name: 'bug',
		color: 'red',
		description: '',
		isFavourite: false,
		isPrivate: false,
		createdAt: '2026-01-01T00:00:00Z',
		updatedAt: '2026-01-01T00:00:00Z'
	};

	beforeEach(() => {
		labelsStore.setItems([bugLabel]);
		settingsStore.value = { ...settingsStore.value, bugLabelIds: [3] };
		projectsStore.setItems([project(1, 'Toolbox', 'software'), project(2, 'Garden', 'generic')]);
	});

	afterEach(() => {
		labelsStore.clear();
		projectsStore.clear();
		todayProjectsStore.clear();
		appSettingsStore.clear();
		settingsStore.value = { ...settingsStore.value, bugLabelIds: [] };
	});

	async function toggleBug(): Promise<void> {
		await fireEvent.click(screen.getByTestId('quick-add-bug'));
	}

	it('narrows the projects offered on the Today row to software ones', async () => {
		todayProjectsStore.set([1, 2]);
		render(QuickAddDialog, { props: { open: true, defaultTitle: 'Crash on save' } });
		await screen.findByRole('dialog');

		const row = await screen.findByRole('group', { name: 'Today projects' });
		expect(row.textContent).toContain('Toolbox');
		expect(row.textContent).toContain('Garden');

		await toggleBug();

		const narrowed = await screen.findByRole('group', { name: 'Today projects' });
		expect(narrowed.textContent).toContain('Toolbox');
		expect(narrowed.textContent).not.toContain('Garden');
	});

	it('suggests only software projects while the bug shortcut is on', async () => {
		appSettingsStore.setValue({
			...appSettingsStore.value,
			projectSuggestions: [{ mask: 'crash', projectIds: [1, 2], ignoreCase: true }]
		});
		render(QuickAddDialog, { props: { open: true, defaultTitle: 'Crash on save' } });
		await screen.findByRole('dialog');

		await toggleBug();
		const hint = await screen.findByText('Suggested projects:');
		const chips = hint.parentElement!;

		expect(chips.textContent).toContain('Toolbox');
		expect(chips.textContent).not.toContain('Garden');
	});
});

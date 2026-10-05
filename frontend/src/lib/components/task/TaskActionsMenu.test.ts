import '@testing-library/jest-dom/vitest';
import { render, screen, fireEvent, waitFor, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Task } from '$lib/api/types';
import { taskSelectionStore } from '$lib/stores/taskSelection.svelte';
import { copyTaskId, updateTaskFields } from '$lib/utils/taskActions';
import TaskActionsMenu from './TaskActionsMenu.svelte';

vi.mock('$lib/utils/taskActions', async (importOriginal) => ({
	...(await importOriginal<typeof import('$lib/utils/taskActions')>()),
	copyTaskId: vi.fn().mockResolvedValue(undefined),
	updateTaskFields: vi.fn().mockResolvedValue(undefined)
}));

function task(overrides: Partial<Task> = {}): Task {
	return {
		id: 7,
		title: 'Prepare the release',
		description: '',
		inboxId: null,
		contextId: 1,
		projectId: 3,
		sectionId: null,
		parentId: null,
		priority: 'low',
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
		isComplex: false,
		completedAt: null,
		recurrenceRule: null,
		sourceTaskId: null,
		postponeCount: 0,
		autoSortedAt: null,
		autoSortUndecidedAt: null,
		labels: [],
		url: '',
		createdAt: '',
		updatedAt: '',
		blockedByCount: 0,
		relationCount: 0,
		...overrides
	};
}

function viewport(mobile: boolean) {
	vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({
		matches: mobile && query.includes('max-width'),
		media: query,
		onchange: null,
		addListener: () => {},
		removeListener: () => {},
		addEventListener: () => {},
		removeEventListener: () => {},
		dispatchEvent: () => false
	}));
}

async function openMobile(overrides: Partial<Task> = {}, hasSubtasks = false) {
	render(TaskActionsMenu, {
		props: { task: task(overrides), mutator: { replace: vi.fn(), remove: vi.fn() }, hasSubtasks }
	});
	await fireEvent.click(screen.getByRole('button', { name: 'Task actions' }));
	return await screen.findByRole('dialog', { name: 'Task actions' });
}

beforeEach(() => {
	viewport(true);
	vi.clearAllMocks();
});

afterEach(() => {
	taskSelectionStore.disable();
	vi.restoreAllMocks();
});

describe('TaskActionsMenu mobile sheet', () => {
	it('opens a bottom sheet with planning controls and keeps rare actions on the More page', async () => {
		const dialog = await openMobile();
		expect(dialog).toHaveAttribute('data-side', 'bottom');
		expect(within(dialog).getByText('Prepare the release')).toBeVisible();
		expect(within(dialog).getByRole('button', { name: 'Today' })).toBeVisible();
		expect(within(dialog).getByRole('button', { name: 'P1' })).toBeVisible();
		expect(within(dialog).getByRole('button', { name: 'Duplicate' })).toBeVisible();
		for (const name of [
			'Pin',
			'Select tasks',
			'Create template',
			'Decompose task',
			'Copy ID',
			'Copy as JSON'
		]) {
			expect(within(dialog).queryByRole('button', { name })).toBeNull();
		}

		await fireEvent.click(within(dialog).getByRole('button', { name: 'More' }));
		const more = await screen.findByRole('dialog', { name: 'More' });
		for (const name of ['Pin', 'Select tasks', 'Create template', 'Copy ID', 'Copy as JSON']) {
			expect(within(more).getByRole('button', { name })).toBeVisible();
		}
		expect(within(more).queryByRole('button', { name: 'Today' })).toBeNull();
		await waitFor(() => expect(within(more).getByRole('button', { name: 'Back' })).toHaveFocus());
		await fireEvent.click(within(more).getByRole('button', { name: 'Back' }));
		await waitFor(() => expect(screen.getByRole('button', { name: 'More' })).toHaveFocus());

		await fireEvent.click(screen.getByRole('button', { name: 'More' }));
		await fireEvent.click(screen.getByRole('button', { name: 'Close' }));
		await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
		await fireEvent.click(screen.getByRole('button', { name: 'Task actions' }));
		expect(await screen.findByRole('dialog', { name: 'Task actions' })).toBeVisible();
		expect(screen.queryByRole('button', { name: 'Back' })).toBeNull();
	});

	it('keeps quick toggles open and closes after assigning a date', async () => {
		await openMobile();
		await fireEvent.click(screen.getByRole('button', { name: 'Morning' }));
		expect(updateTaskFields).toHaveBeenLastCalledWith(
			expect.objectContaining({ id: 7 }),
			expect.anything(),
			{ dayPart: 'morning' },
			{ belongs: undefined }
		);
		expect(screen.getByRole('dialog')).toBeVisible();
		await fireEvent.click(screen.getByRole('button', { name: 'P1' }));
		expect(updateTaskFields).toHaveBeenLastCalledWith(
			expect.objectContaining({ id: 7 }),
			expect.anything(),
			{ priority: 'high' },
			{ belongs: undefined }
		);
		expect(screen.getByRole('dialog')).toBeVisible();
		await fireEvent.click(screen.getByRole('button', { name: 'Today' }));
		await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
		expect(updateTaskFields).toHaveBeenLastCalledWith(
			expect.objectContaining({ id: 7 }),
			expect.anything(),
			{ dueAt: expect.any(String), dueHasTime: false },
			{ belongs: undefined }
		);
	});

	it('executes a rare action and closes the sheet', async () => {
		await openMobile();
		await fireEvent.click(screen.getByRole('button', { name: 'More' }));
		await fireEvent.click(screen.getByRole('button', { name: 'Copy ID' }));
		expect(copyTaskId).toHaveBeenCalledWith(expect.objectContaining({ id: 7 }));
		await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
	});

	it('closes before opening the move dialog', async () => {
		await openMobile();
		await fireEvent.click(screen.getByRole('button', { name: 'More' }));
		await fireEvent.click(screen.getByRole('button', { name: 'Move to project…' }));
		expect(await screen.findByRole('dialog', { name: 'Move to project' })).toBeVisible();
		expect(screen.getAllByRole('dialog')).toHaveLength(1);
	});

	it('preserves inbox restrictions', async () => {
		const rendered = await openMobile({ inboxId: 2, projectId: null });
		expect(within(rendered).queryByRole('button', { name: 'Today' })).toBeNull();
		await fireEvent.click(screen.getByRole('button', { name: 'More' }));
		for (const name of ['Pin', 'Mark as private', 'Decompose task']) {
			expect(screen.queryByRole('button', { name })).toBeNull();
		}
	});

	it('disables decomposition for a task with subtasks', async () => {
		await openMobile({}, true);
		await fireEvent.click(screen.getByRole('button', { name: 'More' }));
		expect(screen.getByRole('button', { name: 'Decompose task' })).toBeDisabled();
	});
});

describe('TaskActionsMenu desktop menu', () => {
	it('opens a dropdown with the existing primary actions', async () => {
		viewport(false);
		render(TaskActionsMenu, {
			props: { task: task(), mutator: { replace: vi.fn(), remove: vi.fn() } }
		});
		await fireEvent.click(screen.getByRole('button', { name: 'Task actions' }));
		const menu = await screen.findByRole('menu');
		expect(within(menu).getByRole('menuitem', { name: 'Pin' })).toBeVisible();
		expect(within(menu).getByRole('menuitem', { name: 'Copy ID' })).toBeVisible();
		expect(screen.queryByRole('dialog')).toBeNull();
	});
});

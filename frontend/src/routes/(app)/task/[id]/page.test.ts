import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { tick } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Task } from '$lib/api/types';

const api = vi.hoisted(() => ({
	get: vi.fn(),
	complete: vi.fn(),
	uncomplete: vi.fn(),
	update: vi.fn()
}));
vi.mock('$app/state', () => ({ page: { params: { id: '7' } } }));
vi.mock('$lib/api/client', async (importOriginal) => ({
	...(await importOriginal<typeof import('$lib/api/client')>()),
	getApiClient: () => ({})
}));
vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('$lib/api/endpoints/tasks', () => ({ tasks: api }));
vi.mock('$lib/stores/sound.svelte', () => ({ soundStore: { playTaskStatus: vi.fn() } }));
vi.mock('$lib/stores/followUp.svelte', () => ({ followUpStore: { push: vi.fn() } }));

import TaskPage from './+page.svelte';

function task(overrides: Partial<Task> = {}): Task {
	return {
		id: 7,
		title: 'Plant tulips',
		description: '',
		inboxId: null,
		contextId: 1,
		projectId: null,
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
		isComplex: false,
		completedAt: null,
		recurrenceRule: null,
		sourceTaskId: null,
		postponeCount: 0,
		autoSortedAt: null,
		autoSortUndecidedAt: null,
		blockedByCount: 0,
		relationCount: 0,
		labels: [],
		url: '',
		createdAt: '2026-09-13T09:00:00.000Z',
		updatedAt: '2026-09-13T09:00:00.000Z',
		...overrides
	};
}

function blockedTask(blocker: Task, count = 1): Task {
	return task({
		title: 'Task A',
		blockedByCount: count,
		relationCount: 1,
		relations: [
			{ id: 1, type: 'blocks', direction: 'incoming', task: blocker, createdAt: blocker.createdAt }
		],
		subtasks: { items: [blocker], total: 1, limit: 0, offset: 0 }
	});
}

describe('task detail dependencies', () => {
	beforeEach(() => vi.resetAllMocks());

	it('refreshes blockers and relation peers on remote changes without changing the task revision', async () => {
		const blocker = task({ id: 8, title: 'Task B', parentId: 7 });
		api.get.mockResolvedValue(blockedTask(blocker));
		render(TaskPage);
		await waitFor(() => expect(screen.getByRole('button', { name: /blocked/i })).toBeDisabled());

		const completed = { ...blocker, status: 'completed' as const };
		api.get.mockResolvedValue(blockedTask(completed, 0));
		window.dispatchEvent(new CustomEvent('turboist:invalidate', { detail: { scope: 'tasks' } }));
		await waitFor(() =>
			expect(screen.getByRole('button', { name: 'Mark complete' })).toBeEnabled()
		);
		expect(screen.getAllByRole('link', { name: 'Task B' })[0]).toHaveClass('line-through');

		api.get.mockResolvedValue(blockedTask(blocker));
		window.dispatchEvent(new CustomEvent('turboist:invalidate', { detail: { scope: 'tasks' } }));
		await waitFor(() => expect(screen.getByRole('button', { name: /blocked/i })).toBeDisabled());
		expect(screen.getAllByRole('link', { name: 'Task B' })[0]).not.toHaveClass('line-through');
	});

	it('refreshes dependencies after completing and uncompleting a blocker on this screen', async () => {
		const blocker = task({ id: 8, title: 'Task B', parentId: 7 });
		const completed = { ...blocker, status: 'completed' as const };
		api.get.mockResolvedValue(blockedTask(blocker));
		api.complete.mockResolvedValue(completed);
		api.uncomplete.mockResolvedValue(blocker);
		render(TaskPage);
		await waitFor(() => expect(screen.getByRole('button', { name: /blocked/i })).toBeDisabled());

		api.get.mockResolvedValue(blockedTask(completed, 0));
		await fireEvent.click(screen.getByRole('button', { name: 'Mark complete' }));
		await waitFor(() => expect(api.complete).toHaveBeenCalledWith(expect.anything(), 8, undefined));
		await waitFor(() =>
			expect(screen.getByRole('button', { name: 'Mark complete' })).toBeEnabled()
		);
		expect(api.get).toHaveBeenCalledTimes(2);

		await fireEvent.click(screen.getByRole('button', { name: /completed/i }));
		api.get.mockResolvedValue(blockedTask(blocker));
		await fireEvent.click(screen.getByRole('button', { name: 'Mark incomplete' }));
		await waitFor(() => expect(screen.getByRole('button', { name: /blocked/i })).toBeDisabled());
		expect(api.get).toHaveBeenCalledTimes(3);
	});

	it('releases a sibling subtask when its blocker completes', async () => {
		const blocker = task({ id: 8, title: 'Task B', parentId: 7 });
		const dependent = task({ id: 9, title: 'Task A', parentId: 7, blockedByCount: 1 });
		const root = task({ subtasks: { items: [dependent, blocker], total: 2, limit: 0, offset: 0 } });
		api.get.mockResolvedValue(root);
		const completed = { ...blocker, status: 'completed' as const };
		api.complete.mockResolvedValue(completed);
		render(TaskPage);
		await waitFor(() => expect(screen.getByRole('button', { name: /blocked/i })).toBeDisabled());
		api.get.mockResolvedValue({
			...root,
			subtasks: { ...root.subtasks, items: [{ ...dependent, blockedByCount: 0 }, completed] }
		});
		await fireEvent.click(screen.getAllByRole('button', { name: 'Mark complete' })[1]);
		await waitFor(() => expect(screen.queryByRole('button', { name: /blocked/i })).toBeNull());
		expect(screen.getAllByRole('button', { name: 'Mark complete' })).toHaveLength(2);
	});

	it('does not autosave when only dependency data changes', async () => {
		const blocker = task({ id: 8, title: 'Task B', parentId: 7 });
		api.get.mockResolvedValue(blockedTask(blocker));
		render(TaskPage);
		await waitFor(() => expect(screen.getByRole('button', { name: /blocked/i })).toBeDisabled());
		await new Promise((resolve) => setTimeout(resolve, 0));
		vi.useFakeTimers();
		try {
			api.get.mockResolvedValue(blockedTask({ ...blocker, status: 'completed' }, 0));
			window.dispatchEvent(new CustomEvent('turboist:invalidate', { detail: { scope: 'tasks' } }));
			await tick();
			await vi.advanceTimersByTimeAsync(1600);
			expect(screen.getByRole('button', { name: 'Mark complete' })).toBeEnabled();
			expect(api.update).not.toHaveBeenCalled();
		} finally {
			vi.useRealTimers();
		}
	});
});

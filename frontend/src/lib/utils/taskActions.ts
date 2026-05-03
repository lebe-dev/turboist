import type { Task, TaskInput, TroikiCategory } from '$lib/api/types';
import { tasks as tasksApi } from '$lib/api/endpoints/tasks';
import { getApiClient } from '$lib/api/client';
import { ApiError } from '$lib/api/errors';
import { planStatsStore } from '$lib/stores/planStats.svelte';
import { pinnedTasksStore } from '$lib/stores/pinnedTasks.svelte';
import { troikiStore } from '$lib/stores/troiki.svelte';
import { toast } from 'svelte-sonner';

export function describeError(err: unknown, fallback: string): string {
	if (err instanceof ApiError) return err.message || fallback;
	if (err instanceof Error) return err.message;
	return fallback;
}

export interface ListMutator {
	replace(task: Task): void;
	remove(id: number): void;
	insertAfter?: (id: number, task: Task) => void;
}

export interface ToggleCompleteOptions {
	// On views that filter to open tasks (Today, Overdue, Backlog, …), a task
	// that just got completed is no longer in the result set, so drop it from
	// local state. On unfiltered views (Project, Context, Label, Search) it
	// must stay so the user can immediately uncomplete it.
	removeWhenCompleted?: boolean;
	// Predicate that decides whether the (still-open) updated task belongs in
	// the current view. Recurring tasks stay open after completion but get
	// their due_at advanced, which can move them out of date-bound views like
	// Today/Tomorrow/Overdue.
	belongs?: (task: Task) => boolean;
}

export async function toggleComplete(
	task: Task,
	mutator: ListMutator,
	options: ToggleCompleteOptions = {}
): Promise<void> {
	const { removeWhenCompleted = true, belongs } = options;
	const client = getApiClient();
	try {
		const updated =
			task.status === 'completed'
				? await tasksApi.uncomplete(client, task.id)
				: await tasksApi.complete(client, task.id);
		if (updated.status === 'completed' && removeWhenCompleted) mutator.remove(task.id);
		else if (updated.status !== 'completed' && belongs && !belongs(updated)) mutator.remove(task.id);
		else mutator.replace(updated);
	} catch (err) {
		toast.error(describeError(err, 'Failed to update task'));
	}
}

export async function togglePin(task: Task, mutator: ListMutator): Promise<void> {
	const client = getApiClient();
	const wasPin = task.isPinned;
	try {
		const updated = wasPin
			? await tasksApi.unpin(client, task.id)
			: await tasksApi.pin(client, task.id);
		mutator.replace(updated);
		if (wasPin) pinnedTasksStore.removeItem(task.id);
		else pinnedTasksStore.addItem(updated);
	} catch (err) {
		toast.error(describeError(err, 'Failed to pin task'));
	}
}

export async function deleteTask(task: Task, mutator: ListMutator): Promise<void> {
	const client = getApiClient();
	if (!confirm(`Delete "${task.title}"? Subtasks will also be removed.`)) return;
	try {
		await tasksApi.remove(client, task.id);
		mutator.remove(task.id);
	} catch (err) {
		toast.error(describeError(err, 'Failed to delete task'));
	}
}

export interface BelongsOption {
	belongs?: (task: Task) => boolean;
}

function applyUpdate(updated: Task, mutator: ListMutator, belongs?: (t: Task) => boolean): void {
	if (belongs && !belongs(updated)) mutator.remove(updated.id);
	else mutator.replace(updated);
}

export async function updateTaskFields(
	task: Task,
	mutator: ListMutator,
	patch: TaskInput,
	options: BelongsOption = {}
): Promise<void> {
	const client = getApiClient();
	try {
		const updated = await tasksApi.update(client, task.id, patch);
		applyUpdate(updated, mutator, options.belongs);
	} catch (err) {
		toast.error(describeError(err, 'Failed to update task'));
	}
}

export async function moveToBacklog(
	task: Task,
	mutator: ListMutator,
	options: BelongsOption = {}
): Promise<void> {
	const client = getApiClient();
	try {
		if (task.dueAt) {
			await tasksApi.update(client, task.id, { dueAt: null, dueHasTime: false });
		}
		const updated = await tasksApi.plan(client, task.id, { state: 'backlog' });
		applyUpdate(updated, mutator, options.belongs);
		void planStatsStore.load().catch(() => {});
	} catch (err) {
		toast.error(describeError(err, 'Failed to move to backlog'));
	}
}

export async function removeFromBacklog(
	task: Task,
	mutator: ListMutator,
	options: BelongsOption = {}
): Promise<void> {
	const client = getApiClient();
	try {
		const updated = await tasksApi.plan(client, task.id, { state: 'none' });
		applyUpdate(updated, mutator, options.belongs);
		void planStatsStore.load().catch(() => {});
	} catch (err) {
		toast.error(describeError(err, 'Failed to remove from backlog'));
	}
}

export async function duplicateTask(task: Task, mutator: ListMutator): Promise<void> {
	const client = getApiClient();
	try {
		const created = await tasksApi.duplicate(client, task.id);
		mutator.insertAfter?.(task.id, created);
		toast.success('Duplicated');
	} catch (err) {
		toast.error(describeError(err, 'Failed to duplicate task'));
	}
}

const TROIKI_LABEL: Record<TroikiCategory, string> = {
	important: 'Important',
	medium: 'Medium',
	rest: 'Rest'
};

export async function setTroikiCategory(
	task: Task,
	category: TroikiCategory | null,
	mutator: ListMutator,
	options: BelongsOption = {}
): Promise<void> {
	if (task.troikiCategory === category) return;
	const client = getApiClient();
	try {
		const updated = await tasksApi.setTroikiCategory(client, task.id, category);
		applyUpdate(updated, mutator, options.belongs);
		troikiStore.applyTaskUpdate(updated);
	} catch (err) {
		if (err instanceof ApiError && err.code === 'troiki_slot_full') {
			const label = category ? TROIKI_LABEL[category] : 'Troiki';
			toast.error(err.message || `${label} slot is full`);
			return;
		}
		toast.error(describeError(err, 'Failed to update Troiki category'));
	}
}

export async function copyTaskTitle(task: Task): Promise<void> {
	try {
		await navigator.clipboard.writeText(task.title);
		toast.success('Copied');
	} catch (err) {
		toast.error(describeError(err, 'Failed to copy'));
	}
}

import type { Task, TaskInput } from '$lib/api/types';
import { tasks as tasksApi } from '$lib/api/endpoints/tasks';
import { getApiClient } from '$lib/api/client';
import { ApiError } from '$lib/api/errors';
import { planStatsStore } from '$lib/stores/planStats.svelte';
import { pinnedTasksStore } from '$lib/stores/pinnedTasks.svelte';
import { followUpStore } from '$lib/stores/followUp.svelte';
import { toast } from 'svelte-sonner';
import { get } from 'svelte/store';
import { t } from '$lib/i18n';

function tr(key: string, values?: Record<string, string | number | Date>): string {
	return get(t)(key, values ? { values } : undefined);
}

export function describeError(err: unknown, fallback: string): string {
	if (err instanceof ApiError) return err.message || fallback;
	if (err instanceof Error) return err.message;
	return fallback;
}

export interface ListMutator {
	replace(task: Task): void;
	remove(id: number): void;
	insertAfter?: (id: number, task: Task) => void;
	add?: (task: Task) => void;
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
	const wasOpen = task.status !== 'completed';
	try {
		const updated = wasOpen
			? await tasksApi.complete(client, task.id)
			: await tasksApi.uncomplete(client, task.id);
		if (updated.status === 'completed' && removeWhenCompleted) mutator.remove(task.id);
		else if (updated.status !== 'completed' && belongs && !belongs(updated)) mutator.remove(task.id);
		else mutator.replace(updated);

		if (wasOpen && updated.status === 'completed' && !updated.recurrenceRule) {
			const undoFn = async () => {
				const restored = await tasksApi.uncomplete(client, task.id);
				if (removeWhenCompleted) {
					if (mutator.add) mutator.add(restored);
					else mutator.replace(restored);
				} else {
					mutator.replace(restored);
				}
			};
			followUpStore.push(updated, undoFn);
		}
	} catch (err) {
		toast.error(describeError(err, tr('task.toast.failedUpdate')));
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
		toast.error(describeError(err, tr('task.toast.failedPin')));
	}
}

export async function deleteTask(task: Task, mutator: ListMutator): Promise<void> {
	const client = getApiClient();
	if (!confirm(tr('task.toast.confirmDelete', { title: task.title }))) return;
	try {
		await tasksApi.remove(client, task.id);
		mutator.remove(task.id);
	} catch (err) {
		toast.error(describeError(err, tr('task.toast.failedDelete')));
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
		toast.error(describeError(err, tr('task.toast.failedUpdate')));
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
		toast.error(describeError(err, tr('task.toast.failedMoveToBacklog')));
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
		toast.error(describeError(err, tr('task.toast.failedRemoveFromBacklog')));
	}
}

export async function duplicateTask(task: Task, mutator: ListMutator): Promise<void> {
	const client = getApiClient();
	try {
		const created = await tasksApi.duplicate(client, task.id);
		mutator.insertAfter?.(task.id, created);
		toast.success(tr('task.toast.duplicated'));
	} catch (err) {
		toast.error(describeError(err, tr('task.toast.failedDuplicate')));
	}
}

export async function moveTaskToProject(
	task: Task,
	contextId: number,
	projectId: number,
	mutator: ListMutator,
	options: BelongsOption & { projectCompleted?: boolean } = {}
): Promise<void> {
	if (task.projectId === projectId) return;
	const client = getApiClient();
	try {
		let updated = await tasksApi.move(client, task.id, { contextId, projectId });
		if (options.projectCompleted && updated.status !== 'completed') {
			updated = await tasksApi.complete(client, updated.id);
		}
		applyUpdate(updated, mutator, options.belongs);
		toast.success(
			options.projectCompleted ? tr('task.toast.movedAndCompleted') : tr('task.toast.moved')
		);
	} catch (err) {
		toast.error(describeError(err, tr('task.toast.failedMove')));
	}
}

export async function decomposeTask(
	task: Task,
	titles: string[],
	mutator: ListMutator
): Promise<boolean> {
	const client = getApiClient();
	try {
		const res = await tasksApi.decompose(client, task.id, titles);
		mutator.remove(task.id);
		for (const t of res.created) {
			if (mutator.add) mutator.add(t);
			else mutator.replace(t);
		}
		toast.success(tr('task.toast.decomposed', { count: res.created.length }));
		return true;
	} catch (err) {
		toast.error(describeError(err, tr('task.toast.failedDecompose')));
		return false;
	}
}

export async function copyTaskTitle(task: Task): Promise<void> {
	try {
		await navigator.clipboard.writeText(task.title);
		toast.success(tr('task.toast.copied'));
	} catch (err) {
		toast.error(describeError(err, tr('task.toast.failedCopy')));
	}
}

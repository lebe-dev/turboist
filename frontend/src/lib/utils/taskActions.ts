import type { Task } from '$lib/api/types';
import { tasks as tasksApi } from '$lib/api/endpoints/tasks';
import { getApiClient } from '$lib/api/client';
import { ApiError } from '$lib/api/errors';
import { toast } from 'svelte-sonner';

export function describeError(err: unknown, fallback: string): string {
	if (err instanceof ApiError) return err.message || fallback;
	if (err instanceof Error) return err.message;
	return fallback;
}

export interface ListMutator {
	replace(task: Task): void;
	remove(id: number): void;
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
	try {
		const updated = task.isPinned
			? await tasksApi.unpin(client, task.id)
			: await tasksApi.pin(client, task.id);
		mutator.replace(updated);
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


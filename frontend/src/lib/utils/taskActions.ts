import type { Task, TaskInput } from '$lib/api/types';
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

export async function toggleComplete(task: Task, mutator: ListMutator): Promise<void> {
	const client = getApiClient();
	try {
		const updated =
			task.status === 'completed'
				? await tasksApi.uncomplete(client, task.id)
				: await tasksApi.complete(client, task.id);
		// completed tasks disappear from views; uncompleted reappears
		if (updated.status === 'completed') mutator.remove(task.id);
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

export async function saveEdit(
	id: number,
	payload: TaskInput,
	mutator: ListMutator
): Promise<void> {
	const client = getApiClient();
	try {
		const updated = await tasksApi.update(client, id, payload);
		mutator.replace(updated);
	} catch (err) {
		toast.error(describeError(err, 'Failed to save task'));
	}
}

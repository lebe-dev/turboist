import { toast } from 'svelte-sonner';
import type { Task } from '$lib/api/types';
import FollowUpToastCard from '$lib/components/task/FollowUpToastCard.svelte';

const AUTO_DISMISS_MS = 5000;

export interface FollowUpItem {
	task: Task;
}

let nextId = 1;

function createFollowUpStore() {
	function push(task: Task, undo: () => Promise<void>): void {
		if (task.recurrenceRule) return;
		const id = `follow-up-${nextId++}`;
		toast.custom(FollowUpToastCard, {
			id,
			duration: AUTO_DISMISS_MS,
			componentProps: { task, undo, toastId: id }
		});
	}

	return { push };
}

export const followUpStore = createFollowUpStore();

import { toast } from 'svelte-sonner';
import type { Task } from '$lib/api/types';
import FollowUpToastCard from '$lib/components/task/FollowUpToastCard.svelte';
import MoveFollowUpToastCard from '$lib/components/task/MoveFollowUpToastCard.svelte';

const AUTO_DISMISS_MS = 5000;
const MOVE_AUTO_DISMISS_MS = 7000;

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

	function pushMove(title: string, destination: string, undo: () => Promise<void>): void {
		const id = `follow-up-${nextId++}`;
		toast.custom(MoveFollowUpToastCard, {
			id,
			duration: MOVE_AUTO_DISMISS_MS,
			componentProps: { title, destination, undo, toastId: id }
		});
	}

	return { push, pushMove };
}

export const followUpStore = createFollowUpStore();

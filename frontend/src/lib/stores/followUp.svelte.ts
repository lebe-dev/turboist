import { SvelteMap } from 'svelte/reactivity';
import type { Task } from '$lib/api/types';

const AUTO_DISMISS_MS = 5000;

export interface FollowUpItem {
	id: number;
	task: Task;
	undo: () => Promise<void>;
}

function createFollowUpStore() {
	let items = $state<FollowUpItem[]>([]);
	let nextId = 1;
	const timers = new SvelteMap<number, ReturnType<typeof setTimeout>>();

	function dismiss(id: number): void {
		const timer = timers.get(id);
		if (timer) {
			clearTimeout(timer);
			timers.delete(id);
		}
		items = items.filter((item) => item.id !== id);
	}

	function push(task: Task, undo: () => Promise<void>): void {
		if (task.recurrenceRule) return;
		const id = nextId++;
		items = [...items, { id, task, undo }];
		const timer = setTimeout(() => dismiss(id), AUTO_DISMISS_MS);
		timers.set(id, timer);
	}

	return {
		get items() {
			return items;
		},
		push,
		dismiss
	};
}

export const followUpStore = createFollowUpStore();

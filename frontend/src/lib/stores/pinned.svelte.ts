import { patchState } from '$lib/api/client';
import type { PinnedTask } from '$lib/api/types';

export type { PinnedTask };

function createPinnedStore() {
	let items = $state<PinnedTask[]>([]);
	let maxPinned = $state(5);
	let _selectedTaskId = $state<string | null>(null);

	function init(tasks: PinnedTask[], max: number): void {
		items = tasks;
		maxPinned = max;
	}

	function pin(task: PinnedTask): void {
		if (items.some((t) => t.id === task.id)) return;
		if (items.length >= maxPinned) return;
		items = [...items, task];
		patchState({ pinned_tasks: items }).catch(console.error);
	}

	function unpin(taskId: string): void {
		items = items.filter((t) => t.id !== taskId);
		patchState({ pinned_tasks: items }).catch(console.error);
	}

	function isPinned(taskId: string): boolean {
		return items.some((t) => t.id === taskId);
	}

	function selectTask(taskId: string): void {
		_selectedTaskId = taskId;
	}

	function consumeSelection(): string | null {
		const id = _selectedTaskId;
		_selectedTaskId = null;
		return id;
	}

	return {
		get items() {
			return items;
		},
		get maxPinned() {
			return maxPinned;
		},
		get isFull() {
			return items.length >= maxPinned;
		},
		get selectedTaskId() {
			return _selectedTaskId;
		},
		pin,
		unpin,
		isPinned,
		init,
		selectTask,
		consumeSelection
	};
}

export const pinnedStore = createPinnedStore();

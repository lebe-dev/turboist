import type { Task } from '../api/types';

class PinnedTasksStore {
	items = $state<Task[]>([]);

	setItems(items: Task[]): void {
		this.items = items;
	}

	addItem(task: Task): void {
		if (!this.items.some((t) => t.id === task.id)) {
			this.items = [...this.items, task];
		}
	}

	removeItem(id: number): void {
		this.items = this.items.filter((t) => t.id !== id);
	}

	clear(): void {
		this.items = [];
	}
}

export const pinnedTasksStore = new PinnedTasksStore();

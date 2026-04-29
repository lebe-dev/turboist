import { views as viewsApi } from '../api/endpoints/views';
import { getApiClient } from '../api/client';
import type { Task } from '../api/types';

class PinnedTasksStore {
	items = $state<Task[]>([]);

	async load(): Promise<Task[]> {
		const res = await viewsApi.pinned(getApiClient());
		this.items = res.items;
		return res.items;
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

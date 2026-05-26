import { views as viewsApi } from '../api/endpoints/views';
import { getApiClient } from '../api/client';
import type { Task } from '../api/types';
import { cacheStoreValue, getCachedStoreValue } from '../offline/storeCache';

const CACHE_KEY = 'pinnedTasks';

class PinnedTasksStore {
	items = $state<Task[]>([]);

	async load(): Promise<Task[]> {
		const res = await viewsApi.pinned(getApiClient());
		this.items = res.items;
		void cacheStoreValue(CACHE_KEY, res.items).catch(() => undefined);
		return res.items;
	}

	async loadCached(): Promise<boolean> {
		const cached = await getCachedStoreValue<Task[]>(CACHE_KEY);
		if (!cached) return false;
		this.items = cached;
		return true;
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

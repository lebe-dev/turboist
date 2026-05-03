import { troiki as troikiApi } from '../api/endpoints/troiki';
import { getApiClient } from '../api/client';
import type { Task, TroikiCategory, TroikiViewResponse } from '../api/types';

const EMPTY: TroikiViewResponse = {
	important: { capacity: 3, tasks: [] },
	medium: { capacity: 0, tasks: [] },
	rest: { capacity: 0, tasks: [] },
	started: false
};

const CATEGORIES: TroikiCategory[] = ['important', 'medium', 'rest'];

class TroikiStore {
	value = $state<TroikiViewResponse>(EMPTY);

	async load(): Promise<TroikiViewResponse> {
		const v = await troikiApi.view(getApiClient());
		this.value = v;
		return v;
	}

	async start(): Promise<TroikiViewResponse> {
		const v = await troikiApi.start(getApiClient());
		this.value = v;
		return v;
	}

	clear(): void {
		this.value = EMPTY;
	}

	applyTaskUpdate(task: Task): void {
		const next: TroikiViewResponse = {
			important: {
				capacity: this.value.important.capacity,
				tasks: this.value.important.tasks.filter((t) => t.id !== task.id)
			},
			medium: {
				capacity: this.value.medium.capacity,
				tasks: this.value.medium.tasks.filter((t) => t.id !== task.id)
			},
			rest: {
				capacity: this.value.rest.capacity,
				tasks: this.value.rest.tasks.filter((t) => t.id !== task.id)
			},
			started: this.value.started
		};
		if (task.troikiCategory && task.status === 'open' && CATEGORIES.includes(task.troikiCategory)) {
			next[task.troikiCategory].tasks = [...next[task.troikiCategory].tasks, task];
		}
		this.value = next;
	}

	removeTask(id: number): void {
		this.value = {
			important: {
				capacity: this.value.important.capacity,
				tasks: this.value.important.tasks.filter((t) => t.id !== id)
			},
			medium: {
				capacity: this.value.medium.capacity,
				tasks: this.value.medium.tasks.filter((t) => t.id !== id)
			},
			rest: {
				capacity: this.value.rest.capacity,
				tasks: this.value.rest.tasks.filter((t) => t.id !== id)
			},
			started: this.value.started
		};
	}
}

export const troikiStore = new TroikiStore();

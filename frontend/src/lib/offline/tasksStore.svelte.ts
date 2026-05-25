import { getDB, type TurboistDB } from './db';
import { onDbChanged } from './stores';
import type { Task } from '$lib/api/types';

export interface OfflineTasksStoreOptions {
	filter?: (task: Task) => boolean;
	db?: TurboistDB;
}

export interface OfflineTasksStore {
	readonly items: Task[];
	readonly loaded: boolean;
	hydrate(): Promise<void>;
	dispose(): void;
}

export function createOfflineTasksStore(opts: OfflineTasksStoreOptions = {}): OfflineTasksStore {
	let items = $state<Task[]>([]);
	let loaded = $state(false);
	const db = opts.db ?? getDB();

	const hydrate = async (): Promise<void> => {
		const rows = await db.tasks.toArray();
		const next: Task[] = [];
		for (const row of rows) {
			if (row.deletedAt !== null) continue;
			const task = row.data as unknown as Task;
			if (opts.filter && !opts.filter(task)) continue;
			next.push(task);
		}
		items = next;
		loaded = true;
	};

	const unsubscribe = onDbChanged('tasks', () => {
		void hydrate();
	});

	return {
		get items() {
			return items;
		},
		get loaded() {
			return loaded;
		},
		hydrate,
		dispose: unsubscribe
	};
}

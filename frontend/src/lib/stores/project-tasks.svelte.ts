import { logger } from '$lib/stores/logger';
import { getTasks } from '$lib/api/client';
import type { Task } from '$lib/api/types';
import { persistTasks, loadPersistedTasks } from '$lib/state/index.svelte';
import { flattenTasks, buildTree, type FlatTask } from '$lib/state/types';
import { wsClient } from '$lib/ws/client.svelte';

const YDOC_KEY = 'projectTasks';

function createProjectTasksStore() {
	let flatTasks = $state<FlatTask[]>([]);
	let loaded = $state(false);
	let loading = $state(false);

	let cleanups: (() => void)[] = [];

	function loadFromCache(): void {
		const cached = loadPersistedTasks(YDOC_KEY);
		if (cached.length > 0) {
			flatTasks = cached;
			loaded = true;
			logger.log('project-tasks', `loaded ${cached.length} tasks from cache`);
		}
	}

	async function fetchFromServer(): Promise<void> {
		loading = true;
		try {
			const res = await getTasks();
			flatTasks = flattenTasks(res.tasks);
			persistTasks(YDOC_KEY, flatTasks);
			loaded = true;
			logger.log('project-tasks', `fetched ${flatTasks.length} tasks from server`);
		} catch (err) {
			logger.warn('project-tasks', `fetch failed: ${err}`);
		} finally {
			loading = false;
		}
	}

	async function start(): Promise<void> {
		loadFromCache();
		await fetchFromServer();

		cleanups.push(
			wsClient.onStateChange((connected) => {
				if (connected) {
					fetchFromServer();
				}
			})
		);
	}

	function stop(): void {
		for (const cleanup of cleanups) cleanup();
		cleanups = [];
	}

	function getProjectTasks(projectId: string): Task[] {
		const filtered = flatTasks.filter((t) => t.project_id === projectId);
		return buildTree(filtered);
	}

	return {
		get loaded() {
			return loaded;
		},
		get loading() {
			return loading;
		},
		start,
		stop,
		refresh: fetchFromServer,
		getProjectTasks
	};
}

export const projectTasksStore = createProjectTasksStore();

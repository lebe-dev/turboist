import { logger } from '$lib/stores/logger';
import { getTasks } from '$lib/api/client';
import type { Task } from '$lib/api/types';
import { flattenTasks, buildTree, type FlatTask } from '$lib/utils/task-tree';
import { wsClient } from '$lib/ws/client.svelte';

function createProjectTasksStore() {
	let flatTasks = $state<FlatTask[]>([]);
	let loaded = $state(false);
	let loading = $state(false);

	let cleanups: (() => void)[] = [];

	async function fetchFromServer(): Promise<void> {
		loading = true;
		try {
			const res = await getTasks();
			flatTasks = flattenTasks(res.tasks);
			loaded = true;
			logger.log('project-tasks', `fetched ${flatTasks.length} tasks from server`);
		} catch (err) {
			logger.warn('project-tasks', `fetch failed: ${err}`);
		} finally {
			loading = false;
		}
	}

	async function start(): Promise<void> {
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

	function removeTaskLocal(id: string): void {
		flatTasks = flatTasks.filter((t) => t.id !== id);
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
		getProjectTasks,
		removeTaskLocal
	};
}

export const projectTasksStore = createProjectTasksStore();

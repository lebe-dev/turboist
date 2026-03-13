import { getConfig, getTasks, getWeeklyTasks, getNextWeekTasks, getTodayTasks, getTomorrowTasks, getCompletedTasks } from '$lib/api/client';
import type { Config, Meta, Task } from '$lib/api/types';
import { contextsStore, type View } from './contexts.svelte';
import { createPoller, type Poller } from '$lib/utils/polling';

const DEFAULT_INTERVAL_MS = 30_000;

const STALE_THRESHOLD_MS = 2 * 60 * 1000; // 2 minutes

function createTasksStore() {
	let tasks = $state<Task[]>([]);
	let meta = $state<Meta>({ context: '', weekly_limit: 0, weekly_count: 0 });
	let config = $state<Config | null>(null);
	let loading = $state(false);
	let error = $state<string | null>(null);
	let isStale = $state(false);

	let poller: Poller | null = null;

	async function fetchTasks(): Promise<void> {
		const contextId = contextsStore.activeContextId ?? undefined;
		const view: View = contextsStore.activeView;

		const fetcherMap: Record<string, typeof getTasks> = {
			weekly: getWeeklyTasks,
			'next-week': getNextWeekTasks,
			today: getTodayTasks,
			tomorrow: getTomorrowTasks,
			completed: getCompletedTasks,
		};
		const fetcher = fetcherMap[view] ?? getTasks;

		const [res, cfg] = await Promise.all([fetcher(contextId), getConfig().catch(() => null)]);
		tasks = res.tasks;
		meta = res.meta;

		if (cfg) {
			config = cfg;
			isStale = cfg.last_synced_at
				? Date.now() - new Date(cfg.last_synced_at).getTime() > STALE_THRESHOLD_MS
				: false;
		} else {
			isStale = false;
		}
	}

	async function start(): Promise<void> {
		loading = true;
		error = null;

		// Получаем poll_interval из конфига
		let intervalMs = DEFAULT_INTERVAL_MS;
		try {
			const cfg = await getConfig();
			console.log('[config] loaded from API', cfg);
			const parsed = cfg.poll_interval * 1000;
			if (Number.isFinite(parsed) && parsed >= 1000) {
				intervalMs = parsed;
			}
		} catch {
			// fallback to default
		}

		poller = createPoller({
			interval: intervalMs,
			fn: fetchTasks,
			onError: (err) => {
				error = err instanceof Error ? err.message : String(err);
			}
		});

		poller.start();
		loading = false;
	}

	function stop(): void {
		poller?.stop();
		poller = null;
	}

	/** Перезапустить polling (при смене контекста/вида) */
	function refresh(): void {
		fetchTasks().catch((err) => {
			error = err instanceof Error ? err.message : String(err);
		});
	}

	/** Optimistically update a task's fields in the local store. */
	function updateTaskLocal(taskId: string, updater: (task: Task) => Task): void {
		function walk(list: Task[]): Task[] {
			return list.map((t) => {
				const updated = t.id === taskId ? updater(t) : t;
				if (updated.children.length > 0) {
					return { ...updated, children: walk(updated.children) };
				}
				return updated;
			});
		}
		tasks = walk(tasks);
	}

	return {
		get tasks() {
			return tasks;
		},
		get meta() {
			return meta;
		},
		get config() {
			return config;
		},
		get loading() {
			return loading;
		},
		get error() {
			return error;
		},
		get isStale() {
			return isStale;
		},
		start,
		stop,
		refresh,
		updateTaskLocal
	};
}

export const tasksStore = createTasksStore();

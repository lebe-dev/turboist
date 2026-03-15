import { getBacklogTasks, getWeeklyTasks, getAppConfig, updateTask, resetWeeklyLabel, patchState } from '$lib/api/client';
import type { Config, Meta, Task } from '$lib/api/types';
import { contextsStore } from './contexts.svelte';
import { createPoller, type Poller } from '$lib/utils/polling';

const DEFAULT_INTERVAL_MS = 30_000;

function createPlanningStore() {
	let active = $state(false);
	let backlogTasks = $state<Task[]>([]);
	let weeklyTasks = $state<Task[]>([]);
	let meta = $state<Meta>({ context: '', weekly_limit: 0, weekly_count: 0, backlog_limit: 0, backlog_count: 0 });
	let config = $state<Config | null>(null);
	let loading = $state(false);
	let mobileTab = $state<'backlog' | 'weekly'>('backlog');

	let poller: Poller | null = null;

	function initActive(isActive: boolean): void {
		active = isActive;
	}

	async function fetchBoth(): Promise<void> {
		const contextId = contextsStore.activeContextId ?? undefined;
		const [backlogRes, weeklyRes, cfg] = await Promise.all([
			getBacklogTasks(contextId),
			getWeeklyTasks(), // no context filter for weekly panel
			getAppConfig()
				.then((c) => c.settings)
				.catch(() => null)
		]);

		backlogTasks = backlogRes.tasks;
		meta = backlogRes.meta;
		weeklyTasks = weeklyRes.tasks;

		if (cfg) {
			config = cfg;
		}
	}

	async function enter(): Promise<void> {
		active = true;
		patchState({ planning_open: true }).catch(console.error);
		loading = true;

		try {
			const appCfg = await getAppConfig();
			config = appCfg.settings;

			let intervalMs = DEFAULT_INTERVAL_MS;
			const parsed = appCfg.settings.poll_interval * 1000;
			if (Number.isFinite(parsed) && parsed >= 1000) {
				intervalMs = parsed;
			}

			await fetchBoth();

			poller = createPoller({
				interval: intervalMs,
				fn: fetchBoth,
				onError: (err) => {
					console.error('[planning] poll error', err);
				}
			});
			poller.start();
		} catch (err) {
			console.error('[planning] enter failed', err);
		} finally {
			loading = false;
		}
	}

	function exit(): void {
		active = false;
		patchState({ planning_open: false }).catch(console.error);
		poller?.stop();
		poller = null;
		backlogTasks = [];
		weeklyTasks = [];
	}

	async function refresh(): Promise<void> {
		try {
			await fetchBoth();
		} catch (err) {
			console.error('[planning] refresh failed', err);
		}
	}

	function isAtLimit(): boolean {
		return meta.weekly_limit > 0 && meta.weekly_count >= meta.weekly_limit;
	}

	async function moveToWeekly(task: Task): Promise<void> {
		if (isAtLimit()) return;
		if (!config) return;

		const weeklyLabel = config.weekly_label;
		const backlogLabel = config.backlog_label;

		// Optimistic: remove from backlog, add to weekly
		backlogTasks = backlogTasks.filter((t) => t.id !== task.id);
		const newLabels = task.labels.filter((l) => l !== backlogLabel);
		if (!newLabels.includes(weeklyLabel)) {
			newLabels.push(weeklyLabel);
		}
		const movedTask = { ...task, labels: newLabels };
		weeklyTasks = [...weeklyTasks, movedTask];
		meta = { ...meta, weekly_count: meta.weekly_count + 1 };

		try {
			await updateTask(task.id, { labels: newLabels });
			await refresh();
		} catch (err) {
			console.error('[planning] moveToWeekly failed', err);
			await refresh();
		}
	}

	async function moveToBacklog(task: Task): Promise<void> {
		if (!config) return;

		const weeklyLabel = config.weekly_label;

		// Optimistic: remove from weekly, decrement count
		weeklyTasks = weeklyTasks.filter((t) => t.id !== task.id);
		meta = { ...meta, weekly_count: Math.max(0, meta.weekly_count - 1) };

		const newLabels = task.labels.filter((l) => l !== weeklyLabel);

		try {
			await updateTask(task.id, { labels: newLabels });
			await refresh();
		} catch (err) {
			console.error('[planning] moveToBacklog failed', err);
			await refresh();
		}
	}

	async function startWeek(): Promise<void> {
		weeklyTasks = [];
		meta = { ...meta, weekly_count: 0 };

		try {
			await resetWeeklyLabel();
			await refresh();
		} catch (err) {
			console.error('[planning] startWeek failed', err);
			await refresh();
		}
	}

	async function updateWeeklyTask(
		taskId: string,
		data: { priority?: number; due_date?: string }
	): Promise<void> {
		// Optimistic update
		weeklyTasks = weeklyTasks.map((t) => {
			if (t.id !== taskId) return t;
			const updated = { ...t };
			if (data.priority !== undefined) {
				updated.priority = data.priority;
			}
			if (data.due_date !== undefined) {
				if (data.due_date === '') {
					updated.due = null;
				} else {
					updated.due = { date: data.due_date, recurring: false };
				}
			}
			return updated;
		});

		try {
			await updateTask(taskId, data);
			await refresh();
		} catch (err) {
			console.error('[planning] updateWeeklyTask failed', err);
			await refresh();
		}
	}

	return {
		get active() {
			return active;
		},
		get backlogTasks() {
			return backlogTasks;
		},
		get weeklyTasks() {
			return weeklyTasks;
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
		get mobileTab() {
			return mobileTab;
		},
		set mobileTab(v: 'backlog' | 'weekly') {
			mobileTab = v;
		},
		get isAtLimit() {
			return isAtLimit();
		},
		initActive,
		enter,
		exit,
		refresh,
		moveToWeekly,
		moveToBacklog,
		startWeek,
		updateWeeklyTask
	};
}

export const planningStore = createPlanningStore();

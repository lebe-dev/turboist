import { getAppConfig, updateTask, resetWeeklyLabel, patchState } from '$lib/api/client';
import type { Config, Meta, Task } from '$lib/api/types';
import { contextsStore } from './contexts.svelte';
import {
	wsClient,
	type SnapshotPlanningData,
	type DeltaPlanningData
} from '$lib/ws/client.svelte';
import { mergeUpserted, filterByIds } from '$lib/ws/merge';

function createPlanningStore() {
	let active = $state(false);
	let backlogTasks = $state<Task[]>([]);
	let weeklyTasks = $state<Task[]>([]);
	let meta = $state<Meta>({
		context: '',
		weekly_limit: 0,
		weekly_count: 0,
		backlog_limit: 0,
		backlog_count: 0
	});
	let config = $state<Config | null>(null);
	let loading = $state(false);
	let mobileTab = $state<'backlog' | 'weekly'>('backlog');

	let cleanups: (() => void)[] = [];

	function initActive(isActive: boolean): void {
		active = isActive;
	}

	function handlePlanningSnapshot(data: unknown): void {
		const d = data as SnapshotPlanningData;
		backlogTasks = d.backlog;
		weeklyTasks = d.weekly;
		meta = d.meta;
		loading = false;
	}

	function handlePlanningDelta(data: unknown): void {
		const d = data as DeltaPlanningData;

		if (d.backlog_removed?.length) {
			backlogTasks = filterByIds(backlogTasks, d.backlog_removed);
		}
		if (d.backlog_upserted?.length) {
			backlogTasks = mergeUpserted(backlogTasks, d.backlog_upserted);
		}

		if (d.weekly_removed?.length) {
			weeklyTasks = filterByIds(weeklyTasks, d.weekly_removed);
		}
		if (d.weekly_upserted?.length) {
			weeklyTasks = mergeUpserted(weeklyTasks, d.weekly_upserted);
		}

		if (d.meta) {
			meta = d.meta;
		}
	}

	async function enter(): Promise<void> {
		active = true;
		patchState({ planning_open: true }).catch(console.error);
		loading = true;

		try {
			const appCfg = await getAppConfig();
			config = appCfg.settings;
		} catch (err) {
			console.error('[planning] config load failed', err);
		}

		// Register WS handlers
		cleanups.push(wsClient.onMessage('snapshot', 'planning', handlePlanningSnapshot));
		cleanups.push(wsClient.onMessage('delta', 'planning', handlePlanningDelta));

		const contextId = contextsStore.activeContextId ?? undefined;
		wsClient.subscribe('planning', { context: contextId });
	}

	function exit(): void {
		active = false;
		patchState({ planning_open: false }).catch(console.error);

		for (const cleanup of cleanups) cleanup();
		cleanups = [];
		wsClient.unsubscribe('planning');

		backlogTasks = [];
		weeklyTasks = [];
	}

	function refresh(): void {
		const contextId = contextsStore.activeContextId ?? undefined;
		wsClient.subscribe('planning', { context: contextId });
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
		} catch (err) {
			console.error('[planning] moveToWeekly failed', err);
			refresh();
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
		} catch (err) {
			console.error('[planning] moveToBacklog failed', err);
			refresh();
		}
	}

	async function startWeek(): Promise<void> {
		weeklyTasks = [];
		meta = { ...meta, weekly_count: 0 };

		try {
			await resetWeeklyLabel();
		} catch (err) {
			console.error('[planning] startWeek failed', err);
			refresh();
		}
	}

	async function acceptAll(): Promise<void> {
		if (!config) return;

		const weeklyLabel = config.weekly_label;
		const backlogLabel = config.backlog_label;
		const tasksToMove = [...backlogTasks];
		if (tasksToMove.length === 0) return;

		// Optimistic: move all backlog tasks to weekly
		const movedTasks = tasksToMove.map((task) => {
			const newLabels = task.labels.filter((l) => l !== backlogLabel);
			if (!newLabels.includes(weeklyLabel)) {
				newLabels.push(weeklyLabel);
			}
			return { ...task, labels: newLabels };
		});
		backlogTasks = [];
		weeklyTasks = [...weeklyTasks, ...movedTasks];
		meta = { ...meta, weekly_count: meta.weekly_count + tasksToMove.length };

		try {
			await Promise.all(
				tasksToMove.map((task) => {
					const newLabels = task.labels.filter((l) => l !== backlogLabel);
					if (!newLabels.includes(weeklyLabel)) {
						newLabels.push(weeklyLabel);
					}
					return updateTask(task.id, { labels: newLabels });
				})
			);
		} catch (err) {
			console.error('[planning] acceptAll failed', err);
		}
		// Cache refresh → hub broadcast → delta will sync automatically
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
		} catch (err) {
			console.error('[planning] updateWeeklyTask failed', err);
			refresh();
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
		acceptAll,
		updateWeeklyTask
	};
}

export const planningStore = createPlanningStore();

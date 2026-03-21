import { logger } from '$lib/stores/logger';
import { getAppConfig, updateTask, resetWeeklyLabel, patchState } from '$lib/api/client';
import type { Config, Meta, Task } from '$lib/api/types';
import { contextsStore } from './contexts.svelte';
import {
	wsClient,
	type SnapshotPlanningData,
	type DeltaPlanningData
} from '$lib/ws/client.svelte';
import { mergeUpserted, filterByIds } from '$lib/ws/merge';
import { isStateReady, persistTasks, persistMeta, loadPersistedTasks, loadPersistedMeta } from '$lib/state/index.svelte';
import { flattenTasks, buildTree, taskToFlat, type FlatTask } from '$lib/state/types';

function createPlanningStore() {
	let active = $state(false);
	let backlogFlat = $state<FlatTask[]>([]);
	let weeklyFlat = $state<FlatTask[]>([]);
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
		backlogFlat = flattenTasks(d.backlog);
		weeklyFlat = flattenTasks(d.weekly);
		meta = d.meta;
		loading = false;

		persistTasks('backlogTasks', backlogFlat);
		persistTasks('weeklyTasks', weeklyFlat);
		persistMeta('planningMeta', d.meta);
	}

	function handlePlanningDelta(data: unknown): void {
		const d = data as DeltaPlanningData;

		// Backlog updates
		let backlogTree = buildTree(backlogFlat);
		if (d.backlog_removed?.length) {
			backlogTree = filterByIds(backlogTree, d.backlog_removed);
		}
		if (d.backlog_upserted?.length) {
			backlogTree = mergeUpserted(backlogTree, d.backlog_upserted);
		}
		backlogFlat = flattenTasks(backlogTree);

		// Weekly updates
		let weeklyTree = buildTree(weeklyFlat);
		if (d.weekly_removed?.length) {
			weeklyTree = filterByIds(weeklyTree, d.weekly_removed);
		}
		if (d.weekly_upserted?.length) {
			weeklyTree = mergeUpserted(weeklyTree, d.weekly_upserted);
		}
		weeklyFlat = flattenTasks(weeklyTree);

		if (d.meta) {
			meta = d.meta;
		}

		persistTasks('backlogTasks', backlogFlat);
		persistTasks('weeklyTasks', weeklyFlat);
		if (d.meta) persistMeta('planningMeta', d.meta);
	}

	async function enter(): Promise<void> {
		active = true;
		logger.log('planning', 'entering planning mode');
		patchState({ planning_open: true }).catch((err) =>
			logger.error('planning', `enter save failed: ${err}`)
		);
		loading = true;

		try {
			const appCfg = await getAppConfig();
			config = appCfg.settings;
		} catch (err) {
			logger.error('planning', `config load failed: ${err}`);
		}

		cleanups.push(wsClient.onMessage('snapshot', 'planning', handlePlanningSnapshot));
		cleanups.push(wsClient.onMessage('delta', 'planning', handlePlanningDelta));

		const contextId = contextsStore.activeContextId ?? undefined;
		wsClient.subscribe('planning', { context: contextId });
	}

	function exit(): void {
		active = false;
		logger.log('planning', 'exiting planning mode');
		patchState({ planning_open: false }).catch((err) =>
			logger.error('planning', `exit save failed: ${err}`)
		);

		for (const cleanup of cleanups) cleanup();
		cleanups = [];
		wsClient.unsubscribe('planning');

		backlogFlat = [];
		weeklyFlat = [];
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

		// Optimistic
		backlogFlat = backlogFlat.filter((t) => t.id !== task.id);
		const newLabels = task.labels.filter((l) => l !== backlogLabel);
		if (!newLabels.includes(weeklyLabel)) newLabels.push(weeklyLabel);
		const movedTask = { ...task, labels: newLabels };
		weeklyFlat = [...weeklyFlat, taskToFlat(movedTask)];
		meta = { ...meta, weekly_count: meta.weekly_count + 1 };

		try {
			await updateTask(task.id, { labels: newLabels });
		} catch (err) {
			logger.error('planning', `moveToWeekly failed: ${err}`);
			refresh();
		}
	}

	async function moveToBacklog(task: Task): Promise<void> {
		if (!config) return;

		const weeklyLabel = config.weekly_label;

		weeklyFlat = weeklyFlat.filter((t) => t.id !== task.id);
		meta = { ...meta, weekly_count: Math.max(0, meta.weekly_count - 1) };

		const newLabels = task.labels.filter((l) => l !== weeklyLabel);

		try {
			await updateTask(task.id, { labels: newLabels });
		} catch (err) {
			logger.error('planning', `moveToBacklog failed: ${err}`);
			refresh();
		}
	}

	async function startWeek(): Promise<void> {
		weeklyFlat = [];
		meta = { ...meta, weekly_count: 0 };

		try {
			await resetWeeklyLabel();
		} catch (err) {
			logger.error('planning', `startWeek failed: ${err}`);
			refresh();
			throw err;
		}
	}

	async function acceptAll(): Promise<void> {
		if (!config) return;

		const weeklyLabel = config.weekly_label;
		const backlogLabel = config.backlog_label;
		const tasksToMove = buildTree(backlogFlat);
		if (tasksToMove.length === 0) return;

		const movedTasks = tasksToMove.map((task) => {
			const newLabels = task.labels.filter((l) => l !== backlogLabel);
			if (!newLabels.includes(weeklyLabel)) newLabels.push(weeklyLabel);
			return { ...task, labels: newLabels };
		});
		backlogFlat = [];
		weeklyFlat = [...weeklyFlat, ...flattenTasks(movedTasks)];
		meta = { ...meta, weekly_count: meta.weekly_count + tasksToMove.length };

		try {
			await Promise.all(
				tasksToMove.map((task) => {
					const newLabels = task.labels.filter((l) => l !== backlogLabel);
					if (!newLabels.includes(weeklyLabel)) newLabels.push(weeklyLabel);
					return updateTask(task.id, { labels: newLabels });
				})
			);
		} catch (err) {
			logger.error('planning', `acceptAll failed: ${err}`);
		}
	}

	async function updateWeeklyTask(
		taskId: string,
		data: { priority?: number; due_date?: string }
	): Promise<void> {
		weeklyFlat = weeklyFlat.map((t) => {
			if (t.id !== taskId) return t;
			const updated = { ...t };
			if (data.priority !== undefined) updated.priority = data.priority;
			if (data.due_date !== undefined) {
				updated.due_date = data.due_date === '' ? null : data.due_date;
				updated.due_recurring = false;
			}
			return updated;
		});

		try {
			await updateTask(taskId, data);
		} catch (err) {
			logger.error('planning', `updateWeeklyTask failed: ${err}`);
			refresh();
		}
	}

	return {
		get active() {
			return active;
		},
		get backlogTasks(): Task[] {
			return buildTree(backlogFlat);
		},
		get weeklyTasks(): Task[] {
			return buildTree(weeklyFlat);
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

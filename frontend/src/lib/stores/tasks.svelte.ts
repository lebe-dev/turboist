import { getAppConfig, getCompletedTasks } from '$lib/api/client';
import type { Config, Meta, Task } from '$lib/api/types';
import { contextsStore, type View } from './contexts.svelte';
import {
	wsClient,
	type SnapshotTasksData,
	type DeltaTasksData
} from '$lib/ws/client.svelte';
import { mergeUpserted, filterByIds } from '$lib/ws/merge';
import { loadTaskSnapshot, loadCompletedTasks, saveCompletedTasks } from '$lib/sync/db';
import { writeSnapshotImmediate, scheduleSnapshotWrite } from '$lib/sync/snapshot-writer';

const STALE_THRESHOLD_MS = 2 * 60 * 1000; // 2 minutes

function createTasksStore() {
	let tasks = $state<Task[]>([]);
	let meta = $state<Meta>({
		context: '',
		weekly_limit: 0,
		weekly_count: 0,
		backlog_limit: 0,
		backlog_count: 0
	});
	let config = $state<Config | null>(null);
	let loading = $state(false);
	let error = $state<string | null>(null);
	let isStale = $state(false);
	let isOffline = $state(false);

	// IDs of tasks optimistically removed — survives fetches until server catches up
	const pendingRemovals = new Set<string>();

	let cleanups: (() => void)[] = [];

	function updateStale(lastSyncedAt?: string): void {
		isStale = lastSyncedAt
			? Date.now() - new Date(lastSyncedAt).getTime() > STALE_THRESHOLD_MS
			: false;
	}

	function applyPendingRemovals(taskList: Task[]): Task[] {
		if (pendingRemovals.size === 0) return taskList;

		// Auto-clear removals the server has caught up with
		function hasId(list: Task[], id: string): boolean {
			return list.some((t) => t.id === id || hasId(t.children, id));
		}
		for (const id of [...pendingRemovals]) {
			if (!hasId(taskList, id)) pendingRemovals.delete(id);
		}

		if (pendingRemovals.size === 0) return taskList;

		function filterPending(list: Task[]): Task[] {
			return list.flatMap((t) => {
				if (pendingRemovals.has(t.id)) return [];
				return [{ ...t, children: filterPending(t.children) }];
			});
		}
		return filterPending(taskList);
	}

	function currentView(): View {
		return contextsStore.activeView;
	}

	function currentContextId(): string | undefined {
		return contextsStore.activeContextId ?? undefined;
	}

	function handleTasksSnapshot(data: unknown): void {
		const d = data as SnapshotTasksData;
		tasks = applyPendingRemovals(d.tasks);
		meta = d.meta;
		loading = false;
		error = null;
		isOffline = false;
		updateStale(d.meta.last_synced_at);

		// Write-behind to IDB (immediate for snapshots)
		writeSnapshotImmediate(currentView(), currentContextId(), d.tasks, d.meta);
	}

	function handleTasksDelta(data: unknown): void {
		const d = data as DeltaTasksData;
		let updated = tasks;
		if (d.removed?.length > 0) {
			updated = filterByIds(updated, d.removed);
		}
		if (d.upserted?.length > 0) {
			updated = mergeUpserted(updated, d.upserted);
		}
		tasks = applyPendingRemovals(updated);
		if (d.meta) {
			meta = d.meta;
			updateStale(d.meta.last_synced_at);
		}

		// Debounced write-behind to IDB for deltas
		scheduleSnapshotWrite(currentView(), currentContextId(), tasks, meta);
	}

	async function loadFromIDB(): Promise<boolean> {
		const view = currentView();
		const contextId = currentContextId();

		if (view === 'completed') {
			const cached = await loadCompletedTasks(contextId);
			if (cached) {
				tasks = cached.tasks;
				isStale = true;
				isOffline = true;
				loading = false;
				return true;
			}
			return false;
		}

		const cached = await loadTaskSnapshot(view, contextId);
		if (cached) {
			tasks = cached.tasks;
			meta = cached.meta;
			isStale = true;
			isOffline = true;
			loading = false;
			return true;
		}
		return false;
	}

	function subscribeWS(): void {
		const contextId = currentContextId();
		const view = currentView();

		// Completed view uses HTTP fetch, not WS
		if (view === 'completed') {
			fetchCompleted(contextId);
			return;
		}

		wsClient.subscribe('tasks', { view, context: contextId });
	}

	async function fetchCompleted(_context?: string): Promise<void> {
		loading = true;
		try {
			const res = await getCompletedTasks();
			tasks = res.tasks;
			meta = res.meta;
			error = null;
			isOffline = false;

			// Cache to IDB
			saveCompletedTasks(currentContextId(), res.tasks).catch(console.error);
		} catch (err) {
			// Fallback to IDB cache on network failure
			const cached = await loadCompletedTasks(currentContextId());
			if (cached) {
				tasks = cached.tasks;
				isStale = true;
				isOffline = true;
				error = null;
			} else {
				error = err instanceof Error ? err.message : String(err);
			}
		} finally {
			loading = false;
		}
	}

	async function start(): Promise<void> {
		loading = true;
		error = null;

		// Load config once
		try {
			const cfg = await getAppConfig();
			config = cfg.settings;
		} catch {
			// Config fetch is best-effort
		}

		// Register WS handlers
		cleanups.push(wsClient.onMessage('snapshot', 'tasks', handleTasksSnapshot));
		cleanups.push(wsClient.onMessage('delta', 'tasks', handleTasksDelta));

		// Try loading from IDB first for instant display while WS connects
		await loadFromIDB();

		subscribeWS();
	}

	function stop(): void {
		for (const cleanup of cleanups) cleanup();
		cleanups = [];
		wsClient.unsubscribe('tasks');
	}

	// Refresh: re-subscribe to get a fresh snapshot from the server.
	function refresh(): Promise<void> {
		subscribeWS();
		return Promise.resolve();
	}

	// Clear tasks, show loading spinner, and re-subscribe (for view transitions).
	async function refreshWithLoading(): Promise<void> {
		tasks = [];
		loading = true;
		error = null;

		// Try IDB for instant view switch
		const hadCache = await loadFromIDB();
		if (hadCache) {
			// Still subscribe to get fresh data
			subscribeWS();
			return;
		}

		subscribeWS();
	}

	// Optimistic local mutations
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

	function removeTaskLocal(taskId: string): void {
		pendingRemovals.add(taskId);
		function walk(list: Task[]): Task[] {
			return list.flatMap((t) => {
				if (t.id === taskId) return [];
				return [{ ...t, children: walk(t.children) }];
			});
		}
		tasks = walk(tasks);
	}

	function clearPendingRemoval(taskId: string): void {
		pendingRemovals.delete(taskId);
	}

	function addTaskLocal(task: Task): void {
		tasks = [task, ...tasks];
	}

	function insertAfterLocal(siblingId: string, newTask: Task): void {
		function walk(list: Task[]): Task[] {
			const result: Task[] = [];
			for (const t of list) {
				const updated = { ...t, children: walk(t.children) };
				result.push(updated);
				if (t.id === siblingId) result.push(newTask);
			}
			return result;
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
		get isOffline() {
			return isOffline;
		},
		start,
		stop,
		refresh,
		refreshWithLoading,
		updateTaskLocal,
		removeTaskLocal,
		clearPendingRemoval,
		addTaskLocal,
		insertAfterLocal
	};
}

export const tasksStore = createTasksStore();

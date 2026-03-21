import { logger } from '$lib/stores/logger';
import { getAppConfig, getCompletedTasks } from '$lib/api/client';
import { actionQueue } from '$lib/sync/action-queue.svelte';
import type { Config, Meta, Task, UpdateTaskRequest } from '$lib/api/types';
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
const OFFLINE_GRACE_MS = 5000; // grace period before showing offline banner

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
	let running = false;
	let hasReceivedSnapshot = false;
	let offlineTimer: ReturnType<typeof setTimeout> | null = null;

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

	// Collect pending updateTask payloads from the action queue into a lookup map.
	function pendingUpdateMap(): Map<string, UpdateTaskRequest> | null {
		const pending = actionQueue.items.filter(
			(a) => a.type === 'updateTask' && (a.status === 'pending' || a.status === 'processing')
		);
		if (pending.length === 0) return null;

		const map = new Map<string, UpdateTaskRequest>();
		for (const action of pending) {
			const { id, data } = action.payload as { id: string; data: UpdateTaskRequest };
			const existing = map.get(id);
			map.set(id, existing ? { ...existing, ...data } : data);
		}
		return map;
	}

	function overlayUpdate(task: Task, update: UpdateTaskRequest): Task {
		const result = { ...task };
		if (update.content !== undefined) result.content = update.content;
		if (update.description !== undefined) result.description = update.description;
		if (update.labels !== undefined) result.labels = update.labels;
		if (update.priority !== undefined) result.priority = update.priority;
		if (update.due_date !== undefined) {
			result.due = update.due_date === '' ? null : { date: update.due_date, recurring: false };
		}
		return result;
	}

	// Re-apply pending updateTask mutations on top of server data so
	// optimistic edits survive WS snapshots / deltas / IDB loads.
	function walkWithUpdates(list: Task[], updates: Map<string, UpdateTaskRequest>): Task[] {
		return list.map((t) => {
			const update = updates.get(t.id);
			const result = update ? overlayUpdate(t, update) : t;
			if (t.children.length > 0) {
				return { ...result, children: walkWithUpdates(t.children, updates) };
			}
			return result;
		});
	}

	function applyPendingUpdates(taskList: Task[]): Task[] {
		const map = pendingUpdateMap();
		if (!map) return taskList;
		return walkWithUpdates(taskList, map);
	}

	// Apply pending queue mutations to a single task tree (for TaskDetailPanel).
	function applyPendingTaskUpdate(task: Task): Task {
		const map = pendingUpdateMap();
		if (!map) return task;
		return walkWithUpdates([task], map)[0];
	}

	function currentView(): View {
		return contextsStore.activeView;
	}

	function currentContextId(): string | undefined {
		return contextsStore.activeContextId ?? undefined;
	}

	function handleTasksSnapshot(data: unknown): void {
		const d = data as SnapshotTasksData;
		logger.log('tasks', `snapshot received: ${d.tasks.length} tasks, synced at: ${d.meta.last_synced_at}`);
		hasReceivedSnapshot = true;
		cancelOfflineTimer();
		tasks = applyPendingUpdates(applyPendingRemovals(d.tasks));
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
		logger.log('tasks', `delta: upserted=${d.upserted?.length ?? 0} removed=${d.removed?.length ?? 0}`);
		let updated = tasks;
		if (d.removed?.length > 0) {
			updated = filterByIds(updated, d.removed);
		}
		if (d.upserted?.length > 0) {
			updated = mergeUpserted(updated, d.upserted);
		}
		tasks = applyPendingUpdates(applyPendingRemovals(updated));
		if (d.meta) {
			meta = d.meta;
			updateStale(d.meta.last_synced_at);
		}

		// Debounced write-behind to IDB for deltas (unwrap $state proxies for structured clone)
		scheduleSnapshotWrite(currentView(), currentContextId(), $state.snapshot(tasks), $state.snapshot(meta));
	}

	// Register WS handlers once — these are stable for the store's lifetime
	wsClient.onMessage('snapshot', 'tasks', handleTasksSnapshot);
	wsClient.onMessage('delta', 'tasks', handleTasksDelta);

	function cancelOfflineTimer(): void {
		if (offlineTimer) {
			clearTimeout(offlineTimer);
			offlineTimer = null;
		}
	}

	function scheduleOfflineCheck(): void {
		cancelOfflineTimer();
		offlineTimer = setTimeout(() => {
			offlineTimer = null;
			if (!hasReceivedSnapshot && !wsClient.connected) {
				logger.warn('tasks', 'no snapshot received within grace period, marking offline');
				isOffline = true;
				isStale = true;
			}
		}, OFFLINE_GRACE_MS);
	}

	async function loadFromIDB(): Promise<boolean> {
		const view = currentView();
		const contextId = currentContextId();

		if (view === 'completed') {
			const cached = await loadCompletedTasks(contextId);
			if (cached) {
				logger.log('tasks', `IDB cache hit (completed): ${cached.tasks.length} tasks`);
				tasks = applyPendingUpdates(cached.tasks);
				loading = false;
				return true;
			}
			logger.log('tasks', 'IDB cache miss (completed)');
			return false;
		}

		const cached = await loadTaskSnapshot(view, contextId);
		if (cached) {
			logger.log('tasks', `IDB cache hit: ${cached.tasks.length} tasks for ${view}`);
			tasks = applyPendingUpdates(cached.tasks);
			meta = cached.meta;
			loading = false;
			return true;
		}
		logger.log('tasks', `IDB cache miss for ${view}`);
		return false;
	}

	function subscribeWS(): void {
		const contextId = currentContextId();
		const view = currentView();

		// Completed view uses HTTP fetch, not WS
		if (view === 'completed') {
			logger.log('tasks', 'fetching completed (HTTP)');
			fetchCompleted(contextId);
			return;
		}

		logger.log('tasks', `subscribing WS: view=${view} context=${contextId}`);
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
			saveCompletedTasks(currentContextId(), res.tasks).catch((e) => logger.error('tasks', String(e)));
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
		if (running) return;
		running = true;
		logger.log('tasks', 'start');
		loading = true;
		error = null;
		hasReceivedSnapshot = false;

		// Load config once
		try {
			const cfg = await getAppConfig();
			config = cfg.settings;
		} catch {
			// Config fetch is best-effort
		}

		// Track WS disconnects to set offline state
		cleanups.push(
			wsClient.onStateChange((connected) => {
				if (!connected && hasReceivedSnapshot) {
					logger.log('tasks', 'WS disconnected after snapshot, marking offline');
					isOffline = true;
				}
				if (connected && isOffline) {
					logger.log('tasks', 'WS reconnected, flushing queued mutations');
					// Flush queued mutations, then re-subscribe
					// to get a fresh snapshot that includes the replayed changes.
					actionQueue.flushNow().then(() => {
						logger.log('tasks', 'Queue flush complete, re-subscribing');
						subscribeWS();
					});
				}
			})
		);

		// Try loading from IDB first for instant display while WS connects
		const hadCache = await loadFromIDB();

		subscribeWS();

		// If we showed cached data, give WS a grace period to deliver fresh snapshot
		if (hadCache) {
			scheduleOfflineCheck();
		}
	}

	function stop(): void {
		if (!running) return;
		running = false;
		cancelOfflineTimer();
		for (const cleanup of cleanups) cleanup();
		cleanups = [];
		wsClient.unsubscribe('tasks');
	}

	// Refresh: re-subscribe to get a fresh snapshot from the server.
	function refresh(): Promise<void> {
		logger.log('tasks', 'refresh');
		hasReceivedSnapshot = false;
		subscribeWS();
		return Promise.resolve();
	}

	// Clear tasks, show loading spinner, and re-subscribe (for view transitions).
	async function refreshWithLoading(): Promise<void> {
		logger.log('tasks', `refreshWithLoading: ${currentView()}`);
		tasks = [];
		loading = true;
		error = null;
		isOffline = false;
		isStale = false;
		hasReceivedSnapshot = false;

		// Try IDB for instant view switch
		const hadCache = await loadFromIDB();

		subscribeWS();

		// Grace period for offline detection
		if (hadCache) {
			scheduleOfflineCheck();
		}
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
		insertAfterLocal,
		applyPendingTaskUpdate
	};
}

export const tasksStore = createTasksStore();

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
import { loadCompletedTasks, saveCompletedTasks } from '$lib/sync/db';
import { isStateReady, persistTasks, persistMeta, loadPersistedTasks, loadPersistedMeta } from '$lib/state/index.svelte';
import { flattenTasks, buildTree, taskToFlat, type FlatTask } from '$lib/state/types';

const STALE_THRESHOLD_MS = 2 * 60 * 1000; // 2 minutes
const OFFLINE_GRACE_MS = 5000; // grace period before showing offline banner

function createTasksStore() {
	// Flat task array — reactive source for UI, persisted to Y.Doc via y-indexeddb
	let flatTasks = $state<FlatTask[]>([]);
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

	// Completed tasks are fetched via HTTP, not WS — kept separate
	let completedTasks = $state<Task[]>([]);
	let completedMeta = $state<Meta>({
		context: '',
		weekly_limit: 0,
		weekly_count: 0,
		backlog_limit: 0,
		backlog_count: 0
	});

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

		const flat = flattenTasks(d.tasks);
		flatTasks = flat;
		meta = d.meta;

		// Persist to Y.Doc (y-indexeddb saves automatically)
		persistTasks('tasks', flat);
		persistMeta('meta', d.meta);

		loading = false;
		error = null;
		isOffline = false;
		updateStale(d.meta.last_synced_at);
	}

	function handleTasksDelta(data: unknown): void {
		const d = data as DeltaTasksData;
		logger.log('tasks', `delta: upserted=${d.upserted?.length ?? 0} removed=${d.removed?.length ?? 0}`);

		let updated = [...flatTasks];

		if (d.removed?.length > 0) {
			const removeSet = new Set(d.removed);
			updated = updated.filter((t) => !removeSet.has(t.id));
		}

		if (d.upserted?.length > 0) {
			const upsertedFlat = flattenTasks(d.upserted);
			for (const flat of upsertedFlat) {
				const idx = updated.findIndex((t) => t.id === flat.id);
				if (idx >= 0) {
					updated[idx] = flat;
				} else {
					updated.push(flat);
				}
			}
		}

		flatTasks = updated;

		if (d.meta) {
			meta = d.meta;
			updateStale(d.meta.last_synced_at);
		}

		// Persist updated state
		persistTasks('tasks', updated);
		if (d.meta) persistMeta('meta', d.meta);
	}

	// Register WS handlers once
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

	function subscribeWS(): void {
		const contextId = currentContextId();
		const view = currentView();

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
			completedTasks = res.tasks;
			completedMeta = res.meta;
			error = null;
			isOffline = false;
			saveCompletedTasks(currentContextId(), res.tasks).catch((e) => logger.error('tasks', String(e)));
		} catch (err) {
			const cached = await loadCompletedTasks(currentContextId());
			if (cached) {
				completedTasks = cached.tasks;
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

		try {
			const cfg = await getAppConfig();
			config = cfg.settings;
		} catch {
			// Config fetch is best-effort
		}

		cleanups.push(
			wsClient.onStateChange((connected) => {
				if (!connected && hasReceivedSnapshot) {
					logger.log('tasks', 'WS disconnected after snapshot, marking offline');
					isOffline = true;
				}
				if (connected && isOffline) {
					logger.log('tasks', 'WS reconnected, flushing queued mutations');
					actionQueue.flushNow().then(() => {
						logger.log('tasks', 'Queue flush complete, re-subscribing');
						subscribeWS();
					});
				}
			})
		);

		// Try loading from y-indexeddb (via Y.Doc) for instant display
		const cached = loadPersistedTasks('tasks');
		if (cached.length > 0) {
			logger.log('tasks', `y-indexeddb cache hit: ${cached.length} tasks`);
			flatTasks = cached;
			const cachedMeta = loadPersistedMeta('meta');
			if (cachedMeta) meta = cachedMeta;
			loading = false;
		}

		subscribeWS();

		if (cached.length > 0) {
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

	function refresh(): Promise<void> {
		logger.log('tasks', 'refresh');
		hasReceivedSnapshot = false;
		subscribeWS();
		return Promise.resolve();
	}

	async function refreshWithLoading(): Promise<void> {
		logger.log('tasks', `refreshWithLoading: ${currentView()}`);
		flatTasks = [];
		completedTasks = [];
		loading = true;
		error = null;
		isOffline = false;
		isStale = false;
		hasReceivedSnapshot = false;

		subscribeWS();
	}

	// Optimistic local mutations
	function updateTaskLocal(taskId: string, updater: (task: Task) => Task): void {
		flatTasks = flatTasks.map((f) => {
			if (f.id !== taskId) return f;
			const task = flatToTaskSingle(f);
			const updated = updater(task);
			return taskToFlat(updated);
		});
		persistTasks('tasks', flatTasks);
	}

	function removeTaskLocal(taskId: string): void {
		// Only add to pendingRemovals overlay — don't modify $state.
		// The tasks getter applies pendingRemovals filter on read.
		pendingRemovals.add(taskId);
	}

	function clearPendingRemoval(taskId: string): void {
		pendingRemovals.delete(taskId);
	}

	function addTaskLocal(task: Task): void {
		flatTasks = [taskToFlat(task), ...flatTasks];
		persistTasks('tasks', flatTasks);
	}

	function insertAfterLocal(siblingId: string, newTask: Task): void {
		const idx = flatTasks.findIndex((t) => t.id === siblingId);
		const flat = taskToFlat(newTask);
		const updated = [...flatTasks];
		if (idx >= 0) {
			updated.splice(idx + 1, 0, flat);
		} else {
			updated.push(flat);
		}
		flatTasks = updated;
		persistTasks('tasks', flatTasks);
	}

	function flatToTaskSingle(flat: FlatTask): Task {
		return {
			id: flat.id,
			content: flat.content,
			description: flat.description,
			project_id: flat.project_id,
			section_id: flat.section_id,
			parent_id: flat.parent_id,
			labels: [...flat.labels],
			priority: flat.priority,
			due: flat.due_date ? { date: flat.due_date, recurring: flat.due_recurring } : null,
			sub_task_count: flat.sub_task_count,
			completed_sub_task_count: flat.completed_sub_task_count,
			completed_at: flat.completed_at,
			added_at: flat.added_at,
			is_project_task: flat.is_project_task,
			children: []
		};
	}

	return {
		get tasks(): Task[] {
			if (currentView() === 'completed') {
				return completedTasks;
			}
			const tree = buildTree(flatTasks);
			return applyPendingUpdates(applyPendingRemovals(tree));
		},
		get meta(): Meta {
			if (currentView() === 'completed') {
				return completedMeta;
			}
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

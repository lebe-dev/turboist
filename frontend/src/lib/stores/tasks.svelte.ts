import { logger } from '$lib/stores/logger';
import { getAppConfig, getCompletedTasks } from '$lib/api/client';
import type { Config, Meta, Task } from '$lib/api/types';
import { contextsStore, type View } from './contexts.svelte';
import {
	wsClient,
	type SnapshotTasksData,
	type DeltaTasksData
} from '$lib/ws/client.svelte';
import { flattenTasks, buildTree, taskToFlat, type FlatTask } from '$lib/utils/task-tree';

const STALE_THRESHOLD_MS = 2 * 60 * 1000; // 2 minutes

function createTasksStore() {
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

	// Map temp task IDs to their reconciled real IDs (for navigation redirect)
	let reconciledIds = $state<Record<string, string>>({});

	let cleanups: (() => void)[] = [];
	let running = false;
	let hasReceivedSnapshot = false;

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

	function currentView(): View {
		return contextsStore.activeView;
	}

	function currentContextId(): string | undefined {
		return contextsStore.activeContextId ?? undefined;
	}

	function handleTasksSnapshot(data: unknown, seq?: number): void {
		if (seq !== undefined && seq !== wsClient.currentSeq) {
			logger.log('tasks', `ignoring stale snapshot (seq=${seq}, current=${wsClient.currentSeq})`);
			return;
		}
		const d = data as SnapshotTasksData;
		logger.log('tasks', `snapshot received: ${d.tasks.length} tasks, synced at: ${d.meta.last_synced_at}`);
		hasReceivedSnapshot = true;

		flatTasks = flattenTasks(d.tasks);
		meta = d.meta;

		loading = false;
		error = null;
		updateStale(d.meta.last_synced_at);
	}

	function handleTasksDelta(data: unknown, seq?: number): void {
		if (seq !== undefined && seq !== wsClient.currentSeq) {
			logger.log('tasks', `ignoring stale delta (seq=${seq}, current=${wsClient.currentSeq})`);
			return;
		}
		const d = data as DeltaTasksData;
		logger.log('tasks', `delta: upserted=${d.upserted?.length ?? 0} removed=${d.removed?.length ?? 0}`);

		let updated = [...flatTasks];

		if (d.removed?.length > 0) {
			const removeSet = new Set(d.removed);
			updated = updated.filter((t) => !removeSet.has(t.id));
		}

		if (d.upserted?.length > 0) {
			const upsertedFlat = flattenTasks(d.upserted);

			// Clear pendingRemovals for upserted tasks — handles recurring tasks
			// that come back with a new due date after item_close.
			for (const f of upsertedFlat) {
				if (pendingRemovals.has(f.id)) {
					pendingRemovals.delete(f.id);
				}
			}

			// Reconcile: remove optimistic temp tasks whose real counterparts arrived
			const newContents = new Set(
				upsertedFlat
					.filter((f) => !updated.some((t) => t.id === f.id))
					.map((f) => f.content)
			);

			// Map content → array of original indices for position-preserving replacement
			const tempPositions = new Map<string, number[]>();

			if (newContents.size > 0) {
				// Record temp→real ID mappings before removing (for navigation redirect)
				const newMappings: Record<string, string> = {};
				for (let i = 0; i < updated.length; i++) {
					const tempTask = updated[i];
					if (!tempTask.id.startsWith('temp-') || !newContents.has(tempTask.content)) continue;
					const realTask = upsertedFlat.find(
						(f) => f.content === tempTask.content && !f.id.startsWith('temp-')
					);
					if (realTask) {
						newMappings[tempTask.id] = realTask.id;
					}
					// Record position before removal
					const positions = tempPositions.get(tempTask.content) ?? [];
					positions.push(i);
					tempPositions.set(tempTask.content, positions);
				}
				if (Object.keys(newMappings).length > 0) {
					reconciledIds = { ...reconciledIds, ...newMappings };
				}

				updated = updated.filter(
					(t) => !t.id.startsWith('temp-') || !newContents.has(t.content)
				);

				// Adjust positions: after removal, earlier removals shift later indices
				for (const [content, positions] of tempPositions) {
					// Sort ascending so we can compute shifts correctly
					positions.sort((a, b) => a - b);
					const allRemovedIndices = [...tempPositions.values()].flat().sort((a, b) => a - b);
					for (let j = 0; j < positions.length; j++) {
						const origIdx = positions[j];
						const removedBefore = allRemovedIndices.filter((ri) => ri < origIdx).length;
						positions[j] = origIdx - removedBefore;
					}
				}
			}

			// Upsert: in-place replacements first, collect position-aware insertions
			const toInsert: { flat: FlatTask; idx: number }[] = [];
			for (const flat of upsertedFlat) {
				const idx = updated.findIndex((t) => t.id === flat.id);
				if (idx >= 0) {
					updated[idx] = flat;
				} else {
					const positions = tempPositions.get(flat.content);
					if (positions && positions.length > 0) {
						toInsert.push({ flat, idx: positions.shift()! });
					} else {
						updated.push(flat);
					}
				}
			}

			// Insert in descending order so splice doesn't shift subsequent indices
			toInsert.sort((a, b) => b.idx - a.idx);
			for (const { flat, idx } of toInsert) {
				updated.splice(idx, 0, flat);
			}
		}

		flatTasks = updated;

		if (d.meta) {
			meta = d.meta;
			updateStale(d.meta.last_synced_at);
		}
	}

	// Register WS handlers once
	wsClient.onMessage('snapshot', 'tasks', handleTasksSnapshot);
	wsClient.onMessage('delta', 'tasks', handleTasksDelta);

	function subscribeWS(): void {
		const contextId = currentContextId();
		const view = currentView();

		if (view === 'completed') {
			logger.log('tasks', 'fetching completed (HTTP)');
			fetchCompleted();
			return;
		}

		logger.log('tasks', `subscribing WS: view=${view} context=${contextId}`);
		wsClient.subscribe('tasks', { view, context: contextId });
	}

	async function fetchCompleted(): Promise<void> {
		loading = true;
		try {
			const res = await getCompletedTasks();
			completedTasks = res.tasks;
			completedMeta = res.meta;
			error = null;
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
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
				if (connected && isStale) {
					logger.log('tasks', 'WS reconnected, re-subscribing');
					isStale = false;
					hasReceivedSnapshot = false;
					subscribeWS();
				}
			})
		);

		subscribeWS();
	}

	function stop(): void {
		if (!running) return;
		running = false;
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
			postpone_count: flat.postpone_count,
			children: []
		};
	}

	return {
		get tasks(): Task[] {
			if (currentView() === 'completed') {
				return completedTasks;
			}
			const tree = buildTree(flatTasks);
			return applyPendingRemovals(tree);
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
		get inboxCount(): number {
			return meta.inbox_count ?? 0;
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
		// Identity function — kept for callers that still reference it (e.g. TaskDetailPanel).
		// Previously overlaid queued mutations; now mutations go directly to server.
		applyPendingTaskUpdate(task: Task): Task {
			return task;
		},
		resolveTaskId(id: string): string | null {
			return reconciledIds[id] ?? null;
		}
	};
}

export const tasksStore = createTasksStore();

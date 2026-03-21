import { openDB, type DBSchema, type IDBPDatabase } from 'idb';
import type { Task, Meta, AppConfig } from '$lib/api/types';

export interface TaskSnapshot {
	tasks: Task[];
	meta: Meta;
	updatedAt: number;
}

export interface CompletedCache {
	tasks: Task[];
	updatedAt: number;
}

export interface QueuedAction {
	id?: number;
	type: 'createTask' | 'updateTask' | 'completeTask' | 'deleteTask' | 'duplicateTask' | 'resetWeeklyLabel' | 'patchState';
	payload: unknown;
	createdAt: number;
	status: 'pending' | 'processing' | 'failed';
	error?: string;
}

interface TurboistDB extends DBSchema {
	taskSnapshots: {
		key: string;
		value: TaskSnapshot;
	};
	completedTasksCache: {
		key: string;
		value: CompletedCache;
	};
	appConfig: {
		key: string;
		value: AppConfig;
	};
	actionQueue: {
		key: number;
		value: QueuedAction;
	};
}

let dbPromise: Promise<IDBPDatabase<TurboistDB>> | null = null;

function getDB(): Promise<IDBPDatabase<TurboistDB>> {
	if (!dbPromise) {
		dbPromise = openDB<TurboistDB>('turboist', 2, {
			upgrade(db, oldVersion) {
				if (oldVersion < 1) {
					// Fresh install: create all stores
					db.createObjectStore('taskSnapshots');
					db.createObjectStore('completedTasksCache');
					db.createObjectStore('appConfig');
				}
				if (oldVersion < 2) {
					// Upgrade from v1 or fresh install: add actionQueue store
					db.createObjectStore('actionQueue', { keyPath: 'id', autoIncrement: true });
				}
			}
		});
	}
	return dbPromise;
}

// Action queue CRUD
export async function saveQueuedAction(action: Omit<QueuedAction, 'id'>): Promise<number> {
	const db = await getDB();
	return db.add('actionQueue', action as QueuedAction) as Promise<number>;
}

export async function loadPendingActions(): Promise<QueuedAction[]> {
	const db = await getDB();
	const all = await db.getAll('actionQueue');
	return all.filter((a) => a.status === 'pending' || a.status === 'failed');
}

export async function removeQueuedAction(id: number): Promise<void> {
	const db = await getDB();
	await db.delete('actionQueue', id);
}

export async function updateQueuedAction(action: QueuedAction): Promise<void> {
	const db = await getDB();
	await db.put('actionQueue', action);
}

export async function clearActionQueue(): Promise<void> {
	const db = await getDB();
	await db.clear('actionQueue');
}

function snapshotKey(view: string, contextId?: string): string {
	return `${view}|${contextId ?? ''}`;
}

// Task snapshots
export async function saveTaskSnapshot(
	view: string,
	contextId: string | undefined,
	tasks: Task[],
	meta: Meta
): Promise<void> {
	const db = await getDB();
	await db.put('taskSnapshots', { tasks, meta, updatedAt: Date.now() }, snapshotKey(view, contextId));
}

export async function loadTaskSnapshot(
	view: string,
	contextId?: string
): Promise<TaskSnapshot | undefined> {
	const db = await getDB();
	return db.get('taskSnapshots', snapshotKey(view, contextId));
}

// Completed tasks cache
export async function saveCompletedTasks(contextId: string | undefined, tasks: Task[]): Promise<void> {
	const db = await getDB();
	await db.put('completedTasksCache', { tasks, updatedAt: Date.now() }, contextId ?? '');
}

export async function loadCompletedTasks(contextId?: string): Promise<CompletedCache | undefined> {
	const db = await getDB();
	return db.get('completedTasksCache', contextId ?? '');
}

// App config
export async function saveAppConfig(config: AppConfig): Promise<void> {
	const db = await getDB();
	await db.put('appConfig', config, 'config');
}

export async function loadAppConfig(): Promise<AppConfig | undefined> {
	const db = await getDB();
	return db.get('appConfig', 'config');
}

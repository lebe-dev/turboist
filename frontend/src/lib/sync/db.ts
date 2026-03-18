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
}

let dbPromise: Promise<IDBPDatabase<TurboistDB>> | null = null;

function getDB(): Promise<IDBPDatabase<TurboistDB>> {
	if (!dbPromise) {
		dbPromise = openDB<TurboistDB>('turboist', 1, {
			upgrade(db) {
				db.createObjectStore('taskSnapshots');
				db.createObjectStore('completedTasksCache');
				db.createObjectStore('appConfig');
			}
		});
	}
	return dbPromise;
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

import { getDB, type StoredEntity, type TurboistDB } from './db';
import { emitDbChanged } from './stores';
import type { Task } from '$lib/api/types';

// cacheTasksFromServer upserts authoritative server tasks into Dexie so that
// future hydrations (and offline reads) reflect what REST views returned. This
// is the transitional bridge: online REST calls keep flowing, but every page
// load also primes the local cache.
export const cacheTasksFromServer = async (
	tasks: readonly Task[],
	db: TurboistDB = getDB()
): Promise<void> => {
	if (tasks.length === 0) return;
	const rows: StoredEntity[] = [];
	for (const task of tasks) {
		const existing = (await db.tasks.where('serverId').equals(task.id).first()) as
			| StoredEntity
			| undefined;
		rows.push({
			clientId: existing?.clientId ?? `srv:${task.id}`,
			serverId: task.id,
			updatedAt: task.updatedAt,
			deletedAt: null,
			data: task as unknown as Record<string, unknown>
		});
	}
	await db.tasks.bulkPut(rows);
	emitDbChanged('tasks');
};

import { getDB, type EntityKind, type StoredEntity, type TurboistDB } from './db';
import { emitDbChanged } from './stores';
import type { Context, Label, Project, ProjectSection, Task } from '$lib/api/types';

interface ServerEntityLike {
	id: number;
	updatedAt: string;
}

const cacheEntities = async <T extends ServerEntityLike>(
	kind: EntityKind,
	items: readonly T[],
	db: TurboistDB
): Promise<void> => {
	if (items.length === 0) return;
	const table = db.table(kind);
	const rows: StoredEntity[] = [];
	for (const item of items) {
		const existing = (await table.where('serverId').equals(item.id).first()) as
			| StoredEntity
			| undefined;
		rows.push({
			clientId: existing?.clientId ?? `srv:${item.id}`,
			serverId: item.id,
			updatedAt: item.updatedAt,
			deletedAt: null,
			data: item as unknown as Record<string, unknown>
		});
	}
	await table.bulkPut(rows);
	emitDbChanged(kind);
};

// cacheTasksFromServer upserts authoritative server tasks into Dexie so that
// future hydrations (and offline reads) reflect what REST views returned. This
// is the transitional bridge: online REST calls keep flowing, but every page
// load also primes the local cache.
export const cacheTasksFromServer = (
	tasks: readonly Task[],
	db: TurboistDB = getDB()
): Promise<void> => cacheEntities('tasks', tasks, db);

export const cacheProjectsFromServer = (
	projects: readonly Project[],
	db: TurboistDB = getDB()
): Promise<void> => cacheEntities('projects', projects, db);

export const cacheSectionsFromServer = (
	sections: readonly ProjectSection[],
	db: TurboistDB = getDB()
): Promise<void> => cacheEntities('sections', sections, db);

export const cacheLabelsFromServer = (
	labels: readonly Label[],
	db: TurboistDB = getDB()
): Promise<void> => cacheEntities('labels', labels, db);

export const cacheContextsFromServer = (
	contexts: readonly Context[],
	db: TurboistDB = getDB()
): Promise<void> => cacheEntities('contexts', contexts, db);

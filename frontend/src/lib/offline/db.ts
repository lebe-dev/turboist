import Dexie, { type Table } from 'dexie';

export type EntityKind = 'tasks' | 'projects' | 'sections' | 'labels' | 'contexts';

export type OutboxStatus = 'pending' | 'inflight' | 'failed';

export type OutboxOp = 'create' | 'update' | 'delete';

export interface StoredEntity {
	clientId: string;
	serverId: number | null;
	updatedAt: string;
	deletedAt: string | null;
	data: Record<string, unknown>;
}

export interface OutboxEntry {
	id: string;
	entity: EntityKind;
	op: OutboxOp;
	clientId: string;
	payload: Record<string, unknown>;
	idempotencyKey: string;
	status: OutboxStatus;
	attempts: number;
	nextAttemptAt: number;
	lastError: string | null;
	createdAt: number;
	parentClientId: string | null;
}

export interface MetaEntry {
	key: string;
	value: unknown;
}

export class TurboistDB extends Dexie {
	tasks!: Table<StoredEntity, string>;
	projects!: Table<StoredEntity, string>;
	sections!: Table<StoredEntity, string>;
	labels!: Table<StoredEntity, string>;
	contexts!: Table<StoredEntity, string>;
	outbox!: Table<OutboxEntry, string>;
	meta!: Table<MetaEntry, string>;

	constructor(name = 'turboist') {
		super(name);
		this.version(1).stores({
			tasks: 'clientId, serverId, updatedAt, deletedAt',
			projects: 'clientId, serverId, updatedAt, deletedAt',
			sections: 'clientId, serverId, updatedAt, deletedAt',
			labels: 'clientId, serverId, updatedAt, deletedAt',
			contexts: 'clientId, serverId, updatedAt, deletedAt',
			outbox: 'id, entity, status, nextAttemptAt, clientId, parentClientId',
			meta: 'key'
		});
	}
}

let instance: TurboistDB | null = null;

export const getDB = (): TurboistDB => {
	if (!instance) instance = new TurboistDB();
	return instance;
};

export const setDBForTests = (db: TurboistDB | null): void => {
	instance = db;
};

export const ENTITY_TABLES: readonly EntityKind[] = [
	'tasks',
	'projects',
	'sections',
	'labels',
	'contexts'
] as const;

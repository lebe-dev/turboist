import {
	getDB,
	type EntityKind,
	type OutboxEntry,
	type OutboxOp,
	type OutboxStatus,
	type TurboistDB
} from './db';
import { newOutboxId } from './ids';

export const RETRY_SCHEDULE_MS: readonly number[] = [
	1_000, 2_000, 5_000, 15_000, 30_000, 60_000
] as const;

export const nextBackoffMs = (attempts: number): number => {
	const idx = Math.min(attempts, RETRY_SCHEDULE_MS.length - 1);
	return RETRY_SCHEDULE_MS[idx];
};

export interface EnqueueInput {
	entity: EntityKind;
	op: OutboxOp;
	clientId: string;
	payload: Record<string, unknown>;
	idempotencyKey?: string;
	parentClientId?: string | null;
}

export const enqueue = async (
	input: EnqueueInput,
	db: TurboistDB = getDB()
): Promise<OutboxEntry> => {
	const entry: OutboxEntry = {
		id: newOutboxId(),
		entity: input.entity,
		op: input.op,
		clientId: input.clientId,
		payload: input.payload,
		idempotencyKey: input.idempotencyKey ?? newOutboxId(),
		status: 'pending',
		attempts: 0,
		nextAttemptAt: Date.now(),
		lastError: null,
		createdAt: Date.now(),
		parentClientId: input.parentClientId ?? null
	};
	await db.outbox.put(entry);
	return entry;
};

export const list = async (
	filter: { status?: OutboxStatus; entity?: EntityKind } = {},
	db: TurboistDB = getDB()
): Promise<OutboxEntry[]> => {
	let collection = db.outbox.toCollection();
	if (filter.status) collection = db.outbox.where('status').equals(filter.status);
	const all = await collection.toArray();
	const filtered = filter.entity ? all.filter((e) => e.entity === filter.entity) : all;
	return filtered.sort((a, b) => a.createdAt - b.createdAt);
};

export const listReady = async (
	now: number = Date.now(),
	db: TurboistDB = getDB()
): Promise<OutboxEntry[]> => {
	const pending = await db.outbox.where('status').equals('pending').toArray();
	return pending
		.filter((e) => e.nextAttemptAt <= now)
		.sort((a, b) => a.createdAt - b.createdAt);
};

export const markInflight = async (id: string, db: TurboistDB = getDB()): Promise<void> => {
	await db.outbox.update(id, { status: 'inflight' as OutboxStatus });
};

export const remove = async (id: string, db: TurboistDB = getDB()): Promise<void> => {
	await db.outbox.delete(id);
};

export const markFailed = async (
	id: string,
	error: string,
	options: { permanent?: boolean } = {},
	db: TurboistDB = getDB()
): Promise<OutboxEntry | undefined> => {
	const entry = await db.outbox.get(id);
	if (!entry) return undefined;
	const attempts = entry.attempts + 1;
	const status: OutboxStatus = options.permanent ? 'failed' : 'pending';
	const nextAttemptAt = options.permanent ? entry.nextAttemptAt : Date.now() + nextBackoffMs(attempts);
	const updated: OutboxEntry = {
		...entry,
		attempts,
		status,
		nextAttemptAt,
		lastError: error
	};
	await db.outbox.put(updated);
	return updated;
};

export const pendingCount = async (db: TurboistDB = getDB()): Promise<number> => {
	return db.outbox.where('status').anyOf('pending', 'inflight').count();
};

export const remapClientId = async (
	oldClientId: string,
	newClientId: string,
	db: TurboistDB = getDB()
): Promise<void> => {
	const refs = await db.outbox.where('parentClientId').equals(oldClientId).toArray();
	for (const r of refs) {
		await db.outbox.update(r.id, { parentClientId: newClientId });
	}
};

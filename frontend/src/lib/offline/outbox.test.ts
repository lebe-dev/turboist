import 'fake-indexeddb/auto';
import { IDBFactory } from 'fake-indexeddb';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { openOfflineDB, type OfflineDB } from './db';
import { enqueueOp, failOp, listQueue, removeOp, retryFailedOp } from './outbox';

const SERVER = 'https://x.example.com';

beforeEach(() => {
	globalThis.indexedDB = new IDBFactory();
});

describe('enqueueOp', () => {
	let db: OfflineDB;
	afterEach(() => db?.close());

	it('stamps the v1 envelope and returns the stored op with a seq', async () => {
		db = await openOfflineDB(SERVER);
		const op = await enqueueOp(
			db,
			{ type: 'task.complete', payload: { taskId: 5 } },
			() => '2026-01-01T00:00:00.000Z'
		);

		expect(op.v).toBe(1);
		expect(op.seq).toBeGreaterThan(0);
		expect(op.type).toBe('task.complete');
		expect(op.payload).toEqual({ taskId: 5 });
		expect(op.attempts).toBe(0);
		expect(op.createdAt).toBe('2026-01-01T00:00:00.000Z');
		expect(op.opId).toMatch(/\S/);
		expect(op.idempotencyKey).toMatch(/\S/);

		// The returned op is exactly what was persisted.
		const [stored] = await listQueue(db);
		expect(stored).toEqual(op);
	});

	it('honors an explicitly supplied idempotencyKey and opId (replay reuse)', async () => {
		db = await openOfflineDB(SERVER);
		const op = await enqueueOp(db, {
			type: 'task.uncomplete',
			payload: { taskId: 2 },
			idempotencyKey: 'key-1',
			opId: 'op-1'
		});
		expect(op.idempotencyKey).toBe('key-1');
		expect(op.opId).toBe('op-1');
	});

	it('generates a distinct idempotencyKey and opId per enqueue by default', async () => {
		db = await openOfflineDB(SERVER);
		const a = await enqueueOp(db, { type: 'task.complete', payload: { taskId: 1 } });
		const b = await enqueueOp(db, { type: 'task.complete', payload: { taskId: 2 } });
		expect(a.idempotencyKey).not.toBe(b.idempotencyKey);
		expect(a.opId).not.toBe(b.opId);
	});
});

describe('listQueue / removeOp', () => {
	let db: OfflineDB;
	afterEach(() => db?.close());

	it('lists in FIFO (seq) order and removes by seq', async () => {
		db = await openOfflineDB(SERVER);
		const a = await enqueueOp(db, { type: 'task.complete', payload: { taskId: 1 } });
		const b = await enqueueOp(db, { type: 'task.complete', payload: { taskId: 2 } });
		const c = await enqueueOp(db, { type: 'task.complete', payload: { taskId: 3 } });

		expect((await listQueue(db)).map((o) => o.seq)).toEqual([a.seq, b.seq, c.seq]);

		await removeOp(db, b.seq);
		expect((await listQueue(db)).map((o) => o.payload.taskId)).toEqual([1, 3]);
	});
});

describe('failOp', () => {
	let db: OfflineDB;
	afterEach(() => db?.close());

	it('moves an op out of the queue into failedOps with failure metadata', async () => {
		db = await openOfflineDB(SERVER);
		const op = await enqueueOp(db, { type: 'task.complete', payload: { taskId: 1 } });

		await failOp(db, op, 'conflict', 'gone', () => '2026-02-02T00:00:00.000Z');

		expect(await listQueue(db)).toHaveLength(0);
		const failed = await db.listFailed();
		expect(failed).toHaveLength(1);
		expect(failed[0]).toMatchObject({
			seq: op.seq,
			type: 'task.complete',
			errorCode: 'conflict',
			errorMessage: 'gone',
			failedAt: '2026-02-02T00:00:00.000Z'
		});
	});
});

describe('retryFailedOp', () => {
	let db: OfflineDB;
	afterEach(() => db?.close());

	it('re-enqueues a quarantined op with attempts reset to 0, keeping its idempotency key and opId', async () => {
		db = await openOfflineDB(SERVER);
		const op = await enqueueOp(db, {
			type: 'task.complete',
			payload: { taskId: 7 },
			idempotencyKey: 'idem-7',
			opId: 'op-7'
		});
		// A replay spent the retry budget then quarantined it.
		op.attempts = 4;
		await failOp(db, op, 'server_error', 'boom', () => '2026-03-03T00:00:00.000Z');
		expect(await db.listFailed()).toHaveLength(1);

		const requeued = await retryFailedOp(db, op.seq);

		expect(requeued).not.toBeNull();
		expect(requeued!.attempts).toBe(0);
		expect(requeued!.idempotencyKey).toBe('idem-7');
		expect(requeued!.opId).toBe('op-7');
		expect(requeued!.type).toBe('task.complete');
		expect(requeued!.payload).toEqual({ taskId: 7 });

		// It left failedOps and is back in the outbox exactly once.
		expect(await db.listFailed()).toHaveLength(0);
		const queue = await listQueue(db);
		expect(queue).toHaveLength(1);
		expect(queue[0].idempotencyKey).toBe('idem-7');
		expect(queue[0].attempts).toBe(0);
	});

	it('returns null for an unknown seq and touches neither store', async () => {
		db = await openOfflineDB(SERVER);
		const result = await retryFailedOp(db, 999);
		expect(result).toBeNull();
		expect(await db.listFailed()).toHaveLength(0);
		expect(await listQueue(db)).toHaveLength(0);
	});
});

describe('deleteFailed (Remove)', () => {
	let db: OfflineDB;
	afterEach(() => db?.close());

	it('drops a single quarantined op, leaving the rest and the outbox intact', async () => {
		db = await openOfflineDB(SERVER);
		const a = await enqueueOp(db, { type: 'task.complete', payload: { taskId: 1 } });
		const b = await enqueueOp(db, { type: 'task.uncomplete', payload: { taskId: 2 } });
		await enqueueOp(db, { type: 'task.complete', payload: { taskId: 3 } });
		await failOp(db, a, 'conflict', 'gone');
		await failOp(db, b, 'conflict', 'gone');
		expect(await db.listFailed()).toHaveLength(2);

		await db.deleteFailed(a.seq);

		const failed = await db.listFailed();
		expect(failed.map((o) => o.seq)).toEqual([b.seq]);
		// The still-queued op is untouched.
		expect(await listQueue(db)).toHaveLength(1);
	});
});

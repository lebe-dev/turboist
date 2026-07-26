import 'fake-indexeddb/auto';
import { IDBFactory } from 'fake-indexeddb';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '../api/errors';
import { openOfflineDB, type OfflineDB } from './db';
import { createReplayEngine, enqueueOp, listQueue, type ReplayClient, type ReplayHooks } from './outbox';

// Replay engine (FEATURE-OFFLINE-ARCH.md §4.6) covering the §6 end-to-end
// scenarios 2–4: happy drain clears the queue; a mutation the server already
// executed (response lost) replays with the SAME idempotency key; a 4xx conflict
// (404) quarantines the op while the queue keeps draining.

const SERVER = 'https://x.example.com';
const NOW = '2026-07-15T00:00:00.000Z';

beforeEach(() => {
	globalThis.indexedDB = new IDBFactory();
});

function makeHooks(): ReplayHooks & {
	setPendingOps: ReturnType<typeof vi.fn>;
	noteSynced: ReturnType<typeof vi.fn>;
	onFailed: ReturnType<typeof vi.fn>;
} {
	return {
		setPendingOps: vi.fn(),
		noteSynced: vi.fn(),
		onFailed: vi.fn()
	};
}

function engineFor(
	db: OfflineDB,
	client: ReplayClient,
	hooks: ReplayHooks,
	debounceMs = 5
) {
	return createReplayEngine({
		getDb: async () => db,
		getClient: () => client,
		hooks,
		now: () => NOW,
		debounceMs
	});
}

async function waitUntil(pred: () => boolean | Promise<boolean>, ms = 1000): Promise<void> {
	const start = Date.now();
	while (!(await pred())) {
		if (Date.now() - start > ms) throw new Error('waitUntil timed out');
		await new Promise((r) => setTimeout(r, 5));
	}
}

describe('createReplayEngine — happy drain (scenario 2)', () => {
	let db: OfflineDB;
	afterEach(() => db?.close());

	it('replays every queued op in FIFO order and clears the queue', async () => {
		db = await openOfflineDB(SERVER);
		await enqueueOp(db, { type: 'task.complete', payload: { taskId: 5 } });
		await enqueueOp(db, { type: 'task.complete', payload: { taskId: 6 } });

		const client: ReplayClient = { fetch: vi.fn(async () => ({ id: 1, status: 'completed' })) };
		const hooks = makeHooks();
		const engine = engineFor(db, client, hooks);

		await engine.sync();

		expect(await listQueue(db)).toHaveLength(0);
		const fetchMock = client.fetch as ReturnType<typeof vi.fn>;
		expect(fetchMock).toHaveBeenCalledTimes(2);
		// FIFO: op with the lower seq (taskId 5) replays first.
		expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/tasks/5/complete');
		expect(fetchMock.mock.calls[1][0]).toBe('/api/v1/tasks/6/complete');

		expect(hooks.setPendingOps).toHaveBeenLastCalledWith(0);
		expect(hooks.noteSynced).toHaveBeenCalledTimes(1);
		expect(await db.getMeta('lastSyncAt')).toBe(NOW);
		expect(hooks.onFailed).not.toHaveBeenCalled();
	});

	it('passes skipOffline and the op idempotency key on each replay', async () => {
		db = await openOfflineDB(SERVER);
		await enqueueOp(db, { type: 'task.complete', payload: { taskId: 5 }, idempotencyKey: 'k-5' });

		const client: ReplayClient = { fetch: vi.fn(async () => ({ id: 5, status: 'completed' })) };
		const engine = engineFor(db, client, makeHooks());
		await engine.sync();

		expect(client.fetch).toHaveBeenCalledWith(
			'/api/v1/tasks/5/complete',
			expect.objectContaining({ method: 'POST', skipOffline: true, idempotencyKey: 'k-5' })
		);
	});

	it('does nothing (no synced signal) when the queue is already empty', async () => {
		db = await openOfflineDB(SERVER);
		const client: ReplayClient = { fetch: vi.fn() };
		const hooks = makeHooks();
		await engineFor(db, client, hooks).sync();

		expect(client.fetch).not.toHaveBeenCalled();
		expect(hooks.noteSynced).not.toHaveBeenCalled();
		expect(hooks.setPendingOps).not.toHaveBeenCalled();
	});
});

describe('createReplayEngine — lost response replayed with the same key (scenario 3)', () => {
	let db: OfflineDB;
	afterEach(() => db?.close());

	it('reuses the stored idempotency key so the backend returns its recorded response', async () => {
		db = await openOfflineDB(SERVER);
		// The mutation reached the server and executed, but the response was lost on
		// the wire, so it stayed queued. On replay the backend recognises the key
		// and hands back the stored response (X-Idempotent-Replay) — no duplicate.
		await enqueueOp(db, {
			type: 'task.complete',
			payload: { taskId: 7 },
			idempotencyKey: 'stored-key-123'
		});

		const client: ReplayClient = { fetch: vi.fn(async () => ({ id: 7, status: 'completed' })) };
		const engine = engineFor(db, client, makeHooks());
		await engine.sync();

		const [, init] = (client.fetch as ReturnType<typeof vi.fn>).mock.calls[0];
		expect(init.idempotencyKey).toBe('stored-key-123');
		expect(await listQueue(db)).toHaveLength(0);
	});
});

describe('createReplayEngine — conflict quarantine (scenario 4)', () => {
	let db: OfflineDB;
	afterEach(() => db?.close());

	it('moves a 404 op to failedOps and keeps draining the rest', async () => {
		db = await openOfflineDB(SERVER);
		const a = await enqueueOp(db, { type: 'task.complete', payload: { taskId: 8 } });
		await enqueueOp(db, { type: 'task.complete', payload: { taskId: 9 } });

		const client: ReplayClient = {
			fetch: vi.fn(async (path: string) => {
				if (path === '/api/v1/tasks/8/complete') {
					throw new ApiError('not_found', 'task gone', 404);
				}
				return { id: 9, status: 'completed' };
			})
		};
		const hooks = makeHooks();
		const engine = engineFor(db, client, hooks);
		await engine.sync();

		// Both ops left the outbox: one quarantined, one succeeded.
		expect(await listQueue(db)).toHaveLength(0);
		const failed = await db.listFailed();
		expect(failed).toHaveLength(1);
		expect(failed[0].seq).toBe(a.seq);
		expect(failed[0].errorCode).toBe('not_found');
		expect(failed[0].failedAt).toBe(NOW);
		// The 404 did NOT stop the loop — op 9 was still attempted.
		expect(client.fetch).toHaveBeenCalledTimes(2);
		expect(hooks.onFailed).toHaveBeenCalledWith(1);
		// Queue is empty afterwards, so a refetch is still signalled.
		expect(hooks.noteSynced).toHaveBeenCalledTimes(1);
	});
});

describe('createReplayEngine — network drop mid-replay', () => {
	let db: OfflineDB;
	afterEach(() => db?.close());

	it('stops the loop, bumps attempts and leaves the queue intact on status 0', async () => {
		db = await openOfflineDB(SERVER);
		await enqueueOp(db, { type: 'task.complete', payload: { taskId: 1 } });
		await enqueueOp(db, { type: 'task.complete', payload: { taskId: 2 } });

		const client: ReplayClient = {
			fetch: vi.fn(async () => {
				throw new ApiError('network_error', 'offline', 0);
			})
		};
		const hooks = makeHooks();
		const engine = engineFor(db, client, hooks);
		await engine.sync();

		expect(client.fetch).toHaveBeenCalledTimes(1); // bailed after the first op
		const queue = await listQueue(db);
		expect(queue).toHaveLength(2);
		expect(queue[0].attempts).toBe(1);
		expect(hooks.setPendingOps).toHaveBeenLastCalledWith(2);
		expect(hooks.noteSynced).not.toHaveBeenCalled();
		expect(hooks.onFailed).not.toHaveBeenCalled();
	});
});

describe('createReplayEngine — server errors (5xx backoff)', () => {
	let db: OfflineDB;
	afterEach(() => db?.close());

	it('stops the loop and keeps the op while the retry budget remains', async () => {
		db = await openOfflineDB(SERVER);
		await enqueueOp(db, { type: 'task.complete', payload: { taskId: 1 } });
		await enqueueOp(db, { type: 'task.complete', payload: { taskId: 2 } });

		const client: ReplayClient = {
			fetch: vi.fn(async () => {
				throw new ApiError('internal_error', 'boom', 500);
			})
		};
		const hooks = makeHooks();
		const engine = engineFor(db, client, hooks);
		await engine.sync();

		expect(client.fetch).toHaveBeenCalledTimes(1); // don't hammer a down server
		const queue = await listQueue(db);
		expect(queue).toHaveLength(2);
		expect(queue[0].attempts).toBe(1);
		expect(await db.listFailed()).toHaveLength(0);
		expect(hooks.onFailed).not.toHaveBeenCalled();
	});

	it('quarantines the op once attempts reach the cap', async () => {
		db = await openOfflineDB(SERVER);
		const op = await enqueueOp(db, { type: 'task.complete', payload: { taskId: 1 } });
		// Pretend four earlier kicks already failed on 5xx.
		op.attempts = 4;
		await db.updateOutbox(op);

		const client: ReplayClient = {
			fetch: vi.fn(async () => {
				throw new ApiError('internal_error', 'boom', 500);
			})
		};
		const hooks = makeHooks();
		const engine = engineFor(db, client, hooks);
		await engine.sync();

		expect(await listQueue(db)).toHaveLength(0);
		const failed = await db.listFailed();
		expect(failed).toHaveLength(1);
		expect(failed[0].attempts).toBe(5);
		expect(hooks.onFailed).toHaveBeenCalledWith(1);
	});
});

describe('createReplayEngine — single-flight', () => {
	let db: OfflineDB;
	afterEach(() => db?.close());

	it('does not process an op twice under concurrent sync() calls', async () => {
		db = await openOfflineDB(SERVER);
		await enqueueOp(db, { type: 'task.complete', payload: { taskId: 5 } });

		const client: ReplayClient = { fetch: vi.fn(async () => ({ id: 5, status: 'completed' })) };
		const engine = engineFor(db, client, makeHooks());

		await Promise.all([engine.sync(), engine.sync(), engine.sync()]);

		expect(client.fetch).toHaveBeenCalledTimes(1);
		expect(await listQueue(db)).toHaveLength(0);
	});

	it('coalesces a burst of kick()s into a single drain', async () => {
		db = await openOfflineDB(SERVER);
		await enqueueOp(db, { type: 'task.complete', payload: { taskId: 5 } });

		const client: ReplayClient = { fetch: vi.fn(async () => ({ id: 5, status: 'completed' })) };
		const engine = engineFor(db, client, makeHooks());

		engine.kick();
		engine.kick();
		engine.kick();
		await waitUntil(async () => (await listQueue(db)).length === 0);

		expect(client.fetch).toHaveBeenCalledTimes(1);
	});
});

describe('createReplayEngine — tmpId reconciliation', () => {
	let db: OfflineDB;
	afterEach(() => db?.close());

	it('remembers tmpId → server id after a task.createInbox replays', async () => {
		db = await openOfflineDB(SERVER);
		await enqueueOp(db, {
			type: 'task.createInbox',
			payload: { input: { title: 'Buy milk' }, tmpId: -3 }
		});

		const client: ReplayClient = {
			fetch: vi.fn(async () => ({ id: 42, title: 'Buy milk', status: 'open' }))
		};
		const engine = engineFor(db, client, makeHooks());
		await engine.sync();

		expect(engine.resolveTmpId(-3)).toBe(42);
		expect(engine.tmpIdMap.get(-3)).toBe(42);
		expect(await listQueue(db)).toHaveLength(0);
	});
});

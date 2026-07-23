import 'fake-indexeddb/auto';
import { IDBFactory } from 'fake-indexeddb';
import { beforeEach, describe, expect, it } from 'vitest';
import { clearOfflineData, createOfflineBridge, listFailedOps, removeFailed } from './index';
import { openOfflineDB, type FailedOp } from './db';
import { statusStore } from './status.svelte';

beforeEach(() => {
	// Fresh in-memory IndexedDB per test so cached entries never leak.
	globalThis.indexedDB = new IDBFactory();
	// The bridge drives the shared status singleton; reset it to a known state.
	statusStore.clearStale();
	statusStore.noteOutcome(true);
	statusStore.setPendingOps(0);
});

function bridge() {
	return createOfflineBridge({ serverUrl: '' });
}

describe('createOfflineBridge', () => {
	it('drives the status heuristic from noteRequestOutcome', () => {
		const b = bridge();

		b.noteRequestOutcome(false);
		expect(statusStore.online).toBe(false);
		expect(b.isOffline()).toBe(true);

		b.noteRequestOutcome(true);
		expect(statusStore.online).toBe(true);
		expect(b.isOffline()).toBe(false);
	});

	it('round-trips a payload through cachePut / cacheGet', async () => {
		const b = bridge();
		await b.cachePut('/api/v1/tasks', { view: 'today' }, [{ id: 1 }]);

		const hit = await b.cacheGet('/api/v1/tasks', { view: 'today' });
		expect(hit?.payload).toEqual([{ id: 1 }]);
		expect(typeof hit?.storedAt).toBe('string');
	});

	it('marks the page stale on a cache hit and clears it on a fresh write-through', async () => {
		const b = bridge();
		await b.cachePut('/api/v1/tasks', undefined, [{ id: 1 }]);
		statusStore.clearStale();

		await b.cacheGet('/api/v1/tasks', undefined);
		expect(statusStore.servedStale).toBe(true);

		await b.cachePut('/api/v1/tasks', undefined, [{ id: 2 }]);
		expect(statusStore.servedStale).toBe(false);
	});

	it('serves the task detail GET from a task cached inside a list', async () => {
		const b = bridge();
		await b.cachePut('/api/v1/inbox', undefined, {
			tasks: [
				{ id: 7, title: 't7', status: 'open', dayPart: 'none', parentId: null },
				{ id: 8, title: 't8', status: 'open', dayPart: 'none', parentId: 7 }
			]
		});

		const hit = await b.cacheGet('/api/v1/tasks/7', { subtasks: 'true' });
		expect(hit?.payload).toMatchObject({ id: 7, subtasks: { items: [{ id: 8 }], total: 1 } });
	});

	it('returns null and leaves servedStale untouched on a cache miss', async () => {
		const b = bridge();
		statusStore.clearStale();

		const hit = await b.cacheGet('/api/v1/does-not-exist', undefined);
		expect(hit).toBeNull();
		expect(statusStore.servedStale).toBe(false);
	});

});

describe('createOfflineBridge tryEnqueue (offline mutation queue)', () => {
	it('returns null for a request no whitelisted op handles', async () => {
		const b = bridge();
		expect(await b.tryEnqueue('/api/v1/tasks/1/move', 'POST', {})).toBeNull();
	});

	it('returns null (blocked) for an op targeting a tmp task (id < 0)', async () => {
		const b = bridge();
		expect(await b.tryEnqueue('/api/v1/tasks/-3/complete', 'POST', undefined)).toBeNull();
	});

	it('enqueues task.complete and synthesizes a completed Task', async () => {
		const b = bridge();
		const queued = await b.tryEnqueue('/api/v1/tasks/5/complete', 'POST', undefined);

		expect(queued).not.toBeNull();
		const task = queued!.response as { id: number; status: string; completedAt: string | null };
		expect(task.id).toBe(5);
		expect(task.status).toBe('completed');
		expect(typeof task.completedAt).toBe('string');
	});

	it('persists the caller-provided Idempotency-Key on the enqueued op (§6.3)', async () => {
		const b = bridge();
		const queued = await b.tryEnqueue('/api/v1/tasks/5/complete', 'POST', undefined, 'fg-key-123');
		expect(queued).not.toBeNull();

		// The op must carry the SAME key the failed foreground request already sent,
		// so replay is recognised as a lost-response retry, not a fresh mutation.
		const db = await openOfflineDB('');
		const ops = await db.listOutbox();
		db.close();
		expect(ops).toHaveLength(1);
		expect(ops[0].idempotencyKey).toBe('fg-key-123');
	});

	it('mints a negative tmpId for task.createInbox and echoes the input', async () => {
		const b = bridge();
		const queued = await b.tryEnqueue('/api/v1/inbox/tasks', 'POST', { title: 'Buy milk' });

		expect(queued).not.toBeNull();
		const task = queued!.response as { id: number; title: string };
		expect(task.id).toBeLessThan(0);
		expect(task.title).toBe('Buy milk');
	});

	// §6.5 end-to-end at the integration level: a task created in the inbox while
	// offline exists only in the outbox (negative id). Completing THAT task offline
	// is unsupported — tryEnqueue must decline (null), which the client turns into an
	// `offline_unsupported` ApiError. Uses the ACTUAL minted tmpId (not a hardcoded
	// -N) so it exercises the full create → block-complete sequence.
	it('blocks completing a task that was itself created offline (§6.5)', async () => {
		const b = bridge();

		const created = await b.tryEnqueue('/api/v1/inbox/tasks', 'POST', { title: 'Buy milk' });
		const tmpId = (created!.response as { id: number }).id;
		expect(tmpId).toBeLessThan(0);
		expect(statusStore.pendingOps).toBe(1);

		// Completing the not-yet-synced task is blocked (it has no server id yet).
		const complete = await b.tryEnqueue(`/api/v1/tasks/${tmpId}/complete`, 'POST', undefined);
		expect(complete).toBeNull();
		// The declined op enqueued nothing — only the create is still pending.
		expect(statusStore.pendingOps).toBe(1);
	});

	it('bumps statusStore.pendingOps to the outbox length after each enqueue', async () => {
		const b = bridge();
		expect(statusStore.pendingOps).toBe(0);

		await b.tryEnqueue('/api/v1/tasks/5/complete', 'POST', undefined);
		expect(statusStore.pendingOps).toBe(1);

		await b.tryEnqueue('/api/v1/inbox/tasks', 'POST', { title: 'x' });
		expect(statusStore.pendingOps).toBe(2);

		// A declined (unsupported) op does not change the count.
		await b.tryEnqueue('/api/v1/tasks/1/move', 'POST', {});
		expect(statusStore.pendingOps).toBe(2);
	});
});

describe('clearOfflineData (§4.9 confirmed-logout wipe)', () => {
	it('wipes the read-through cache + outbox and resets pendingOps + failedOps', async () => {
		const b = bridge();
		await b.cachePut('/api/v1/tasks', { view: 'today' }, [{ id: 1 }]);
		await b.tryEnqueue('/api/v1/tasks/5/complete', 'POST', undefined);
		await b.tryEnqueue('/api/v1/inbox/tasks', 'POST', { title: 'x' });
		statusStore.setFailedOps(3);
		expect(statusStore.pendingOps).toBe(2);

		await clearOfflineData();

		expect(statusStore.pendingOps).toBe(0);
		expect(statusStore.failedOps).toBe(0);
		expect(await b.cacheGet('/api/v1/tasks', { view: 'today' })).toBeNull();
		// The outbox is empty: the next enqueue restarts the count at 1.
		await b.tryEnqueue('/api/v1/tasks/5/complete', 'POST', undefined);
		expect(statusStore.pendingOps).toBe(1);
	});
});

describe('failedOps recovery wiring (§4.7.3)', () => {
	function failedOp(seq: number): FailedOp {
		return {
			v: 1,
			seq,
			opId: `op-${seq}`,
			idempotencyKey: `idem-${seq}`,
			type: 'task.complete',
			payload: { taskId: seq },
			createdAt: '2026-01-01T00:00:00.000Z',
			attempts: 3,
			failedAt: '2026-01-02T00:00:00.000Z',
			errorCode: 'conflict',
			errorMessage: 'task not found'
		};
	}

	it('listFailedOps reads the quarantined ops from the shared bridge DB', async () => {
		bridge(); // captures the shared dbHandle
		// Seed via a second connection to the same (shared) database.
		const seed = await openOfflineDB('');
		await seed.pushFailed(failedOp(1));
		await seed.pushFailed(failedOp(2));
		seed.close();

		expect((await listFailedOps()).map((o) => o.seq)).toEqual([1, 2]);
	});

	it('removeFailed drops the chosen op and refreshes statusStore.failedOps', async () => {
		bridge();
		const seed = await openOfflineDB('');
		await seed.pushFailed(failedOp(1));
		await seed.pushFailed(failedOp(2));
		seed.close();
		statusStore.setFailedOps(2);

		await removeFailed(1);

		expect((await listFailedOps()).map((o) => o.seq)).toEqual([2]);
		expect(statusStore.failedOps).toBe(1);
	});
});

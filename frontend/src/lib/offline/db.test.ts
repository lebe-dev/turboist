import 'fake-indexeddb/auto';
import { IDBFactory } from 'fake-indexeddb';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
	buildCacheKey,
	openOfflineDB,
	MAX_RESPONSES,
	type CachedResponse,
	type OfflineDB,
	type QueuedOp
} from './db';

const SERVER_A = 'https://a.example.com';
const SERVER_B = 'https://b.example.com';

function response(key: string, storedAt: string, payload: unknown = { key }): CachedResponse {
	return { cacheKey: key, payload, storedAt, path: '/api/v1/tasks' };
}

function op(overrides: Partial<Omit<QueuedOp, 'seq'>> = {}): Omit<QueuedOp, 'seq'> {
	return {
		v: 1,
		opId: 'op-' + Math.random().toString(36).slice(2),
		idempotencyKey: 'idem-' + Math.random().toString(36).slice(2),
		type: 'task.complete',
		payload: { taskId: 1 },
		createdAt: '2026-01-01T00:00:00.000Z',
		attempts: 0,
		...overrides
	};
}

beforeEach(() => {
	// Fresh in-memory IndexedDB for every test.
	globalThis.indexedDB = new IDBFactory();
});

describe('buildCacheKey', () => {
	it('returns the bare path when there is no query', () => {
		expect(buildCacheKey('/api/v1/tasks')).toBe('/api/v1/tasks');
		expect(buildCacheKey('/api/v1/tasks', null)).toBe('/api/v1/tasks');
		expect(buildCacheKey('/api/v1/tasks', {})).toBe('/api/v1/tasks');
	});

	it('sorts keys and drops null/undefined so equivalent queries collapse', () => {
		const a = buildCacheKey('/api/v1/tasks', { b: 2, a: 1, c: undefined, d: null });
		const b = buildCacheKey('/api/v1/tasks', { a: 1, b: 2 });
		expect(a).toBe('/api/v1/tasks?a=1&b=2');
		expect(a).toBe(b);
	});
});

describe('openOfflineDB', () => {
	let db: OfflineDB;

	afterEach(() => {
		db?.close();
	});

	it('reports available and round-trips responses, meta and outbox', async () => {
		db = await openOfflineDB(SERVER_A);
		expect(db.available).toBe(true);

		await db.putResponse(response('/api/v1/tasks', '2026-01-01T00:00:00.000Z', { items: [1, 2] }));
		const got = await db.getResponse('/api/v1/tasks');
		expect(got).not.toBeNull();
		expect(got?.payload).toEqual({ items: [1, 2] });
		expect(got?.storedAt).toBe('2026-01-01T00:00:00.000Z');
		expect(await db.getResponse('/missing')).toBeNull();

		await db.setMeta('lastSyncAt', '2026-01-02T00:00:00.000Z');
		expect(await db.getMeta<string>('lastSyncAt')).toBe('2026-01-02T00:00:00.000Z');
		expect(await db.getMeta('never-set')).toBeUndefined();

		const seq = await db.enqueue(op({ type: 'task.createInbox' }));
		expect(seq).toBeGreaterThan(0);
		const queued = await db.listOutbox();
		expect(queued).toHaveLength(1);
		expect(queued[0].seq).toBe(seq);
		expect(queued[0].type).toBe('task.createInbox');

		// autoIncrement + FIFO ordering
		const seq2 = await db.enqueue(op());
		expect(seq2).toBeGreaterThan(seq);
		expect((await db.listOutbox()).map((o) => o.seq)).toEqual([seq, seq2]);

		await db.deleteOutbox(seq);
		expect((await db.listOutbox()).map((o) => o.seq)).toEqual([seq2]);

		// serverUrl is persisted for the binding check
		expect(await db.getMeta<string>('serverUrl')).toBe(SERVER_A);
	});

	it('mints strictly-decreasing negative tmp ids, persisted across reopen', async () => {
		db = await openOfflineDB(SERVER_A);
		expect(await db.nextTmpId()).toBe(-1);
		expect(await db.nextTmpId()).toBe(-2);
		expect(await db.getMeta<number>('tmpIdCounter')).toBe(2);
		db.close();

		// The counter survives a restart so a tmp id is never reused.
		db = await openOfflineDB(SERVER_A);
		expect(await db.nextTmpId()).toBe(-3);
	});

	it('moveToFailed relocates a queued op into failedOps with failure metadata', async () => {
		db = await openOfflineDB(SERVER_A);
		const seq = await db.enqueue(op({ type: 'task.complete', payload: { taskId: 5 } }));
		const [queued] = await db.listOutbox();

		await db.moveToFailed(queued, {
			failedAt: '2026-02-02T00:00:00.000Z',
			errorCode: 'conflict',
			errorMessage: 'task not found'
		});

		expect(await db.listOutbox()).toHaveLength(0);
		const failed = await db.listFailed();
		expect(failed).toHaveLength(1);
		expect(failed[0]).toMatchObject({
			seq,
			type: 'task.complete',
			payload: { taskId: 5 },
			failedAt: '2026-02-02T00:00:00.000Z',
			errorCode: 'conflict',
			errorMessage: 'task not found'
		});
	});

	it('evicts the oldest responses beyond the 500 cap', async () => {
		db = await openOfflineDB(SERVER_A);
		const base = Date.UTC(2020, 0, 1);
		const over = MAX_RESPONSES + 5; // 505 total inserts
		for (let i = 0; i < over; i++) {
			const storedAt = new Date(base + i * 1000).toISOString();
			await db.putResponse(response(`k${i}`, storedAt));
		}
		const all = await db.getAllResponses();
		expect(all).toHaveLength(MAX_RESPONSES);
		// The five oldest by storedAt are gone; the rest survive.
		for (let i = 0; i < 5; i++) {
			expect(await db.getResponse(`k${i}`)).toBeNull();
		}
		expect(await db.getResponse('k5')).not.toBeNull();
		expect(await db.getResponse(`k${over - 1}`)).not.toBeNull();
	});

	it('wipes responses and quarantines incompatible outbox ops on schemaVersion mismatch', async () => {
		db = await openOfflineDB(SERVER_A);
		await db.putResponse(response('/api/v1/tasks', '2026-01-01T00:00:00.000Z'));
		const keptSeq = await db.enqueue(op({ v: 1 }));
		const staleSeq = await db.enqueue(op({ v: 0, type: 'task.uncomplete' }));
		// Simulate a DB written by an older bundle.
		await db.setMeta('schemaVersion', 0);
		db.close();

		db = await openOfflineDB(SERVER_A);
		expect(await db.getAllResponses()).toHaveLength(0);

		const remaining = await db.listOutbox();
		expect(remaining.map((o) => o.seq)).toEqual([keptSeq]);

		const failed = await db.listFailed();
		expect(failed).toHaveLength(1);
		expect(failed[0].seq).toBe(staleSeq);
		expect(failed[0].errorCode).toBe('schema_migration');
		expect(failed[0].failedAt).toBeTruthy();

		// schemaVersion is repaired to the current constant.
		expect(await db.getMeta<number>('schemaVersion')).toBe(1);
	});

	it('clears everything when the server url changes, warning about pending ops', async () => {
		db = await openOfflineDB(SERVER_A);
		await db.putResponse(response('/api/v1/tasks', '2026-01-01T00:00:00.000Z'));
		await db.enqueue(op());
		db.close();

		const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
		db = await openOfflineDB(SERVER_B);
		expect(warn).toHaveBeenCalled();
		warn.mockRestore();

		expect(await db.getAllResponses()).toHaveLength(0);
		expect(await db.listOutbox()).toHaveLength(0);
		expect(await db.listFailed()).toHaveLength(0);
		expect(await db.getMeta<string>('serverUrl')).toBe(SERVER_B);
	});

	it('does not warn on a server change when the outbox is empty', async () => {
		db = await openOfflineDB(SERVER_A);
		await db.putResponse(response('/api/v1/tasks', '2026-01-01T00:00:00.000Z'));
		db.close();

		const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
		db = await openOfflineDB(SERVER_B);
		expect(warn).not.toHaveBeenCalled();
		warn.mockRestore();

		expect(await db.getAllResponses()).toHaveLength(0);
		expect(await db.getMeta<string>('serverUrl')).toBe(SERVER_B);
	});

	it('degrades to a no-op implementation when IndexedDB is unavailable', async () => {
		const saved = globalThis.indexedDB;
		// Emulate old-Safari private mode / SSR: no IndexedDB at all.
		(globalThis as unknown as { indexedDB: unknown }).indexedDB = undefined;
		const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
		try {
			const noop = await openOfflineDB(SERVER_A);
			expect(noop.available).toBe(false);
			expect(warn).toHaveBeenCalled();
			// Every method resolves without throwing and yields empty results.
			await expect(noop.putResponse(response('/x', '2026-01-01T00:00:00.000Z'))).resolves.toBeUndefined();
			expect(await noop.getResponse('/x')).toBeNull();
			expect(await noop.getAllResponses()).toEqual([]);
			expect(await noop.listOutbox()).toEqual([]);
			expect(await noop.listFailed()).toEqual([]);
			expect(await noop.getMeta('serverUrl')).toBeUndefined();
			await expect(noop.enqueue(op())).resolves.toBeDefined();
			await expect(noop.clearAll()).resolves.toBeUndefined();
			noop.close();
		} finally {
			warn.mockRestore();
			globalThis.indexedDB = saved;
		}
	});

	it('degrades to a no-op when opening IndexedDB throws', async () => {
		const saved = globalThis.indexedDB;
		(globalThis as unknown as { indexedDB: { open: () => never } }).indexedDB = {
			open() {
				throw new Error('SecurityError: access denied');
			}
		} as never;
		const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
		try {
			const noop = await openOfflineDB(SERVER_A);
			expect(noop.available).toBe(false);
			expect(warn).toHaveBeenCalled();
		} finally {
			warn.mockRestore();
			globalThis.indexedDB = saved;
		}
	});
});

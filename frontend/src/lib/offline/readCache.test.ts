import 'fake-indexeddb/auto';
import { IDBFactory } from 'fake-indexeddb';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { openOfflineDB, type OfflineDB } from './db';
import { createReadCache, isCacheable, type ReadCache } from './readCache';

const SERVER = 'https://x.example.com';

beforeEach(() => {
	// Fresh in-memory IndexedDB for every test.
	globalThis.indexedDB = new IDBFactory();
});

describe('isCacheable', () => {
	it('caches GET paths under /api/v1', () => {
		expect(isCacheable('/api/v1/tasks')).toBe(true);
		expect(isCacheable('/api/v1/views/today')).toBe(true);
		expect(isCacheable('/api/v1/inbox')).toBe(true);
	});

	it('excludes the one-shot events ticket and the backup blob (§4.3)', () => {
		expect(isCacheable('/api/v1/events/ticket')).toBe(false);
		expect(isCacheable('/api/v1/backup')).toBe(false);
	});

	it('does not cache paths outside /api/v1', () => {
		expect(isCacheable('/api/config')).toBe(false);
		expect(isCacheable('/auth/refresh')).toBe(false);
		expect(isCacheable('/anything')).toBe(false);
	});

	it('strips a trailing query string before matching', () => {
		expect(isCacheable('/api/v1/backup?foo=1')).toBe(false);
		expect(isCacheable('/api/v1/tasks?done=1')).toBe(true);
	});
});

describe('createReadCache', () => {
	let db: OfflineDB;
	let cache: ReadCache;

	afterEach(() => {
		db?.close();
	});

	it('write-through then read hit returns the stored payload', async () => {
		db = await openOfflineDB(SERVER);
		cache = createReadCache(db, () => '2026-07-15T00:00:00.000Z');

		await cache.put('/api/v1/tasks', { limit: 50 }, { items: [1, 2] });
		const hit = await cache.get('/api/v1/tasks', { limit: 50 });

		expect(hit).not.toBeNull();
		expect(hit?.payload).toEqual({ items: [1, 2] });
		expect(hit?.storedAt).toBe('2026-07-15T00:00:00.000Z');
		expect(hit?.path).toBe('/api/v1/tasks');
	});

	it('returns null on a miss', async () => {
		db = await openOfflineDB(SERVER);
		cache = createReadCache(db);
		expect(await cache.get('/api/v1/tasks')).toBeNull();
	});

	it('does not cache excluded paths (§4.3)', async () => {
		db = await openOfflineDB(SERVER);
		cache = createReadCache(db);

		await cache.put('/api/v1/events/ticket', undefined, { ticket: 'abc' });
		await cache.put('/api/v1/backup', undefined, { blob: 'big' });

		expect(await cache.get('/api/v1/events/ticket')).toBeNull();
		expect(await cache.get('/api/v1/backup')).toBeNull();
		expect(await db.getAllResponses()).toHaveLength(0);
	});

	it('does not cache paths outside /api/v1', async () => {
		db = await openOfflineDB(SERVER);
		cache = createReadCache(db);
		await cache.put('/api/config', undefined, { public: false });
		expect(await db.getAllResponses()).toHaveLength(0);
	});

	it('collapses queries that differ only in key order to one entry (cacheKey stability)', async () => {
		db = await openOfflineDB(SERVER);
		cache = createReadCache(db);

		await cache.put('/api/v1/tasks', { a: 1, b: 2 }, { v: 'first' });
		await cache.put('/api/v1/tasks', { b: 2, a: 1 }, { v: 'second' });

		expect(await db.getAllResponses()).toHaveLength(1);
		// A read with either ordering hits the same (latest) entry.
		expect((await cache.get('/api/v1/tasks', { a: 1, b: 2 }))?.payload).toEqual({ v: 'second' });
		expect((await cache.get('/api/v1/tasks', { b: 2, a: 1 }))?.payload).toEqual({ v: 'second' });
	});

	it('drops null/undefined query values when keying', async () => {
		db = await openOfflineDB(SERVER);
		cache = createReadCache(db);
		await cache.put('/api/v1/tasks', { limit: 50, skip: undefined, cursor: null }, { ok: true });
		// The same request without the empty params reads the same entry.
		expect((await cache.get('/api/v1/tasks', { limit: 50 }))?.payload).toEqual({ ok: true });
	});

	it('does not store an undefined payload (e.g. a 204 GET)', async () => {
		db = await openOfflineDB(SERVER);
		cache = createReadCache(db);
		await cache.put('/api/v1/tasks', undefined, undefined);
		expect(await cache.get('/api/v1/tasks')).toBeNull();
	});

	it('exposes getAll and putEntry so op patchers can rewrite entries (§4.5)', async () => {
		db = await openOfflineDB(SERVER);
		cache = createReadCache(db, () => '2026-07-15T00:00:00.000Z');
		await cache.put('/api/v1/tasks', undefined, { items: [] });

		const all = await cache.getAll();
		expect(all).toHaveLength(1);

		await cache.putEntry({ ...all[0], payload: { items: [{ id: 1 }] } });
		expect((await cache.get('/api/v1/tasks'))?.payload).toEqual({ items: [{ id: 1 }] });
	});

	it('findTask is a defined reader stub that resolves to null for now (§4.5)', async () => {
		db = await openOfflineDB(SERVER);
		cache = createReadCache(db);
		expect(await cache.findTask(1)).toBeNull();
	});
});

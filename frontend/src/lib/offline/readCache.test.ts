import 'fake-indexeddb/auto';
import { IDBFactory } from 'fake-indexeddb';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { openOfflineDB, type OfflineDB } from './db';
import { createReadCache, isCacheable, matchTaskDetailPath, type ReadCache } from './readCache';

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

});

describe('findTask cross-shape scan (§4.5)', () => {
	let db: OfflineDB;
	let cache: ReadCache;

	afterEach(() => {
		db?.close();
	});

	// Minimal Task carrying a Task-distinctive field (dayPart) so it is told apart
	// from a Project, which shares the responses store and also has id/status.
	function task(id: number, extra: Record<string, unknown> = {}) {
		return { id, title: `t${id}`, status: 'open', dayPart: 'none', ...extra };
	}

	async function open() {
		db = await openOfflineDB(SERVER);
		cache = createReadCache(db);
	}

	it('returns null when nothing is cached', async () => {
		await open();
		expect(await cache.findTask(1)).toBeNull();
	});

	it('finds a task inside a Page<Task> (.items)', async () => {
		await open();
		await cache.put('/api/v1/tasks', { limit: 50 }, {
			items: [task(1), task(2)],
			total: 2,
			limit: 50,
			offset: 0
		});
		expect(await cache.findTask(2)).toMatchObject({ id: 2, title: 't2' });
		expect(await cache.findTask(99)).toBeNull();
	});

	it('finds a task inside an InboxResponse (.tasks[])', async () => {
		await open();
		await cache.put('/api/v1/inbox', undefined, {
			count: 1,
			warnThresholdExceeded: false,
			tasks: [task(3)]
		});
		expect(await cache.findTask(3)).toMatchObject({ id: 3 });
	});

	it('finds a task inside a TodayBundle (today / overdue / completedToday)', async () => {
		await open();
		await cache.put('/api/v1/views/today', undefined, {
			today: { items: [task(4)], total: 1 },
			overdue: { items: [task(5)], total: 1 },
			completedToday: { items: [task(6, { status: 'completed' })], total: 1 }
		});
		expect(await cache.findTask(4)).toMatchObject({ id: 4 });
		expect(await cache.findTask(5)).toMatchObject({ id: 5 });
		expect(await cache.findTask(6)).toMatchObject({ id: 6, status: 'completed' });
	});

	it('finds a task inside a ProjectBundle (.tasks.items) without matching the project', async () => {
		await open();
		await cache.put('/api/v1/projects/1/bundle', undefined, {
			// The project shares the id namespace but is NOT a task and must be ignored.
			project: { id: 1, title: 'proj', status: 'open', projectType: 'generic' },
			sections: { items: [], total: 0, limit: 0, offset: 0 },
			tasks: { items: [task(7)], total: 1, limit: 50, offset: 0 }
		});
		expect(await cache.findTask(7)).toMatchObject({ id: 7 });
		// id 1 is the project, not a task — no false positive.
		expect(await cache.findTask(1)).toBeNull();
	});

	it('finds a bare Task response (GET /api/v1/tasks/:id)', async () => {
		await open();
		await cache.put('/api/v1/tasks/8', undefined, task(8));
		expect(await cache.findTask(8)).toMatchObject({ id: 8 });
	});

	it('finds a task nested in a TroikiViewResponse (slot → projects[].tasks[])', async () => {
		await open();
		await cache.put('/api/v1/troiki', undefined, {
			started: true,
			important: {
				capacity: 3,
				// The project shares the id namespace but is NOT a task and must be ignored.
				projects: [{ id: 1, title: 'proj', status: 'open', projectType: 'generic', tasks: [task(10)] }]
			},
			medium: { capacity: 3, projects: [{ id: 2, title: 'proj2', status: 'open', projectType: 'generic', tasks: [task(11)] }] },
			rest: { capacity: 3, projects: [] }
		});
		expect(await cache.findTask(10)).toMatchObject({ id: 10 });
		expect(await cache.findTask(11)).toMatchObject({ id: 11 });
		// The Troiki projects are not tasks — no false positive on their ids.
		expect(await cache.findTask(1)).toBeNull();
		// And the detail synthesis can rebuild the page for such a task.
		expect(await cache.findTaskDetail(10, false)).not.toBeNull();
	});
});

describe('matchTaskDetailPath', () => {
	it('matches the task detail GET and extracts the id (tmp ids included)', () => {
		expect(matchTaskDetailPath('/api/v1/tasks/42')).toBe(42);
		expect(matchTaskDetailPath('/api/v1/tasks/42?subtasks=true')).toBe(42);
		expect(matchTaskDetailPath('/api/v1/tasks/-3')).toBe(-3);
	});

	it('does not match list or sub-resource paths', () => {
		expect(matchTaskDetailPath('/api/v1/tasks')).toBeNull();
		expect(matchTaskDetailPath('/api/v1/tasks/42/complete')).toBeNull();
		expect(matchTaskDetailPath('/api/v1/projects/42')).toBeNull();
	});
});

describe('findTaskDetail synthesis for the task page', () => {
	let db: OfflineDB;
	let cache: ReadCache;

	afterEach(() => {
		db?.close();
	});

	function task(id: number, extra: Record<string, unknown> = {}) {
		return { id, title: `t${id}`, status: 'open', dayPart: 'none', parentId: null, ...extra };
	}

	async function open(now?: () => string) {
		db = await openOfflineDB(SERVER);
		cache = createReadCache(db, now);
	}

	it('returns null when the task is nowhere in cache', async () => {
		await open();
		await cache.put('/api/v1/inbox', undefined, { tasks: [task(1)] });
		expect(await cache.findTaskDetail(2, true)).toBeNull();
	});

	it('synthesizes a detail payload from a task cached inside a list', async () => {
		await open(() => '2026-07-15T00:00:00.000Z');
		await cache.put('/api/v1/inbox', undefined, { tasks: [task(1), task(2)] });

		const hit = await cache.findTaskDetail(2, false);
		expect(hit?.payload).toMatchObject({ id: 2, title: 't2' });
		expect(hit?.storedAt).toBe('2026-07-15T00:00:00.000Z');
	});

	it('collects subtasks by parentId across cached entries', async () => {
		await open();
		await cache.put('/api/v1/views/today', undefined, { today: { items: [task(1)] } });
		await cache.put('/api/v1/inbox', undefined, {
			tasks: [task(10, { parentId: 1 }), task(11, { parentId: 1 }), task(12)]
		});

		const hit = await cache.findTaskDetail(1, true);
		const payload = hit?.payload as { subtasks: { items: { id: number }[]; total: number } };
		expect(payload.subtasks.items.map((t) => t.id).sort()).toEqual([10, 11]);
		expect(payload.subtasks.total).toBe(2);
	});

	it('reuses subtasks embedded in a previously cached detail response', async () => {
		await open();
		await cache.put('/api/v1/tasks/1', { subtasks: 'true' }, {
			...task(1),
			subtasks: { items: [task(10, { parentId: 1 })], total: 1, limit: 50, offset: 0 }
		});

		// A request without the subtasks flag misses the exact key, but the task is
		// still reachable through the detail scan.
		const hit = await cache.findTaskDetail(1, true);
		const payload = hit?.payload as { subtasks: { items: { id: number }[] } };
		expect(payload.subtasks.items.map((t) => t.id)).toEqual([10]);
	});

	it('omits the subtasks field when the caller did not ask for it', async () => {
		await open();
		await cache.put('/api/v1/inbox', undefined, {
			tasks: [task(1), task(10, { parentId: 1 })]
		});
		const hit = await cache.findTaskDetail(1, false);
		expect(hit?.payload).not.toHaveProperty('subtasks');
	});
});

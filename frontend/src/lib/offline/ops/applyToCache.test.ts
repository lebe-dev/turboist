import 'fake-indexeddb/auto';
import { IDBFactory } from 'fake-indexeddb';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { openOfflineDB, type OfflineDB } from '../db';
import { createReadCache, type ReadCache } from '../readCache';
import { taskComplete } from './taskComplete';
import { taskCreateInbox } from './taskCreateInbox';
import { taskUncomplete } from './taskUncomplete';

// applyToCache cache-patchers (FEATURE-OFFLINE-ARCH.md §4.5 applyToCache row) and
// the §6.2 "offline change survives a restart" scenario: an op enqueued offline
// patches the cached GET records so the optimistic result is still there after the
// IndexedDB is closed and reopened (an app relaunch while still offline).

const SERVER = 'https://x.example.com';

beforeEach(() => {
	globalThis.indexedDB = new IDBFactory();
});

// A cached Task carrying the Task-distinctive fields (dayPart/planState) so the
// patcher tells it apart from a Project sharing the responses store.
function task(id: number, extra: Record<string, unknown> = {}) {
	return {
		id,
		title: `t${id}`,
		status: 'open',
		dayPart: 'none',
		priority: 'no-priority',
		planState: 'none',
		completedAt: null,
		...extra
	};
}

interface CachedTask {
	id: number;
	status: string;
	completedAt: string | null;
	[k: string]: unknown;
}
interface PageShape {
	items: CachedTask[];
}
interface InboxShape {
	count: number;
	warnThresholdExceeded: boolean;
	tasks: CachedTask[];
}
interface TodayShape {
	today: { items: CachedTask[] };
	overdue: { items: CachedTask[] };
	completedToday: { items: CachedTask[] };
}

async function payload<T>(cache: ReadCache, path: string, query?: unknown): Promise<T> {
	const hit = await cache.get(path, query);
	if (!hit) throw new Error(`cache miss for ${path}`);
	return hit.payload as T;
}

describe('taskComplete.applyToCache', () => {
	let db: OfflineDB;
	afterEach(() => db?.close());

	it('marks the task completed in every cached record, leaving others untouched', async () => {
		db = await openOfflineDB(SERVER);
		const cache = createReadCache(db);
		await cache.put(
			'/api/v1/tasks',
			{ limit: 50 },
			{ items: [task(1), task(2)], total: 2, limit: 50, offset: 0 }
		);
		await cache.put('/api/v1/inbox', undefined, {
			count: 2,
			warnThresholdExceeded: false,
			tasks: [task(1), task(3)]
		});
		await cache.put('/api/v1/views/today', undefined, {
			today: { items: [task(1)], total: 1 },
			overdue: { items: [], total: 0 },
			completedToday: { items: [], total: 0 }
		});

		await taskComplete.applyToCache({ taskId: 1, completedAt: '2026-03-03T00:00:00.000Z' }, cache);

		const page = await payload<PageShape>(cache, '/api/v1/tasks', { limit: 50 });
		expect(page.items.find((t) => t.id === 1)).toMatchObject({
			status: 'completed',
			completedAt: '2026-03-03T00:00:00.000Z'
		});
		// A sibling task in the same record is untouched.
		expect(page.items.find((t) => t.id === 2)).toMatchObject({ status: 'open', completedAt: null });

		const inbox = await payload<InboxShape>(cache, '/api/v1/inbox');
		expect(inbox.tasks.find((t) => t.id === 1)).toMatchObject({ status: 'completed' });
		expect(inbox.tasks.find((t) => t.id === 3)).toMatchObject({ status: 'open' });

		const today = await payload<TodayShape>(cache, '/api/v1/views/today');
		expect(today.today.items[0]).toMatchObject({
			status: 'completed',
			completedAt: '2026-03-03T00:00:00.000Z'
		});
	});

	it('defaults completedAt to the injected clock when the payload omits it', async () => {
		db = await openOfflineDB(SERVER);
		const cache = createReadCache(db);
		await cache.put('/api/v1/tasks', undefined, {
			items: [task(5)],
			total: 1,
			limit: 50,
			offset: 0
		});

		await taskComplete.applyToCache({ taskId: 5 }, cache, () => '2026-09-09T00:00:00.000Z');

		const page = await payload<PageShape>(cache, '/api/v1/tasks');
		expect(page.items[0]).toMatchObject({
			status: 'completed',
			completedAt: '2026-09-09T00:00:00.000Z'
		});
	});

	it('patches a bare Task response (GET /tasks/:id)', async () => {
		db = await openOfflineDB(SERVER);
		const cache = createReadCache(db);
		await cache.put('/api/v1/tasks/8', undefined, task(8));

		await taskComplete.applyToCache({ taskId: 8, completedAt: '2026-03-03T00:00:00.000Z' }, cache);

		expect(await payload<CachedTask>(cache, '/api/v1/tasks/8')).toMatchObject({
			id: 8,
			status: 'completed',
			completedAt: '2026-03-03T00:00:00.000Z'
		});
	});

	it('is a no-op when the task is nowhere in cache', async () => {
		db = await openOfflineDB(SERVER);
		const cache = createReadCache(db);
		await cache.put('/api/v1/tasks', undefined, {
			items: [task(1)],
			total: 1,
			limit: 50,
			offset: 0
		});

		await taskComplete.applyToCache({ taskId: 999 }, cache);

		const page = await payload<PageShape>(cache, '/api/v1/tasks');
		expect(page.items[0]).toMatchObject({ id: 1, status: 'open' });
	});
});

describe('taskUncomplete.applyToCache', () => {
	let db: OfflineDB;
	afterEach(() => db?.close());

	it('marks the task open with completedAt cleared in every record', async () => {
		db = await openOfflineDB(SERVER);
		const cache = createReadCache(db);
		const done = { status: 'completed', completedAt: '2026-01-01T00:00:00.000Z' };
		await cache.put('/api/v1/inbox', undefined, {
			count: 1,
			warnThresholdExceeded: false,
			tasks: [task(1, done)]
		});
		await cache.put('/api/v1/tasks', undefined, {
			items: [task(1, done)],
			total: 1,
			limit: 50,
			offset: 0
		});

		await taskUncomplete.applyToCache({ taskId: 1 }, cache);

		expect((await payload<InboxShape>(cache, '/api/v1/inbox')).tasks[0]).toMatchObject({
			status: 'open',
			completedAt: null
		});
		expect((await payload<PageShape>(cache, '/api/v1/tasks')).items[0]).toMatchObject({
			status: 'open',
			completedAt: null
		});
	});
});

describe('taskCreateInbox.applyToCache', () => {
	let db: OfflineDB;
	afterEach(() => db?.close());

	it('inserts the synthesized task into inbox records and bumps count', async () => {
		db = await openOfflineDB(SERVER);
		const cache = createReadCache(db);
		await cache.put('/api/v1/inbox', undefined, {
			count: 1,
			warnThresholdExceeded: false,
			tasks: [task(1)]
		});

		await taskCreateInbox.applyToCache(
			{ input: { title: 'New', priority: 'high' }, tmpId: -3 },
			cache,
			() => '2026-06-06T00:00:00.000Z'
		);

		const inbox = await payload<InboxShape>(cache, '/api/v1/inbox');
		expect(inbox.count).toBe(2);
		expect(inbox.tasks).toHaveLength(2);
		expect(inbox.tasks[1]).toMatchObject({
			id: -3,
			title: 'New',
			priority: 'high',
			status: 'open',
			completedAt: null,
			createdAt: '2026-06-06T00:00:00.000Z'
		});
	});

	it('does not touch non-inbox records (e.g. a Page<Task>)', async () => {
		db = await openOfflineDB(SERVER);
		const cache = createReadCache(db);
		await cache.put('/api/v1/tasks', undefined, {
			items: [task(1)],
			total: 1,
			limit: 50,
			offset: 0
		});

		await taskCreateInbox.applyToCache({ input: { title: 'x' }, tmpId: -1 }, cache);

		expect((await payload<PageShape>(cache, '/api/v1/tasks')).items).toHaveLength(1);
	});

	it('is idempotent when applied twice for the same tmpId', async () => {
		db = await openOfflineDB(SERVER);
		const cache = createReadCache(db);
		await cache.put('/api/v1/inbox', undefined, {
			count: 0,
			warnThresholdExceeded: false,
			tasks: []
		});

		await taskCreateInbox.applyToCache({ input: { title: 'x' }, tmpId: -1 }, cache);
		await taskCreateInbox.applyToCache({ input: { title: 'x' }, tmpId: -1 }, cache);

		const inbox = await payload<InboxShape>(cache, '/api/v1/inbox');
		expect(inbox.tasks).toHaveLength(1);
		expect(inbox.count).toBe(1);
	});
});

describe('§6.2 an offline change survives a restart (db reopen)', () => {
	let db: OfflineDB;
	afterEach(() => db?.close());

	it('completing offline persists across a db reopen', async () => {
		db = await openOfflineDB(SERVER);
		let cache = createReadCache(db);
		await cache.put('/api/v1/views/today', undefined, {
			today: { items: [task(1)], total: 1 },
			overdue: { items: [], total: 0 },
			completedToday: { items: [], total: 0 }
		});
		await cache.put('/api/v1/inbox', undefined, {
			count: 1,
			warnThresholdExceeded: false,
			tasks: [task(1)]
		});

		await taskComplete.applyToCache({ taskId: 1, completedAt: '2026-03-03T00:00:00.000Z' }, cache);

		// Simulate an app relaunch while still offline: close and reopen the same
		// IndexedDB (the fake-indexeddb factory keeps the data across the reopen).
		db.close();
		db = await openOfflineDB(SERVER);
		cache = createReadCache(db);

		const today = await payload<TodayShape>(cache, '/api/v1/views/today');
		expect(today.today.items[0]).toMatchObject({
			status: 'completed',
			completedAt: '2026-03-03T00:00:00.000Z'
		});
		expect((await payload<InboxShape>(cache, '/api/v1/inbox')).tasks[0]).toMatchObject({
			status: 'completed'
		});
	});

	it('creating an inbox task offline persists across a db reopen', async () => {
		db = await openOfflineDB(SERVER);
		let cache = createReadCache(db);
		await cache.put('/api/v1/inbox', undefined, {
			count: 0,
			warnThresholdExceeded: false,
			tasks: []
		});

		const tmpId = await db.nextTmpId();
		await taskCreateInbox.applyToCache(
			{ input: { title: 'Buy milk' }, tmpId },
			cache,
			() => '2026-06-06T00:00:00.000Z'
		);

		db.close();
		db = await openOfflineDB(SERVER);
		cache = createReadCache(db);

		const inbox = await payload<InboxShape>(cache, '/api/v1/inbox');
		expect(inbox.count).toBe(1);
		expect(inbox.tasks).toHaveLength(1);
		expect(inbox.tasks[0]).toMatchObject({ id: tmpId, title: 'Buy milk', status: 'open' });
	});

	it('uncompleting offline persists across a db reopen', async () => {
		db = await openOfflineDB(SERVER);
		let cache = createReadCache(db);
		await cache.put('/api/v1/inbox', undefined, {
			count: 1,
			warnThresholdExceeded: false,
			tasks: [task(1, { status: 'completed', completedAt: '2026-01-01T00:00:00.000Z' })]
		});

		await taskUncomplete.applyToCache({ taskId: 1 }, cache);

		db.close();
		db = await openOfflineDB(SERVER);
		cache = createReadCache(db);

		expect((await payload<InboxShape>(cache, '/api/v1/inbox')).tasks[0]).toMatchObject({
			status: 'open',
			completedAt: null
		});
	});
});

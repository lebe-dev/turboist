import 'fake-indexeddb/auto';
import { IDBFactory } from 'fake-indexeddb';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { openOfflineDB, type OfflineDB } from '../db';
import { createReadCache, type ReadCache } from '../readCache';
import { taskUncomplete } from './taskUncomplete';
import { BLOCKED_TMP } from './types';

const SERVER = 'https://x.example.com';

beforeEach(() => {
	globalThis.indexedDB = new IDBFactory();
});

function task(id: number, extra: Record<string, unknown> = {}) {
	return {
		id,
		title: `t${id}`,
		status: 'completed',
		dayPart: 'none',
		priority: 'no-priority',
		planState: 'none',
		completedAt: '2026-01-01T00:00:00.000Z',
		...extra
	};
}

describe('taskUncomplete.match', () => {
	it('matches POST /tasks/:id/uncomplete for a real id', () => {
		expect(taskUncomplete.match('/api/v1/tasks/7/uncomplete', 'POST', undefined)).toEqual({
			taskId: 7
		});
	});

	it('returns the block marker for a tmp id (id < 0)', () => {
		expect(taskUncomplete.match('/api/v1/tasks/-7/uncomplete', 'POST', undefined)).toEqual({
			taskId: -7,
			[BLOCKED_TMP]: true
		});
	});

	it('does not match other paths or methods', () => {
		expect(taskUncomplete.match('/api/v1/tasks/7/complete', 'POST', undefined)).toBeNull();
		expect(taskUncomplete.match('/api/v1/tasks/7/uncomplete', 'GET', undefined)).toBeNull();
	});
});

describe('taskUncomplete.buildRequest', () => {
	it('rebuilds the uncomplete request (no body)', () => {
		expect(taskUncomplete.buildRequest({ taskId: 7 })).toEqual({
			path: '/api/v1/tasks/7/uncomplete',
			method: 'POST'
		});
	});
});

describe('taskUncomplete.synthesizeResponse', () => {
	let db: OfflineDB;
	let cache: ReadCache;
	afterEach(() => db?.close());

	async function open() {
		db = await openOfflineDB(SERVER);
		cache = createReadCache(db);
	}

	it('returns the cached task reopened with completedAt cleared', async () => {
		await open();
		await cache.put('/api/v1/inbox', undefined, {
			count: 1,
			warnThresholdExceeded: false,
			tasks: [task(7)]
		});
		const res = (await taskUncomplete.synthesizeResponse({ taskId: 7 }, cache)) as Record<
			string,
			unknown
		>;

		expect(res).toMatchObject({ id: 7, title: 't7', status: 'open', completedAt: null });
	});

	it('falls back to a minimal open task on a cache miss', async () => {
		await open();
		const res = (await taskUncomplete.synthesizeResponse(
			{ taskId: 9 },
			cache,
			() => '2026-04-04T00:00:00.000Z'
		)) as Record<string, unknown>;

		expect(res).toMatchObject({
			id: 9,
			status: 'open',
			completedAt: null,
			createdAt: '2026-04-04T00:00:00.000Z'
		});
	});
});

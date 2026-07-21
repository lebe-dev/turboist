import 'fake-indexeddb/auto';
import { IDBFactory } from 'fake-indexeddb';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { openOfflineDB, type OfflineDB } from '../db';
import { createReadCache, type ReadCache } from '../readCache';
import { taskComplete } from './taskComplete';
import { BLOCKED_TMP } from './types';

const SERVER = 'https://x.example.com';

beforeEach(() => {
	globalThis.indexedDB = new IDBFactory();
});

// A realistic cached Task (carries the Task-distinctive fields findTask needs).
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

describe('taskComplete.match', () => {
	it('matches POST /tasks/:id/complete for a real id', () => {
		expect(taskComplete.match('/api/v1/tasks/5/complete', 'POST', undefined)).toEqual({
			taskId: 5
		});
	});

	it('extracts completedAt from the body when present', () => {
		expect(
			taskComplete.match('/api/v1/tasks/5/complete', 'POST', {
				completedAt: '2026-01-01T00:00:00.000Z'
			})
		).toEqual({ taskId: 5, completedAt: '2026-01-01T00:00:00.000Z' });
	});

	it('returns the block marker for a tmp id (id < 0)', () => {
		expect(taskComplete.match('/api/v1/tasks/-2/complete', 'POST', undefined)).toEqual({
			taskId: -2,
			[BLOCKED_TMP]: true
		});
	});

	it('does not match other paths or methods', () => {
		expect(taskComplete.match('/api/v1/tasks/5/uncomplete', 'POST', undefined)).toBeNull();
		expect(taskComplete.match('/api/v1/tasks/5/complete', 'GET', undefined)).toBeNull();
		expect(taskComplete.match('/api/v1/inbox/tasks', 'POST', {})).toBeNull();
	});
});

describe('taskComplete.buildRequest', () => {
	it('rebuilds the complete request without a body', () => {
		expect(taskComplete.buildRequest({ taskId: 5 })).toEqual({
			path: '/api/v1/tasks/5/complete',
			method: 'POST',
			body: undefined
		});
	});

	it('includes completedAt in the body when set', () => {
		expect(taskComplete.buildRequest({ taskId: 5, completedAt: '2026-01-01T00:00:00.000Z' })).toEqual(
			{
				path: '/api/v1/tasks/5/complete',
				method: 'POST',
				body: { completedAt: '2026-01-01T00:00:00.000Z' }
			}
		);
	});
});

describe('taskComplete.synthesizeResponse', () => {
	let db: OfflineDB;
	let cache: ReadCache;
	afterEach(() => db?.close());

	async function open() {
		db = await openOfflineDB(SERVER);
		cache = createReadCache(db);
	}

	it('returns the cached task marked completed with the given completedAt', async () => {
		await open();
		await cache.put('/api/v1/tasks', { limit: 50 }, {
			items: [task(5)],
			total: 1,
			limit: 50,
			offset: 0
		});
		const res = (await taskComplete.synthesizeResponse(
			{ taskId: 5, completedAt: '2026-03-03T00:00:00.000Z' },
			cache
		)) as Record<string, unknown>;

		expect(res).toMatchObject({
			id: 5,
			title: 't5',
			status: 'completed',
			completedAt: '2026-03-03T00:00:00.000Z'
		});
	});

	it('falls back to a minimal completed task on a cache miss', async () => {
		await open();
		const res = (await taskComplete.synthesizeResponse(
			{ taskId: 9 },
			cache,
			() => '2026-04-04T00:00:00.000Z'
		)) as Record<string, unknown>;

		expect(res).toMatchObject({
			id: 9,
			status: 'completed',
			completedAt: '2026-04-04T00:00:00.000Z',
			createdAt: '2026-04-04T00:00:00.000Z'
		});
	});

	it('marks a recurring task complete without advancing its RRULE (§4.5)', async () => {
		await open();
		await cache.put('/api/v1/tasks', undefined, {
			items: [task(7, { recurrenceRule: 'FREQ=DAILY' })],
			total: 1,
			limit: 50,
			offset: 0
		});
		const res = (await taskComplete.synthesizeResponse(
			{ taskId: 7, completedAt: '2026-05-05T00:00:00.000Z' },
			cache
		)) as Record<string, unknown>;

		// A copy simply flipped to completed; the next occurrence is NOT created offline.
		expect(res.status).toBe('completed');
		expect(res.recurrenceRule).toBe('FREQ=DAILY');
	});
});

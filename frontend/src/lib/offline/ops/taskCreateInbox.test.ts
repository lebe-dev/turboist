import 'fake-indexeddb/auto';
import { IDBFactory } from 'fake-indexeddb';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { openOfflineDB, type OfflineDB } from '../db';
import { createReadCache, type ReadCache } from '../readCache';
import { taskCreateInbox } from './taskCreateInbox';

const SERVER = 'https://x.example.com';

beforeEach(() => {
	globalThis.indexedDB = new IDBFactory();
});

describe('taskCreateInbox.match', () => {
	it('matches POST /inbox/tasks and captures the input', () => {
		expect(taskCreateInbox.match('/api/v1/inbox/tasks', 'POST', { title: 'Buy milk' })).toEqual({
			input: { title: 'Buy milk' }
		});
	});

	it('does not match other paths or methods', () => {
		expect(taskCreateInbox.match('/api/v1/tasks/1/complete', 'POST', {})).toBeNull();
		expect(taskCreateInbox.match('/api/v1/inbox/tasks', 'GET', undefined)).toBeNull();
	});
});

describe('taskCreateInbox.buildRequest', () => {
	it('replays the create with the stored input', () => {
		expect(taskCreateInbox.buildRequest({ input: { title: 'x' }, tmpId: -1 })).toEqual({
			path: '/api/v1/inbox/tasks',
			method: 'POST',
			body: { title: 'x' }
		});
	});
});

describe('taskCreateInbox.synthesizeResponse', () => {
	let db: OfflineDB;
	let cache: ReadCache;
	afterEach(() => db?.close());

	it('synthesizes a tmp Task from the input with the negative id and client timestamps', async () => {
		db = await openOfflineDB(SERVER);
		cache = createReadCache(db);

		const res = (await taskCreateInbox.synthesizeResponse(
			{ input: { title: 'Buy milk', priority: 'high', isComplex: true }, tmpId: -4 },
			cache,
			() => '2026-06-06T00:00:00.000Z'
		)) as Record<string, unknown>;

		expect(res).toMatchObject({
			id: -4,
			title: 'Buy milk',
			priority: 'high',
			isComplex: true,
			status: 'open',
			completedAt: null,
			labels: [],
			createdAt: '2026-06-06T00:00:00.000Z',
			updatedAt: '2026-06-06T00:00:00.000Z'
		});
	});

	it('defaults optional fields when the input is minimal', async () => {
		db = await openOfflineDB(SERVER);
		cache = createReadCache(db);

		const res = (await taskCreateInbox.synthesizeResponse(
			{ input: { title: 'bare' }, tmpId: -1 },
			cache,
			() => '2026-06-06T00:00:00.000Z'
		)) as Record<string, unknown>;

		expect(res).toMatchObject({
			id: -1,
			title: 'bare',
			priority: 'no-priority',
			dayPart: 'none',
			planState: 'none',
			isPrivate: false,
			isComplex: false
		});
	});
});

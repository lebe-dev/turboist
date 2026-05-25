import 'fake-indexeddb/auto';
import { describe, it, expect, beforeEach } from 'vitest';
import { TurboistDB } from './db';
import {
	createTaskOffline,
	updateTaskOffline,
	deleteTaskOffline,
	completeTaskOffline,
	uncompleteTaskOffline,
	moveTaskOffline,
	isSyntheticTaskId
} from './taskMutations';
import { flush, type FetchResponse, type SyncFetch } from './sync';

const makeFetcher = (
	responses: Array<FetchResponse | Error>
): { fetcher: SyncFetch; calls: Array<{ path: string; method: string; body?: unknown; headers?: Record<string, string> }> } => {
	const calls: Array<{ path: string; method: string; body?: unknown; headers?: Record<string, string> }> = [];
	let i = 0;
	const fetcher: SyncFetch = async (path, init) => {
		calls.push({ path, method: init.method, body: init.body, headers: init.headers });
		const r = responses[i++];
		if (r instanceof Error) throw r;
		return r;
	};
	return { fetcher, calls };
};

describe('taskMutations', () => {
	let db: TurboistDB;

	beforeEach(async () => {
		db = new TurboistDB(`test-${Math.random().toString(36).slice(2)}`);
		await db.open();
	});

	it('createTaskOffline: writes synthetic Dexie row, enqueues POST outbox', async () => {
		const task = await createTaskOffline(
			{ title: 'buy milk' },
			{ contextId: 7, db }
		);
		expect(isSyntheticTaskId(task.id)).toBe(true);
		expect(task.clientId).toBeTruthy();

		const row = await db.tasks.get(task.clientId);
		expect(row?.serverId).toBeNull();
		expect((row?.data as { title: string }).title).toBe('buy milk');

		const out = await db.outbox.toArray();
		expect(out).toHaveLength(1);
		expect(out[0].op).toBe('create');
		expect(out[0].entity).toBe('tasks');
		const body = out[0].payload.body as Record<string, unknown>;
		expect(body.title).toBe('buy milk');
		expect(body.clientId).toBe(task.clientId);
		expect(out[0].payload.path).toBe('/api/v1/contexts/7/tasks');
	});

	it('offline create → flush → server returns real id (remap)', async () => {
		const task = await createTaskOffline(
			{ title: 'remap me' },
			{ contextId: 3, db }
		);
		expect(task.id).toBeLessThan(0);

		const { fetcher, calls } = makeFetcher([
			{
				status: 201,
				data: {
					id: 4242,
					clientId: task.clientId,
					updatedAt: '2026-05-25T10:00:00.000Z',
					title: 'remap me'
				}
			}
		]);

		const result = await flush(fetcher, db);
		expect(result.sent).toBe(1);
		expect(calls[0].path).toBe('/api/v1/contexts/3/tasks');
		expect(calls[0].method).toBe('POST');
		expect(calls[0].headers?.['Idempotency-Key']).toBeTruthy();

		const row = await db.tasks.get(task.clientId);
		expect(row?.serverId).toBe(4242);
		expect((row?.data as { id: number }).id).toBe(4242);
		expect(await db.outbox.count()).toBe(0);
	});

	it('completeTaskOffline: marks completed locally, enqueues POST /complete, no fetch', async () => {
		await db.tasks.put({
			clientId: 'srv:10',
			serverId: 10,
			updatedAt: '2026-05-25T09:00:00.000Z',
			deletedAt: null,
			data: {
				id: 10,
				title: 'x',
				status: 'open',
				clientId: 'srv:10'
			} as unknown as Record<string, unknown>
		});

		const updated = await completeTaskOffline({ id: 10 }, '2026-05-25T11:00:00.000Z', {
			db
		});
		expect(updated.status).toBe('completed');
		expect(updated.completedAt).toBe('2026-05-25T11:00:00.000Z');

		const row = await db.tasks.get('srv:10');
		expect((row?.data as { status: string }).status).toBe('completed');

		const out = await db.outbox.toArray();
		expect(out).toHaveLength(1);
		expect(out[0].payload.path).toBe('/api/v1/tasks/10/complete');
		expect(out[0].payload.method).toBe('POST');
		expect(out[0].payload.body).toEqual({ completedAt: '2026-05-25T11:00:00.000Z' });
	});

	it('LWW on update: server response with newer updatedAt overwrites local data', async () => {
		await db.tasks.put({
			clientId: 'srv:5',
			serverId: 5,
			updatedAt: '2026-05-25T08:00:00.000Z',
			deletedAt: null,
			data: {
				id: 5,
				title: 'old',
				status: 'open',
				clientId: 'srv:5'
			} as unknown as Record<string, unknown>
		});

		await updateTaskOffline({ id: 5 }, { title: 'local-edit' }, { db });
		const localRow = await db.tasks.get('srv:5');
		expect((localRow?.data as { title: string }).title).toBe('local-edit');

		const { fetcher, calls } = makeFetcher([
			{
				status: 200,
				data: {
					id: 5,
					clientId: 'srv:5',
					updatedAt: '2026-05-25T10:00:00.000Z',
					title: 'server-wins',
					status: 'open'
				}
			}
		]);
		const r = await flush(fetcher, db);
		expect(r.sent).toBe(1);
		expect(calls[0].path).toBe('/api/v1/tasks/5');
		expect(calls[0].method).toBe('PATCH');
		const body = calls[0].body as Record<string, unknown>;
		expect(body.baseUpdatedAt).toBe('2026-05-25T08:00:00.000Z');

		const after = await db.tasks.get('srv:5');
		expect((after?.data as { title: string }).title).toBe('server-wins');
		expect(after?.updatedAt).toBe('2026-05-25T10:00:00.000Z');
	});

	it('deleteTaskOffline: soft-deletes locally and enqueues DELETE', async () => {
		await db.tasks.put({
			clientId: 'srv:1',
			serverId: 1,
			updatedAt: '2026-05-25T08:00:00.000Z',
			deletedAt: null,
			data: { id: 1, title: 'x', clientId: 'srv:1' } as unknown as Record<string, unknown>
		});

		await deleteTaskOffline({ id: 1 }, { db });
		const row = await db.tasks.get('srv:1');
		expect(row?.deletedAt).not.toBeNull();

		const out = await db.outbox.toArray();
		expect(out).toHaveLength(1);
		expect(out[0].op).toBe('delete');
		expect(out[0].payload.path).toBe('/api/v1/tasks/1');
	});

	it('moveTaskOffline: applies new placement and enqueues POST /move', async () => {
		await db.tasks.put({
			clientId: 'srv:2',
			serverId: 2,
			updatedAt: '2026-05-25T08:00:00.000Z',
			deletedAt: null,
			data: {
				id: 2,
				inboxId: 1,
				contextId: null,
				projectId: null,
				sectionId: null,
				clientId: 'srv:2'
			} as unknown as Record<string, unknown>
		});

		await moveTaskOffline(
			{ id: 2 },
			{ contextId: 9, projectId: 4 },
			{ db }
		);
		const row = await db.tasks.get('srv:2');
		const data = row?.data as { inboxId: number | null; contextId: number | null; projectId: number | null };
		expect(data.inboxId).toBeNull();
		expect(data.contextId).toBe(9);
		expect(data.projectId).toBe(4);

		const out = await db.outbox.toArray();
		expect(out).toHaveLength(1);
		expect(out[0].payload.path).toBe('/api/v1/tasks/2/move');
		expect(out[0].payload.body).toEqual({ contextId: 9, projectId: 4 });
	});

	it('uncompleteTaskOffline: flips status and enqueues POST /uncomplete', async () => {
		await db.tasks.put({
			clientId: 'srv:3',
			serverId: 3,
			updatedAt: '2026-05-25T08:00:00.000Z',
			deletedAt: null,
			data: {
				id: 3,
				status: 'completed',
				completedAt: '2026-05-25T07:00:00.000Z',
				clientId: 'srv:3'
			} as unknown as Record<string, unknown>
		});
		const next = await uncompleteTaskOffline({ id: 3 }, { db });
		expect(next.status).toBe('open');
		const out = await db.outbox.toArray();
		expect(out[0].payload.path).toBe('/api/v1/tasks/3/uncomplete');
	});

	it('createTaskOffline + flush stays offline if fetcher fails (backoff)', async () => {
		await createTaskOffline({ title: 'pending' }, { contextId: 1, db });
		const { fetcher } = makeFetcher([new Error('offline')]);
		const r = await flush(fetcher, db);
		expect(r.failed).toBe(1);
		expect(await db.outbox.count()).toBe(1);
	});

	it('inbox create routes through /inbox/tasks', async () => {
		const task = await createTaskOffline({ title: 'inboxed' }, { inbox: true, db });
		expect(task.inboxId).toBe(1);
		const out = await db.outbox.toArray();
		expect(out[0].payload.path).toBe('/api/v1/inbox/tasks');
	});
});

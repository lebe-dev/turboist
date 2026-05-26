import 'fake-indexeddb/auto';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { TurboistDB } from './db';
import {
	createTaskOffline,
	updateTaskOffline,
	deleteTaskOffline,
	completeTaskOffline,
	uncompleteTaskOffline,
	moveTaskOffline,
	planTaskOffline,
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
				title: 'local-edit',
				status: 'open',
				clientId: 'srv:5'
			} as unknown as Record<string, unknown>
		});

		const futureTs = new Date(Date.now() + 60_000).toISOString();
		const { fetcher, calls } = makeFetcher([
			{
				status: 200,
				data: {
					id: 5,
					clientId: 'srv:5',
					updatedAt: futureTs,
					title: 'server-wins',
					status: 'open'
				}
			}
		]);

		await updateTaskOffline({ id: 5 }, { title: 'local-edit' }, { db });

		const r = await flush(fetcher, db);
		expect(r.sent).toBe(1);
		expect(calls[0].path).toBe('/api/v1/tasks/5');
		expect(calls[0].method).toBe('PATCH');

		const after = await db.tasks.get('srv:5');
		expect((after?.data as { title: string }).title).toBe('server-wins');
		expect(after?.updatedAt).toBe(futureTs);
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

	it('subtask under synthetic parent uses {ref:<parentClientId>} in path', async () => {
		const parent = await createTaskOffline({ title: 'P' }, { contextId: 7, db });
		expect(isSyntheticTaskId(parent.id)).toBe(true);
		const child = await createTaskOffline(
			{ title: 'C' },
			{ parentId: parent.id, db }
		);
		expect(child.clientId).toBeTruthy();
		const childEntry = (await db.outbox.toArray()).find(
			(e) => (e.payload.body as { title?: string }).title === 'C'
		);
		expect(childEntry?.payload.path).toBe(
			`/api/v1/tasks/{ref:${parent.clientId}}/subtasks`
		);
		expect(childEntry?.parentClientId).toBe(parent.clientId);
	});

	it('inbox create routes through /inbox/tasks', async () => {
		const task = await createTaskOffline({ title: 'inboxed' }, { inbox: true, db });
		expect(task.inboxId).toBe(1);
		const out = await db.outbox.toArray();
		expect(out[0].payload.path).toBe('/api/v1/inbox/tasks');
	});

	it('createTaskOffline: rolls back tasks row if enqueue fails', async () => {
		const spy = vi
			.spyOn(db.outbox, 'put')
			.mockRejectedValueOnce(new Error('outbox-down'));
		await expect(
			createTaskOffline({ title: 'rollback me' }, { contextId: 7, db })
		).rejects.toThrow();
		spy.mockRestore();
		expect(await db.tasks.count()).toBe(0);
		expect(await db.outbox.count()).toBe(0);
	});

	it('updateTaskOffline: rolls back local patch if enqueue fails', async () => {
		await db.tasks.put({
			clientId: 'srv:50',
			serverId: 50,
			updatedAt: '2026-05-25T08:00:00.000Z',
			deletedAt: null,
			data: {
				id: 50,
				title: 'before',
				status: 'open',
				clientId: 'srv:50'
			} as unknown as Record<string, unknown>
		});
		const spy = vi
			.spyOn(db.outbox, 'put')
			.mockRejectedValueOnce(new Error('outbox-down'));
		await expect(
			updateTaskOffline({ id: 50 }, { title: 'after' }, { db })
		).rejects.toThrow();
		spy.mockRestore();
		const row = await db.tasks.get('srv:50');
		expect((row?.data as { title: string }).title).toBe('before');
		expect(row?.updatedAt).toBe('2026-05-25T08:00:00.000Z');
		expect(await db.outbox.count()).toBe(0);
	});

	it('complete followed by uncomplete collapses to one /uncomplete entry', async () => {
		await db.tasks.put({
			clientId: 'srv:200',
			serverId: 200,
			updatedAt: '2026-05-25T08:00:00.000Z',
			deletedAt: null,
			data: {
				id: 200,
				title: 't',
				status: 'open',
				clientId: 'srv:200'
			} as unknown as Record<string, unknown>
		});
		await completeTaskOffline({ id: 200 }, '2026-05-25T09:00:00.000Z', { db });
		await uncompleteTaskOffline({ id: 200 }, { db });
		const out = await db.outbox.toArray();
		expect(out).toHaveLength(1);
		expect(out[0].payload.path).toBe('/api/v1/tasks/200/uncomplete');
		const row = await db.tasks.get('srv:200');
		expect((row?.data as { status: string }).status).toBe('open');
	});

	it('uncomplete followed by complete collapses to one /complete entry', async () => {
		await db.tasks.put({
			clientId: 'srv:201',
			serverId: 201,
			updatedAt: '2026-05-25T08:00:00.000Z',
			deletedAt: null,
			data: {
				id: 201,
				title: 't',
				status: 'completed',
				completedAt: '2026-05-25T07:00:00.000Z',
				clientId: 'srv:201'
			} as unknown as Record<string, unknown>
		});
		await uncompleteTaskOffline({ id: 201 }, { db });
		await completeTaskOffline({ id: 201 }, '2026-05-25T09:00:00.000Z', { db });
		const out = await db.outbox.toArray();
		expect(out).toHaveLength(1);
		expect(out[0].payload.path).toBe('/api/v1/tasks/201/complete');
	});

	it('deleteTaskOffline: drops pending update/complete mutations for the same task', async () => {
		await db.tasks.put({
			clientId: 'srv:80',
			serverId: 80,
			updatedAt: '2026-05-25T08:00:00.000Z',
			deletedAt: null,
			data: { id: 80, title: 't', status: 'open', clientId: 'srv:80' } as unknown as Record<
				string,
				unknown
			>
		});
		await updateTaskOffline({ id: 80 }, { title: 'patched' }, { db });
		await completeTaskOffline({ id: 80 }, '2026-05-25T09:00:00.000Z', { db });
		expect(await db.outbox.count()).toBe(2);
		await deleteTaskOffline({ id: 80 }, { db });
		const remaining = await db.outbox.toArray();
		expect(remaining).toHaveLength(1);
		expect(remaining[0].op).toBe('delete');
	});

	it('moveTaskOffline: inboxId branch clears context/project/section', async () => {
		await db.tasks.put({
			clientId: 'srv:30',
			serverId: 30,
			updatedAt: '2026-05-25T08:00:00.000Z',
			deletedAt: null,
			data: {
				id: 30,
				inboxId: null,
				contextId: 9,
				projectId: 4,
				sectionId: 2,
				clientId: 'srv:30'
			} as unknown as Record<string, unknown>
		});
		await moveTaskOffline({ id: 30 }, { inboxId: 1 }, { db });
		const row = await db.tasks.get('srv:30');
		const data = row?.data as {
			inboxId: number | null;
			contextId: number | null;
			projectId: number | null;
			sectionId: number | null;
		};
		expect(data.inboxId).toBe(1);
		expect(data.contextId).toBeNull();
		expect(data.projectId).toBeNull();
		expect(data.sectionId).toBeNull();
		const out = await db.outbox.toArray();
		expect(out[0].payload.path).toBe('/api/v1/tasks/30/move');
		expect(out[0].payload.body).toEqual({ inboxId: 1 });
	});

	it('moveTaskOffline: parentId branch only updates parentId', async () => {
		await db.tasks.put({
			clientId: 'srv:31',
			serverId: 31,
			updatedAt: '2026-05-25T08:00:00.000Z',
			deletedAt: null,
			data: {
				id: 31,
				parentId: null,
				inboxId: null,
				contextId: 9,
				projectId: 4,
				sectionId: 2,
				clientId: 'srv:31'
			} as unknown as Record<string, unknown>
		});
		await moveTaskOffline({ id: 31 }, { parentId: 77 }, { db });
		const row = await db.tasks.get('srv:31');
		const data = row?.data as {
			parentId: number | null;
			inboxId: number | null;
			contextId: number | null;
		};
		expect(data.parentId).toBe(77);
		expect(data.contextId).toBe(9);
		expect(data.inboxId).toBeNull();
		const out = await db.outbox.toArray();
		expect(out[0].payload.body).toEqual({ parentId: 77 });
	});

	it('planTaskOffline: updates planState and enqueues POST /plan', async () => {
		await db.tasks.put({
			clientId: 'srv:40',
			serverId: 40,
			updatedAt: '2026-05-25T08:00:00.000Z',
			deletedAt: null,
			data: {
				id: 40,
				title: 't',
				planState: 'none',
				clientId: 'srv:40'
			} as unknown as Record<string, unknown>
		});
		const next = await planTaskOffline({ id: 40 }, { state: 'week' }, { db });
		expect(next.planState).toBe('week');
		const row = await db.tasks.get('srv:40');
		expect((row?.data as { planState: string }).planState).toBe('week');
		const out = await db.outbox.toArray();
		expect(out).toHaveLength(1);
		expect(out[0].payload.path).toBe('/api/v1/tasks/40/plan');
		expect(out[0].payload.method).toBe('POST');
		expect(out[0].payload.body).toEqual({ state: 'week' });
	});

	it('planTaskOffline on synthetic task uses {serverId} placeholder', async () => {
		const task = await createTaskOffline({ title: 's' }, { contextId: 1, db });
		await planTaskOffline({ id: task.id, clientId: task.clientId }, { state: 'backlog' }, { db });
		const planEntry = (await db.outbox.toArray()).find(
			(e) => (e.payload as { path?: string }).path?.endsWith('/plan')
		);
		expect(planEntry?.payload.path).toBe('/api/v1/tasks/{serverId}/plan');
	});

	it('complete↔uncomplete multi-cycle collapses to a single last entry', async () => {
		await db.tasks.put({
			clientId: 'srv:202',
			serverId: 202,
			updatedAt: '2026-05-25T08:00:00.000Z',
			deletedAt: null,
			data: {
				id: 202,
				title: 't',
				status: 'open',
				clientId: 'srv:202'
			} as unknown as Record<string, unknown>
		});
		await completeTaskOffline({ id: 202 }, '2026-05-25T09:00:00.000Z', { db });
		await uncompleteTaskOffline({ id: 202 }, { db });
		await completeTaskOffline({ id: 202 }, '2026-05-25T09:01:00.000Z', { db });
		await uncompleteTaskOffline({ id: 202 }, { db });
		const out = await db.outbox.toArray();
		expect(out).toHaveLength(1);
		expect(out[0].payload.path).toBe('/api/v1/tasks/202/uncomplete');
		const row = await db.tasks.get('srv:202');
		expect((row?.data as { status: string }).status).toBe('open');
	});

	it('subtask under synthetic parent: full flush resolves ref to real serverId', async () => {
		const parent = await createTaskOffline({ title: 'P' }, { contextId: 7, db });
		const child = await createTaskOffline(
			{ title: 'C' },
			{ parentId: parent.id, db }
		);
		expect(child.clientId).toBeTruthy();

		const { fetcher, calls } = makeFetcher([
			{
				status: 201,
				data: {
					id: 500,
					clientId: parent.clientId,
					updatedAt: '2026-05-25T10:00:00.000Z',
					title: 'P'
				}
			},
			{
				status: 201,
				data: {
					id: 501,
					clientId: child.clientId,
					updatedAt: '2026-05-25T10:00:01.000Z',
					title: 'C',
					parentId: 500
				}
			}
		]);
		const r = await flush(fetcher, db);
		expect(r.sent).toBe(2);
		expect(calls[1].path).toBe('/api/v1/tasks/500/subtasks');
		const childRow = await db.tasks.get(child.clientId);
		expect(childRow?.serverId).toBe(501);
		expect(await db.outbox.count()).toBe(0);
	});

	it('deleteTaskOffline: keeps row alive if enqueue fails', async () => {
		await db.tasks.put({
			clientId: 'srv:51',
			serverId: 51,
			updatedAt: '2026-05-25T08:00:00.000Z',
			deletedAt: null,
			data: { id: 51, title: 'x', clientId: 'srv:51' } as unknown as Record<string, unknown>
		});
		const spy = vi
			.spyOn(db.outbox, 'put')
			.mockRejectedValueOnce(new Error('outbox-down'));
		await expect(deleteTaskOffline({ id: 51 }, { db })).rejects.toThrow();
		spy.mockRestore();
		const row = await db.tasks.get('srv:51');
		expect(row?.deletedAt).toBeNull();
		expect(await db.outbox.count()).toBe(0);
	});
});

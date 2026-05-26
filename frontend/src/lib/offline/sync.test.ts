import 'fake-indexeddb/auto';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { TurboistDB } from './db';
import { enqueue } from './outbox';
import { ref } from './ids';
import {
	flush,
	pull,
	type FetchResponse,
	type SyncFetch
} from './sync';
import { onDbChanged } from './stores';

const makeFetcher = (
	responses: Array<FetchResponse | Error> | ((path: string, init: { method: string; body?: unknown }) => FetchResponse | Promise<FetchResponse>)
): { fetcher: SyncFetch; calls: Array<{ path: string; method: string; headers?: Record<string, string>; body?: unknown }> } => {
	const calls: Array<{ path: string; method: string; headers?: Record<string, string>; body?: unknown }> = [];
	let i = 0;
	const fetcher: SyncFetch = async (path, init) => {
		calls.push({ path, method: init.method, headers: init.headers, body: init.body });
		if (typeof responses === 'function') {
			return responses(path, init);
		}
		const r = responses[i++];
		if (r instanceof Error) throw r;
		return r;
	};
	return { fetcher, calls };
};

describe('sync.pull', () => {
	let db: TurboistDB;

	beforeEach(async () => {
		db = new TurboistDB(`test-${Math.random().toString(36).slice(2)}`);
		await db.open();
	});

	it('upserts bundle entities into Dexie and stores lastPulledAt', async () => {
		const { fetcher, calls } = makeFetcher([
			{
				status: 200,
				data: {
					now: '2026-05-25T10:00:00.000Z',
					tasks: [
						{ id: 1, clientId: 'c1', updatedAt: '2026-05-25T09:59:00.000Z', title: 'a' }
					],
					projects: [],
					sections: [],
					labels: [],
					contexts: [
						{ id: 7, clientId: null, updatedAt: '2026-05-25T09:00:00.000Z', name: 'work' }
					]
				}
			}
		]);

		const bundle = await pull(undefined, fetcher, db);

		expect(bundle.now).toBe('2026-05-25T10:00:00.000Z');
		expect(calls[0].path).toBe('/api/v1/sync/pull');
		expect(calls[0].method).toBe('POST');

		const task = await db.tasks.get('c1');
		expect(task?.serverId).toBe(1);
		expect((task?.data as { title: string }).title).toBe('a');

		const ctxRow = await db.contexts.get('srv:7');
		expect(ctxRow?.serverId).toBe(7);

		const meta = await db.meta.get('lastPulledAt');
		expect(meta?.value).toBe('2026-05-25T10:00:00.000Z');
	});

	it('passes since as query parameter when provided', async () => {
		const { fetcher, calls } = makeFetcher([
			{
				status: 200,
				data: {
					now: '2026-05-25T10:00:00.000Z',
					tasks: [],
					projects: [],
					sections: [],
					labels: [],
					contexts: []
				}
			}
		]);
		await pull('2026-05-25T09:00:00.000Z', fetcher, db);
		expect(calls[0].path).toBe('/api/v1/sync/pull?since=2026-05-25T09%3A00%3A00.000Z');
	});

	it('preserves existing clientId when an upserted row matches by serverId', async () => {
		await db.tasks.put({
			clientId: 'local-cid',
			serverId: 42,
			updatedAt: '2026-05-25T08:00:00.000Z',
			deletedAt: null,
			data: { title: 'old' }
		});
		const { fetcher } = makeFetcher([
			{
				status: 200,
				data: {
					now: '2026-05-25T10:00:00.000Z',
					tasks: [
						{ id: 42, clientId: null, updatedAt: '2026-05-25T09:59:00.000Z', title: 'new' }
					],
					projects: [],
					sections: [],
					labels: [],
					contexts: []
				}
			}
		]);
		await pull(undefined, fetcher, db);
		const task = await db.tasks.get('local-cid');
		expect(task?.serverId).toBe(42);
		expect((task?.data as { title: string }).title).toBe('new');
	});

	it('emits db-changed events per non-empty entity table', async () => {
		const handler = vi.fn();
		const off = onDbChanged('tasks', handler);
		const { fetcher } = makeFetcher([
			{
				status: 200,
				data: {
					now: '2026-05-25T10:00:00.000Z',
					tasks: [{ id: 1, clientId: 'c1', updatedAt: 'x' }],
					projects: [],
					sections: [],
					labels: [],
					contexts: []
				}
			}
		]);
		await pull(undefined, fetcher, db);
		expect(handler).toHaveBeenCalledTimes(1);
		off();
	});
});

describe('sync.flush', () => {
	let db: TurboistDB;

	beforeEach(async () => {
		db = new TurboistDB(`test-${Math.random().toString(36).slice(2)}`);
		await db.open();
	});

	it('happy path: POST create returns serverId, outbox drained', async () => {
		await db.tasks.put({
			clientId: 'c1',
			serverId: null,
			updatedAt: '2026-05-25T10:00:00.000Z',
			deletedAt: null,
			data: { title: 't' }
		});
		await enqueue(
			{
				entity: 'tasks',
				op: 'create',
				clientId: 'c1',
				payload: {
					method: 'POST',
					path: '/api/v1/contexts/7/tasks',
					body: { title: 't', clientId: 'c1' }
				}
			},
			db
		);
		const { fetcher, calls } = makeFetcher([
			{
				status: 201,
				data: {
					id: 99,
					clientId: 'c1',
					updatedAt: '2026-05-25T10:00:01.000Z',
					title: 't'
				}
			}
		]);

		const result = await flush(fetcher, db);
		expect(result.sent).toBe(1);
		expect(calls[0].headers?.['Idempotency-Key']).toBeTruthy();
		expect(calls[0].body).toMatchObject({ title: 't', clientId: 'c1' });

		const task = await db.tasks.get('c1');
		expect(task?.serverId).toBe(99);
		expect(await db.outbox.count()).toBe(0);
	});

	it('remap: child entry waits until parent resolves serverId, then refs resolve', async () => {
		// parent task (offline)
		await db.tasks.put({
			clientId: 'parent-cid',
			serverId: null,
			updatedAt: '2026-05-25T10:00:00.000Z',
			deletedAt: null,
			data: { title: 'P' }
		});
		await enqueue(
			{
				entity: 'tasks',
				op: 'create',
				clientId: 'parent-cid',
				payload: {
					method: 'POST',
					path: '/api/v1/contexts/7/tasks',
					body: { title: 'P', clientId: 'parent-cid' }
				}
			},
			db
		);
		// child task (offline) referencing parent by ref
		await db.tasks.put({
			clientId: 'child-cid',
			serverId: null,
			updatedAt: '2026-05-25T10:00:00.001Z',
			deletedAt: null,
			data: { title: 'C' }
		});
		await enqueue(
			{
				entity: 'tasks',
				op: 'create',
				clientId: 'child-cid',
				parentClientId: 'parent-cid',
				payload: {
					method: 'POST',
					path: '/api/v1/tasks/{ref:parent-cid}/subtasks',
					body: { title: 'C', clientId: 'child-cid', parentId: ref('parent-cid') }
				}
			},
			db
		);

		const { fetcher, calls } = makeFetcher([
			{
				status: 201,
				data: { id: 100, clientId: 'parent-cid', updatedAt: '2026-05-25T10:01:00.000Z' }
			},
			{
				status: 201,
				data: { id: 101, clientId: 'child-cid', updatedAt: '2026-05-25T10:01:01.000Z' }
			}
		]);

		const result = await flush(fetcher, db);
		expect(result.sent).toBe(2);
		expect(calls[1].path).toBe('/api/v1/tasks/100/subtasks');
		expect(calls[1].body).toMatchObject({ parentId: 100 });

		expect(await db.outbox.count()).toBe(0);
		const child = await db.tasks.get('child-cid');
		expect(child?.serverId).toBe(101);
	});

	it('LWW conflict: 200 with server version overwrites local data', async () => {
		await db.tasks.put({
			clientId: 'c1',
			serverId: 5,
			updatedAt: '2026-05-25T09:00:00.000Z',
			deletedAt: null,
			data: { title: 'local-stale' }
		});
		await enqueue(
			{
				entity: 'tasks',
				op: 'update',
				clientId: 'c1',
				payload: {
					method: 'PATCH',
					path: '/api/v1/tasks/{serverId}',
					body: { title: 'local-stale', baseUpdatedAt: '2026-05-25T09:00:00.000Z' }
				}
			},
			db
		);
		const { fetcher } = makeFetcher([
			{
				status: 200,
				data: {
					id: 5,
					clientId: 'c1',
					updatedAt: '2026-05-25T09:30:00.000Z',
					title: 'server-wins'
				}
			}
		]);
		const result = await flush(fetcher, db);
		expect(result.sent).toBe(1);
		const task = await db.tasks.get('c1');
		expect((task?.data as { title: string }).title).toBe('server-wins');
		expect(task?.updatedAt).toBe('2026-05-25T09:30:00.000Z');
	});

	it('410 tombstone: drops outbox entry and marks local row deleted', async () => {
		await db.tasks.put({
			clientId: 'c1',
			serverId: 5,
			updatedAt: '2026-05-25T09:00:00.000Z',
			deletedAt: null,
			data: { title: 't' }
		});
		await enqueue(
			{
				entity: 'tasks',
				op: 'update',
				clientId: 'c1',
				payload: {
					method: 'PATCH',
					path: '/api/v1/tasks/{serverId}',
					body: { title: 'try-edit' }
				}
			},
			db
		);
		const { fetcher } = makeFetcher([{ status: 410, data: null }]);
		const result = await flush(fetcher, db);
		expect(result.dropped).toBe(1);
		expect(await db.outbox.count()).toBe(0);
		const row = await db.tasks.get('c1');
		expect(row?.deletedAt).not.toBeNull();
	});

	it('network error: marks entry as pending with backoff, no progress, loop exits', async () => {
		await db.tasks.put({
			clientId: 'c1',
			serverId: null,
			updatedAt: '2026-05-25T10:00:00.000Z',
			deletedAt: null,
			data: {}
		});
		const out = await enqueue(
			{
				entity: 'tasks',
				op: 'create',
				clientId: 'c1',
				payload: {
					method: 'POST',
					path: '/api/v1/contexts/1/tasks',
					body: { title: 't' }
				}
			},
			db
		);
		const { fetcher } = makeFetcher([new Error('offline')]);
		const result = await flush(fetcher, db);
		expect(result.failed).toBe(1);
		expect(result.sent).toBe(0);
		const reloaded = await db.outbox.get(out.id);
		expect(reloaded?.status).toBe('pending');
		expect(reloaded?.attempts).toBe(1);
		expect(reloaded?.nextAttemptAt).toBeGreaterThan(Date.now());
	});

	it('4xx response: marks entry as permanently failed', async () => {
		await db.tasks.put({
			clientId: 'c1',
			serverId: null,
			updatedAt: '2026-05-25T10:00:00.000Z',
			deletedAt: null,
			data: {}
		});
		const out = await enqueue(
			{
				entity: 'tasks',
				op: 'create',
				clientId: 'c1',
				payload: {
					method: 'POST',
					path: '/api/v1/contexts/1/tasks',
					body: { title: 't' }
				}
			},
			db
		);
		const { fetcher } = makeFetcher([{ status: 400, data: { error: { code: 'validation_failed' } } }]);
		const result = await flush(fetcher, db);
		expect(result.failed).toBe(1);
		const reloaded = await db.outbox.get(out.id);
		expect(reloaded?.status).toBe('failed');
	});

	it('DELETE for a never-uploaded row is a no-op drop', async () => {
		await db.tasks.put({
			clientId: 'c1',
			serverId: null,
			updatedAt: '2026-05-25T10:00:00.000Z',
			deletedAt: null,
			data: {}
		});
		await enqueue(
			{
				entity: 'tasks',
				op: 'delete',
				clientId: 'c1',
				payload: { method: 'DELETE', path: '/api/v1/tasks/{serverId}' }
			},
			db
		);
		const { fetcher, calls } = makeFetcher([]);
		const result = await flush(fetcher, db);
		expect(result.dropped).toBe(1);
		expect(calls).toHaveLength(0);
		expect(await db.outbox.count()).toBe(0);
		const row = await db.tasks.get('c1');
		expect(row?.deletedAt).not.toBeNull();
	});

	it('LWW: server response with older updatedAt does not overwrite local data', async () => {
		await db.tasks.put({
			clientId: 'lww',
			serverId: 8,
			updatedAt: '2026-05-26T12:01:00.000Z',
			deletedAt: null,
			data: { id: 8, title: 'local-fresh', clientId: 'lww' }
		});
		await enqueue(
			{
				entity: 'tasks',
				op: 'update',
				clientId: 'lww',
				payload: {
					method: 'PATCH',
					path: '/api/v1/tasks/8',
					body: { title: 'local-fresh' }
				}
			},
			db
		);
		const { fetcher } = makeFetcher([
			{
				status: 200,
				data: {
					id: 8,
					clientId: 'lww',
					updatedAt: '2026-05-26T12:00:00.000Z',
					title: 'server-replay'
				}
			}
		]);
		await flush(fetcher, db);
		const row = await db.tasks.get('lww');
		expect((row?.data as { title: string }).title).toBe('local-fresh');
		expect(row?.serverId).toBe(8);
	});

	it('applyServerResponse preserves local tombstone if server response has deletedAt:null', async () => {
		const ts = '2026-05-26T10:00:00.000Z';
		await db.tasks.put({
			clientId: 'tomb',
			serverId: 7,
			updatedAt: ts,
			deletedAt: ts,
			data: { id: 7, title: 't', clientId: 'tomb' }
		});
		await enqueue(
			{
				entity: 'tasks',
				op: 'update',
				clientId: 'tomb',
				payload: {
					method: 'PATCH',
					path: '/api/v1/tasks/7',
					body: { title: 'late-edit' }
				}
			},
			db
		);
		const { fetcher } = makeFetcher([
			{
				status: 200,
				data: {
					id: 7,
					clientId: 'tomb',
					updatedAt: '2026-05-26T10:01:00.000Z',
					title: 'late-edit',
					deletedAt: null
				}
			}
		]);
		await flush(fetcher, db);
		const row = await db.tasks.get('tomb');
		expect(row?.deletedAt).toBe(ts);
	});

	it('parallel flush calls coalesce: each entry sent once', async () => {
		await db.tasks.put({
			clientId: 'c1',
			serverId: null,
			updatedAt: '2026-05-25T10:00:00.000Z',
			deletedAt: null,
			data: {}
		});
		await enqueue(
			{
				entity: 'tasks',
				op: 'create',
				clientId: 'c1',
				payload: {
					method: 'POST',
					path: '/api/v1/contexts/1/tasks',
					body: { title: 't', clientId: 'c1' }
				}
			},
			db
		);

		let resolveServer: (value: FetchResponse) => void = () => {};
		const serverPromise = new Promise<FetchResponse>((res) => {
			resolveServer = res;
		});
		const calls: Array<{ path: string }> = [];
		const fetcher: SyncFetch = async (path) => {
			calls.push({ path });
			return serverPromise;
		};

		const a = flush(fetcher, db);
		const b = flush(fetcher, db);
		resolveServer({
			status: 201,
			data: { id: 12, clientId: 'c1', updatedAt: '2026-05-25T10:00:01.000Z' }
		});
		const [ra, rb] = await Promise.all([a, b]);
		expect(calls).toHaveLength(1);
		expect(ra).toBe(rb);
		expect(ra.sent).toBe(1);
		expect(await db.outbox.count()).toBe(0);
	});

	it('Idempotency-Key is stable across retries', async () => {
		await db.tasks.put({
			clientId: 'c1',
			serverId: null,
			updatedAt: '2026-05-25T10:00:00.000Z',
			deletedAt: null,
			data: {}
		});
		const out = await enqueue(
			{
				entity: 'tasks',
				op: 'create',
				clientId: 'c1',
				payload: {
					method: 'POST',
					path: '/api/v1/contexts/1/tasks',
					body: { title: 't' }
				}
			},
			db
		);
		const { fetcher: fetcher1, calls: calls1 } = makeFetcher([new Error('boom')]);
		await flush(fetcher1, db);
		// reschedule manually
		await db.outbox.update(out.id, { nextAttemptAt: Date.now() - 1 });
		const { fetcher: fetcher2, calls: calls2 } = makeFetcher([
			{ status: 201, data: { id: 10, clientId: 'c1', updatedAt: 't' } }
		]);
		await flush(fetcher2, db);
		expect(calls1[0].headers?.['Idempotency-Key']).toBe(calls2[0].headers?.['Idempotency-Key']);
	});
});

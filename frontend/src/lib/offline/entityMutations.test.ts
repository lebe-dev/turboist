import 'fake-indexeddb/auto';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { TurboistDB } from './db';
import {
	createProjectOffline,
	updateProjectOffline,
	deleteProjectOffline,
	createSectionOffline,
	updateSectionOffline,
	deleteSectionOffline,
	createLabelOffline,
	updateLabelOffline,
	deleteLabelOffline,
	createContextOffline,
	updateContextOffline,
	deleteContextOffline,
	isSyntheticEntityId
} from './entityMutations';
import { flush, type FetchResponse, type SyncFetch } from './sync';

const makeFetcher = (
	responses: Array<FetchResponse | Error>
): {
	fetcher: SyncFetch;
	calls: Array<{ path: string; method: string; body?: unknown; headers?: Record<string, string> }>;
} => {
	const calls: Array<{
		path: string;
		method: string;
		body?: unknown;
		headers?: Record<string, string>;
	}> = [];
	let i = 0;
	const fetcher: SyncFetch = async (path, init) => {
		calls.push({ path, method: init.method, body: init.body, headers: init.headers });
		const r = responses[i++];
		if (r instanceof Error) throw r;
		return r;
	};
	return { fetcher, calls };
};

describe('entityMutations: projects', () => {
	let db: TurboistDB;

	beforeEach(async () => {
		db = new TurboistDB(`test-${Math.random().toString(36).slice(2)}`);
		await db.open();
	});

	it('create offline → flush → remap to server id', async () => {
		const proj = await createProjectOffline({ title: 'Alpha' }, { contextId: 7, db });
		expect(isSyntheticEntityId(proj.id)).toBe(true);
		expect(proj.clientId).toBeTruthy();

		const row = await db.projects.get(proj.clientId);
		expect(row?.serverId).toBeNull();
		expect((row?.data as { title: string }).title).toBe('Alpha');

		const out = await db.outbox.toArray();
		expect(out).toHaveLength(1);
		expect(out[0].entity).toBe('projects');
		expect(out[0].payload.path).toBe('/api/v1/contexts/7/projects');
		const body = out[0].payload.body as Record<string, unknown>;
		expect(body.title).toBe('Alpha');
		expect(body.clientId).toBe(proj.clientId);

		const { fetcher, calls } = makeFetcher([
			{
				status: 201,
				data: {
					id: 555,
					clientId: proj.clientId,
					updatedAt: '2026-05-25T10:00:00.000Z',
					title: 'Alpha'
				}
			}
		]);
		const result = await flush(fetcher, db);
		expect(result.sent).toBe(1);
		expect(calls[0].headers?.['Idempotency-Key']).toBeTruthy();
		const after = await db.projects.get(proj.clientId);
		expect(after?.serverId).toBe(555);
		expect(await db.outbox.count()).toBe(0);
	});

	it('update offline patches Dexie + enqueues PATCH with baseUpdatedAt', async () => {
		await db.projects.put({
			clientId: 'srv:11',
			serverId: 11,
			updatedAt: '2026-05-25T08:00:00.000Z',
			deletedAt: null,
			data: { id: 11, title: 'old', clientId: 'srv:11' }
		});
		const next = await updateProjectOffline({ id: 11 }, { title: 'new' }, { db });
		expect(next.title).toBe('new');
		const out = await db.outbox.toArray();
		expect(out[0].payload.path).toBe('/api/v1/projects/11');
		expect(out[0].payload.method).toBe('PATCH');
		const body = out[0].payload.body as Record<string, unknown>;
		expect(body.baseUpdatedAt).toBe('2026-05-25T08:00:00.000Z');
		expect(body.title).toBe('new');
	});

	it('delete offline marks tombstone and enqueues DELETE', async () => {
		await db.projects.put({
			clientId: 'srv:12',
			serverId: 12,
			updatedAt: '2026-05-25T08:00:00.000Z',
			deletedAt: null,
			data: { id: 12, title: 'x', clientId: 'srv:12' }
		});
		await deleteProjectOffline({ id: 12 }, { db });
		const row = await db.projects.get('srv:12');
		expect(row?.deletedAt).not.toBeNull();
		const out = await db.outbox.toArray();
		expect(out[0].op).toBe('delete');
		expect(out[0].payload.path).toBe('/api/v1/projects/12');
	});
});

describe('entityMutations: sections', () => {
	let db: TurboistDB;

	beforeEach(async () => {
		db = new TurboistDB(`test-${Math.random().toString(36).slice(2)}`);
		await db.open();
	});

	it('create offline → flush → remap', async () => {
		const sec = await createSectionOffline({ title: 'S1' }, { projectId: 3, db });
		const out = await db.outbox.toArray();
		expect(out[0].entity).toBe('sections');
		expect(out[0].payload.path).toBe('/api/v1/projects/3/sections');

		const { fetcher } = makeFetcher([
			{
				status: 201,
				data: {
					id: 99,
					clientId: sec.clientId,
					updatedAt: '2026-05-25T10:00:00.000Z',
					title: 'S1',
					projectId: 3,
					position: 0
				}
			}
		]);
		const r = await flush(fetcher, db);
		expect(r.sent).toBe(1);
		const row = await db.sections.get(sec.clientId);
		expect(row?.serverId).toBe(99);
	});

	it('section under synthetic project uses {ref:<clientId>} in path', async () => {
		const proj = await createProjectOffline({ title: 'P' }, { contextId: 1, db });
		expect(isSyntheticEntityId(proj.id)).toBe(true);
		const sec = await createSectionOffline({ title: 'S' }, { projectId: proj.id, db });
		expect(sec.clientId).toBeTruthy();
		const sectionEntry = (await db.outbox.toArray()).find((e) => e.entity === 'sections');
		expect(sectionEntry?.payload.path).toBe(
			`/api/v1/projects/{ref:${proj.clientId}}/sections`
		);
		expect(sectionEntry?.parentClientId).toBe(proj.clientId);
	});

	it('update + delete enqueue correct paths', async () => {
		await db.sections.put({
			clientId: 'srv:20',
			serverId: 20,
			updatedAt: '2026-05-25T08:00:00.000Z',
			deletedAt: null,
			data: { id: 20, title: 'old', clientId: 'srv:20' }
		});
		await updateSectionOffline({ id: 20 }, { title: 'new' }, { db });
		await deleteSectionOffline({ id: 20 }, { db });
		const out = await db.outbox.toArray();
		expect(out).toHaveLength(1);
		expect(out[0].op).toBe('delete');
		expect(out[0].payload.path).toBe('/api/v1/sections/20');
	});
});

describe('entityMutations: labels', () => {
	let db: TurboistDB;

	beforeEach(async () => {
		db = new TurboistDB(`test-${Math.random().toString(36).slice(2)}`);
		await db.open();
	});

	it('create at /labels and remap on flush', async () => {
		const lab = await createLabelOffline({ name: 'urgent', color: 'red' }, { db });
		const out = await db.outbox.toArray();
		expect(out[0].entity).toBe('labels');
		expect(out[0].payload.path).toBe('/api/v1/labels');

		const { fetcher } = makeFetcher([
			{
				status: 201,
				data: {
					id: 77,
					clientId: lab.clientId,
					updatedAt: '2026-05-25T10:00:00.000Z',
					name: 'urgent',
					color: 'red'
				}
			}
		]);
		await flush(fetcher, db);
		const row = await db.labels.get(lab.clientId);
		expect(row?.serverId).toBe(77);
	});

	it('update + delete', async () => {
		await db.labels.put({
			clientId: 'srv:30',
			serverId: 30,
			updatedAt: '2026-05-25T08:00:00.000Z',
			deletedAt: null,
			data: { id: 30, name: 'old', clientId: 'srv:30' }
		});
		await updateLabelOffline({ id: 30 }, { name: 'new' }, { db });
		await deleteLabelOffline({ id: 30 }, { db });
		const row = await db.labels.get('srv:30');
		expect(row?.deletedAt).not.toBeNull();
		const out = await db.outbox.toArray();
		expect(out).toHaveLength(1);
		expect(out[0].op).toBe('delete');
		expect(out[0].payload.path).toBe('/api/v1/labels/30');
	});
});

describe('entityMutations: contexts', () => {
	let db: TurboistDB;

	beforeEach(async () => {
		db = new TurboistDB(`test-${Math.random().toString(36).slice(2)}`);
		await db.open();
	});

	it('create offline and remap', async () => {
		const ctx = await createContextOffline({ name: 'Work', color: 'blue' }, { db });
		const out = await db.outbox.toArray();
		expect(out[0].entity).toBe('contexts');
		expect(out[0].payload.path).toBe('/api/v1/contexts');

		const { fetcher } = makeFetcher([
			{
				status: 201,
				data: {
					id: 88,
					clientId: ctx.clientId,
					updatedAt: '2026-05-25T10:00:00.000Z',
					name: 'Work',
					color: 'blue'
				}
			}
		]);
		await flush(fetcher, db);
		const row = await db.contexts.get(ctx.clientId);
		expect(row?.serverId).toBe(88);
	});

	it('update + delete', async () => {
		await db.contexts.put({
			clientId: 'srv:40',
			serverId: 40,
			updatedAt: '2026-05-25T08:00:00.000Z',
			deletedAt: null,
			data: { id: 40, name: 'old', clientId: 'srv:40' }
		});
		await updateContextOffline({ id: 40 }, { name: 'new' }, { db });
		await deleteContextOffline({ id: 40 }, { db });
		const out = await db.outbox.toArray();
		expect(out).toHaveLength(1);
		expect(out[0].op).toBe('delete');
		expect(out[0].payload.path).toBe('/api/v1/contexts/40');
	});

	it('create then delete offline before flush: collapses to a single noop-delete', async () => {
		const ctx = await createContextOffline({ name: 'tmp' }, { db });
		await deleteContextOffline({ id: ctx.id, clientId: ctx.clientId }, { db });
		expect(await db.outbox.count()).toBe(1);
		const remaining = await db.outbox.toArray();
		expect(remaining[0].op).toBe('delete');

		const { fetcher, calls } = makeFetcher([]);
		const result = await flush(fetcher, db);
		expect(calls).toHaveLength(0);
		expect(result.dropped).toBe(1);
		expect(await db.outbox.count()).toBe(0);
		const row = await db.contexts.get(ctx.clientId);
		expect(row?.deletedAt).not.toBeNull();
	});
});

describe('entityMutations: rollback on enqueue failure', () => {
	let db: TurboistDB;

	beforeEach(async () => {
		db = new TurboistDB(`test-${Math.random().toString(36).slice(2)}`);
		await db.open();
	});

	it('deleteProjectOffline: drops pending update mutations for the same project', async () => {
		await db.projects.put({
			clientId: 'srv:90',
			serverId: 90,
			updatedAt: '2026-05-25T08:00:00.000Z',
			deletedAt: null,
			data: { id: 90, title: 'p', clientId: 'srv:90' }
		});
		await updateProjectOffline({ id: 90 }, { title: 'p2' }, { db });
		expect(await db.outbox.count()).toBe(1);
		await deleteProjectOffline({ id: 90 }, { db });
		const remaining = await db.outbox.toArray();
		expect(remaining).toHaveLength(1);
		expect(remaining[0].op).toBe('delete');
	});

	it('createProjectOffline: rolls back projects row if enqueue fails', async () => {
		const spy = vi
			.spyOn(db.outbox, 'put')
			.mockRejectedValueOnce(new Error('outbox-down'));
		await expect(
			createProjectOffline({ title: 'will rollback' }, { contextId: 7, db })
		).rejects.toThrow();
		spy.mockRestore();
		expect(await db.projects.count()).toBe(0);
		expect(await db.outbox.count()).toBe(0);
	});

	it('updateLabelOffline: rolls back local patch if enqueue fails', async () => {
		await db.labels.put({
			clientId: 'srv:60',
			serverId: 60,
			updatedAt: '2026-05-25T08:00:00.000Z',
			deletedAt: null,
			data: { id: 60, name: 'before', clientId: 'srv:60' }
		});
		const spy = vi
			.spyOn(db.outbox, 'put')
			.mockRejectedValueOnce(new Error('outbox-down'));
		await expect(updateLabelOffline({ id: 60 }, { name: 'after' }, { db })).rejects.toThrow();
		spy.mockRestore();
		const row = await db.labels.get('srv:60');
		expect((row?.data as { name: string }).name).toBe('before');
		expect(await db.outbox.count()).toBe(0);
	});

	it('deleteSectionOffline: keeps row alive if enqueue fails', async () => {
		await db.sections.put({
			clientId: 'srv:70',
			serverId: 70,
			updatedAt: '2026-05-25T08:00:00.000Z',
			deletedAt: null,
			data: { id: 70, title: 's', clientId: 'srv:70' }
		});
		const spy = vi
			.spyOn(db.outbox, 'put')
			.mockRejectedValueOnce(new Error('outbox-down'));
		await expect(deleteSectionOffline({ id: 70 }, { db })).rejects.toThrow();
		spy.mockRestore();
		const row = await db.sections.get('srv:70');
		expect(row?.deletedAt).toBeNull();
		expect(await db.outbox.count()).toBe(0);
	});
});

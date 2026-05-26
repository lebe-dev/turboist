import 'fake-indexeddb/auto';
import { describe, it, expect, beforeEach } from 'vitest';
import { TurboistDB } from './db';
import {
	enqueue,
	list,
	listReady,
	markInflight,
	markFailed,
	remove,
	pendingCount,
	remapClientId,
	nextBackoffMs,
	INFLIGHT_TIMEOUT_MS,
	RETRY_SCHEDULE_MS
} from './outbox';

describe('outbox', () => {
	let db: TurboistDB;

	beforeEach(async () => {
		db = new TurboistDB(`test-${Math.random().toString(36).slice(2)}`);
		await db.open();
	});

	it('enqueue persists a pending entry with monotonic backoff metadata', async () => {
		const entry = await enqueue(
			{ entity: 'tasks', op: 'create', clientId: 'c1', payload: { title: 'x' } },
			db
		);
		expect(entry.status).toBe('pending');
		expect(entry.attempts).toBe(0);
		expect(entry.idempotencyKey).toBeTruthy();
		const fromDb = await db.outbox.get(entry.id);
		expect(fromDb?.clientId).toBe('c1');
	});

	it('list filters by status and entity, ordered by createdAt', async () => {
		const a = await enqueue(
			{ entity: 'tasks', op: 'create', clientId: 'a', payload: {} },
			db
		);
		await new Promise((r) => setTimeout(r, 2));
		await enqueue(
			{ entity: 'projects', op: 'create', clientId: 'b', payload: {} },
			db
		);
		const tasks = await list({ entity: 'tasks' }, db);
		expect(tasks).toHaveLength(1);
		expect(tasks[0].id).toBe(a.id);
		const pending = await list({ status: 'pending' }, db);
		expect(pending).toHaveLength(2);
	});

	it('markInflight transitions status', async () => {
		const e = await enqueue(
			{ entity: 'tasks', op: 'create', clientId: 'c1', payload: {} },
			db
		);
		await markInflight(e.id, db);
		const fromDb = await db.outbox.get(e.id);
		expect(fromDb?.status).toBe('inflight');
	});

	it('markFailed schedules backoff and increments attempts', async () => {
		const e = await enqueue(
			{ entity: 'tasks', op: 'create', clientId: 'c1', payload: {} },
			db
		);
		const before = Date.now();
		const updated = await markFailed(e.id, 'net', {}, db);
		expect(updated?.attempts).toBe(1);
		expect(updated?.status).toBe('pending');
		expect(updated?.lastError).toBe('net');
		expect(updated!.nextAttemptAt).toBeGreaterThanOrEqual(before + RETRY_SCHEDULE_MS[1]);
	});

	it('markFailed with permanent marks as failed', async () => {
		const e = await enqueue(
			{ entity: 'tasks', op: 'create', clientId: 'c1', payload: {} },
			db
		);
		const updated = await markFailed(e.id, '4xx', { permanent: true }, db);
		expect(updated?.status).toBe('failed');
	});

	it('remove deletes the entry', async () => {
		const e = await enqueue(
			{ entity: 'tasks', op: 'create', clientId: 'c1', payload: {} },
			db
		);
		await remove(e.id, db);
		expect(await db.outbox.get(e.id)).toBeUndefined();
	});

	it('listReady excludes entries whose nextAttemptAt is in the future', async () => {
		const e = await enqueue(
			{ entity: 'tasks', op: 'create', clientId: 'c1', payload: {} },
			db
		);
		await markFailed(e.id, 'net', {}, db);
		const ready = await listReady(Date.now(), db);
		expect(ready).toHaveLength(0);
		const readyLater = await listReady(Date.now() + 60_000, db);
		expect(readyLater).toHaveLength(1);
	});

	it('pendingCount counts pending and inflight', async () => {
		const a = await enqueue(
			{ entity: 'tasks', op: 'create', clientId: 'a', payload: {} },
			db
		);
		await enqueue(
			{ entity: 'tasks', op: 'create', clientId: 'b', payload: {} },
			db
		);
		await markInflight(a.id, db);
		expect(await pendingCount(db)).toBe(2);
	});

	it('remapClientId rewrites parentClientId in dependent entries', async () => {
		await enqueue(
			{ entity: 'tasks', op: 'create', clientId: 'parent', payload: {} },
			db
		);
		const child = await enqueue(
			{
				entity: 'tasks',
				op: 'create',
				clientId: 'child',
				parentClientId: 'parent',
				payload: {}
			},
			db
		);
		await remapClientId('parent', 'parent-new', db);
		const reloaded = await db.outbox.get(child.id);
		expect(reloaded?.parentClientId).toBe('parent-new');
	});

	it('nextBackoffMs follows the schedule and caps at the last step', () => {
		expect(nextBackoffMs(0)).toBe(RETRY_SCHEDULE_MS[0]);
		expect(nextBackoffMs(2)).toBe(RETRY_SCHEDULE_MS[2]);
		expect(nextBackoffMs(99)).toBe(RETRY_SCHEDULE_MS[RETRY_SCHEDULE_MS.length - 1]);
	});

	it('listReady reaps inflight entries older than INFLIGHT_TIMEOUT_MS', async () => {
		const e = await enqueue(
			{ entity: 'tasks', op: 'create', clientId: 'reaper', payload: {} },
			db
		);
		await markInflight(e.id, db);

		const justAfter = await listReady(Date.now() + 1_000, db);
		expect(justAfter.find((r) => r.id === e.id)).toBeUndefined();

		const wayLater = await listReady(Date.now() + INFLIGHT_TIMEOUT_MS + 1_000, db);
		expect(wayLater.find((r) => r.id === e.id)).toBeDefined();
	});
});

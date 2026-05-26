import 'fake-indexeddb/auto';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { setDBForTests, TurboistDB } from './db';
import { enqueue, markFailed } from './outbox';
import { outboxStatusStore } from './outboxStatus.svelte';

describe('outboxStatusStore', () => {
	let db: TurboistDB;

	beforeEach(async () => {
		db = new TurboistDB(`test-${Math.random().toString(36).slice(2)}`);
		await db.open();
		setDBForTests(db);
		outboxStatusStore.pending = 0;
		outboxStatusStore.failed = [];
		outboxStatusStore.online = true;
	});

	afterEach(() => {
		outboxStatusStore.stop();
		setDBForTests(null);
	});

	it('refresh counts pending+inflight and collects failed entries', async () => {
		await enqueue({ entity: 'tasks', op: 'create', clientId: 'a', payload: {} }, db);
		const b = await enqueue({ entity: 'tasks', op: 'update', clientId: 'b', payload: {} }, db);
		await db.outbox.update(b.id, { status: 'inflight' });
		const c = await enqueue({ entity: 'tasks', op: 'delete', clientId: 'c', payload: {} }, db);
		await markFailed(c.id, 'http_400', { permanent: true }, db);

		await outboxStatusStore.refresh();
		expect(outboxStatusStore.pending).toBe(2);
		expect(outboxStatusStore.failed).toHaveLength(1);
		expect(outboxStatusStore.failed[0].lastError).toBe('http_400');
	});

	it('retry moves failed entry back to pending and clears error', async () => {
		const entry = await enqueue(
			{ entity: 'tasks', op: 'create', clientId: 'x', payload: {} },
			db
		);
		await markFailed(entry.id, 'boom', { permanent: true }, db);
		await outboxStatusStore.refresh();
		expect(outboxStatusStore.failed).toHaveLength(1);

		await outboxStatusStore.retry(entry.id);
		const row = await db.outbox.get(entry.id);
		expect(row?.status).toBe('pending');
		expect(row?.attempts).toBe(0);
		expect(row?.lastError).toBeNull();
		expect(outboxStatusStore.failed).toHaveLength(0);
		expect(outboxStatusStore.pending).toBe(1);
	});

	it('discard removes the entry from outbox', async () => {
		const entry = await enqueue(
			{ entity: 'tasks', op: 'create', clientId: 'y', payload: {} },
			db
		);
		await markFailed(entry.id, 'nope', { permanent: true }, db);
		await outboxStatusStore.discard(entry.id);
		expect(await db.outbox.get(entry.id)).toBeUndefined();
		expect(outboxStatusStore.failed).toHaveLength(0);
	});

	it('online listeners flip the reactive flag', async () => {
		outboxStatusStore.start();
		expect(outboxStatusStore.online).toBe(navigator.onLine);
		window.dispatchEvent(new Event('offline'));
		expect(outboxStatusStore.online).toBe(false);
		window.dispatchEvent(new Event('online'));
		expect(outboxStatusStore.online).toBe(true);
	});
});

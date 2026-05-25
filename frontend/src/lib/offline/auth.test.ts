import 'fake-indexeddb/auto';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { TurboistDB } from './db';
import { createDexieAuthAdapter } from './auth';

describe('createDexieAuthAdapter', () => {
	let db: TurboistDB;

	beforeEach(async () => {
		db = new TurboistDB(`auth-test-${Math.random().toString(36).slice(2)}`);
		await db.open();
	});

	it('saveUser then loadUser round-trips id and user', async () => {
		const adapter = createDexieAuthAdapter({ db });
		await adapter.saveUser({ id: 42, username: 'eu', totpEnabled: false });
		const loaded = await adapter.loadUser();
		expect(loaded).toEqual({
			id: 42,
			user: { id: 42, username: 'eu', totpEnabled: false }
		});
	});

	it('loadUser returns null when no user has been stored', async () => {
		const adapter = createDexieAuthAdapter({ db });
		expect(await adapter.loadUser()).toBeNull();
	});

	it('hasData returns true only when an entity table has rows', async () => {
		const adapter = createDexieAuthAdapter({ db });
		expect(await adapter.hasData()).toBe(false);
		await db.tasks.put({
			clientId: 'c1',
			serverId: 1,
			updatedAt: 'x',
			deletedAt: null,
			data: {}
		});
		expect(await adapter.hasData()).toBe(true);
	});

	it('clear wipes entity tables, outbox and meta', async () => {
		const adapter = createDexieAuthAdapter({ db });
		await adapter.saveUser({ id: 1, username: 'eu', totpEnabled: false });
		await db.tasks.put({
			clientId: 'c1',
			serverId: 1,
			updatedAt: 'x',
			deletedAt: null,
			data: {}
		});
		await db.outbox.put({
			id: 'o1',
			entity: 'tasks',
			op: 'create',
			clientId: 'c1',
			payload: {},
			idempotencyKey: 'k',
			status: 'pending',
			attempts: 0,
			nextAttemptAt: 0,
			lastError: null,
			createdAt: 0,
			parentClientId: null
		});

		await adapter.clear();

		expect(await db.tasks.count()).toBe(0);
		expect(await db.outbox.count()).toBe(0);
		expect(await db.meta.count()).toBe(0);
		expect(await adapter.loadUser()).toBeNull();
	});

	it('onAuthenticatedRefresh delegates to the provided callback', async () => {
		const cb = vi.fn().mockResolvedValue(undefined);
		const adapter = createDexieAuthAdapter({ db, onAuthenticatedRefresh: cb });
		await adapter.onAuthenticatedRefresh();
		expect(cb).toHaveBeenCalledTimes(1);
	});
});

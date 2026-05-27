import 'fake-indexeddb/auto';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { TurboistDB } from './db';
import { createDexieAuthAdapter } from './auth';

const lsStore = new Map<string, string>();
Object.defineProperty(globalThis, 'localStorage', {
	value: {
		getItem: (key: string) => lsStore.get(key) ?? null,
		setItem: (key: string, value: string) => lsStore.set(key, value),
		removeItem: (key: string) => lsStore.delete(key),
		clear: () => lsStore.clear()
	},
	writable: true,
	configurable: true
});

describe('createDexieAuthAdapter', () => {
	let db: TurboistDB;

	beforeEach(async () => {
		db = new TurboistDB(`auth-test-${Math.random().toString(36).slice(2)}`);
		await db.open();
		lsStore.clear();
	});

	afterEach(() => {
		lsStore.clear();
	});

	it('saveUser then loadUser round-trips only whitelisted fields', async () => {
		const adapter = createDexieAuthAdapter({ db });
		await adapter.saveUser({
			id: 42,
			username: 'eu',
			totpEnabled: true,
			// extra fields that should not be persisted
			email: 'eu@example.com'
		} as unknown as Parameters<typeof adapter.saveUser>[0]);
		const stored = await db.meta.get('user');
		expect(stored?.value).toEqual({ id: 42, username: 'eu' });

		const loaded = await adapter.loadUser();
		expect(loaded?.id).toBe(42);
		expect(loaded?.user.username).toBe('eu');
		expect(loaded?.user.totpEnabled).toBe(false);
		expect((loaded?.user as unknown as { email?: string }).email).toBeUndefined();
	});

	it('loadUser returns null when no user has been stored', async () => {
		const adapter = createDexieAuthAdapter({ db });
		expect(await adapter.loadUser()).toBeNull();
	});

	it('loadUser falls back to localStorage when IndexedDB is empty', async () => {
		const adapter = createDexieAuthAdapter({ db });
		await adapter.saveUser({ id: 7, username: 'alice', totpEnabled: false });

		await db.meta.clear();

		const loaded = await adapter.loadUser();
		expect(loaded?.id).toBe(7);
		expect(loaded?.user.username).toBe('alice');
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

	it('hasData returns true via localStorage when IndexedDB was evicted', async () => {
		const adapter = createDexieAuthAdapter({ db });
		await db.projects.put({
			clientId: 'p1',
			serverId: 2,
			updatedAt: 'x',
			deletedAt: null,
			data: {}
		});
		expect(await adapter.hasData()).toBe(true);

		for (const t of ['tasks', 'projects', 'sections', 'labels', 'contexts'] as const) {
			await db.table(t).clear();
		}

		expect(await adapter.hasData()).toBe(true);
	});

	it('clear wipes entity tables, outbox, meta, and localStorage', async () => {
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
		expect(await adapter.hasData()).toBe(true);

		await adapter.clear();

		expect(await db.tasks.count()).toBe(0);
		expect(await db.outbox.count()).toBe(0);
		expect(await db.meta.count()).toBe(0);
		expect(await adapter.loadUser()).toBeNull();
		expect(await adapter.hasData()).toBe(false);
	});

	it('onAuthenticatedRefresh delegates to the provided callback', async () => {
		const cb = vi.fn().mockResolvedValue(undefined);
		const adapter = createDexieAuthAdapter({ db, onAuthenticatedRefresh: cb });
		await adapter.onAuthenticatedRefresh();
		expect(cb).toHaveBeenCalledTimes(1);
	});
});

import 'fake-indexeddb/auto';
import { describe, it, expect, beforeEach } from 'vitest';
import { TurboistDB, ENTITY_TABLES, type StoredEntity } from './db';

describe('TurboistDB schema', () => {
	let db: TurboistDB;

	beforeEach(async () => {
		db = new TurboistDB(`test-${Math.random().toString(36).slice(2)}`);
		await db.open();
	});

	it('exposes all expected entity tables', () => {
		for (const t of ENTITY_TABLES) {
			expect(db.table(t)).toBeDefined();
		}
		expect(db.table('outbox')).toBeDefined();
		expect(db.table('meta')).toBeDefined();
	});

	it('stores and retrieves an entity by clientId', async () => {
		const entity: StoredEntity = {
			clientId: 'c1',
			serverId: null,
			updatedAt: '2026-01-01T00:00:00Z',
			deletedAt: null,
			data: { title: 'hello' }
		};
		await db.tasks.put(entity);
		const got = await db.tasks.get('c1');
		expect(got?.data.title).toBe('hello');
	});

	it('queries tasks by serverId index', async () => {
		await db.tasks.put({
			clientId: 'c1',
			serverId: 42,
			updatedAt: '2026-01-01T00:00:00Z',
			deletedAt: null,
			data: {}
		});
		const found = await db.tasks.where('serverId').equals(42).first();
		expect(found?.clientId).toBe('c1');
	});

	it('meta table keyed by `key`', async () => {
		await db.meta.put({ key: 'lastPulledAt', value: '2026-01-01T00:00:00Z' });
		const got = await db.meta.get('lastPulledAt');
		expect(got?.value).toBe('2026-01-01T00:00:00Z');
	});
});

import 'fake-indexeddb/auto';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { TurboistDB } from './db';
import { enqueue } from './outbox';
import { syncRunner } from './runner';
import type { SyncFetch, FetchResponse } from './sync';

const tick = (): Promise<void> => new Promise((r) => setTimeout(r, 0));

const makeFetcher = (
	response: FetchResponse
): { fetcher: SyncFetch; calls: Array<{ path: string }> } => {
	const calls: Array<{ path: string }> = [];
	const fetcher: SyncFetch = async (path) => {
		calls.push({ path });
		return response;
	};
	return { fetcher, calls };
};

const enqueueOne = async (db: TurboistDB): Promise<void> => {
	await db.tasks.put({
		clientId: `c-${Math.random().toString(36).slice(2)}`,
		serverId: null,
		updatedAt: '2026-05-25T10:00:00.000Z',
		deletedAt: null,
		data: {}
	});
	await enqueue(
		{
			entity: 'tasks',
			op: 'create',
			clientId: `c-${Math.random().toString(36).slice(2)}`,
			payload: {
				method: 'POST',
				path: '/api/v1/contexts/1/tasks',
				body: { title: 't' }
			}
		},
		db
	);
};

describe('syncRunner', () => {
	let db: TurboistDB;

	beforeEach(async () => {
		db = new TurboistDB(`test-${Math.random().toString(36).slice(2)}`);
		await db.open();
	});

	afterEach(() => {
		syncRunner.stop();
	});

	it('start triggers initial flush', async () => {
		await enqueueOne(db);
		const { fetcher, calls } = makeFetcher({
			status: 201,
			data: { id: 1, clientId: 'c1', updatedAt: '2026-05-25T10:00:01.000Z' }
		});
		syncRunner.start(fetcher, db);
		await vi.waitFor(() => expect(calls.length).toBeGreaterThan(0));
	});

	it('online event triggers flush', async () => {
		const { fetcher, calls } = makeFetcher({
			status: 201,
			data: { id: 1, clientId: 'c1', updatedAt: '2026-05-25T10:00:01.000Z' }
		});
		syncRunner.start(fetcher, db);
		await tick();
		await tick();

		await enqueueOne(db);
		window.dispatchEvent(new Event('online'));
		await vi.waitFor(async () => expect(await db.outbox.count()).toBe(0));
		expect(calls.length).toBeGreaterThan(0);
	});

	it('stop unsubscribes listeners', async () => {
		const { fetcher, calls } = makeFetcher({
			status: 201,
			data: { id: 1, clientId: 'c1', updatedAt: '2026-05-25T10:00:01.000Z' }
		});
		syncRunner.start(fetcher, db);
		await tick();
		syncRunner.stop();

		await enqueueOne(db);
		await tick();
		window.dispatchEvent(new Event('online'));
		await tick();
		expect(calls).toHaveLength(0);
	});

	it('skips flush when navigator.onLine is false', async () => {
		const onLineSpy = vi.spyOn(navigator, 'onLine', 'get').mockReturnValue(false);
		await enqueueOne(db);
		const { fetcher, calls } = makeFetcher({
			status: 201,
			data: { id: 1, clientId: 'c1', updatedAt: 'x' }
		});
		syncRunner.start(fetcher, db);
		await tick();
		await tick();
		expect(calls).toHaveLength(0);
		onLineSpy.mockRestore();
	});
});

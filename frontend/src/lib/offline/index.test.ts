import 'fake-indexeddb/auto';
import { IDBFactory } from 'fake-indexeddb';
import { beforeEach, describe, expect, it } from 'vitest';
import { createOfflineBridge } from './index';
import { statusStore } from './status.svelte';

beforeEach(() => {
	// Fresh in-memory IndexedDB per test so cached entries never leak.
	globalThis.indexedDB = new IDBFactory();
	// The bridge drives the shared status singleton; reset it to a known state.
	statusStore.clearStale();
	statusStore.noteOutcome(true);
});

function bridge() {
	return createOfflineBridge({ serverUrl: '' });
}

describe('createOfflineBridge', () => {
	it('drives the status heuristic from noteRequestOutcome', () => {
		const b = bridge();

		b.noteRequestOutcome(false);
		expect(statusStore.online).toBe(false);
		expect(b.isOffline()).toBe(true);

		b.noteRequestOutcome(true);
		expect(statusStore.online).toBe(true);
		expect(b.isOffline()).toBe(false);
	});

	it('round-trips a payload through cachePut / cacheGet', async () => {
		const b = bridge();
		await b.cachePut('/api/v1/tasks', { view: 'today' }, [{ id: 1 }]);

		const hit = await b.cacheGet('/api/v1/tasks', { view: 'today' });
		expect(hit?.payload).toEqual([{ id: 1 }]);
		expect(typeof hit?.storedAt).toBe('string');
	});

	it('marks the page stale on a cache hit and clears it on a fresh write-through', async () => {
		const b = bridge();
		await b.cachePut('/api/v1/tasks', undefined, [{ id: 1 }]);
		statusStore.clearStale();

		await b.cacheGet('/api/v1/tasks', undefined);
		expect(statusStore.servedStale).toBe(true);

		await b.cachePut('/api/v1/tasks', undefined, [{ id: 2 }]);
		expect(statusStore.servedStale).toBe(false);
	});

	it('returns null and leaves servedStale untouched on a cache miss', async () => {
		const b = bridge();
		statusStore.clearStale();

		const hit = await b.cacheGet('/api/v1/does-not-exist', undefined);
		expect(hit).toBeNull();
		expect(statusStore.servedStale).toBe(false);
	});

	it('reports mutations as unsupported offline (null) until the outbox lands', async () => {
		const b = bridge();
		expect(await b.tryEnqueue('/api/v1/tasks', 'POST', {})).toBeNull();
	});
});

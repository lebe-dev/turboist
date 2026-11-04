import { describe, expect, it, beforeEach, afterEach, vi } from 'vitest';
import { recentItemsStore, RECENT_ITEMS_MEMORY } from './recentItems.svelte';

function createStorageMock(): Storage {
	const store = new Map<string, string>();
	return {
		getItem: (k) => store.get(k) ?? null,
		setItem: (k, v) => void store.set(k, String(v)),
		removeItem: (k) => void store.delete(k),
		clear: () => store.clear(),
		key: (i) => [...store.keys()][i] ?? null,
		get length() {
			return store.size;
		}
	} as Storage;
}

describe('recentItemsStore', () => {
	beforeEach(() => {
		vi.stubGlobal('localStorage', createStorageMock());
		recentItemsStore.clear();
	});
	afterEach(() => {
		vi.unstubAllGlobals();
	});

	it('keeps tasks and projects in one most-recent-first list', () => {
		recentItemsStore.visit({ kind: 'task', id: 1, title: 'a', labels: [] });
		recentItemsStore.visit({ kind: 'project', id: 1, title: 'p', labels: [] });
		recentItemsStore.visit({ kind: 'task', id: 2, title: 'b', labels: [] });
		expect(recentItemsStore.entries.map((e) => `${e.kind}:${e.id}`)).toEqual([
			'task:2',
			'project:1',
			'task:1'
		]);
	});

	it('moves a revisited entry to the front and refreshes its snapshot', () => {
		recentItemsStore.visit({ kind: 'task', id: 1, title: 'old', labels: [] });
		recentItemsStore.visit({ kind: 'task', id: 2, title: 'b', labels: [] });
		recentItemsStore.visit({ kind: 'task', id: 1, title: 'new', labels: ['x'] });
		expect(recentItemsStore.entries).toHaveLength(2);
		expect(recentItemsStore.entries[0]).toEqual({ kind: 'task', id: 1, title: 'new', labels: ['x'] });
	});

	it('caps the memory', () => {
		for (let i = 1; i <= RECENT_ITEMS_MEMORY + 5; i++) {
			recentItemsStore.visit({ kind: 'task', id: i, title: String(i), labels: [] });
		}
		expect(recentItemsStore.entries).toHaveLength(RECENT_ITEMS_MEMORY);
	});

	it('forgets an entry and persists', () => {
		recentItemsStore.visit({ kind: 'task', id: 1, title: 'a', labels: [] });
		recentItemsStore.visit({ kind: 'project', id: 1, title: 'p', labels: [] });
		recentItemsStore.forget('task', 1);
		expect(recentItemsStore.entries.map((e) => e.kind)).toEqual(['project']);
		expect(JSON.parse(localStorage.getItem('turboist:recentItems')!)).toHaveLength(1);
	});
});

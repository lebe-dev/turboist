import { describe, expect, it, beforeEach, afterEach, vi } from 'vitest';
import {
	recentProjectsStore,
	RECENT_PROJECTS_MEMORY,
	RECENT_PROJECTS_SHOWN
} from './recentProjects.svelte';

const candidates = Array.from({ length: 10 }, (_, i) => ({ id: i + 1 }));

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

describe('recentProjectsStore', () => {
	beforeEach(() => {
		vi.stubGlobal('localStorage', createStorageMock());
		recentProjectsStore.clear();
	});
	afterEach(() => {
		vi.unstubAllGlobals();
	});

	it('orders visits most-recent-first', () => {
		recentProjectsStore.visit(1);
		recentProjectsStore.visit(2);
		recentProjectsStore.visit(3);
		expect(recentProjectsStore.ids).toEqual([3, 2, 1]);
	});

	it('moves a repeat visit to the front without duplicating it', () => {
		recentProjectsStore.visit(1);
		recentProjectsStore.visit(2);
		recentProjectsStore.visit(1);
		expect(recentProjectsStore.ids).toEqual([1, 2]);
	});

	it('caps the remembered history', () => {
		for (let i = 1; i <= RECENT_PROJECTS_MEMORY + 5; i++) recentProjectsStore.visit(i);
		expect(recentProjectsStore.ids).toHaveLength(RECENT_PROJECTS_MEMORY);
		expect(recentProjectsStore.ids[0]).toBe(RECENT_PROJECTS_MEMORY + 5);
	});

	it('picks the recent slice in visit order', () => {
		recentProjectsStore.visit(7);
		recentProjectsStore.visit(4);
		recentProjectsStore.visit(9);
		expect(recentProjectsStore.pick(candidates)).toEqual([{ id: 9 }, { id: 4 }, { id: 7 }]);
	});

	it('skips ids missing from the candidate list', () => {
		recentProjectsStore.visit(999);
		recentProjectsStore.visit(4);
		expect(recentProjectsStore.pick(candidates)).toEqual([{ id: 4 }]);
	});

	it('honours the limit', () => {
		for (const id of [1, 2, 3, 4, 5]) recentProjectsStore.visit(id);
		expect(recentProjectsStore.pick(candidates)).toHaveLength(RECENT_PROJECTS_SHOWN);
		expect(recentProjectsStore.pick(candidates, 2)).toEqual([{ id: 5 }, { id: 4 }]);
	});

	it('picks nothing when the candidate list already fits in the recent row', () => {
		recentProjectsStore.visit(1);
		recentProjectsStore.visit(2);
		expect(recentProjectsStore.pick(candidates.slice(0, RECENT_PROJECTS_SHOWN))).toEqual([]);
	});

	it('persists to localStorage', () => {
		recentProjectsStore.visit(5);
		recentProjectsStore.visit(6);
		expect(JSON.parse(localStorage.getItem('turboist:recentProjects') ?? '[]')).toEqual([6, 5]);
	});
});

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { persistCollapsedIds, readCollapsedIds } from './subtaskCollapse';

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

describe('subtaskCollapse persistence', () => {
	beforeEach(() => {
		vi.stubGlobal('localStorage', createStorageMock());
	});
	afterEach(() => {
		vi.unstubAllGlobals();
	});

	it('round-trips persisted ids', () => {
		persistCollapsedIds(5, new Set([3, 1, 2]));
		expect(readCollapsedIds(5).sort((a, b) => a - b)).toEqual([1, 2, 3]);
	});

	it('returns empty array for an unknown project', () => {
		expect(readCollapsedIds(999)).toEqual([]);
	});

	it('isolates state between projects', () => {
		persistCollapsedIds(1, [10, 20]);
		persistCollapsedIds(2, [30]);
		expect(readCollapsedIds(1).sort((a, b) => a - b)).toEqual([10, 20]);
		expect(readCollapsedIds(2)).toEqual([30]);
	});

	it('returns empty array for malformed JSON', () => {
		localStorage.setItem('turboist:subtaskCollapse:7', '{not json');
		expect(readCollapsedIds(7)).toEqual([]);
	});

	it('returns empty array when stored value is not an array', () => {
		localStorage.setItem('turboist:subtaskCollapse:8', '{"a":1}');
		expect(readCollapsedIds(8)).toEqual([]);
	});

	it('filters out non-number values', () => {
		localStorage.setItem('turboist:subtaskCollapse:9', JSON.stringify([1, 'x', 2, null, 3]));
		expect(readCollapsedIds(9)).toEqual([1, 2, 3]);
	});
});

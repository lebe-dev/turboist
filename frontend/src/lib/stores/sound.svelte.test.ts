import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const playStatusTone = vi.fn();
vi.mock('../utils/sound', () => ({ playStatusTone: (completed: boolean) => playStatusTone(completed) }));

import { soundStore } from './sound.svelte';

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

describe('soundStore', () => {
	beforeEach(() => {
		playStatusTone.mockReset();
		vi.stubGlobal('localStorage', createStorageMock());
		soundStore.setEnabled(true);
	});
	afterEach(() => {
		vi.unstubAllGlobals();
	});

	it('is enabled by default', () => {
		expect(soundStore.enabled).toBe(true);
	});

	it('plays the ascending tone on completion and the descending one on reopen', () => {
		soundStore.playTaskStatus(true);
		soundStore.playTaskStatus(false);
		expect(playStatusTone.mock.calls).toEqual([[true], [false]]);
	});

	it('stays silent while muted', () => {
		soundStore.setEnabled(false);
		soundStore.playTaskStatus(true);
		expect(playStatusTone).not.toHaveBeenCalled();
	});

	it('persists the preference per device', () => {
		soundStore.setEnabled(false);
		expect(localStorage.getItem('turboist:sound-enabled')).toBe('0');
		soundStore.setEnabled(true);
		expect(localStorage.getItem('turboist:sound-enabled')).toBe('1');
	});
});

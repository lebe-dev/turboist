import { describe, it, expect, beforeEach } from 'vitest';
import { labelFilterStore } from './label-filter.svelte';

beforeEach(() => {
	labelFilterStore.clear();
});

describe('labelFilterStore', () => {
	it('starts with null activeLabel', () => {
		expect(labelFilterStore.activeLabel).toBe(null);
	});

	it('set updates activeLabel', () => {
		labelFilterStore.set('work');
		expect(labelFilterStore.activeLabel).toBe('work');
	});

	it('clear resets activeLabel to null', () => {
		labelFilterStore.set('work');
		labelFilterStore.clear();
		expect(labelFilterStore.activeLabel).toBe(null);
	});

	it('clear is a no-op when already null', () => {
		labelFilterStore.clear();
		expect(labelFilterStore.activeLabel).toBe(null);
	});
});

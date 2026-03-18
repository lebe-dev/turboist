import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('$lib/stores/logger', () => ({
	logger: { log: vi.fn(), warn: vi.fn(), error: vi.fn() }
}));

vi.mock('$lib/api/client', () => ({
	patchState: vi.fn(() => Promise.resolve())
}));

import { collapsedStore } from './collapsed.svelte';

beforeEach(() => {
	collapsedStore.init([]);
});

describe('collapsedStore', () => {
	it('starts empty after init', () => {
		expect(collapsedStore.hasAny).toBe(false);
		expect(collapsedStore.isCollapsed('x')).toBe(false);
	});

	it('init hydrates from saved IDs', () => {
		collapsedStore.init(['a', 'b']);

		expect(collapsedStore.hasAny).toBe(true);
		expect(collapsedStore.isCollapsed('a')).toBe(true);
		expect(collapsedStore.isCollapsed('b')).toBe(true);
		expect(collapsedStore.isCollapsed('c')).toBe(false);
	});

	it('toggle adds an ID', () => {
		collapsedStore.toggle('x');

		expect(collapsedStore.isCollapsed('x')).toBe(true);
		expect(collapsedStore.hasAny).toBe(true);
	});

	it('toggle removes an existing ID', () => {
		collapsedStore.init(['x']);
		collapsedStore.toggle('x');

		expect(collapsedStore.isCollapsed('x')).toBe(false);
	});

	it('collapseAll replaces all IDs', () => {
		collapsedStore.init(['old']);
		collapsedStore.collapseAll(['a', 'b', 'c']);

		expect(collapsedStore.isCollapsed('old')).toBe(false);
		expect(collapsedStore.isCollapsed('a')).toBe(true);
		expect(collapsedStore.isCollapsed('b')).toBe(true);
		expect(collapsedStore.isCollapsed('c')).toBe(true);
	});

	it('expandAll clears everything', () => {
		collapsedStore.init(['a', 'b']);
		collapsedStore.expandAll();

		expect(collapsedStore.hasAny).toBe(false);
		expect(collapsedStore.isCollapsed('a')).toBe(false);
	});

	it('toggle persists state', async () => {
		const { patchState } = await import('$lib/api/client');
		collapsedStore.toggle('x');

		expect(patchState).toHaveBeenCalledWith({ collapsed_ids: ['x'] });
	});
});

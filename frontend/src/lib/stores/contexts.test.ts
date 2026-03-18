import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('$lib/stores/logger', () => ({
	logger: { log: vi.fn(), warn: vi.fn(), error: vi.fn() }
}));

vi.mock('$lib/api/client', () => ({
	patchState: vi.fn(() => Promise.resolve())
}));

vi.mock('./label-filter.svelte', () => ({
	labelFilterStore: { clear: vi.fn(), set: vi.fn() }
}));

import { contextsStore } from './contexts.svelte';
import type { Context } from '$lib/api/types';

const contexts: Context[] = [
	{
		id: 'work',
		display_name: 'Work',
		inherit_labels: false,
		filters: { projects: [], sections: [], labels: [] }
	},
	{
		id: 'personal',
		display_name: 'Personal',
		inherit_labels: false,
		filters: { projects: [], sections: [], labels: [] }
	}
];

beforeEach(() => {
	vi.clearAllMocks();
	contextsStore.init(contexts, '', 'all');
});

describe('contextsStore', () => {
	it('init sets contexts and defaults', () => {
		expect(contextsStore.contexts).toEqual(contexts);
		expect(contextsStore.activeContextId).toBeNull();
		expect(contextsStore.activeView).toBe('all');
	});

	it('init restores saved context', () => {
		contextsStore.init(contexts, 'work', 'today');

		expect(contextsStore.activeContextId).toBe('work');
		expect(contextsStore.activeView).toBe('today');
	});

	it('init resets invalid context to null', () => {
		contextsStore.init(contexts, 'nonexistent', 'all');

		expect(contextsStore.activeContextId).toBeNull();
	});

	it('setContext updates active context', () => {
		contextsStore.setContext('personal');

		expect(contextsStore.activeContextId).toBe('personal');
	});

	it('setContext persists state', async () => {
		const { patchState } = await import('$lib/api/client');
		contextsStore.setContext('work');

		expect(patchState).toHaveBeenCalledWith({ active_context_id: 'work' });
	});

	it('setContext clears label filter', async () => {
		const { labelFilterStore } = await import('./label-filter.svelte');
		contextsStore.setContext('work');

		expect(labelFilterStore.clear).toHaveBeenCalled();
	});

	it('setContext(null) sends empty string', async () => {
		const { patchState } = await import('$lib/api/client');
		contextsStore.setContext(null);

		expect(patchState).toHaveBeenCalledWith({ active_context_id: '' });
	});

	it('setView updates active view', () => {
		contextsStore.setView('weekly');

		expect(contextsStore.activeView).toBe('weekly');
	});

	it('setView persists state', async () => {
		const { patchState } = await import('$lib/api/client');
		contextsStore.setView('backlog');

		expect(patchState).toHaveBeenCalledWith({ active_view: 'backlog' });
	});

	it('setView clears label filter', async () => {
		const { labelFilterStore } = await import('./label-filter.svelte');
		contextsStore.setView('today');

		expect(labelFilterStore.clear).toHaveBeenCalled();
	});
});

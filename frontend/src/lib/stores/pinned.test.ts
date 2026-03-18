import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('$lib/stores/logger', () => ({
	logger: { log: vi.fn(), warn: vi.fn(), error: vi.fn() }
}));

vi.mock('$lib/api/client', () => ({
	patchState: vi.fn(() => Promise.resolve())
}));

import { pinnedStore } from './pinned.svelte';

beforeEach(() => {
	pinnedStore.init([], 3);
});

describe('pinnedStore', () => {
	it('starts empty after init', () => {
		expect(pinnedStore.items).toEqual([]);
		expect(pinnedStore.isFull).toBe(false);
	});

	it('init hydrates with tasks and max', () => {
		const tasks = [
			{ id: '1', content: 'A' },
			{ id: '2', content: 'B' }
		];
		pinnedStore.init(tasks, 5);

		expect(pinnedStore.items).toEqual(tasks);
		expect(pinnedStore.maxPinned).toBe(5);
	});

	it('pin adds a task', () => {
		pinnedStore.pin({ id: '1', content: 'Task 1' });

		expect(pinnedStore.items).toHaveLength(1);
		expect(pinnedStore.isPinned('1')).toBe(true);
	});

	it('pin rejects duplicate', () => {
		pinnedStore.pin({ id: '1', content: 'Task 1' });
		pinnedStore.pin({ id: '1', content: 'Task 1' });

		expect(pinnedStore.items).toHaveLength(1);
	});

	it('pin rejects when full', () => {
		pinnedStore.init([], 2);
		pinnedStore.pin({ id: '1', content: 'A' });
		pinnedStore.pin({ id: '2', content: 'B' });
		pinnedStore.pin({ id: '3', content: 'C' });

		expect(pinnedStore.items).toHaveLength(2);
		expect(pinnedStore.isFull).toBe(true);
		expect(pinnedStore.isPinned('3')).toBe(false);
	});

	it('unpin removes a task', () => {
		pinnedStore.pin({ id: '1', content: 'A' });
		pinnedStore.pin({ id: '2', content: 'B' });
		pinnedStore.unpin('1');

		expect(pinnedStore.items).toHaveLength(1);
		expect(pinnedStore.isPinned('1')).toBe(false);
		expect(pinnedStore.isPinned('2')).toBe(true);
	});

	it('isFull reflects current count vs max', () => {
		pinnedStore.init([], 1);
		expect(pinnedStore.isFull).toBe(false);

		pinnedStore.pin({ id: '1', content: 'A' });
		expect(pinnedStore.isFull).toBe(true);

		pinnedStore.unpin('1');
		expect(pinnedStore.isFull).toBe(false);
	});

	it('selectTask and consumeSelection work', () => {
		expect(pinnedStore.selectedTaskId).toBeNull();

		pinnedStore.selectTask('abc');
		expect(pinnedStore.selectedTaskId).toBe('abc');

		const consumed = pinnedStore.consumeSelection();
		expect(consumed).toBe('abc');
		expect(pinnedStore.selectedTaskId).toBeNull();
	});

	it('consumeSelection returns null when nothing selected', () => {
		expect(pinnedStore.consumeSelection()).toBeNull();
	});

	it('pin persists state', async () => {
		const { patchState } = await import('$lib/api/client');
		pinnedStore.pin({ id: '1', content: 'A' });

		expect(patchState).toHaveBeenCalledWith({
			pinned_tasks: [{ id: '1', content: 'A' }]
		});
	});
});

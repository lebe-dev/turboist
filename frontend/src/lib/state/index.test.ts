import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

// Mock y-indexeddb: emit 'synced' immediately so initState() resolves
vi.mock('y-indexeddb', () => {
	return {
		IndexeddbPersistence: class {
			private _listeners: Record<string, Function[]> = {};
			constructor() {
				// Emit 'synced' on next microtask so the `once('synced', ...)` listener is attached
				queueMicrotask(() => {
					const cbs = this._listeners['synced'] ?? [];
					for (const cb of cbs) cb();
				});
			}
			once(event: string, cb: Function) {
				(this._listeners[event] ??= []).push(cb);
			}
			destroy() {}
		}
	};
});

import {
	initState,
	destroyState,
	persistTasks,
	loadPersistedTasks,
	persistMeta,
	loadPersistedMeta,
} from './index.svelte';
import type { FlatTask } from './types';
import type { Meta } from '$lib/api/types';

function makeFlatTask(id: string): FlatTask {
	return {
		id,
		content: `Task ${id}`,
		description: '',
		project_id: 'proj-1',
		section_id: null,
		parent_id: null,
		labels: ['work'],
		priority: 2,
		due_date: '2026-04-01',
		due_recurring: false,
		sub_task_count: 0,
		completed_sub_task_count: 0,
		completed_at: null,
		added_at: '2026-01-01T00:00:00Z',
		is_project_task: false,
		postpone_count: 0,
	};
}

describe('Y.Doc persistence round-trip', () => {
	beforeEach(async () => {
		await initState();
	});

	afterEach(() => {
		destroyState();
	});

	it('persistTasks + loadPersistedTasks round-trips FlatTask[] correctly', () => {
		const tasks = [makeFlatTask('a'), makeFlatTask('b')];
		persistTasks('tasks', tasks);

		const loaded = loadPersistedTasks('tasks');
		expect(loaded).toHaveLength(2);
		expect(loaded[0].id).toBe('a');
		expect(loaded[1].id).toBe('b');
		expect(loaded[0].labels).toEqual(['work']);
		expect(loaded[0].due_date).toBe('2026-04-01');
		expect(loaded[0].priority).toBe(2);
	});

	it('persistMeta + loadPersistedMeta round-trips Meta correctly', () => {
		const meta: Meta = {
			context: 'ctx-1',
			weekly_limit: 5,
			weekly_count: 3,
			backlog_limit: 10,
			backlog_count: 7,
			last_synced_at: '2026-04-01T12:00:00Z',
		};
		persistMeta('meta', meta);

		const loaded = loadPersistedMeta('meta');
		expect(loaded).not.toBeNull();
		expect(loaded!.context).toBe('ctx-1');
		expect(loaded!.weekly_limit).toBe(5);
		expect(loaded!.weekly_count).toBe(3);
		expect(loaded!.backlog_limit).toBe(10);
		expect(loaded!.backlog_count).toBe(7);
		expect(loaded!.last_synced_at).toBe('2026-04-01T12:00:00Z');
	});

	it('persistTasks overwrites previous data', () => {
		persistTasks('tasks', [makeFlatTask('old')]);
		persistTasks('tasks', [makeFlatTask('new')]);

		const loaded = loadPersistedTasks('tasks');
		expect(loaded).toHaveLength(1);
		expect(loaded[0].id).toBe('new');
	});

	it('loadPersistedMeta returns null for uninitialized key', () => {
		const loaded = loadPersistedMeta('nonexistent');
		expect(loaded).toBeNull();
	});
});

describe('Y.Doc persistence without init', () => {
	it('loadPersistedTasks returns [] when doc not initialized', () => {
		destroyState();
		const loaded = loadPersistedTasks('tasks');
		expect(loaded).toEqual([]);
	});

	it('loadPersistedMeta returns null when doc not initialized', () => {
		destroyState();
		const loaded = loadPersistedMeta('meta');
		expect(loaded).toBeNull();
	});

	it('persistTasks is a no-op when doc not initialized', () => {
		destroyState();
		persistTasks('tasks', [makeFlatTask('a')]);
		expect(loadPersistedTasks('tasks')).toEqual([]);
	});
});

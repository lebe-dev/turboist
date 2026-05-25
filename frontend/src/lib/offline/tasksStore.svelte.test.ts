import 'fake-indexeddb/auto';
import { describe, it, expect, beforeEach } from 'vitest';
import { flushSync } from 'svelte';
import { TurboistDB, type StoredEntity } from './db';
import { emitDbChanged } from './stores';
import { createOfflineTasksStore } from './tasksStore.svelte';
import { cacheTasksFromServer } from './cache';
import type { Task } from '$lib/api/types';

const makeTask = (over: Partial<Task> = {}): Task => ({
	id: 1,
	title: 'task',
	description: '',
	inboxId: null,
	contextId: null,
	projectId: null,
	sectionId: null,
	parentId: null,
	priority: 'no-priority',
	status: 'open',
	dueAt: null,
	dueHasTime: false,
	deadlineAt: null,
	deadlineHasTime: false,
	dayPart: 'none',
	planState: 'none',
	isPinned: false,
	pinnedAt: null,
	isPrivate: false,
	completedAt: null,
	recurrenceRule: null,
	postponeCount: 0,
	labels: [],
	url: '',
	createdAt: '2026-01-01T00:00:00.000Z',
	updatedAt: '2026-01-01T00:00:00.000Z',
	...over
});

const putTask = async (db: TurboistDB, task: Task, deletedAt: string | null = null): Promise<void> => {
	const row: StoredEntity = {
		clientId: `srv:${task.id}`,
		serverId: task.id,
		updatedAt: task.updatedAt,
		deletedAt,
		data: task as unknown as Record<string, unknown>
	};
	await db.tasks.put(row);
};

describe('createOfflineTasksStore', () => {
	let db: TurboistDB;

	beforeEach(async () => {
		db = new TurboistDB(`test-${Math.random().toString(36).slice(2)}`);
		await db.open();
	});

	it('hydrates non-deleted tasks from Dexie', async () => {
		await putTask(db, makeTask({ id: 1, title: 'a' }));
		await putTask(db, makeTask({ id: 2, title: 'b' }));
		await putTask(db, makeTask({ id: 3, title: 'gone' }), '2026-02-01T00:00:00.000Z');

		const cleanup = $effect.root(() => {});
		const store = createOfflineTasksStore({ db });
		try {
			await store.hydrate();
			expect(store.loaded).toBe(true);
			const titles = store.items.map((t) => t.title).sort();
			expect(titles).toEqual(['a', 'b']);
		} finally {
			store.dispose();
			cleanup();
		}
	});

	it('applies an optional filter predicate', async () => {
		await putTask(db, makeTask({ id: 1, status: 'open' }));
		await putTask(db, makeTask({ id: 2, status: 'completed' }));

		const cleanup = $effect.root(() => {});
		const store = createOfflineTasksStore({
			db,
			filter: (t) => t.status === 'open'
		});
		try {
			await store.hydrate();
			expect(store.items).toHaveLength(1);
			expect(store.items[0].id).toBe(1);
		} finally {
			store.dispose();
			cleanup();
		}
	});

	it('re-hydrates on turboist:db-changed:tasks events', async () => {
		const cleanup = $effect.root(() => {});
		const store = createOfflineTasksStore({ db });
		try {
			await store.hydrate();
			expect(store.items).toHaveLength(0);

			await putTask(db, makeTask({ id: 42, title: 'added later' }));
			emitDbChanged('tasks');
			await new Promise((r) => setTimeout(r, 0));
			flushSync();

			expect(store.items).toHaveLength(1);
			expect(store.items[0].title).toBe('added later');
		} finally {
			store.dispose();
			cleanup();
		}
	});

	it('dispose stops reacting to events', async () => {
		const cleanup = $effect.root(() => {});
		const store = createOfflineTasksStore({ db });
		await store.hydrate();
		store.dispose();

		await putTask(db, makeTask({ id: 5 }));
		emitDbChanged('tasks');
		await new Promise((r) => setTimeout(r, 0));

		expect(store.items).toHaveLength(0);
		cleanup();
	});
});

describe('cacheTasksFromServer', () => {
	let db: TurboistDB;

	beforeEach(async () => {
		db = new TurboistDB(`test-${Math.random().toString(36).slice(2)}`);
		await db.open();
	});

	it('upserts server tasks and emits db-changed', async () => {
		let fired = 0;
		const handler = () => {
			fired++;
		};
		window.addEventListener('turboist:db-changed:tasks', handler);
		// emitDbChanged uses an internal EventTarget; subscribe via onDbChanged for parity
		const { onDbChanged } = await import('./stores');
		const off = onDbChanged('tasks', () => {
			fired++;
		});

		await cacheTasksFromServer([makeTask({ id: 100, title: 'cached' })], db);
		const row = await db.tasks.where('serverId').equals(100).first();
		expect(row?.data.title).toBe('cached');
		expect(fired).toBeGreaterThan(0);

		off();
		window.removeEventListener('turboist:db-changed:tasks', handler);
	});

	it('preserves existing clientId on subsequent upserts', async () => {
		await db.tasks.put({
			clientId: 'local-abc',
			serverId: 7,
			updatedAt: '2026-01-01T00:00:00.000Z',
			deletedAt: null,
			data: {}
		});
		await cacheTasksFromServer([makeTask({ id: 7, title: 'refreshed' })], db);
		const row = await db.tasks.where('serverId').equals(7).first();
		expect(row?.clientId).toBe('local-abc');
		expect((row?.data as unknown as Task).title).toBe('refreshed');
	});

	it('no-ops on empty input', async () => {
		await cacheTasksFromServer([], db);
		expect(await db.tasks.count()).toBe(0);
	});
});

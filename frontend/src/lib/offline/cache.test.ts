import 'fake-indexeddb/auto';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { TurboistDB } from './db';
import {
	cacheTasksFromServer,
	cacheProjectsFromServer,
	cacheSectionsFromServer,
	cacheLabelsFromServer,
	cacheContextsFromServer
} from './cache';
import { onDbChanged } from './stores';
import type { Task, Project, ProjectSection, Label, Context } from '$lib/api/types';

describe('cache.cacheXFromServer', () => {
	let db: TurboistDB;

	beforeEach(async () => {
		db = new TurboistDB(`test-${Math.random().toString(36).slice(2)}`);
		await db.open();
	});

	it('cacheTasksFromServer: empty array is a no-op and emits nothing', async () => {
		const handler = vi.fn();
		const off = onDbChanged('tasks', handler);
		await cacheTasksFromServer([], db);
		expect(await db.tasks.count()).toBe(0);
		expect(handler).not.toHaveBeenCalled();
		off();
	});

	it('cacheTasksFromServer: inserts new rows with srv:<id> clientId', async () => {
		const tasks = [
			{ id: 1, updatedAt: '2026-05-25T10:00:00.000Z', title: 'a' } as unknown as Task,
			{ id: 2, updatedAt: '2026-05-25T10:00:01.000Z', title: 'b' } as unknown as Task
		];
		await cacheTasksFromServer(tasks, db);
		const a = await db.tasks.get('srv:1');
		const b = await db.tasks.get('srv:2');
		expect(a?.serverId).toBe(1);
		expect((a?.data as { title: string }).title).toBe('a');
		expect(b?.serverId).toBe(2);
	});

	it('cacheTasksFromServer: preserves existing clientId when matched by serverId', async () => {
		await db.tasks.put({
			clientId: 'local-cid',
			serverId: 42,
			updatedAt: '2026-05-25T08:00:00.000Z',
			deletedAt: null,
			data: { title: 'old' }
		});
		await cacheTasksFromServer(
			[{ id: 42, updatedAt: '2026-05-25T09:00:00.000Z', title: 'new' } as unknown as Task],
			db
		);
		const row = await db.tasks.get('local-cid');
		expect(row?.serverId).toBe(42);
		expect((row?.data as { title: string }).title).toBe('new');
		// no srv:42 row should exist (would mean we forgot to preserve clientId)
		expect(await db.tasks.get('srv:42')).toBeUndefined();
	});

	it('cacheTasksFromServer: emits db-changed for the entity once', async () => {
		const handler = vi.fn();
		const off = onDbChanged('tasks', handler);
		await cacheTasksFromServer(
			[{ id: 1, updatedAt: 't', title: 'a' } as unknown as Task],
			db
		);
		expect(handler).toHaveBeenCalledTimes(1);
		off();
	});

	it('cacheProjectsFromServer / cacheSectionsFromServer / cacheLabelsFromServer / cacheContextsFromServer: write to their tables', async () => {
		await cacheProjectsFromServer(
			[{ id: 1, updatedAt: 't', name: 'P' } as unknown as Project],
			db
		);
		await cacheSectionsFromServer(
			[{ id: 2, updatedAt: 't', name: 'S' } as unknown as ProjectSection],
			db
		);
		await cacheLabelsFromServer(
			[{ id: 3, updatedAt: 't', name: 'L' } as unknown as Label],
			db
		);
		await cacheContextsFromServer(
			[{ id: 4, updatedAt: 't', name: 'C' } as unknown as Context],
			db
		);
		expect(await db.projects.get('srv:1')).toBeDefined();
		expect(await db.sections.get('srv:2')).toBeDefined();
		expect(await db.labels.get('srv:3')).toBeDefined();
		expect(await db.contexts.get('srv:4')).toBeDefined();
	});

	it('cacheTasksFromServer: bulk insert with a mix of new and existing rows', async () => {
		await db.tasks.put({
			clientId: 'keep-me',
			serverId: 1,
			updatedAt: '2026-05-25T08:00:00.000Z',
			deletedAt: null,
			data: { title: 'old' }
		});
		await cacheTasksFromServer(
			[
				{ id: 1, updatedAt: '2026-05-25T09:00:00.000Z', title: '1-new' } as unknown as Task,
				{ id: 2, updatedAt: '2026-05-25T09:00:01.000Z', title: '2-new' } as unknown as Task
			],
			db
		);
		const existing = await db.tasks.get('keep-me');
		expect((existing?.data as { title: string }).title).toBe('1-new');
		const fresh = await db.tasks.get('srv:2');
		expect((fresh?.data as { title: string }).title).toBe('2-new');
	});
});

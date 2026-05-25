import 'fake-indexeddb/auto';
import { describe, it, expect, beforeEach } from 'vitest';
import { flushSync } from 'svelte';
import { TurboistDB, type StoredEntity } from './db';
import { emitDbChanged } from './stores';
import { createOfflineEntityStore } from './entityStore.svelte';
import type { Project } from '$lib/api/types';

const putProject = async (
	db: TurboistDB,
	p: Partial<Project> & { id: number },
	deletedAt: string | null = null
): Promise<void> => {
	const row: StoredEntity = {
		clientId: `srv:${p.id}`,
		serverId: p.id,
		updatedAt: p.updatedAt ?? '2026-01-01T00:00:00.000Z',
		deletedAt,
		data: p as unknown as Record<string, unknown>
	};
	await db.projects.put(row);
};

describe('createOfflineEntityStore', () => {
	let db: TurboistDB;

	beforeEach(async () => {
		db = new TurboistDB(`test-${Math.random().toString(36).slice(2)}`);
		await db.open();
	});

	it('hydrates non-deleted entities and re-hydrates on db-changed', async () => {
		await putProject(db, { id: 1, title: 'A' });
		await putProject(db, { id: 2, title: 'B' });
		await putProject(db, { id: 3, title: 'gone' }, '2026-02-01T00:00:00.000Z');

		const cleanup = $effect.root(() => {});
		const store = createOfflineEntityStore<Project>('projects', { db });
		try {
			await store.hydrate();
			expect(store.loaded).toBe(true);
			expect(store.items.map((p) => p.title).sort()).toEqual(['A', 'B']);

			await putProject(db, { id: 4, title: 'C' });
			emitDbChanged('projects');
			await new Promise((r) => setTimeout(r, 0));
			flushSync();
			expect(store.items).toHaveLength(3);
		} finally {
			store.dispose();
			cleanup();
		}
	});

	it('applies optional filter predicate', async () => {
		await putProject(db, { id: 1, title: 'public', isPrivate: false } as Partial<Project> & { id: number });
		await putProject(db, { id: 2, title: 'secret', isPrivate: true } as Partial<Project> & { id: number });

		const cleanup = $effect.root(() => {});
		const store = createOfflineEntityStore<Project>('projects', {
			db,
			filter: (p) => !p.isPrivate
		});
		try {
			await store.hydrate();
			expect(store.items).toHaveLength(1);
			expect(store.items[0].title).toBe('public');
		} finally {
			store.dispose();
			cleanup();
		}
	});
});

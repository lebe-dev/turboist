import * as Y from 'yjs';
import { IndexeddbPersistence } from 'y-indexeddb';
import type { FlatTask } from './types';
import type { Meta, PinnedTask } from '$lib/api/types';

let doc: Y.Doc | null = null;
let persistence: IndexeddbPersistence | null = null;

/** Initialize Y.Doc + y-indexeddb. Resolves when persisted data is loaded from IndexedDB. */
export async function initState(): Promise<void> {
	doc = new Y.Doc();
	persistence = new IndexeddbPersistence('turboist-workspace', doc);
	await new Promise<void>((resolve) => {
		persistence!.once('synced', resolve);
	});
}

export function isStateReady(): boolean {
	return doc !== null;
}

export function destroyState(): void {
	persistence?.destroy();
	doc?.destroy();
	persistence = null;
	doc = null;
}

// --- Task array persistence ---

export function persistTasks(key: string, tasks: FlatTask[]): void {
	if (!doc) return;
	doc.transact(() => {
		const arr = doc!.getArray(key);
		arr.delete(0, arr.length);
		arr.push(tasks);
	});
}

export function loadPersistedTasks(key: string): FlatTask[] {
	if (!doc) return [];
	return doc.getArray(key).toJSON() as FlatTask[];
}

// --- Meta persistence ---

export function persistMeta(key: string, meta: Meta): void {
	if (!doc) return;
	const map = doc.getMap(key);
	doc.transact(() => {
		map.set('context', meta.context);
		map.set('weekly_limit', meta.weekly_limit);
		map.set('weekly_count', meta.weekly_count);
		map.set('backlog_limit', meta.backlog_limit);
		map.set('backlog_count', meta.backlog_count);
		map.set('last_synced_at', meta.last_synced_at ?? null);
	});
}

export function loadPersistedMeta(key: string): Meta | null {
	if (!doc) return null;
	const map = doc.getMap(key);
	if (map.size === 0) return null;
	return {
		context: (map.get('context') as string) ?? '',
		weekly_limit: (map.get('weekly_limit') as number) ?? 0,
		weekly_count: (map.get('weekly_count') as number) ?? 0,
		backlog_limit: (map.get('backlog_limit') as number) ?? 0,
		backlog_count: (map.get('backlog_count') as number) ?? 0,
		last_synced_at: map.get('last_synced_at') as string | undefined
	};
}

// --- UI state persistence ---

export interface PersistedUI {
	active_context_id: string;
	active_view: string;
	sidebar_collapsed: boolean;
	collapsed_ids: string[];
	pinned_tasks: PinnedTask[];
}

export function persistUI(data: Partial<PersistedUI>): void {
	if (!doc) return;
	const map = doc.getMap('ui');
	doc.transact(() => {
		for (const [k, v] of Object.entries(data)) {
			map.set(k, v);
		}
	});
}

export function loadPersistedUI(): PersistedUI | null {
	if (!doc) return null;
	const map = doc.getMap('ui');
	if (map.size === 0) return null;
	return {
		active_context_id: (map.get('active_context_id') as string) ?? '',
		active_view: (map.get('active_view') as string) ?? 'all',
		sidebar_collapsed: (map.get('sidebar_collapsed') as boolean) ?? false,
		collapsed_ids: (map.get('collapsed_ids') as string[]) ?? [],
		pinned_tasks: (map.get('pinned_tasks') as PinnedTask[]) ?? []
	};
}

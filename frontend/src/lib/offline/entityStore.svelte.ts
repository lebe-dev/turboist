import { getDB, type EntityKind, type TurboistDB } from './db';
import { onDbChanged } from './stores';

export interface OfflineEntityStoreOptions<T> {
	filter?: (item: T) => boolean;
	db?: TurboistDB;
}

export interface OfflineEntityStore<T> {
	readonly items: T[];
	readonly loaded: boolean;
	hydrate(): Promise<void>;
	dispose(): void;
}

export function createOfflineEntityStore<T>(
	kind: EntityKind,
	opts: OfflineEntityStoreOptions<T> = {}
): OfflineEntityStore<T> {
	let items = $state<T[]>([]);
	let loaded = $state(false);
	const db = opts.db ?? getDB();

	const hydrate = async (): Promise<void> => {
		const rows = await db.table(kind).toArray();
		const next: T[] = [];
		for (const row of rows) {
			if (row.deletedAt !== null) continue;
			const item = row.data as unknown as T;
			if (opts.filter && !opts.filter(item)) continue;
			next.push(item);
		}
		items = next;
		loaded = true;
	};

	const unsubscribe = onDbChanged(kind, () => {
		void hydrate();
	});

	return {
		get items() {
			return items;
		},
		get loaded() {
			return loaded;
		},
		hydrate,
		dispose: unsubscribe
	};
}

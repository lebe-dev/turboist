import type { User } from '../api/types';
import { getDB, ENTITY_TABLES, type TurboistDB } from './db';

export interface OfflineAuthAdapter {
	saveUser(user: User): Promise<void>;
	loadUser(): Promise<{ id: number; user: User } | null>;
	hasData(): Promise<boolean>;
	clear(): Promise<void>;
	onAuthenticatedRefresh(): Promise<void>;
}

export const NOOP_OFFLINE_AUTH_ADAPTER: OfflineAuthAdapter = {
	async saveUser() {},
	async loadUser() {
		return null;
	},
	async hasData() {
		return false;
	},
	async clear() {},
	async onAuthenticatedRefresh() {}
};

const USER_ID_KEY = 'userId';
const USER_INFO_KEY = 'user';

export interface DexieAuthAdapterOptions {
	db?: TurboistDB;
	onAuthenticatedRefresh?: () => Promise<void> | void;
}

export const createDexieAuthAdapter = (
	opts: DexieAuthAdapterOptions = {}
): OfflineAuthAdapter => {
	const getDb = (): TurboistDB => opts.db ?? getDB();
	return {
		async saveUser(user) {
			const db = getDb();
			await db.meta.put({ key: USER_ID_KEY, value: user.id });
			await db.meta.put({ key: USER_INFO_KEY, value: user });
		},
		async loadUser() {
			const db = getDb();
			const idEntry = await db.meta.get(USER_ID_KEY);
			const userEntry = await db.meta.get(USER_INFO_KEY);
			const id = idEntry?.value;
			const user = userEntry?.value;
			if (typeof id !== 'number' || !user || typeof user !== 'object') return null;
			return { id, user: user as User };
		},
		async hasData() {
			const db = getDb();
			for (const t of ENTITY_TABLES) {
				const count = await db.table(t).count();
				if (count > 0) return true;
			}
			return false;
		},
		async clear() {
			const db = getDb();
			await Promise.all([
				...ENTITY_TABLES.map((t) => db.table(t).clear()),
				db.outbox.clear(),
				db.meta.clear()
			]);
		},
		async onAuthenticatedRefresh() {
			if (opts.onAuthenticatedRefresh) await opts.onAuthenticatedRefresh();
		}
	};
};

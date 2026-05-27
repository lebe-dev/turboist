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
const LS_USER_KEY = 'turboist_offline_user';
const LS_HAS_DATA_KEY = 'turboist_offline_has_data';

interface CachedUserFields {
	id: number;
	username: string;
}

const pickCachedFields = (user: User): CachedUserFields => ({
	id: user.id,
	username: user.username
});

const hydrateFromCache = (cached: CachedUserFields): User => ({
	id: cached.id,
	username: cached.username,
	totpEnabled: false
});

function lsSet(key: string, value: string): void {
	try {
		localStorage.setItem(key, value);
	} catch {
		// Safari private mode or quota — best-effort
	}
}

function lsGet(key: string): string | null {
	try {
		return localStorage.getItem(key);
	} catch {
		return null;
	}
}

function lsRemove(key: string): void {
	try {
		localStorage.removeItem(key);
	} catch {
		// best-effort
	}
}

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
			await db.meta.put({ key: USER_INFO_KEY, value: pickCachedFields(user) });
			lsSet(LS_USER_KEY, JSON.stringify(pickCachedFields(user)));
		},
		async loadUser() {
			const db = getDb();
			const idEntry = await db.meta.get(USER_ID_KEY);
			const userEntry = await db.meta.get(USER_INFO_KEY);
			const id = idEntry?.value;
			const cached = userEntry?.value as Partial<CachedUserFields> | undefined;
			if (typeof id === 'number' && cached && typeof cached.username === 'string') {
				return { id, user: hydrateFromCache({ id, username: cached.username }) };
			}
			// iOS WebKit can evict IndexedDB while localStorage survives
			const raw = lsGet(LS_USER_KEY);
			if (!raw) return null;
			try {
				const parsed = JSON.parse(raw) as Partial<CachedUserFields>;
				if (typeof parsed.id !== 'number' || typeof parsed.username !== 'string') return null;
				return { id: parsed.id, user: hydrateFromCache({ id: parsed.id, username: parsed.username }) };
			} catch {
				return null;
			}
		},
		async hasData() {
			const db = getDb();
			for (const t of ENTITY_TABLES) {
				const count = await db.table(t).count();
				if (count > 0) {
					lsSet(LS_HAS_DATA_KEY, '1');
					return true;
				}
			}
			// IndexedDB was evicted but we know data existed before
			return lsGet(LS_HAS_DATA_KEY) === '1';
		},
		async clear() {
			const db = getDb();
			await Promise.all([
				...ENTITY_TABLES.map((t) => db.table(t).clear()),
				db.outbox.clear(),
				db.meta.clear()
			]);
			lsRemove(LS_USER_KEY);
			lsRemove(LS_HAS_DATA_KEY);
		},
		async onAuthenticatedRefresh() {
			if (opts.onAuthenticatedRefresh) await opts.onAuthenticatedRefresh();
		}
	};
};

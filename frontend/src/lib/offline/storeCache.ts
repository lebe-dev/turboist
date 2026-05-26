import { getDB, type TurboistDB } from './db';

const PREFIX = 'store:';

export async function cacheStoreValue(
	name: string,
	value: unknown,
	db: TurboistDB = getDB()
): Promise<void> {
	await db.meta.put({ key: `${PREFIX}${name}`, value });
}

export async function getCachedStoreValue<T>(
	name: string,
	db: TurboistDB = getDB()
): Promise<T | undefined> {
	const row = await db.meta.get(`${PREFIX}${name}`);
	return row?.value as T | undefined;
}

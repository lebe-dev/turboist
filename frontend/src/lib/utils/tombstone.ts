/**
 * Offline-sync / federation overlay (Federation v1 F0.1).
 *
 * Every synchronized entity carries a `deletedAt` soft-delete tombstone. The
 * backend already filters tombstones out of every list/get response, but a
 * federation-pushed update can in principle deliver a tombstone to a store, so
 * these helpers give the stores a single, defensive place to drop tombstoned
 * rows. `clientId`/`deletedAt` are the wire fields added in F0.1.
 */

export interface Tombstoneable {
	deletedAt: string | null;
}

/** isLive reports whether an entity has no soft-delete tombstone. */
export function isLive(entity: Tombstoneable): boolean {
	return entity.deletedAt == null;
}

/** dropTombstones returns only the live (non-soft-deleted) entities. */
export function dropTombstones<T extends Tombstoneable>(items: T[]): T[] {
	return items.filter(isLive);
}

import { describe, expect, it } from 'vitest';
import { dropTombstones, isLive } from './tombstone';

describe('isLive', () => {
	it('returns true when deletedAt is null', () => {
		expect(isLive({ deletedAt: null })).toBe(true);
	});

	it('returns false when deletedAt is set', () => {
		expect(isLive({ deletedAt: '2026-06-01T00:00:00.000Z' })).toBe(false);
	});
});

describe('dropTombstones', () => {
	it('keeps live entities and drops tombstoned ones', () => {
		const items = [
			{ id: 1, deletedAt: null },
			{ id: 2, deletedAt: '2026-06-01T00:00:00.000Z' },
			{ id: 3, deletedAt: null }
		];
		const live = dropTombstones(items);
		expect(live.map((i) => i.id)).toEqual([1, 3]);
	});

	it('returns an empty array when all entities are tombstoned', () => {
		const items = [{ deletedAt: '2026-06-01T00:00:00.000Z' }];
		expect(dropTombstones(items)).toEqual([]);
	});
});

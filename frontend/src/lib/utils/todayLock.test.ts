import { describe, expect, it } from 'vitest';
import { wouldTodayLockRefuse } from './todayLock';

const base = {
	lockActive: true,
	dueKey: '2026-11-11',
	todayKey: '2026-11-11',
	projectId: 5,
	todayProjectIds: [5, 7]
};

describe('wouldTodayLockRefuse', () => {
	it('lets anything through while no lock runs', () => {
		expect(wouldTodayLockRefuse({ ...base, lockActive: false, projectId: 9 })).toBe(false);
		expect(wouldTodayLockRefuse({ ...base, lockActive: false, projectId: null })).toBe(false);
	});

	it("allows today in one of today's projects", () => {
		expect(wouldTodayLockRefuse(base)).toBe(false);
	});

	it('refuses today in another project', () => {
		expect(wouldTodayLockRefuse({ ...base, projectId: 9 })).toBe(true);
	});

	it('refuses an overdue date the same way as today', () => {
		expect(wouldTodayLockRefuse({ ...base, dueKey: '2026-11-10', projectId: 9 })).toBe(true);
	});

	it('refuses today in the inbox even when the project set is unknown', () => {
		expect(wouldTodayLockRefuse({ ...base, projectId: null })).toBe(true);
		expect(wouldTodayLockRefuse({ ...base, projectId: null, todayProjectIds: null })).toBe(true);
	});

	it('leaves an unknown project set to the server', () => {
		expect(wouldTodayLockRefuse({ ...base, projectId: 9, todayProjectIds: null })).toBe(false);
	});

	it('refuses every project when Today has none', () => {
		expect(wouldTodayLockRefuse({ ...base, projectId: 9, todayProjectIds: [] })).toBe(true);
	});

	it('allows another day and no date at all', () => {
		expect(wouldTodayLockRefuse({ ...base, dueKey: '2026-11-12', projectId: 9 })).toBe(false);
		expect(wouldTodayLockRefuse({ ...base, dueKey: '', projectId: null })).toBe(false);
	});
});

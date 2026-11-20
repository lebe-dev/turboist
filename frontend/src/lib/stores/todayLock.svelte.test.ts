import { beforeEach, describe, expect, it, vi } from 'vitest';

const fetchMock = vi.fn();
vi.mock('../api/client', () => ({ getApiClient: () => ({ fetch: fetchMock }) }));

import { isTodayLockActive, todayLockStore } from './todayLock.svelte';

const NOW = new Date('2026-11-11T14:00:00.000Z');

describe('isTodayLockActive', () => {
	it('is off when the lock was never turned on', () => {
		expect(isTodayLockActive(null, NOW)).toBe(false);
		expect(isTodayLockActive(undefined, NOW)).toBe(false);
		expect(isTodayLockActive('', NOW)).toBe(false);
	});

	it('runs until its end and stops exactly at it', () => {
		expect(isTodayLockActive('2026-11-11T21:00:00.000Z', NOW)).toBe(true);
		expect(isTodayLockActive('2026-11-11T14:00:00.000Z', NOW)).toBe(false);
		expect(isTodayLockActive('2026-11-10T21:00:00.000Z', NOW)).toBe(false);
	});

	it('treats an unreadable end as no lock', () => {
		expect(isTodayLockActive('not a time', NOW)).toBe(false);
	});
});

describe('todayLockStore', () => {
	beforeEach(() => {
		fetchMock.mockReset();
		todayLockStore.clear();
	});

	it('takes the end from the server settings', () => {
		todayLockStore.setUntil('2026-11-11T21:00:00.000Z');
		expect(todayLockStore.until).toBe('2026-11-11T21:00:00.000Z');
		expect(todayLockStore.isActive(NOW)).toBe(true);
		todayLockStore.setUntil(null);
		expect(todayLockStore.isActive(NOW)).toBe(false);
		todayLockStore.setUntil(undefined);
		expect(todayLockStore.until).toBeNull();
	});

	it('locks through the endpoint and keeps the answer', async () => {
		fetchMock.mockResolvedValue({ lockedUntil: '2026-11-11T21:00:00.000Z' });
		await todayLockStore.lock();
		expect(fetchMock).toHaveBeenCalledWith('/api/v1/today/lock', { method: 'POST' });
		expect(todayLockStore.until).toBe('2026-11-11T21:00:00.000Z');
		expect(todayLockStore.isActive(NOW)).toBe(true);
	});

	it('leaves the state alone when locking fails', async () => {
		fetchMock.mockRejectedValue(new Error('offline'));
		await expect(todayLockStore.lock()).rejects.toThrow('offline');
		expect(todayLockStore.until).toBeNull();
	});

	it('reads the live set of projects a running lock lets work into', async () => {
		fetchMock.mockResolvedValue({
			active: true,
			lockedUntil: '2026-11-11T21:00:00.000Z',
			projectIds: [2, 7]
		});
		await todayLockStore.refreshAllowed();
		expect(fetchMock).toHaveBeenCalledWith('/api/v1/today/lock');
		expect(todayLockStore.allowedProjectIds).toEqual([2, 7]);
	});

	it('forgets the set once no lock runs, so a stale one cannot judge a draft', async () => {
		fetchMock.mockResolvedValue({ active: false, lockedUntil: null, projectIds: [] });
		await todayLockStore.refreshAllowed();
		expect(todayLockStore.allowedProjectIds).toBeNull();
	});

	it('leaves the set unknown when the read fails, and the server decides', async () => {
		todayLockStore.forgetAllowed();
		expect(todayLockStore.allowedProjectIds).toBeNull();
		fetchMock.mockRejectedValue(new Error('offline'));
		await todayLockStore.refreshAllowed();
		expect(todayLockStore.allowedProjectIds).toBeNull();
	});

	it('drops the last read on demand', async () => {
		fetchMock.mockResolvedValue({
			active: true,
			lockedUntil: '2026-11-11T21:00:00.000Z',
			projectIds: [2]
		});
		await todayLockStore.refreshAllowed();
		expect(todayLockStore.allowedProjectIds).toEqual([2]);
		todayLockStore.forgetAllowed();
		expect(todayLockStore.allowedProjectIds).toBeNull();
	});
});

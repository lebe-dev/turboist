import { describe, expect, it, vi } from 'vitest';
import { runLogout } from './logoutFlow';

describe('runLogout (§4.9 unsent-changes gate)', () => {
	it('empty outbox → logs out and clears offline data, no confirmation', async () => {
		const confirmDiscard = vi.fn().mockResolvedValue(true);
		const logout = vi.fn().mockResolvedValue(undefined);
		const clearOffline = vi.fn().mockResolvedValue(undefined);

		const done = await runLogout({
			pendingOps: () => 0,
			confirmDiscard,
			logout,
			clearOffline
		});

		expect(done).toBe(true);
		expect(confirmDiscard).not.toHaveBeenCalled();
		expect(logout).toHaveBeenCalledTimes(1);
		expect(clearOffline).toHaveBeenCalledTimes(1);
	});

	it('non-empty outbox + cancel → does NOT log out and does NOT clear the DB', async () => {
		const confirmDiscard = vi.fn().mockResolvedValue(false);
		const logout = vi.fn().mockResolvedValue(undefined);
		const clearOffline = vi.fn().mockResolvedValue(undefined);

		const done = await runLogout({
			pendingOps: () => 3,
			confirmDiscard,
			logout,
			clearOffline
		});

		expect(done).toBe(false);
		expect(confirmDiscard).toHaveBeenCalledWith(3);
		expect(logout).not.toHaveBeenCalled();
		expect(clearOffline).not.toHaveBeenCalled();
	});

	it('non-empty outbox + confirm → logs out then clears the DB, in order', async () => {
		const order: string[] = [];
		const confirmDiscard = vi.fn().mockResolvedValue(true);
		const logout = vi.fn().mockImplementation(async () => {
			order.push('logout');
		});
		const clearOffline = vi.fn().mockImplementation(async () => {
			order.push('clear');
		});

		const done = await runLogout({
			pendingOps: () => 2,
			confirmDiscard,
			logout,
			clearOffline
		});

		expect(done).toBe(true);
		expect(confirmDiscard).toHaveBeenCalledWith(2);
		expect(order).toEqual(['logout', 'clear']);
	});

	it('confirms before wiping quarantined failedOps even when the outbox is empty', async () => {
		const confirmDiscard = vi.fn().mockResolvedValue(false);
		const logout = vi.fn().mockResolvedValue(undefined);
		const clearOffline = vi.fn().mockResolvedValue(undefined);

		const done = await runLogout({
			pendingOps: () => 0,
			failedOps: () => 2,
			confirmDiscard,
			logout,
			clearOffline
		});

		// failedOps are never silently wiped: the user was asked, and cancelled.
		expect(done).toBe(false);
		expect(confirmDiscard).toHaveBeenCalledWith(2);
		expect(logout).not.toHaveBeenCalled();
		expect(clearOffline).not.toHaveBeenCalled();
	});

	it('confirms with the combined pending + failed count', async () => {
		const confirmDiscard = vi.fn().mockResolvedValue(true);
		const logout = vi.fn().mockResolvedValue(undefined);
		const clearOffline = vi.fn().mockResolvedValue(undefined);

		const done = await runLogout({
			pendingOps: () => 3,
			failedOps: () => 2,
			confirmDiscard,
			logout,
			clearOffline
		});

		expect(done).toBe(true);
		expect(confirmDiscard).toHaveBeenCalledWith(5);
	});
});

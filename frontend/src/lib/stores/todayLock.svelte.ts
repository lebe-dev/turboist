// The Today focus lock: until it runs out (the next midnight in the server
// timezone), nothing may be taken into Today from a project that has no open
// task there, nor from the inbox. The server enforces it (409 `today_locked`);
// this store only knows when the lock ends, so the Today screen can show it.
//
// Server-owned: it is never written optimistically, only from the server's
// answer — the lock endpoint's response and `settings.todayLockUntil` in every
// /config read, boot and refresh alike — so a refresh can never clobber it.

import { today as todayApi } from '../api/endpoints/today';
import { getApiClient } from '../api/client';

/** Whether a lock ending at `until` still runs at `now`. */
export function isTodayLockActive(until: string | null | undefined, now: Date): boolean {
	if (!until) return false;
	const end = Date.parse(until);
	return Number.isFinite(end) && end > now.getTime();
}

function createTodayLockStore() {
	let until = $state<string | null>(null);
	let allowed = $state<number[] | null>(null);

	return {
		/** When the lock ends, or null if it was never turned on. */
		get until(): string | null {
			return until;
		},
		isActive(now: Date): boolean {
			return isTodayLockActive(until, now);
		},
		setUntil(next: string | null | undefined): void {
			until = next ?? null;
		},
		/** Projects the server lets work for today into, as last read; null when unknown. */
		get allowedProjectIds(): number[] | null {
			return allowed;
		},
		/** Reads the live set from the server. A failure leaves the set unknown: the server decides. */
		async refreshAllowed(): Promise<void> {
			try {
				const state = await todayApi.lockState(getApiClient());
				allowed = state.active ? state.projectIds : null;
			} catch {
				allowed = null;
			}
		},
		/** Drops the last read so a stale set never judges the next draft. */
		forgetAllowed(): void {
			allowed = null;
		},
		/** Turns the lock on. Locking again while it runs keeps its end. */
		async lock(): Promise<void> {
			const res = await todayApi.lock(getApiClient());
			until = res.lockedUntil;
		},
		clear(): void {
			until = null;
			allowed = null;
		}
	};
}

export const todayLockStore = createTodayLockStore();

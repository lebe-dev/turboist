import { state as stateApi } from '../api/endpoints/state';
import { getApiClient } from '../api/client';
import type { UserState } from '../api/types';

// sameUserState compares two states by content. The backend echoes `userState`
// back as a verbatim JSON blob, so it may carry keys this client does not model;
// comparing every key (not just `activeContextId`) keeps the check honest as the
// shape grows. `undefined` and `null` are the same "no active context".
function sameUserState(a: UserState, b: UserState): boolean {
	const keys = new Set([...Object.keys(a), ...Object.keys(b)]);
	for (const key of keys) {
		const left = (a as Record<string, unknown>)[key] ?? null;
		const right = (b as Record<string, unknown>)[key] ?? null;
		if (left !== right) return false;
	}
	return true;
}

class UserStateStore {
	value = $state<UserState>({});

	// Number of local writes whose PATCH has not come back yet. While non-zero,
	// server state is not applied — see reconcileFromServer.
	private pendingWrites = 0;

	setValue(v: UserState): void {
		this.value = v ?? {};
	}

	// reconcileFromServer applies server truth from a mid-session /api/v1/config
	// refresh, but stands down while a local write is unacknowledged: the
	// optimistic value below is newer than anything the server can report, and
	// clobbering it would visibly revert the user's context switch AND, via the
	// activeContextId effect on today/tomorrow/week, trigger a full page refetch.
	// The in-flight PATCH is the one that wins; the next refresh picks it up.
	//
	// An unchanged payload is not applied at all. Assigning an equal-but-new object
	// still changes the `$state` source's identity, which invalidates every reader
	// — and the readers here are the `activeContextId` effects on
	// today/tomorrow/week/next-week/completed, whose refetch blanks the list behind
	// a spinner. A phone reconnecting on every unlock would otherwise blank the
	// open list on each wake, swallowing the first tap.
	reconcileFromServer(v: UserState): void {
		if (this.pendingWrites > 0) return;
		const next = v ?? {};
		if (sameUserState(this.value, next)) return;
		this.value = next;
	}

	get activeContextId(): number | null {
		return this.value.activeContextId ?? null;
	}

	async setActiveContextId(id: number | null): Promise<void> {
		this.value = { ...this.value, activeContextId: id };
		this.pendingWrites += 1;
		try {
			await stateApi.patch(getApiClient(), { activeContextId: id });
		} finally {
			this.pendingWrites -= 1;
		}
	}

	clear(): void {
		this.value = {};
	}
}

export const userStateStore = new UserStateStore();

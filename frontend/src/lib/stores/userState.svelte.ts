import { state as stateApi } from '../api/endpoints/state';
import { getApiClient } from '../api/client';
import type { UserState } from '../api/types';

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
	reconcileFromServer(v: UserState): void {
		if (this.pendingWrites > 0) return;
		this.value = v ?? {};
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

import { state as stateApi } from '../api/endpoints/state';
import { getApiClient } from '../api/client';
import type { UserState } from '../api/types';

class UserStateStore {
	value = $state<UserState>({});

	async load(): Promise<UserState> {
		const v = await stateApi.get(getApiClient());
		this.value = v ?? {};
		return this.value;
	}

	get activeContextId(): number | null {
		return this.value.activeContextId ?? null;
	}

	async setActiveContextId(id: number | null): Promise<void> {
		this.value = { ...this.value, activeContextId: id };
		await stateApi.patch(getApiClient(), { activeContextId: id });
	}

clear(): void {
		this.value = {};
	}
}

export const userStateStore = new UserStateStore();

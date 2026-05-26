import { state as stateApi } from '../api/endpoints/state';
import { getApiClient } from '../api/client';
import type { UserState } from '../api/types';
import { cacheStoreValue, getCachedStoreValue } from '../offline/storeCache';

const CACHE_KEY = 'userState';

class UserStateStore {
	value = $state<UserState>({});

	async load(): Promise<UserState> {
		const v = await stateApi.get(getApiClient());
		this.value = v ?? {};
		void cacheStoreValue(CACHE_KEY, this.value).catch(() => undefined);
		return this.value;
	}

	async loadCached(): Promise<boolean> {
		const cached = await getCachedStoreValue<UserState>(CACHE_KEY);
		if (!cached) return false;
		this.value = cached;
		return true;
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

import { config as configApi } from '../api/endpoints/config';
import { getApiClient } from '../api/client';
import type { ConfigResponse } from '../api/types';
import { cacheStoreValue, getCachedStoreValue } from '../offline/storeCache';

const CACHE_KEY = 'config';

class ConfigStore {
	value = $state<ConfigResponse | null>(null);

	async load(): Promise<ConfigResponse> {
		const cfg = await configApi.get(getApiClient());
		this.value = cfg;
		void cacheStoreValue(CACHE_KEY, cfg).catch(() => undefined);
		return cfg;
	}

	async loadCached(): Promise<boolean> {
		const cached = await getCachedStoreValue<ConfigResponse>(CACHE_KEY);
		if (!cached) return false;
		this.value = cached;
		return true;
	}

	clear(): void {
		this.value = null;
	}
}

export const configStore = new ConfigStore();

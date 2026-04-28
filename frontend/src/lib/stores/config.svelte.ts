import { config as configApi } from '../api/endpoints/config';
import { getApiClient } from '../api/client';
import type { ConfigResponse } from '../api/types';

class ConfigStore {
	value = $state<ConfigResponse | null>(null);

	async load(): Promise<ConfigResponse> {
		const cfg = await configApi.get(getApiClient());
		this.value = cfg;
		return cfg;
	}

	clear(): void {
		this.value = null;
	}
}

export const configStore = new ConfigStore();

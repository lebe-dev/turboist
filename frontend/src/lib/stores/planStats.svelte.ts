import { views as viewsApi } from '../api/endpoints/views';
import { getApiClient } from '../api/client';
import type { PlanStatsResponse } from '../api/types';
import { cacheStoreValue, getCachedStoreValue } from '../offline/storeCache';

const CACHE_KEY = 'planStats';

class PlanStatsStore {
	value = $state<PlanStatsResponse | null>(null);

	async load(): Promise<PlanStatsResponse> {
		const stats = await viewsApi.planStats(getApiClient());
		this.value = stats;
		void cacheStoreValue(CACHE_KEY, stats).catch(() => undefined);
		return stats;
	}

	async loadCached(): Promise<boolean> {
		const cached = await getCachedStoreValue<PlanStatsResponse>(CACHE_KEY);
		if (!cached) return false;
		this.value = cached;
		return true;
	}

	set(value: PlanStatsResponse): void {
		this.value = value;
	}

	clear(): void {
		this.value = null;
	}
}

export const planStatsStore = new PlanStatsStore();

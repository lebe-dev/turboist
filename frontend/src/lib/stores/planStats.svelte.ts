import { views as viewsApi } from '../api/endpoints/views';
import { getApiClient } from '../api/client';
import type { PlanStatsResponse } from '../api/types';

class PlanStatsStore {
	value = $state<PlanStatsResponse | null>(null);

	async load(): Promise<PlanStatsResponse> {
		const stats = await viewsApi.planStats(getApiClient());
		this.value = stats;
		return stats;
	}

	clear(): void {
		this.value = null;
	}
}

export const planStatsStore = new PlanStatsStore();

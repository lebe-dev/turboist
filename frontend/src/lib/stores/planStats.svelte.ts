import type { PlanStatsResponse } from '../api/types';

// Write-only from the outside: the counters arrive inside an aggregate — the
// /api/v1/config bootstrap or the /api/v1/stats/sidebar bundle — never from a
// GET of their own. The standalone /api/v1/stats/plan endpoint was a strict
// subset of both and has been removed; use refreshSidebarBundle() from
// lib/realtime/refresh.ts to re-pull just the counters.
class PlanStatsStore {
	value = $state<PlanStatsResponse | null>(null);

	setValue(v: PlanStatsResponse): void {
		this.value = v;
	}

	clear(): void {
		this.value = null;
	}
}

export const planStatsStore = new PlanStatsStore();

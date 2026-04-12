import type { ConstraintsConfig, DailyConstraintsResponse } from '$lib/api/types';

function createConstraintsStore() {
	let enabled = $state(false);
	let labelBlocks = $state<Map<string, number>>(new Map());
	let dailyConstraints = $state<DailyConstraintsResponse>({
		needs_selection: false,
		items: [],
		rerolls_used: 0,
		max_rerolls: 0,
		pool_size: 0,
		confirmed: false
	});
	let postponeBudget = $state<{ limit: number; used: number }>({ limit: 0, used: 0 });
	let dayPartCaps = $state<Map<string, number>>(new Map());
	let priorityFloor = $state(4);
	let constraintPool = $state<string[]>([]);

	function init(config: ConstraintsConfig): void {
		enabled = config.enabled;
		priorityFloor = config.priority_floor;
		postponeBudget = { limit: config.postpone_budget, used: config.postpone_budget_used };

		const blocks = new Map<string, number>();
		for (const lb of config.label_blocks) {
			blocks.set(lb.label, lb.remaining_seconds);
		}
		labelBlocks = blocks;

		const caps = new Map<string, number>();
		for (const dc of config.day_part_caps) {
			caps.set(dc.label, dc.max_tasks);
		}
		dayPartCaps = caps;
	}

	function updateDailyConstraints(response: DailyConstraintsResponse): void {
		dailyConstraints = response;
	}

	function isLabelBlocked(labels: string[]): boolean {
		if (!enabled) return false;
		for (const label of labels) {
			if (labelBlocks.has(label)) return true;
		}
		return false;
	}

	function getBlockedLabelSeconds(labels: string[]): number | null {
		if (!enabled) return null;
		for (const label of labels) {
			const remaining = labelBlocks.get(label);
			if (remaining !== undefined) return remaining;
		}
		return null;
	}

	function isPostponeExhausted(): boolean {
		if (!enabled) return false;
		if (postponeBudget.limit === 0) return false;
		return postponeBudget.used >= postponeBudget.limit;
	}

	function isPriorityBelowFloor(priority: number): boolean {
		if (!enabled) return false;
		return priority < priorityFloor;
	}

	function getDayPartCap(label: string): number | null {
		if (!enabled) return null;
		return dayPartCaps.get(label) ?? null;
	}

	function incrementPostponeUsed(): void {
		postponeBudget = { limit: postponeBudget.limit, used: postponeBudget.used + 1 };
	}

	return {
		get enabled() {
			return enabled;
		},
		get labelBlocks() {
			return labelBlocks;
		},
		get dailyConstraints() {
			return dailyConstraints;
		},
		get postponeBudget() {
			return postponeBudget;
		},
		get dayPartCaps() {
			return dayPartCaps;
		},
		get priorityFloor() {
			return priorityFloor;
		},
		get constraintPool() {
			return constraintPool;
		},
		set constraintPool(value: string[]) {
			constraintPool = value;
		},
		init,
		updateDailyConstraints,
		isLabelBlocked,
		getBlockedLabelSeconds,
		isPostponeExhausted,
		isPriorityBelowFloor,
		getDayPartCap,
		incrementPostponeUsed
	};
}

export const constraintsStore = createConstraintsStore();

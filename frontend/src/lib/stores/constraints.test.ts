import { describe, it, expect, beforeEach } from 'vitest';
import { constraintsStore } from './constraints.svelte';
import type { ConstraintsConfig } from '$lib/api/types';

function makeConfig(overrides: Partial<ConstraintsConfig> = {}): ConstraintsConfig {
	return {
		enabled: true,
		label_blocks: [],
		day_part_caps: [],
		priority_floor: 4,
		postpone_budget: 0,
		postpone_budget_used: 0,
		...overrides
	};
}

beforeEach(() => {
	constraintsStore.init(makeConfig({ enabled: false }));
});

describe('constraintsStore', () => {
	describe('init', () => {
		it('sets enabled and priority floor', () => {
			constraintsStore.init(makeConfig({ enabled: true, priority_floor: 3 }));
			expect(constraintsStore.enabled).toBe(true);
			expect(constraintsStore.priorityFloor).toBe(3);
		});

		it('populates label blocks map', () => {
			constraintsStore.init(
				makeConfig({
					label_blocks: [
						{ label: 'social-media', remaining_seconds: 86400 },
						{ label: 'gaming', remaining_seconds: 3600 }
					]
				})
			);
			expect(constraintsStore.labelBlocks.size).toBe(2);
			expect(constraintsStore.labelBlocks.get('social-media')).toBe(86400);
			expect(constraintsStore.labelBlocks.get('gaming')).toBe(3600);
		});

		it('populates day part caps map', () => {
			constraintsStore.init(
				makeConfig({
					day_part_caps: [
						{ label: 'morning', max_tasks: 3 },
						{ label: 'evening', max_tasks: 2 }
					]
				})
			);
			expect(constraintsStore.dayPartCaps.size).toBe(2);
			expect(constraintsStore.dayPartCaps.get('morning')).toBe(3);
			expect(constraintsStore.dayPartCaps.get('evening')).toBe(2);
		});

		it('sets postpone budget', () => {
			constraintsStore.init(makeConfig({ postpone_budget: 5, postpone_budget_used: 3 }));
			expect(constraintsStore.postponeBudget).toEqual({ limit: 5, used: 3 });
		});
	});

	describe('isLabelBlocked', () => {
		it('returns false when disabled', () => {
			constraintsStore.init(
				makeConfig({
					enabled: false,
					label_blocks: [{ label: 'blocked', remaining_seconds: 100 }]
				})
			);
			expect(constraintsStore.isLabelBlocked(['blocked'])).toBe(false);
		});

		it('returns true when any label is blocked', () => {
			constraintsStore.init(
				makeConfig({
					label_blocks: [{ label: 'social-media', remaining_seconds: 86400 }]
				})
			);
			expect(constraintsStore.isLabelBlocked(['work', 'social-media'])).toBe(true);
		});

		it('returns false when no labels are blocked', () => {
			constraintsStore.init(
				makeConfig({
					label_blocks: [{ label: 'social-media', remaining_seconds: 86400 }]
				})
			);
			expect(constraintsStore.isLabelBlocked(['work', 'exercise'])).toBe(false);
		});

		it('returns false for empty labels array', () => {
			constraintsStore.init(
				makeConfig({
					label_blocks: [{ label: 'social-media', remaining_seconds: 86400 }]
				})
			);
			expect(constraintsStore.isLabelBlocked([])).toBe(false);
		});
	});

	describe('getBlockedLabelSeconds', () => {
		it('returns null when disabled', () => {
			constraintsStore.init(
				makeConfig({
					enabled: false,
					label_blocks: [{ label: 'blocked', remaining_seconds: 100 }]
				})
			);
			expect(constraintsStore.getBlockedLabelSeconds(['blocked'])).toBeNull();
		});

		it('returns remaining seconds for blocked label', () => {
			constraintsStore.init(
				makeConfig({
					label_blocks: [{ label: 'gaming', remaining_seconds: 7200 }]
				})
			);
			expect(constraintsStore.getBlockedLabelSeconds(['gaming'])).toBe(7200);
		});

		it('returns null when no labels blocked', () => {
			constraintsStore.init(makeConfig({ label_blocks: [] }));
			expect(constraintsStore.getBlockedLabelSeconds(['work'])).toBeNull();
		});
	});

	describe('isPostponeExhausted', () => {
		it('returns false when disabled', () => {
			constraintsStore.init(
				makeConfig({ enabled: false, postpone_budget: 3, postpone_budget_used: 3 })
			);
			expect(constraintsStore.isPostponeExhausted()).toBe(false);
		});

		it('returns false when budget is 0 (unlimited)', () => {
			constraintsStore.init(makeConfig({ postpone_budget: 0, postpone_budget_used: 10 }));
			expect(constraintsStore.isPostponeExhausted()).toBe(false);
		});

		it('returns false when under budget', () => {
			constraintsStore.init(makeConfig({ postpone_budget: 5, postpone_budget_used: 3 }));
			expect(constraintsStore.isPostponeExhausted()).toBe(false);
		});

		it('returns true when at budget', () => {
			constraintsStore.init(makeConfig({ postpone_budget: 3, postpone_budget_used: 3 }));
			expect(constraintsStore.isPostponeExhausted()).toBe(true);
		});

		it('returns true when over budget', () => {
			constraintsStore.init(makeConfig({ postpone_budget: 3, postpone_budget_used: 5 }));
			expect(constraintsStore.isPostponeExhausted()).toBe(true);
		});
	});

	describe('isPriorityBelowFloor', () => {
		it('returns false when disabled', () => {
			constraintsStore.init(makeConfig({ enabled: false, priority_floor: 2 }));
			expect(constraintsStore.isPriorityBelowFloor(4)).toBe(false);
		});

		it('returns false when priority equals floor', () => {
			constraintsStore.init(makeConfig({ priority_floor: 3 }));
			expect(constraintsStore.isPriorityBelowFloor(3)).toBe(false);
		});

		it('returns true when priority is below floor (lower number = lower priority in Todoist)', () => {
			constraintsStore.init(makeConfig({ priority_floor: 3 }));
			expect(constraintsStore.isPriorityBelowFloor(1)).toBe(true);
		});

		it('returns false when priority is above floor (higher number = higher priority in Todoist)', () => {
			constraintsStore.init(makeConfig({ priority_floor: 2 }));
			expect(constraintsStore.isPriorityBelowFloor(3)).toBe(false);
		});

		it('handles default floor of 1 (no restriction, all priorities pass)', () => {
			constraintsStore.init(makeConfig({ priority_floor: 1 }));
			expect(constraintsStore.isPriorityBelowFloor(1)).toBe(false);
			expect(constraintsStore.isPriorityBelowFloor(4)).toBe(false);
		});
	});

	describe('getDayPartCap', () => {
		it('returns null when disabled', () => {
			constraintsStore.init(
				makeConfig({
					enabled: false,
					day_part_caps: [{ label: 'morning', max_tasks: 3 }]
				})
			);
			expect(constraintsStore.getDayPartCap('morning')).toBeNull();
		});

		it('returns cap when label has a cap', () => {
			constraintsStore.init(
				makeConfig({
					day_part_caps: [{ label: 'morning', max_tasks: 5 }]
				})
			);
			expect(constraintsStore.getDayPartCap('morning')).toBe(5);
		});

		it('returns null when label has no cap', () => {
			constraintsStore.init(
				makeConfig({
					day_part_caps: [{ label: 'morning', max_tasks: 5 }]
				})
			);
			expect(constraintsStore.getDayPartCap('evening')).toBeNull();
		});
	});

	describe('incrementPostponeUsed', () => {
		it('increments used count by 1', () => {
			constraintsStore.init(makeConfig({ postpone_budget: 5, postpone_budget_used: 2 }));
			constraintsStore.incrementPostponeUsed();
			expect(constraintsStore.postponeBudget).toEqual({ limit: 5, used: 3 });
		});

		it('increments past the limit', () => {
			constraintsStore.init(makeConfig({ postpone_budget: 3, postpone_budget_used: 3 }));
			constraintsStore.incrementPostponeUsed();
			expect(constraintsStore.postponeBudget).toEqual({ limit: 3, used: 4 });
		});

		it('makes isPostponeExhausted true when reaching limit', () => {
			constraintsStore.init(makeConfig({ postpone_budget: 2, postpone_budget_used: 1 }));
			expect(constraintsStore.isPostponeExhausted()).toBe(false);
			constraintsStore.incrementPostponeUsed();
			expect(constraintsStore.isPostponeExhausted()).toBe(true);
		});
	});

	describe('updateDailyConstraints', () => {
		it('updates daily constraints state', () => {
			constraintsStore.updateDailyConstraints({
				needs_selection: false,
				items: ['no sugar', 'no phone'],
				rerolls_used: 1,
				max_rerolls: 2,
				pool_size: 5,
				confirmed: true
			});
			expect(constraintsStore.dailyConstraints.items).toEqual(['no sugar', 'no phone']);
			expect(constraintsStore.dailyConstraints.confirmed).toBe(true);
			expect(constraintsStore.dailyConstraints.rerolls_used).toBe(1);
		});
	});
});

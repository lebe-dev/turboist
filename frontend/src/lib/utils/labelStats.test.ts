import { describe, it, expect } from 'vitest';
import {
	buildLabelRadar,
	buildLabelStatsRows,
	isLabelRowActive,
	labelStatsTotals,
	lastUsedDaysAgo,
	radarPoint,
	splitLabelStatsRows
} from './labelStats';
import type { LabelStatsItem, LabelStatsPeriod, LabelStatsPeriodStats } from '../api/types';

function period(applied: number, previousApplied = 0, completed = 0): LabelStatsPeriodStats {
	return { applied, previousApplied, completed };
}

function makeItem(
	name: string,
	periods: Partial<Record<LabelStatsPeriod, LabelStatsPeriodStats>>,
	overrides: Partial<LabelStatsItem> = {}
): LabelStatsItem {
	return {
		label: {
			id: name.length * 100 + name.charCodeAt(0),
			name,
			color: 'blue',
			description: '',
			isFavourite: false,
			isPrivate: false,
			createdAt: '2026-01-01T00:00:00.000Z',
			updatedAt: '2026-01-01T00:00:00.000Z'
		},
		totalTasks: 0,
		openTasks: 0,
		overdue: 0,
		projects: 0,
		lastUsedAt: null,
		periods: {
			week: period(0),
			month: period(0),
			quarter: period(0),
			...periods
		},
		...overrides
	};
}

describe('buildLabelStatsRows', () => {
	it('ranks by applications in the selected period', () => {
		const items = [
			makeItem('rare', { week: period(1) }),
			makeItem('frequent', { week: period(9) }),
			makeItem('medium', { week: period(4) })
		];
		expect(buildLabelStatsRows(items, 'week').map((r) => r.item.label.name)).toEqual([
			'frequent',
			'medium',
			'rare'
		]);
	});

	it('re-ranks when the period changes', () => {
		const items = [
			makeItem('spiky', { week: period(5), month: period(5) }),
			makeItem('steady', { week: period(2), month: period(40) })
		];
		expect(buildLabelStatsRows(items, 'week')[0].item.label.name).toBe('spiky');
		expect(buildLabelStatsRows(items, 'month')[0].item.label.name).toBe('steady');
	});

	it('breaks an application tie by completions, then by name', () => {
		const items = [
			makeItem('b-name', { week: period(3, 0, 1) }),
			makeItem('a-name', { week: period(3, 0, 1) }),
			makeItem('more-done', { week: period(3, 0, 7) })
		];
		expect(buildLabelStatsRows(items, 'week').map((r) => r.item.label.name)).toEqual([
			'more-done',
			'a-name',
			'b-name'
		]);
	});

	it('exposes the delta against the previous window', () => {
		const rows = buildLabelStatsRows(
			[makeItem('up', { week: period(6, 2) }), makeItem('down', { week: period(1, 5) })],
			'week'
		);
		expect(rows.find((r) => r.item.label.name === 'up')?.delta).toBe(4);
		expect(rows.find((r) => r.item.label.name === 'down')?.delta).toBe(-4);
	});
});

describe('isLabelRowActive', () => {
	it('counts a completion-only label as active', () => {
		const [row] = buildLabelStatsRows([makeItem('old-tag', { week: period(0, 0, 3) })], 'week');
		expect(isLabelRowActive(row)).toBe(true);
	});

	it('counts a label with no activity as idle', () => {
		const [row] = buildLabelStatsRows([makeItem('idle', {})], 'week');
		expect(isLabelRowActive(row)).toBe(false);
	});
});

describe('splitLabelStatsRows', () => {
	it('separates idle labels and orders them most-recently-used first', () => {
		const items = [
			makeItem('active', { week: period(2) }),
			makeItem('stale', {}, { lastUsedAt: '2026-05-01T10:00:00.000Z' }),
			makeItem('never', {}),
			makeItem('recent', {}, { lastUsedAt: '2026-07-01T10:00:00.000Z' })
		];
		const { active, idle } = splitLabelStatsRows(buildLabelStatsRows(items, 'week'));
		expect(active.map((r) => r.item.label.name)).toEqual(['active']);
		expect(idle.map((r) => r.item.label.name)).toEqual(['recent', 'stale', 'never']);
	});
});

describe('labelStatsTotals', () => {
	it('sums the selected period and the period-independent overdue backlog', () => {
		const rows = buildLabelStatsRows(
			[
				makeItem('a', { week: period(3, 0, 2) }, { overdue: 1 }),
				makeItem('b', { week: period(4, 0, 5) }, { overdue: 2 }),
				makeItem('idle', {}, { overdue: 4 })
			],
			'week'
		);
		expect(labelStatsTotals(rows)).toEqual({ applied: 7, completed: 7, overdue: 7 });
	});

	it('is all zeros for an empty label set', () => {
		expect(labelStatsTotals([])).toEqual({ applied: 0, completed: 0, overdue: 0 });
	});

	it('follows the selected period', () => {
		const items = [makeItem('a', { week: period(1, 0, 1), month: period(9, 0, 8) })];
		expect(labelStatsTotals(buildLabelStatsRows(items, 'week')).applied).toBe(1);
		expect(labelStatsTotals(buildLabelStatsRows(items, 'month')).applied).toBe(9);
	});
});

describe('lastUsedDaysAgo', () => {
	const now = new Date('2026-07-26T09:00:00.000Z');

	it('returns null when the label was never used', () => {
		expect(lastUsedDaysAgo(null, 'UTC', now)).toBeNull();
	});

	it('returns null for an unparseable timestamp', () => {
		expect(lastUsedDaysAgo('not-a-date', 'UTC', now)).toBeNull();
	});

	it('counts whole days, not elapsed hours', () => {
		// 23:59 yesterday is one day ago even though it is minutes back.
		expect(lastUsedDaysAgo('2026-07-25T23:59:00.000Z', 'UTC', now)).toBe(1);
		expect(lastUsedDaysAgo('2026-07-26T00:01:00.000Z', 'UTC', now)).toBe(0);
		expect(lastUsedDaysAgo('2026-07-16T12:00:00.000Z', 'UTC', now)).toBe(10);
	});

	it('counts days in the given timezone, not UTC', () => {
		// 22:00 UTC is already the next day in Tokyo (+09:00), so from Tokyo's
		// "today" it is the same day, while UTC still calls it yesterday.
		const tokyoNow = new Date('2026-07-26T22:00:00.000Z');
		expect(lastUsedDaysAgo('2026-07-26T22:00:00.000Z', 'Asia/Tokyo', tokyoNow)).toBe(0);
		expect(lastUsedDaysAgo('2026-07-26T13:00:00.000Z', 'Asia/Tokyo', tokyoNow)).toBe(1);
	});

	it('never returns a negative age for a future timestamp', () => {
		expect(lastUsedDaysAgo('2026-07-30T09:00:00.000Z', 'UTC', now)).toBe(0);
	});
});

describe('buildLabelRadar', () => {
	const rowsOf = (items: LabelStatsItem[]) => buildLabelStatsRows(items, 'week');

	it('needs at least three labels with activity to make a shape', () => {
		const rows = rowsOf([
			makeItem('a', { week: period(3) }),
			makeItem('b', { week: period(1) }),
			makeItem('c', {})
		]);
		expect(buildLabelRadar(rows)).toBeNull();
	});

	it('gives a spoke to a label used only in the previous window', () => {
		const radar = buildLabelRadar(
			rowsOf([
				makeItem('a', { week: period(4) }),
				makeItem('b', { week: period(2) }),
				makeItem('gone', { week: period(0, 8) })
			])
		);
		expect(radar?.axes.map((a) => a.row.item.label.name)).toEqual(['a', 'b', 'gone']);
		expect(radar?.scaleMax).toBe(8);
		const gone = radar?.axes.find((a) => a.row.item.label.name === 'gone');
		expect(gone?.current).toBe(0);
		expect(gone?.previous).toBe(1);
	});

	it('orders spokes by name so a label keeps its direction across periods', () => {
		const items = [
			makeItem('zeta', { week: period(9), month: period(1) }),
			makeItem('alpha', { week: period(1), month: period(9) }),
			makeItem('mid', { week: period(5), month: period(5) })
		];
		const week = buildLabelRadar(buildLabelStatsRows(items, 'week'));
		const month = buildLabelRadar(buildLabelStatsRows(items, 'month'));
		expect(week?.axes.map((a) => a.row.item.label.name)).toEqual(['alpha', 'mid', 'zeta']);
		expect(month?.axes.map((a) => [a.row.item.label.name, a.angle])).toEqual(
			week?.axes.map((a) => [a.row.item.label.name, a.angle])
		);
		expect(week?.axes[0].angle).toBe(0);
	});

	it('keeps the busiest labels when there are more than the cap', () => {
		const radar = buildLabelRadar(
			rowsOf([
				makeItem('a', { week: period(1) }),
				makeItem('b', { week: period(5) }),
				makeItem('c', { week: period(4) }),
				makeItem('d', { week: period(0, 3) })
			]),
			3
		);
		expect(radar?.axes.map((a) => a.row.item.label.name)).toEqual(['b', 'c', 'd']);
		expect(radar?.hidden).toBe(1);
	});

	it('maps a spoke fraction onto the chart, twelve o’clock first', () => {
		const [x, y] = radarPoint(0, 1, 100);
		expect(x).toBeCloseTo(0);
		expect(y).toBeCloseTo(-100);
		const [x2, y2] = radarPoint(Math.PI / 2, 0.5, 100);
		expect(x2).toBeCloseTo(50);
		expect(y2).toBeCloseTo(0);
	});
});

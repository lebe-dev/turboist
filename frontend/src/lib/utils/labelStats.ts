import type { LabelStatsItem, LabelStatsPeriod } from '../api/types';
import { dayKeyInTz, daysBetweenKeys, parseIso } from './format';

/**
 * One label's row in the usage ranking, flattened for the selected period.
 * `delta` is the change against the equally long window right before it — the
 * trend arrow the page renders.
 */
export interface LabelStatsRow {
	item: LabelStatsItem;
	applied: number;
	previousApplied: number;
	completed: number;
	delta: number;
}

/**
 * Flatten every label onto the selected period and rank it: most applications
 * first, then most completions, then by name so the order is stable across
 * refetches. Never-used labels stay in the list with zeros — the page splits
 * them out as cleanup candidates.
 */
export function buildLabelStatsRows(
	items: LabelStatsItem[],
	period: LabelStatsPeriod
): LabelStatsRow[] {
	return items
		.map((item) => {
			const stats = item.periods[period];
			return {
				item,
				applied: stats.applied,
				previousApplied: stats.previousApplied,
				completed: stats.completed,
				delta: stats.applied - stats.previousApplied
			};
		})
		.sort(
			(a, b) =>
				b.applied - a.applied ||
				b.completed - a.completed ||
				a.item.label.name.localeCompare(b.item.label.name)
		);
}

/**
 * A label counts as active in the window when it was applied to a task OR a task
 * carrying it was completed there. Completions matter on their own: finishing
 * work tagged months ago is still activity under that label.
 */
export function isLabelRowActive(row: LabelStatsRow): boolean {
	return row.applied > 0 || row.completed > 0;
}

/**
 * Split the ranking into the active rows (ranking order preserved) and the idle
 * ones, the latter ordered by how recently they were last used — most recent
 * first, never-used last.
 */
export function splitLabelStatsRows(rows: LabelStatsRow[]): {
	active: LabelStatsRow[];
	idle: LabelStatsRow[];
} {
	const active = rows.filter(isLabelRowActive);
	const idle = rows
		.filter((row) => !isLabelRowActive(row))
		.sort((a, b) => (b.item.lastUsedAt ?? '').localeCompare(a.item.lastUsedAt ?? ''));
	return { active, idle };
}

/**
 * Headline counters for the selected window. `applied` and `completed` are
 * period-scoped; `overdue` is not — it is the current backlog of past-due open
 * tasks under these labels, which is why it does not move when the period does.
 */
export interface LabelStatsTotals {
	applied: number;
	completed: number;
	overdue: number;
}

export function labelStatsTotals(rows: LabelStatsRow[]): LabelStatsTotals {
	return rows.reduce<LabelStatsTotals>(
		(totals, row) => ({
			applied: totals.applied + row.applied,
			completed: totals.completed + row.completed,
			overdue: totals.overdue + row.item.overdue
		}),
		{ applied: 0, completed: 0, overdue: 0 }
	);
}

/**
 * Whole-day age of a label's most recent application in the given timezone:
 * 0 = today, 1 = yesterday, null = never used (or an unparseable timestamp).
 * Days are counted between day keys, not by dividing elapsed milliseconds, so a
 * tag from 23:59 yesterday reads as "yesterday" rather than "today".
 */
export function lastUsedDaysAgo(
	lastUsedAt: string | null,
	tz?: string | null,
	now: Date = new Date()
): number | null {
	if (!lastUsedAt) return null;
	const date = parseIso(lastUsedAt);
	if (!date) return null;
	return Math.max(0, daysBetweenKeys(dayKeyInTz(date, tz), dayKeyInTz(now, tz)));
}

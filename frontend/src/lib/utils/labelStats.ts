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

/** The radar never draws more spokes than this: past a dozen the names collide. */
export const LABEL_RADAR_MAX_AXES = 12;

/** Fewer spokes than this make a line or a point, not a shape. */
export const LABEL_RADAR_MIN_AXES = 3;

/**
 * One spoke of the label radar. `current` and `previous` are fractions of the
 * chart's radius (0..1) for the selected window and the one before it; `angle`
 * is in radians, clockwise from twelve o'clock.
 */
export interface LabelRadarAxis {
	row: LabelStatsRow;
	angle: number;
	current: number;
	previous: number;
}

export interface LabelRadar {
	axes: LabelRadarAxis[];
	/** The application count the outer ring stands for. */
	scaleMax: number;
	/** Labels with activity that did not get a spoke because of the cap. */
	hidden: number;
}

/**
 * Lay the labels out as a radar: every label is a direction, and the shape shows
 * which way the work pulls in the selected window, with the window before it as a
 * second outline to compare against.
 *
 * A label earns a spoke when it was applied in either window, so a label that
 * stopped pulling still shows where the pull used to be. Past the cap, the
 * busiest labels win. Spokes are then placed by name rather than by count, so a
 * label keeps its direction when the period changes and only the shape moves.
 * Returns null when there are too few spokes to make a shape.
 */
export function buildLabelRadar(
	rows: LabelStatsRow[],
	maxAxes: number = LABEL_RADAR_MAX_AXES
): LabelRadar | null {
	const weight = (row: LabelStatsRow) => Math.max(row.applied, row.previousApplied);
	const candidates = rows
		.filter((row) => weight(row) > 0)
		.sort((a, b) => weight(b) - weight(a) || a.item.label.name.localeCompare(b.item.label.name));
	const picked = candidates
		.slice(0, maxAxes)
		.sort(
			(a, b) =>
				a.item.label.name.localeCompare(b.item.label.name) || a.item.label.id - b.item.label.id
		);
	if (picked.length < LABEL_RADAR_MIN_AXES) return null;
	const scaleMax = Math.max(1, ...picked.map(weight));
	return {
		axes: picked.map((row, index) => ({
			row,
			angle: (2 * Math.PI * index) / picked.length,
			current: row.applied / scaleMax,
			previous: row.previousApplied / scaleMax
		})),
		scaleMax,
		hidden: candidates.length - picked.length
	};
}

/** Where a point `fraction` of the way along a spoke lands, for a chart centred on 0,0. */
export function radarPoint(angle: number, fraction: number, radius: number): [number, number] {
	return [Math.sin(angle) * fraction * radius, -Math.cos(angle) * fraction * radius];
}

import type { ConfigResponse, DayPart, Task } from '$lib/api/types';
import { dayKeyInTz, dayStartUtcInTz } from './format';

export interface DayPartInterval {
	start: number;
	end: number;
}

export interface DayPartGroup {
	part: DayPart;
	label: string;
	interval: DayPartInterval | null;
	tasks: Task[];
}

const DAY_PART_ORDER: Array<{ part: DayPart; label: string }> = [
	{ part: 'morning', label: 'Morning' },
	{ part: 'afternoon', label: 'Afternoon' },
	{ part: 'evening', label: 'Evening' },
	{ part: 'none', label: 'Anytime' }
];

function intervalFor(
	part: DayPart,
	dayParts: ConfigResponse['dayParts'] | undefined
): DayPartInterval | null {
	if (!dayParts || part === 'none') return null;
	return dayParts[part];
}

export function groupByDayPart(
	tasks: Task[],
	dayParts?: ConfigResponse['dayParts']
): DayPartGroup[] {
	const buckets = new Map<DayPart, Task[]>();
	for (const t of tasks) {
		const key = t.dayPart ?? 'none';
		const arr = buckets.get(key);
		if (arr) arr.push(t);
		else buckets.set(key, [t]);
	}
	return DAY_PART_ORDER.filter((g) => (buckets.get(g.part)?.length ?? 0) > 0).map((g) => ({
		part: g.part,
		label: g.label,
		interval: intervalFor(g.part, dayParts),
		tasks: buckets.get(g.part)!
	}));
}

// activeDayPart returns the phase whose interval contains `now` in the given
// timezone. Returns null if none matches (e.g. late night before morning starts).
export function activeDayPart(
	now: Date,
	dayParts: ConfigResponse['dayParts'] | undefined,
	tz?: string | null
): DayPart | null {
	if (!dayParts) return null;
	const hour = hourInTz(now, tz);
	for (const part of ['morning', 'afternoon', 'evening'] as const) {
		const iv = dayParts[part];
		if (hour >= iv.start && hour < iv.end) return part;
	}
	return null;
}

function hourInTz(date: Date, tz?: string | null): number {
	if (!tz) return date.getHours();
	const parts = new Intl.DateTimeFormat('en-US', {
		hour12: false,
		hour: '2-digit',
		timeZone: tz
	}).formatToParts(date);
	const h = parts.find((p) => p.type === 'hour')?.value ?? '0';
	const n = parseInt(h, 10);
	// Intl returns "24" for midnight in some engines — normalize.
	return n === 24 ? 0 : n;
}

export interface DayGroup {
	dayKey: string;
	label: string;
	date: Date;
	tasks: Task[];
}

const DAY_MS = 24 * 60 * 60 * 1000;

function labelFor(key: string, todayKey: string, tz?: string | null): string {
	const todayStart = dayStartUtcInTz(todayKey, tz);
	const target = dayStartUtcInTz(key, tz);
	const diff = Math.round((target.getTime() - todayStart.getTime()) / DAY_MS);
	if (diff === 0) return 'Today';
	if (diff === 1) return 'Tomorrow';
	if (diff === -1) return 'Yesterday';
	return target.toLocaleDateString('en-US', {
		timeZone: tz || undefined,
		weekday: 'short',
		month: 'short',
		day: 'numeric'
	});
}

export function groupByDay(tasks: Task[], tz?: string | null): DayGroup[] {
	const buckets = new Map<string, { date: Date; tasks: Task[] }>();
	const noDate: Task[] = [];
	for (const t of tasks) {
		if (!t.dueAt) {
			noDate.push(t);
			continue;
		}
		const d = new Date(t.dueAt);
		const key = dayKeyInTz(d, tz);
		const bucket = buckets.get(key);
		if (bucket) bucket.tasks.push(t);
		else buckets.set(key, { date: dayStartUtcInTz(key, tz), tasks: [t] });
	}
	const todayKey = dayKeyInTz(new Date(), tz);
	const groups: DayGroup[] = [...buckets.entries()]
		.sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0))
		.map(([key, v]) => ({
			dayKey: key,
			label: labelFor(key, todayKey, tz),
			date: v.date,
			tasks: v.tasks
		}));
	if (noDate.length) {
		groups.push({ dayKey: 'no-date', label: 'No date', date: new Date(0), tasks: noDate });
	}
	return groups;
}

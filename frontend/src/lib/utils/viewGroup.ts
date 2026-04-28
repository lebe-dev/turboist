import type { DayPart, Task } from '$lib/api/types';

export interface DayPartGroup {
	part: DayPart;
	label: string;
	tasks: Task[];
}

const DAY_PART_ORDER: Array<{ part: DayPart; label: string }> = [
	{ part: 'morning', label: 'Morning' },
	{ part: 'afternoon', label: 'Afternoon' },
	{ part: 'evening', label: 'Evening' },
	{ part: 'none', label: 'Anytime' }
];

export function groupByDayPart(tasks: Task[]): DayPartGroup[] {
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
		tasks: buckets.get(g.part)!
	}));
}

export interface DayGroup {
	dayKey: string;
	label: string;
	date: Date;
	tasks: Task[];
}

const DAY_MS = 24 * 60 * 60 * 1000;

function startOfDayLocal(d: Date): Date {
	const x = new Date(d);
	x.setHours(0, 0, 0, 0);
	return x;
}

function dayKey(d: Date): string {
	const pad = (n: number) => String(n).padStart(2, '0');
	return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

function labelFor(date: Date): string {
	const today = startOfDayLocal(new Date());
	const target = startOfDayLocal(date);
	const diff = Math.round((target.getTime() - today.getTime()) / DAY_MS);
	if (diff === 0) return 'Today';
	if (diff === 1) return 'Tomorrow';
	if (diff === -1) return 'Yesterday';
	return date.toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' });
}

export function groupByDay(tasks: Task[]): DayGroup[] {
	const buckets = new Map<string, { date: Date; tasks: Task[] }>();
	const noDate: Task[] = [];
	for (const t of tasks) {
		if (!t.dueAt) {
			noDate.push(t);
			continue;
		}
		const d = new Date(t.dueAt);
		const key = dayKey(d);
		const bucket = buckets.get(key);
		if (bucket) bucket.tasks.push(t);
		else buckets.set(key, { date: startOfDayLocal(d), tasks: [t] });
	}
	const groups: DayGroup[] = [...buckets.entries()]
		.sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0))
		.map(([key, v]) => ({ dayKey: key, label: labelFor(v.date), date: v.date, tasks: v.tasks }));
	if (noDate.length) {
		groups.push({ dayKey: 'no-date', label: 'No date', date: new Date(0), tasks: noDate });
	}
	return groups;
}

import type { DayPart, Priority } from '../api/types';

export function toIsoUtc(date: Date): string {
	return date.toISOString().replace(/\.\d{3}Z$/, '.000Z');
}

export function parseIso(value: string | null | undefined): Date | null {
	if (!value) return null;
	const d = new Date(value);
	return Number.isNaN(d.getTime()) ? null : d;
}

const DAY_MS = 24 * 60 * 60 * 1000;

function startOfDayLocal(date: Date): Date {
	const d = new Date(date);
	d.setHours(0, 0, 0, 0);
	return d;
}

/**
 * Render a due date relative to "today" in the local TZ:
 *   Today / Tomorrow / Yesterday / Mon, Apr 28 / Apr 28, 2027.
 * If `withTime` is true and the timestamp had a time component, append HH:MM.
 */
export function formatDay(value: string | Date | null | undefined, withTime = false): string {
	const date = typeof value === 'string' ? parseIso(value) : (value ?? null);
	if (!date) return '';

	const today = startOfDayLocal(new Date());
	const target = startOfDayLocal(date);
	const diffDays = Math.round((target.getTime() - today.getTime()) / DAY_MS);

	let day: string;
	if (diffDays === 0) day = 'Today';
	else if (diffDays === 1) day = 'Tomorrow';
	else if (diffDays === -1) day = 'Yesterday';
	else if (diffDays > 1 && diffDays < 7)
		day = date.toLocaleDateString('en-US', { weekday: 'short' });
	else if (date.getFullYear() === today.getFullYear())
		day = date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
	else day = date.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });

	if (!withTime) return day;
	const time = date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
	return `${day} ${time}`;
}

export function formatDayPart(part: DayPart): string {
	switch (part) {
		case 'morning':
			return 'Morning';
		case 'afternoon':
			return 'Afternoon';
		case 'evening':
			return 'Evening';
		case 'none':
		default:
			return '';
	}
}

export function formatPriority(p: Priority): string {
	switch (p) {
		case 'high':
			return 'P1';
		case 'medium':
			return 'P2';
		case 'low':
			return 'P3';
		case 'no-priority':
		default:
			return 'P4';
	}
}

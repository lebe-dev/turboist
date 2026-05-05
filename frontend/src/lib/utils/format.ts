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

/**
 * Format a Date as YYYY-MM-DD in the given IANA timezone.
 * Falls back to the browser's local timezone when `tz` is empty.
 */
export function dayKeyInTz(date: Date, tz?: string | null): string {
	const fmt = new Intl.DateTimeFormat('en-CA', {
		timeZone: tz || undefined,
		year: 'numeric',
		month: '2-digit',
		day: '2-digit'
	});
	return fmt.format(date);
}

/** Shift a YYYY-MM-DD day key by the given number of whole days. */
export function shiftDayKey(key: string, days: number): string {
	const [y, m, d] = key.split('-').map(Number);
	const dt = new Date(Date.UTC(y, m - 1, d));
	dt.setUTCDate(dt.getUTCDate() + days);
	const yy = dt.getUTCFullYear();
	const mm = String(dt.getUTCMonth() + 1).padStart(2, '0');
	const dd = String(dt.getUTCDate()).padStart(2, '0');
	return `${yy}-${mm}-${dd}`;
}

/** Returns the offset (in minutes) of the given IANA timezone at the given instant. */
function tzOffsetMinutes(date: Date, tz: string): number {
	const dtf = new Intl.DateTimeFormat('en-US', {
		timeZone: tz,
		year: 'numeric',
		month: '2-digit',
		day: '2-digit',
		hour: '2-digit',
		minute: '2-digit',
		second: '2-digit',
		hour12: false
	});
	const parts: Record<string, string> = {};
	for (const p of dtf.formatToParts(date)) {
		if (p.type !== 'literal') parts[p.type] = p.value;
	}
	const hour = Number(parts.hour) === 24 ? 0 : Number(parts.hour);
	const asUtc = Date.UTC(
		Number(parts.year),
		Number(parts.month) - 1,
		Number(parts.day),
		hour,
		Number(parts.minute),
		Number(parts.second)
	);
	return Math.round((asUtc - date.getTime()) / 60000);
}

/**
 * Returns the UTC instant for midnight at the start of `dayKey` (YYYY-MM-DD)
 * in the given IANA timezone. When `tz` is empty, falls back to the browser's
 * local timezone (matching `new Date('YYYY-MM-DDT00:00:00')`).
 */
export function dayStartUtcInTz(dayKey: string, tz?: string | null): Date {
	if (!tz) return new Date(`${dayKey}T00:00:00`);
	const [y, m, d] = dayKey.split('-').map(Number);
	const targetMs = Date.UTC(y, m - 1, d);
	let t = new Date(targetMs);
	// Two passes handle DST transitions where the offset on the naive guess
	// differs from the offset at the corrected instant.
	t = new Date(targetMs - tzOffsetMinutes(t, tz) * 60000);
	t = new Date(targetMs - tzOffsetMinutes(t, tz) * 60000);
	return t;
}

/**
 * Returns HH:MM (24h) for `date` in the given IANA timezone, or browser-local
 * when `tz` is empty.
 */
export function timeKeyInTz(date: Date, tz?: string | null): string {
	const fmt = new Intl.DateTimeFormat('en-GB', {
		timeZone: tz || undefined,
		hour: '2-digit',
		minute: '2-digit',
		hour12: false
	});
	return fmt.format(date);
}

/**
 * Render a due date relative to "today" in the configured timezone (falls back
 * to browser-local when `tz` is empty):
 *   Today / Tomorrow / Yesterday / Mon, Apr 28 / Apr 28, 2027.
 * If `withTime` is true, append HH:MM in the same timezone.
 */
export function formatDay(
	value: string | Date | null | undefined,
	withTime = false,
	tz?: string | null
): string {
	const date = typeof value === 'string' ? parseIso(value) : (value ?? null);
	if (!date) return '';

	const todayKey = dayKeyInTz(new Date(), tz);
	const targetKey = dayKeyInTz(date, tz);
	const todayStart = dayStartUtcInTz(todayKey, tz);
	const targetStart = dayStartUtcInTz(targetKey, tz);
	const diffDays = Math.round((targetStart.getTime() - todayStart.getTime()) / DAY_MS);

	const localeOpts: Intl.DateTimeFormatOptions = { timeZone: tz || undefined };

	let day: string;
	if (diffDays === 0) day = 'Today';
	else if (diffDays === 1) day = 'Tomorrow';
	else if (diffDays === -1) day = 'Yesterday';
	else if (diffDays > 1 && diffDays < 7)
		day = date.toLocaleDateString('en-US', { ...localeOpts, weekday: 'short' });
	else if (targetKey.slice(0, 4) === todayKey.slice(0, 4))
		day = date.toLocaleDateString('en-US', { ...localeOpts, month: 'short', day: 'numeric' });
	else
		day = date.toLocaleDateString('en-US', {
			...localeOpts,
			month: 'short',
			day: 'numeric',
			year: 'numeric'
		});

	if (!withTime) return day;
	const time = date.toLocaleTimeString([], {
		...localeOpts,
		hour: '2-digit',
		minute: '2-digit'
	});
	return `${day} ${time}`;
}

/**
 * Returns true when a task with the given due instant should be flagged as
 * overdue in the configured timezone. Matches backend semantics: a task is
 * overdue once its due day is strictly before today (regardless of time).
 */
export function isOverdue(
	dueAt: string | Date | null | undefined,
	tz?: string | null,
	now: Date = new Date()
): boolean {
	const date = typeof dueAt === 'string' ? parseIso(dueAt) : (dueAt ?? null);
	if (!date) return false;
	return dayKeyInTz(date, tz) < dayKeyInTz(now, tz);
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

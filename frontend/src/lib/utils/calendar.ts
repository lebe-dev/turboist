import type { CalendarEvent, ConfigResponse, DayPart } from '$lib/api/types';
import { dayKeyInTz, dayStartUtcInTz, timeKeyInTz } from './format';

export interface CalendarDayGroup {
	dayKey: string;
	label: string;
	events: CalendarEvent[];
}

export interface CalendarDayPartGroup {
	part: DayPart;
	events: CalendarEvent[];
}

export function eventTimeLabel(
	event: CalendarEvent,
	tz?: string | null,
	allDayLabel = 'All day'
): string {
	if (event.allDay) return allDayLabel;
	const start = new Date(event.start);
	const end = new Date(event.end);
	const startLabel = timeKeyInTz(start, tz);
	if (Number.isNaN(end.getTime()) || end <= start) return startLabel;
	return `${startLabel}-${timeKeyInTz(end, tz)}`;
}

export function sortCalendarEvents(events: CalendarEvent[]): CalendarEvent[] {
	return [...events].sort((a, b) => {
		const aStart = eventSortKey(a);
		const bStart = eventSortKey(b);
		if (aStart !== bStart) return aStart < bStart ? -1 : 1;
		return a.title.localeCompare(b.title);
	});
}

export function isPastCalendarEvent(
	event: CalendarEvent,
	now: Date = new Date(),
	tz?: string | null
): boolean {
	if (event.allDay) {
		if (!event.endDate) return false;
		return event.endDate <= dayKeyInTz(now, tz);
	}
	const end = new Date(event.end);
	if (Number.isNaN(end.getTime())) return false;
	return end <= now;
}

export function groupCalendarEventsByDayPart(
	events: CalendarEvent[],
	dayParts?: ConfigResponse['dayParts'],
	tz?: string | null
): CalendarDayPartGroup[] {
	const buckets = new Map<DayPart, CalendarEvent[]>();
	for (const event of sortCalendarEvents(events)) {
		const key = eventDayPart(event, dayParts, tz);
		const bucket = buckets.get(key);
		if (bucket) bucket.push(event);
		else buckets.set(key, [event]);
	}
	const order: DayPart[] = ['morning', 'afternoon', 'evening', 'none'];
	return order
		.filter((part) => (buckets.get(part)?.length ?? 0) > 0)
		.map((part) => ({ part, events: buckets.get(part)! }));
}

export function calendarEventsOrEmpty(
	load: Promise<CalendarEvent[]>,
	timeoutMs = 22000
): Promise<CalendarEvent[]> {
	let timer: ReturnType<typeof setTimeout> | undefined;
	const timeout = new Promise<CalendarEvent[]>((resolve) => {
		timer = setTimeout(() => resolve([]), timeoutMs);
	});
	return Promise.race([load.catch(() => []), timeout]).finally(() => {
		if (timer) clearTimeout(timer);
	});
}

function eventSortKey(event: CalendarEvent): string {
	if (event.allDay && event.startDate) return event.startDate;
	return event.start;
}

function eventDayPart(
	event: CalendarEvent,
	dayParts?: ConfigResponse['dayParts'],
	tz?: string | null
): DayPart {
	if (event.allDay || !dayParts) return 'none';
	const start = new Date(event.start);
	if (Number.isNaN(start.getTime())) return 'none';
	const hour = hourInTz(start, tz);
	for (const part of ['morning', 'afternoon', 'evening'] as const) {
		const iv = dayParts[part];
		if (hour >= iv.start && hour < iv.end) return part;
	}
	return 'none';
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
	return n === 24 ? 0 : n;
}

export function groupCalendarEventsByDay(
	events: CalendarEvent[],
	labels: { today: string; tomorrow: string; yesterday: string },
	tz?: string | null
): CalendarDayGroup[] {
	const buckets = new Map<string, CalendarEvent[]>();
	for (const event of sortCalendarEvents(events)) {
		const key =
			event.allDay && event.startDate ? event.startDate : dayKeyInTz(new Date(event.start), tz);
		const bucket = buckets.get(key);
		if (bucket) bucket.push(event);
		else buckets.set(key, [event]);
	}
	const todayKey = dayKeyInTz(new Date(), tz);
	const todayStart = dayStartUtcInTz(todayKey, tz);
	const dayMs = 24 * 60 * 60 * 1000;
	return [...buckets.entries()]
		.sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0))
		.map(([key, dayEvents]) => {
			const target = dayStartUtcInTz(key, tz);
			const diff = Math.round((target.getTime() - todayStart.getTime()) / dayMs);
			let label = labels.today;
			if (diff === 1) label = labels.tomorrow;
			else if (diff === -1) label = labels.yesterday;
			else if (diff !== 0) {
				label = target.toLocaleDateString(undefined, {
					timeZone: tz || undefined,
					weekday: 'short',
					month: 'short',
					day: 'numeric'
				});
			}
			return { dayKey: key, label, events: dayEvents };
		});
}

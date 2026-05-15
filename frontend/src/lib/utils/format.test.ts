import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
	dayKeyInTz,
	daysBetweenKeys,
	dayStartUtcInTz,
	formatDay,
	formatDayKeyRange,
	formatDayPart,
	formatPriority,
	isOverdue,
	nextWeekRangeKeys,
	parseIso,
	shiftDayKey,
	timeKeyInTz,
	toIsoUtc,
	weekRangeKeys
} from './format';

describe('toIsoUtc', () => {
	it('forces .000Z suffix', () => {
		const d = new Date('2026-04-28T12:34:56.789Z');
		expect(toIsoUtc(d)).toBe('2026-04-28T12:34:56.000Z');
	});
});

describe('parseIso', () => {
	it('returns null for empty values', () => {
		expect(parseIso(null)).toBeNull();
		expect(parseIso(undefined)).toBeNull();
		expect(parseIso('')).toBeNull();
	});

	it('returns null for invalid input', () => {
		expect(parseIso('not-a-date')).toBeNull();
	});

	it('parses ISO strings', () => {
		const d = parseIso('2026-04-28T00:00:00.000Z');
		expect(d).not.toBeNull();
		expect(d!.getUTCFullYear()).toBe(2026);
	});
});

describe('formatDay', () => {
	beforeEach(() => {
		vi.useFakeTimers();
		vi.setSystemTime(new Date('2026-04-28T10:00:00'));
	});
	afterEach(() => vi.useRealTimers());

	it('returns empty string for null', () => {
		expect(formatDay(null)).toBe('');
	});

	it('renders Today / Tomorrow / Yesterday', () => {
		expect(formatDay(new Date('2026-04-28T15:00:00'))).toBe('Today');
		expect(formatDay(new Date('2026-04-29T08:00:00'))).toBe('Tomorrow');
		expect(formatDay(new Date('2026-04-27T08:00:00'))).toBe('Yesterday');
	});

	it('renders weekday name within the next week', () => {
		const out = formatDay(new Date('2026-05-02T08:00:00'));
		expect(out).toMatch(/^[A-Z][a-z]{2}$/);
	});

	it('appends time when withTime=true', () => {
		const out = formatDay(new Date('2026-04-28T15:30:00'), true);
		expect(out.startsWith('Today ')).toBe(true);
		expect(out).toMatch(/\d{2}:\d{2}/);
	});
});

describe('formatDayPart', () => {
	it('maps day parts to labels', () => {
		expect(formatDayPart('morning')).toBe('Morning');
		expect(formatDayPart('afternoon')).toBe('Afternoon');
		expect(formatDayPart('evening')).toBe('Evening');
		expect(formatDayPart('none')).toBe('');
	});
});

describe('formatPriority', () => {
	it('renders P1..P4', () => {
		expect(formatPriority('high')).toBe('P1');
		expect(formatPriority('medium')).toBe('P2');
		expect(formatPriority('low')).toBe('P3');
		expect(formatPriority('no-priority')).toBe('P4');
	});
});

describe('dayKeyInTz', () => {
	// 2026-04-28T22:30:00Z is 2026-04-29 02:30 in Tbilisi (+04:00) and
	// 2026-04-28 15:30 in Los Angeles (-07:00).
	const instant = new Date('2026-04-28T22:30:00.000Z');

	it('returns the calendar day in UTC', () => {
		expect(dayKeyInTz(instant, 'UTC')).toBe('2026-04-28');
	});

	it('rolls over to the next day in eastern timezones', () => {
		expect(dayKeyInTz(instant, 'Asia/Tbilisi')).toBe('2026-04-29');
	});

	it('stays on the previous day in western timezones', () => {
		expect(dayKeyInTz(instant, 'America/Los_Angeles')).toBe('2026-04-28');
	});

	it('falls back to local timezone for empty tz', () => {
		// Don't pin local TZ — just ensure it returns a YYYY-MM-DD string.
		expect(dayKeyInTz(instant, '')).toMatch(/^\d{4}-\d{2}-\d{2}$/);
		expect(dayKeyInTz(instant, null)).toMatch(/^\d{4}-\d{2}-\d{2}$/);
	});
});

describe('daysBetweenKeys', () => {
	it('returns 0 for the same key', () => {
		expect(daysBetweenKeys('2026-05-11', '2026-05-11')).toBe(0);
	});

	it('counts forward whole days', () => {
		expect(daysBetweenKeys('2026-05-11', '2026-05-17')).toBe(6);
	});

	it('returns a negative count when "to" precedes "from"', () => {
		expect(daysBetweenKeys('2026-05-17', '2026-05-11')).toBe(-6);
	});

	it('spans month and year boundaries', () => {
		expect(daysBetweenKeys('2025-12-29', '2026-01-05')).toBe(7);
	});
});

describe('shiftDayKey', () => {
	it('moves forward across a month boundary', () => {
		expect(shiftDayKey('2026-04-30', 1)).toBe('2026-05-01');
	});

	it('moves backward across a year boundary', () => {
		expect(shiftDayKey('2026-01-01', -1)).toBe('2025-12-31');
	});

	it('handles leap day forward', () => {
		expect(shiftDayKey('2024-02-29', 1)).toBe('2024-03-01');
	});

	it('handles leap day backward', () => {
		expect(shiftDayKey('2024-03-01', -1)).toBe('2024-02-29');
	});

	it('returns the same key on zero shift', () => {
		expect(shiftDayKey('2026-04-28', 0)).toBe('2026-04-28');
	});
});

describe('weekRangeKeys', () => {
	// 2026-05-13 is a Wednesday → Monday should be 2026-05-11, next Monday 2026-05-18.
	const wed = new Date('2026-05-13T10:00:00Z');

	it('returns Mon-startKey and next-Mon endKey for a midweek date', () => {
		expect(weekRangeKeys(wed, 'UTC')).toEqual({
			startKey: '2026-05-11',
			endKey: '2026-05-18'
		});
	});

	it('returns the same week when called on Monday itself', () => {
		const mon = new Date('2026-05-11T10:00:00Z');
		expect(weekRangeKeys(mon, 'UTC')).toEqual({
			startKey: '2026-05-11',
			endKey: '2026-05-18'
		});
	});

	it('keeps Sunday inside the same week', () => {
		const sun = new Date('2026-05-17T22:00:00Z');
		expect(weekRangeKeys(sun, 'UTC')).toEqual({
			startKey: '2026-05-11',
			endKey: '2026-05-18'
		});
	});

	it('shifts the week boundary by tz when crossing midnight', () => {
		// 2026-05-11T01:00Z is Sunday 21:00 in New York → still last week there.
		const lateSunNY = new Date('2026-05-11T01:00:00Z');
		expect(weekRangeKeys(lateSunNY, 'America/New_York')).toEqual({
			startKey: '2026-05-04',
			endKey: '2026-05-11'
		});
	});
});

describe('nextWeekRangeKeys', () => {
	it('returns the Mon..next-Mon range that follows the current week', () => {
		// Wednesday → current week is May 11..18, next is May 18..25.
		const wed = new Date('2026-05-13T10:00:00Z');
		expect(nextWeekRangeKeys(wed, 'UTC')).toEqual({
			startKey: '2026-05-18',
			endKey: '2026-05-25'
		});
	});

	it('keeps the start aligned to Monday when called on Sunday', () => {
		const sun = new Date('2026-05-17T22:00:00Z');
		expect(nextWeekRangeKeys(sun, 'UTC')).toEqual({
			startKey: '2026-05-18',
			endKey: '2026-05-25'
		});
	});

	it('rolls into a new month/year when the week boundary crosses it', () => {
		// Wednesday 2025-12-31 — next week starts Mon 2026-01-05.
		const wed = new Date('2025-12-31T10:00:00Z');
		expect(nextWeekRangeKeys(wed, 'UTC')).toEqual({
			startKey: '2026-01-05',
			endKey: '2026-01-12'
		});
	});
});

describe('formatDayKeyRange', () => {
	it('formats an in-month range in English', () => {
		const out = formatDayKeyRange('2026-05-11', '2026-05-18', 'en', 'UTC');
		expect(out).toMatch(/May/);
		expect(out).toMatch(/11/);
		expect(out).toMatch(/17/);
		expect(out).not.toMatch(/2026/);
	});

	it('formats an in-month range in Russian', () => {
		const out = formatDayKeyRange('2026-05-11', '2026-05-18', 'ru', 'UTC');
		expect(out).toMatch(/ма[яй]/);
		expect(out).toMatch(/11/);
		expect(out).toMatch(/17/);
		expect(out).not.toMatch(/2026/);
	});

	it('includes the year when the range spans different years', () => {
		const out = formatDayKeyRange('2025-12-29', '2026-01-05', 'en', 'UTC');
		expect(out).toMatch(/2025/);
		expect(out).toMatch(/2026/);
	});

	it('uses the inclusive Sunday as the end (endKey is exclusive)', () => {
		const out = formatDayKeyRange('2026-05-11', '2026-05-18', 'en', 'UTC');
		expect(out).not.toMatch(/18/);
	});
});

describe('dayStartUtcInTz', () => {
	it('returns midnight UTC for UTC tz', () => {
		const t = dayStartUtcInTz('2026-04-28', 'UTC');
		expect(t.toISOString()).toBe('2026-04-28T00:00:00.000Z');
	});

	it('returns the correct UTC instant for an eastern tz', () => {
		// Tbilisi is UTC+4 year-round, so midnight there = 20:00 UTC the prior day.
		const t = dayStartUtcInTz('2026-04-28', 'Asia/Tbilisi');
		expect(t.toISOString()).toBe('2026-04-27T20:00:00.000Z');
	});

	it('round-trips through dayKeyInTz', () => {
		for (const tz of ['UTC', 'Asia/Tbilisi', 'America/Los_Angeles', 'Europe/Berlin']) {
			const start = dayStartUtcInTz('2026-04-28', tz);
			expect(dayKeyInTz(start, tz)).toBe('2026-04-28');
		}
	});
});

describe('timeKeyInTz', () => {
	const instant = new Date('2026-04-28T22:30:00.000Z');

	it('formats HH:MM in UTC', () => {
		expect(timeKeyInTz(instant, 'UTC')).toBe('22:30');
	});

	it('formats HH:MM in an offset tz', () => {
		expect(timeKeyInTz(instant, 'Asia/Tbilisi')).toBe('02:30');
	});
});

describe('isOverdue', () => {
	const now = new Date('2026-04-28T10:00:00.000Z');

	it('returns false for null/undefined', () => {
		expect(isOverdue(null, 'UTC', now)).toBe(false);
		expect(isOverdue(undefined, 'UTC', now)).toBe(false);
	});

	it('returns true when due day is strictly before today', () => {
		expect(isOverdue('2026-04-27T23:59:59.000Z', 'UTC', now)).toBe(true);
	});

	it('returns false when due is later today', () => {
		expect(isOverdue('2026-04-28T23:59:59.000Z', 'UTC', now)).toBe(false);
	});

	it('returns false when due is earlier today', () => {
		expect(isOverdue('2026-04-28T00:00:00.000Z', 'UTC', now)).toBe(false);
	});

	it('returns false when due is in the future', () => {
		expect(isOverdue('2026-04-29T00:00:00.000Z', 'UTC', now)).toBe(false);
	});

	it('respects timezone when comparing days', () => {
		// In Tbilisi (+04:00) the "now" instant is 14:00 on 2026-04-28.
		// A due instant of 2026-04-28T20:00Z is 2026-04-29 00:00 Tbilisi → not overdue.
		expect(isOverdue('2026-04-28T20:00:00.000Z', 'Asia/Tbilisi', now)).toBe(false);
		// Same instant compared against UTC where now is 10:00 on 2026-04-28: due is later
		// the same UTC day → not overdue either.
		expect(isOverdue('2026-04-28T20:00:00.000Z', 'UTC', now)).toBe(false);
	});
});

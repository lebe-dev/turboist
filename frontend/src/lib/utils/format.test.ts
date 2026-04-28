import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { formatDay, formatDayPart, formatPriority, parseIso, toIsoUtc } from './format';

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
